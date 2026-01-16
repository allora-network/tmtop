package pkg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	DisplayWrapper *display.Wrapper
	State          *types.State
	LogChannel     chan string

	DB             *db.DB
	ConsensusStore *db.ConsensusStore

	DataFetcher *fetcher.DataFetcher

	mbRPCURLs        *butils.Mailbox[string]
	rpcURLsLastFetch map[string]time.Time

	PauseChannel chan bool
	IsPaused     bool

	cleanupFuncs []func()
}

func NewApp(config *configPkg.Config, version string) *App {
	logChannel := make(chan string, 1000)
	pauseChannel := make(chan bool)

	logger := loggerPkg.GetLogger(logChannel, config).
		With().
		Str("component", "app_manager").
		Logger()

	state := types.NewState(config.RPCHost, logger)

	// Initialize database if configured
	var database *db.DB
	var consensusStore *db.ConsensusStore

	fmt.Println("max retain days", config.MaxRetainDays)

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

		fmt.Println("Using database path:", dbPath)

		dbConfig := db.Config{
			DatabasePath:    dbPath,
			MaxRetainBlocks: config.MaxRetainBlocks,
			MaxRetainDays:   config.MaxRetainDays,
		}

		var err error
		database, err = db.New(dbConfig)
		if err != nil {
			fmt.Println("Failed to initialize database:", err)
			os.Exit(-1)
		}

		consensusStore = db.NewConsensusStore(database, logger)
		logger.Info().Str("path", dbPath).Msg("Database initialized successfully")
	}

	fmt.Println("DB:", database)

	return &App{
		Logger:           logger,
		Version:          version,
		Config:           config,
		DisplayWrapper:   display.NewWrapper(config, state, logger, pauseChannel, version),
		State:            state,
		LogChannel:       logChannel,
		DB:               database,
		ConsensusStore:   consensusStore,
		DataFetcher:      fetcher.NewDataFetcher(config, state, logger),
		mbRPCURLs:        butils.NewMailbox[string](1000),
		rpcURLsLastFetch: make(map[string]time.Time),
		PauseChannel:     pauseChannel,
		IsPaused:         false,
		cleanupFuncs:     make([]func(), 0),
	}
}

func (a *App) Start() {
	// Check if analytics mode is enabled
	if a.Config.AnalyticsMode {
		a.runAnalyticsMode()
		return
	}

	// Set up terminal cleanup on exit
	defer a.restoreTerminal()

	// Set up database cleanup on exit
	if a.DB != nil {
		defer func() {
			if err := a.DB.Close(); err != nil {
				a.Logger.Error().Err(err).Msg("Failed to close database")
			}
		}()
	}

	if a.Config.WithTopologyAPI {
		go a.ServeTopology()
		topology.LogChannel = a.LogChannel
	}

	go a.CrawlRPCURLs()

	go a.GoRefreshConsensus()
	go a.GoRefreshCometNodeInfo()
	go a.GoRefreshUpgrade()
	go a.GoRefreshBlockTime()
	go a.GoRefreshNetInfo()
	go a.SubscribeCometBFT()
	go a.DisplayLogs()
	go a.ListenForPause()

	// Start database cleanup routine if database is enabled
	if a.DB != nil && a.ConsensusStore != nil {
		go a.databaseCleanupRoutine()
	}

	a.DisplayWrapper.Start()
}

func (a *App) ServeTopology() {
	_ = tmhttp.NewServer(
		a.Config.TopologyListenAddr,
		topology.WithHTTPTopologyAPI(a.State),
		topology.WithHTTPPeersAPI(a.State),
		topology.WithHTTPDebugAPI(a.State),
		topology.WithFrontendStaticAssets(),
	).Serve()
}

func (a *App) CrawlRPCURLs() {
	a.mbRPCURLs.Deliver(a.Config.RPCHost)
	timer := time.NewTimer(15 * time.Second)

	for {
		select {
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

		case <-timer.C:
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
	done := make(chan bool)

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			a.RefreshConsensus()
		}
	}
}

func (a *App) RefreshConsensus() {
	if a.IsPaused {
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

	// Persist to database if available
	if a.ConsensusStore != nil && consensus != nil {
		ctx := context.Background()

		// Store validators and height information
		if err := a.ConsensusStore.StoreValidators(ctx, consensus.Height, vals); err != nil {
			a.Logger.Error().Err(err).Int64("height", consensus.Height).Msg("Failed to persist validators to database")
		}

		// Store round data from current state
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
	done := make(chan bool)

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			a.RefreshCometNodeInfo()
		}
	}
}

func (a *App) RefreshCometNodeInfo() {
	if a.IsPaused {
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
	done := make(chan bool)

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			a.RefreshUpgrade()
		}
	}
}

func (a *App) RefreshUpgrade() {
	if a.IsPaused {
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
	done := make(chan bool)

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			a.RefreshBlockTime()
		}
	}
}

func (a *App) RefreshBlockTime() {
	if a.IsPaused {
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
	done := make(chan bool)

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			a.RefreshNetInfo()
		}
	}
}

func (a *App) RefreshNetInfo() {
	if a.IsPaused {
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

	fmt.Println("connecting to websocket...")

	mbEvents := butils.NewMailbox[ctypes.TMEventData](1000)
	a.DataFetcher.Subscribe(mbEvents, "Vote")
	a.DataFetcher.Subscribe(mbEvents, "NewRound")

	for {
		select {
		case <-mbEvents.Notify():
			events := mbEvents.RetrieveAll()
			a.State.AddCometBFTEvents(events)

			// Persist events to database if available
			if a.ConsensusStore != nil && len(events) > 0 {
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
		logString := <-a.LogChannel
		a.DisplayWrapper.DebugText(logString)
	}
}

func (a *App) ListenForPause() {
	for {
		paused := <-a.PauseChannel
		a.IsPaused = paused
	}
}

func (a *App) HandlePanic() {
	if r := recover(); r != nil {
		a.Logger.Error().Interface("panic", r).Msg("Panic caught in goroutine")
		a.shutdown()
		panic(r)
	}
}

// shutdown performs graceful shutdown with terminal cleanup.
func (a *App) shutdown() {
	// Stop the tview application first
	if a.DisplayWrapper != nil && a.DisplayWrapper.App != nil {
		a.DisplayWrapper.App.Stop()
	}

	// Run all registered cleanup functions
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

// databaseCleanupRoutine runs periodic database cleanup.
func (a *App) databaseCleanupRoutine() {
	ticker := time.NewTicker(1 * time.Hour) // Run cleanup every hour
	defer ticker.Stop()

	// Run initial cleanup
	ctx := context.Background()
	if err := a.DB.CleanupOldData(ctx, db.Config{
		DatabasePath:    a.Config.DatabasePath,
		MaxRetainBlocks: a.Config.MaxRetainBlocks,
		MaxRetainDays:   a.Config.MaxRetainDays,
	}); err != nil {
		a.Logger.Error().Err(err).Msg("Failed to cleanup old database data")
	}

	for {
		select {
		case <-ticker.C:
			if err := a.DB.CleanupOldData(ctx, db.Config{
				DatabasePath:    a.Config.DatabasePath,
				MaxRetainBlocks: a.Config.MaxRetainBlocks,
				MaxRetainDays:   a.Config.MaxRetainDays,
			}); err != nil {
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
