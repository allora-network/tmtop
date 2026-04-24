package pkg

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"main/pkg/analytics"
	configPkg "main/pkg/config"
	"main/pkg/db"
	"main/pkg/display"
	"main/pkg/fetcher"
	tmhttp "main/pkg/http"
	loggerPkg "main/pkg/logger"
	"main/pkg/topology"
	"main/pkg/types"

	butils "github.com/brynbellomy/go-utils"
	cnstypes "github.com/cometbft/cometbft/consensus/types"
	ctypes "github.com/cometbft/cometbft/types"
	"github.com/gdamore/tcell/v2"
	"github.com/rs/zerolog"
)

type App struct {
	Logger         zerolog.Logger
	Version        string
	Config         *configPkg.Config
	DisplayWrapper display.Display
	State          *types.State
	mbLogs         *butils.Mailbox[string]

	DB             *db.DB
	ConsensusStore db.ConsensusStorer

	DataFetcher fetcher.Fetcher

	topologyServer *tmhttp.Server

	mbRPCURLs        *butils.Mailbox[string]
	rpcURLsLastFetch map[string]time.Time

	IsPaused atomic.Bool

	chStop    chan struct{}
	closeOnce sync.Once
	wgDone    *sync.WaitGroup

	cleanupFuncs []func()
}

func NewApp(config *configPkg.Config, version string) *App {
	mbLogs := butils.NewMailbox[string](1000)

	logger := loggerPkg.GetLogger(mbLogs, config).
		With().
		Str("component", "app_manager").
		Logger()

	state := types.NewState(config.RPCHost, logger)

	// Initialize database if configured. When disabled, ConsensusStore
	// is a no-op so the rest of the app never needs to nil-check.
	var database *db.DB
	var consensusStore db.ConsensusStorer = db.NopConsensusStore{}

	if config.MaxRetainBlocks > 0 || config.MaxRetainDays > 0 {
		dbPath := config.DatabasePath
		if dbPath == "" {
			// Use default path: ~/.config/tmtop/tmtop.db
			homeDir, err := os.UserHomeDir()
			if err != nil {
				logger.Warn().Err(err).Msg("Could not get user home directory, using current directory for database")
				dbPath = "tmtop.db"
			} else {
				dbPath = filepath.Join(homeDir, ".config", "tmtop", "tmtop.db")
			}
		}

		dbConfig := db.Config{
			DatabasePath:    dbPath,
			MaxRetainBlocks: config.MaxRetainBlocks,
			MaxRetainDays:   config.MaxRetainDays,
		}

		var err error
		database, err = db.New(dbConfig)
		if err != nil {
			logger.Fatal().Err(err).Msg("failed to initialize database")
		}

		consensusStore = db.NewConsensusStore(database, dbConfig, logger)
		logger.Info().Str("path", dbPath).Msg("Database initialized successfully")
	}

	app := &App{
		Logger:           logger,
		Version:          version,
		Config:           config,
		State:            state,
		mbLogs:           mbLogs,
		DB:               database,
		ConsensusStore:   consensusStore,
		DataFetcher:      fetcher.NewDataFetcher(config, state, logger),
		mbRPCURLs:        butils.NewMailbox[string](1000),
		rpcURLsLastFetch: make(map[string]time.Time),
		chStop:           make(chan struct{}),
		wgDone:           &sync.WaitGroup{},
		cleanupFuncs:     make([]func(), 0),
	}
	// Display needs a pointer to the pause flag so its input handler
	// can toggle it in-place.
	app.DisplayWrapper = display.NewWrapper(config, state, logger, &app.IsPaused, version)
	return app
}

func (a *App) Start() {
	// Check if analytics mode is enabled
	if a.Config.AnalyticsMode {
		a.runAnalyticsMode()
		return
	}

	// Set up terminal cleanup on exit
	defer a.restoreTerminal()

	if a.Config.WithTopologyAPI {
		a.spawn(a.ServeTopology)
	}

	a.spawn(a.CrawlRPCURLs)

	a.spawn(a.GoRefreshConsensus)
	a.spawn(a.GoRefreshCometNodeInfo)
	a.spawn(a.GoRefreshUpgrade)
	a.spawn(a.GoRefreshBlockTime)
	a.spawn(a.GoRefreshNetInfo)
	a.spawn(a.SubscribeCometBFT)
	a.spawn(a.DisplayLogs)
	a.spawn(a.databaseCleanupRoutine)

	// Display blocks until the user quits.
	if err := a.DisplayWrapper.Start(); err != nil {
		a.Logger.Error().Err(err).Msg("display exited with error")
	}

	// User requested exit — shut everything else down.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Stop(ctx); err != nil {
		a.Logger.Error().Err(err).Msg("shutdown incomplete")
	}
}

// spawn runs fn as a tracked goroutine. Every goroutine launched by
// the App must go through this so Stop can wait on wgDone.
func (a *App) spawn(fn func()) {
	a.wgDone.Add(1)
	go func() {
		defer a.wgDone.Done()
		fn()
	}()
}

// Stop signals all background goroutines to exit and waits for them,
// bounded by ctx. Shutdown order: websocket/HTTP producers first (so
// they stop writing State and DB), then the consensus store.
func (a *App) Stop(ctx context.Context) error {
	a.closeOnce.Do(func() {
		close(a.chStop)
	})

	// Stop the websocket producer before we stop the consumer goroutines.
	if err := a.DataFetcher.Close(ctx); err != nil {
		a.Logger.Error().Err(err).Msg("DataFetcher close error")
	}

	// Stop the topology HTTP server if it's running.
	if a.topologyServer != nil {
		if err := a.topologyServer.Shutdown(ctx); err != nil {
			a.Logger.Error().Err(err).Msg("topology server shutdown error")
		}
	}

	done := make(chan struct{})
	go func() {
		a.wgDone.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		a.Logger.Error().Msg("shutdown timed out — some goroutines may still be running")
	}

	// Flush the consensus store last, after all writers have stopped.
	if err := a.ConsensusStore.Close(); err != nil {
		a.Logger.Error().Err(err).Msg("Failed to close consensus store")
	}
	return nil
}

func (a *App) ServeTopology() {
	a.topologyServer = tmhttp.NewServer(
		// a.Config.TopologyListenAddr,
		":8001",
		topology.WithHTTPTopologyAPI(a.State),
		topology.WithHTTPPeersAPI(a.State),
		topology.WithHTTPDebugAPI(a.State),
		topology.WithFrontendStaticAssets(),
	)
	if err := a.topologyServer.Serve(); err != nil && err != http.ErrServerClosed {
		a.Logger.Error().Err(err).Msg("topology server error")
	}
}

func (a *App) CrawlRPCURLs() {
	a.mbRPCURLs.Deliver(a.Config.RPCHost)
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.chStop:
			return
		case <-a.mbRPCURLs.Notify():
			var wg sync.WaitGroup
			for _, url := range a.mbRPCURLs.RetrieveAll() {
				if lastFetch, ok := a.rpcURLsLastFetch[url]; ok && time.Since(lastFetch) < 15*time.Second {
					continue
				}
				a.rpcURLsLastFetch[url] = time.Now()

				wg.Add(1)
				go func() {
					defer wg.Done()
					a.fetchRPCInfo(url)
				}()
			}
			wg.Wait()

		case <-ticker.C:
			for _, rpc := range a.State.KnownRPCs().Iter() {
				if time.Since(a.rpcURLsLastFetch[rpc.URL]) >= 15*time.Second {
					a.mbRPCURLs.Deliver(rpc.URL)
				}
			}
		}
	}
}

func (a *App) fetchRPCInfo(rpcURL string) {
	netInfo, err := a.DataFetcher.GetNetInfo(rpcURL)
	if err != nil {
		a.Logger.Error().Err(err).Msg(fmt.Sprintf("error getting /net_info from %s", rpcURL))
		return
	}

	status, err := a.DataFetcher.GetCometNodeStatus(rpcURL)
	if err != nil {
		a.Logger.Error().Err(err).Msg(fmt.Sprintf("error getting /status from %s", rpcURL))
		return
	}

	var rpc types.RPC
	if known, ok := a.State.KnownRPCByURL(rpcURL); ok {
		rpc = known
	}
	rpc.ID = string(status.NodeInfo.ID())
	rpc.URL = rpcURL
	rpc.Moniker = status.NodeInfo.Moniker
	rpc.ValidatorAddress = status.ValidatorInfo.Address.String()

	for _, cv := range a.State.TMValidators {
		if strings.EqualFold(cv.CosmosValidator.ConsensusPubkey.Address().String(), rpc.ValidatorAddress) {
			rpc.ValidatorMoniker = cv.CosmosValidator.Moniker
			break
		}
	}

	a.State.AddKnownRPC(rpc)
	a.State.AddRPCPeers(rpcURL, netInfo.Peers)

	for _, peer := range netInfo.Peers {
		var peerRPC types.RPC
		if known, ok := a.State.KnownRPCByURL(peer.URL()); ok {
			peerRPC = known
		}
		peerRPC.ID = string(peer.NodeInfo.DefaultNodeID)
		peerRPC.IP = peer.RemoteIP
		peerRPC.URL = peer.URL()
		peerRPC.Moniker = peer.NodeInfo.Moniker
		a.State.AddKnownRPC(peerRPC)

		a.mbRPCURLs.Deliver(peer.URL())
	}
}

func (a *App) GoRefreshConsensus() {
	defer a.HandlePanic()

	a.RefreshConsensus()

	ticker := time.NewTicker(a.Config.RefreshRate)
	defer ticker.Stop()

	for {
		select {
		case <-a.chStop:
			return
		case <-ticker.C:
			a.RefreshConsensus()
		}
	}
}

func (a *App) RefreshConsensus() {
	if a.IsPaused.Load() {
		return
	}

	var wg sync.WaitGroup
	var consensus *cnstypes.RoundState
	var vals []types.TMValidator
	var consErr error
	var valsErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		consensus, consErr = a.DataFetcher.GetConsensusState()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		vals, valsErr = a.DataFetcher.GetValidators()
	}()

	wg.Wait()

	if consErr != nil {
		a.Logger.Error().Err(consErr).Msg("Could not fetch consensus data")
		a.State.SetConsensusStateError(consErr)
		a.DisplayWrapper.SetState(a.State)
		return
	}

	if valsErr != nil {
		a.Logger.Error().Err(valsErr).Msg("Could not fetch validators")
		a.State.SetConsensusStateError(valsErr)
		a.DisplayWrapper.SetState(a.State)
		return
	}

	a.State.SetTMValidators(vals)
	a.State.SetConsensusStateError(nil)

	if consensus != nil {
		ctx := context.Background()

		if err := a.ConsensusStore.StoreValidators(ctx, consensus.Height, vals); err != nil {
			a.Logger.Error().Err(err).Int64("height", consensus.Height).Msg("Failed to persist validators to database")
		}

		for hr, roundData := range a.State.VotesByRound.Iter() {
			if err := a.ConsensusStore.StoreRoundData(ctx, hr.Height, hr.Round, roundData, vals); err != nil {
				a.Logger.Debug().Err(err).Int64("height", hr.Height).Int32("round", hr.Round).Msg("Failed to persist round data")
			}
		}
	}

	a.DisplayWrapper.SetState(a.State)
}

func (a *App) GoRefreshCometNodeInfo() {
	defer a.HandlePanic()

	a.RefreshCometNodeInfo()

	ticker := time.NewTicker(a.Config.ChainInfoRefreshRate)
	defer ticker.Stop()

	for {
		select {
		case <-a.chStop:
			return
		case <-ticker.C:
			a.RefreshCometNodeInfo()
		}
	}
}

func (a *App) RefreshCometNodeInfo() {
	if a.IsPaused.Load() {
		return
	}

	nodeStatus, err := a.DataFetcher.GetCometNodeStatus(a.State.CurrentRPC().URL)
	if err != nil {
		a.Logger.Error().Err(err).Msg("Error getting chain info")
		a.State.SetChainInfoError(err)
		a.DisplayWrapper.SetState(a.State)
		return
	}

	a.State.SetChainInfo(nodeStatus)
	a.State.SetChainInfoError(err)
	a.DisplayWrapper.SetState(a.State)
}

func (a *App) GoRefreshUpgrade() {
	defer a.HandlePanic()

	a.RefreshUpgrade()

	ticker := time.NewTicker(a.Config.UpgradeRefreshRate)
	defer ticker.Stop()

	for {
		select {
		case <-a.chStop:
			return
		case <-ticker.C:
			a.RefreshUpgrade()
		}
	}
}

func (a *App) RefreshUpgrade() {
	if a.IsPaused.Load() {
		return
	}

	if a.Config.HaltHeight > 0 {
		upgrade := &types.Upgrade{
			Name:   "halt-height upgrade",
			Height: a.Config.HaltHeight,
		}

		a.State.SetUpgrade(upgrade)
		a.DisplayWrapper.SetState(a.State)
		return
	}

	upgrade, err := a.DataFetcher.GetUpgradePlan()
	if err != nil {
		a.Logger.Error().Err(err).Msg("Error getting upgrade")
		a.State.SetUpgradePlanError(err)
		a.DisplayWrapper.SetState(a.State)
		return
	}

	a.State.SetUpgrade(upgrade)
	a.State.SetUpgradePlanError(err)
	a.DisplayWrapper.SetState(a.State)
}

func (a *App) GoRefreshBlockTime() {
	defer a.HandlePanic()

	a.RefreshBlockTime()

	ticker := time.NewTicker(a.Config.BlockTimeRefreshRate)
	defer ticker.Stop()

	for {
		select {
		case <-a.chStop:
			return
		case <-ticker.C:
			a.RefreshBlockTime()
		}
	}
}

func (a *App) RefreshBlockTime() {
	if a.IsPaused.Load() {
		return
	}

	blockTime, err := a.DataFetcher.GetBlockTime()
	if err != nil {
		a.Logger.Error().Err(err).Msg("Error getting block time")
		return
	}

	a.State.SetBlockTime(blockTime)
	a.DisplayWrapper.SetState(a.State)
}

func (a *App) GoRefreshNetInfo() {
	defer a.HandlePanic()

	a.RefreshNetInfo()

	ticker := time.NewTicker(a.Config.RefreshRate)
	defer ticker.Stop()

	for {
		select {
		case <-a.chStop:
			return
		case <-ticker.C:
			a.RefreshNetInfo()
		}
	}
}

func (a *App) RefreshNetInfo() {
	if a.IsPaused.Load() {
		return
	}

	netInfo, err := a.DataFetcher.GetNetInfo(a.State.CurrentRPC().URL)
	if err != nil {
		a.Logger.Error().Err(err).Msg("Error getting netInfo")
		return
	}

	a.State.SetNetInfo(netInfo)
	a.DisplayWrapper.SetState(a.State)
}

func (a *App) SubscribeCometBFT() {
	defer a.HandlePanic()

	a.Logger.Info().Msg("connecting to websocket")

	mbEvents := butils.NewMailbox[ctypes.TMEventData](1000)
	a.DataFetcher.Subscribe(mbEvents, "Vote")
	a.DataFetcher.Subscribe(mbEvents, "NewRound")

	for {
		select {
		case <-a.chStop:
			return
		case <-mbEvents.Notify():
			events := mbEvents.RetrieveAll()
			a.State.AddCometBFTEvents(events)

			if len(events) > 0 {
				ctx := context.Background()
				if err := a.ConsensusStore.StoreCometBFTEvents(ctx, events, a.State.GetTMValidators()); err != nil {
					a.Logger.Error().Err(err).Msg("Failed to persist CometBFT events to database")
				}
			}

			a.DisplayWrapper.SetState(a.State)
		}
	}
}

func (a *App) DisplayLogs() {
	for {
		select {
		case <-a.chStop:
			return
		case <-a.mbLogs.Notify():
			for _, line := range a.mbLogs.RetrieveAll() {
				a.DisplayWrapper.DebugText(line)
			}
		}
	}
}

func (a *App) HandlePanic() {
	if r := recover(); r != nil {
		a.Logger.Error().Interface("panic", r).Msg("Panic caught in goroutine")
		a.shutdown()
		panic(r)
	}
}

// shutdown is called from HandlePanic. Best-effort cleanup before
// the re-panic takes the process down.
func (a *App) shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if a.DisplayWrapper != nil {
		_ = a.DisplayWrapper.Stop(ctx)
	}
	_ = a.Stop(ctx)
	a.restoreTerminal()
}

// restoreTerminal restores terminal state and cleans up.
func (a *App) restoreTerminal() {
	// Get the default screen to ensure proper cleanup
	screen, err := tcell.NewScreen()
	if err == nil && screen != nil {
		// Initialize screen briefly to ensure proper state
		if err := screen.Init(); err == nil {
			// Clear the screen and restore cursor
			screen.Clear()
			screen.ShowCursor(0, 0)
			screen.Sync()
			screen.Fini()
		}
	}

	// Send additional terminal reset sequences
	fmt.Print("\033[?25h")   // Show cursor
	fmt.Print("\033[0m")     // Reset colors
	fmt.Print("\033[2J")     // Clear screen
	fmt.Print("\033[H")      // Move cursor to top-left
	fmt.Print("\033[?1049l") // Exit alternate screen buffer

	// Run any additional cleanup functions
	for _, cleanup := range a.cleanupFuncs {
		cleanup()
	}
}

// addCleanupFunc registers a function to be called during shutdown.
func (a *App) addCleanupFunc(fn func()) {
	a.cleanupFuncs = append(a.cleanupFuncs, fn)
}

// databaseCleanupRoutine runs periodic retention cleanup.
func (a *App) databaseCleanupRoutine() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	ctx := context.Background()
	if err := a.ConsensusStore.CleanupOldData(ctx); err != nil {
		a.Logger.Error().Err(err).Msg("Failed to cleanup old database data")
	}

	for {
		select {
		case <-a.chStop:
			return
		case <-ticker.C:
			if err := a.ConsensusStore.CleanupOldData(ctx); err != nil {
				a.Logger.Error().Err(err).Msg("Failed to cleanup old database data")
			} else {
				a.Logger.Debug().Msg("Database cleanup completed successfully")
			}
		}
	}
}

// runAnalyticsMode runs the application in analytics mode.
func (a *App) runAnalyticsMode() {
	if a.DB == nil {
		fmt.Printf("Error: Analytics mode requires database to be enabled. Use --database-path flag.\n")
		os.Exit(1)
	}

	cliAnalytics := analytics.NewCLIAnalytics(a.DB, a.Logger)
	ctx := context.Background()

	switch a.Config.AnalyticsCommand {
	case "performance":
		if a.Config.AnalyticsValidator == "" {
			fmt.Printf("Error: --analytics-validator is required for performance analysis\n")
			os.Exit(1)
		}

		err := cliAnalytics.PrintValidatorPerformance(ctx, a.Config.AnalyticsValidator, a.Config.AnalyticsTimeWindow)
		if err != nil {
			fmt.Printf("Error running performance analysis: %v\n", err)
			os.Exit(1)
		}

	case "rankings":
		err := cliAnalytics.PrintValidatorRankings(ctx, a.Config.AnalyticsTimeWindow, 20)
		if err != nil {
			fmt.Printf("Error running rankings analysis: %v\n", err)
			os.Exit(1)
		}

	case "timeseries":
		if a.Config.AnalyticsValidator == "" {
			fmt.Printf("Error: --analytics-validator is required for time series analysis\n")
			os.Exit(1)
		}

		err := cliAnalytics.PrintPerformanceTimeSeries(ctx, a.Config.AnalyticsValidator, a.Config.AnalyticsTimeWindow)
		if err != nil {
			fmt.Printf("Error running time series analysis: %v\n", err)
			os.Exit(1)
		}

	case "debug":
		err := cliAnalytics.PrintDatabaseSummary(ctx)
		if err != nil {
			fmt.Printf("Error running database debug: %v\n", err)
			os.Exit(1)
		}

	case "diagnose":
		if a.Config.AnalyticsValidator == "" {
			fmt.Printf("Error: --analytics-validator is required for validator diagnosis\n")
			os.Exit(1)
		}

		err := cliAnalytics.DiagnoseValidator(ctx, a.Config.AnalyticsValidator, a.Config.AnalyticsTimeWindow)
		if err != nil {
			fmt.Printf("Error running validator diagnosis: %v\n", err)
			os.Exit(1)
		}

	case "search":
		if a.Config.AnalyticsValidator == "" {
			fmt.Printf("Error: --analytics-validator is required as search term for validator search\n")
			os.Exit(1)
		}

		err := cliAnalytics.SearchValidators(ctx, a.Config.AnalyticsValidator)
		if err != nil {
			fmt.Printf("Error running validator search: %v\n", err)
			os.Exit(1)
		}

	default:
		fmt.Printf("Error: Unknown analytics command '%s'. Available commands: performance, rankings, timeseries, debug, diagnose, search\n", a.Config.AnalyticsCommand)
		os.Exit(1)
	}
}
