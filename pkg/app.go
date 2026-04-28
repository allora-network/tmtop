package pkg

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"main/pkg/analytics"
	configPkg "main/pkg/config"
	"main/pkg/crawler"
	"main/pkg/db"
	"main/pkg/display"
	"main/pkg/fetcher"
	tmhttp "main/pkg/http"
	loggerPkg "main/pkg/logger"
	"main/pkg/refresher"
	"main/pkg/topology"
	"main/pkg/types"

	butils "github.com/brynbellomy/go-utils"
	cnstypes "github.com/cometbft/cometbft/consensus/types"
	ctypes "github.com/cometbft/cometbft/types"
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

	IsPaused atomic.Bool

	chStop    chan struct{}
	closeOnce sync.Once
	wgDone    *sync.WaitGroup
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
		Logger:         logger,
		Version:        version,
		Config:         config,
		State:          state,
		mbLogs:         mbLogs,
		DB:             database,
		ConsensusStore: consensusStore,
		DataFetcher:    fetcher.NewDataFetcher(config, state, logger),
		chStop:         make(chan struct{}),
		wgDone:         &sync.WaitGroup{},
	}
	// Display needs a pointer to the pause flag so its input handler
	// can toggle it in-place.
	app.DisplayWrapper = display.NewWrapper(config, state, logger, &app.IsPaused, version)
	return app
}

func (a *App) Start() {
	// Check if analytics mode is enabled
	if a.Config.AnalyticsMode {
		analytics.Run(a.Config, a.DB, a.Logger)
		return
	}

	if a.Config.WithTopologyAPI {
		a.spawn(a.ServeTopology)
	}

	c := crawler.New(a.Config.RPCHost, a.DataFetcher, a.State, a.Logger, a.chStop)
	a.spawn(c.Run)

	a.spawnRefresher("consensus", a.Config.RefreshRate, a.RefreshConsensus)
	a.spawnRefresher("comet-node-info", a.Config.ChainInfoRefreshRate, a.RefreshCometNodeInfo)
	a.spawnRefresher("upgrade", a.Config.UpgradeRefreshRate, a.RefreshUpgrade)
	a.spawnRefresher("block-time", a.Config.BlockTimeRefreshRate, a.RefreshBlockTime)
	a.spawnRefresher("net-info", a.Config.RefreshRate, a.RefreshNetInfo)

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

// spawnRefresher starts a named refresh loop. The loop runs refresh
// immediately, then on each interval tick, exiting on chStop. Panic
// recovery is the same as any other refresh goroutine via HandlePanic.
func (a *App) spawnRefresher(name string, interval time.Duration, refresh func()) {
	a.spawn(func() {
		defer a.HandlePanic()
		refresher.New(name, interval, refresh, a.chStop, a.Logger).Run()
	})
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
		a.Config.TopologyListenAddr,
		//":8080",
		topology.WithHTTPTopologyAPI(a.State),
		topology.WithHTTPPeersAPI(a.State),
		topology.WithHTTPDebugAPI(a.State),
		topology.WithFrontendStaticAssets(),
	)
	if err := a.topologyServer.Serve(); err != nil && err != http.ErrServerClosed {
		a.Logger.Error().Err(err).Msg("topology server error")
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
		a.State.SetConsensusHeight(consensus.Height, int64(consensus.Round), int64(consensus.Step), consensus.StartTime)

		// Seed the proposer for (height, round) directly from
		// /consensus_state. NewRound websocket events only fire at
		// round transitions, so without this seed the proposer-row
		// highlight stays empty at startup and after reconnects.
		if consensus.Validators != nil && consensus.Validators.Proposer != nil {
			a.State.VotesByRound.AddProposer(
				consensus.Height,
				consensus.Round,
				consensus.Validators.Proposer.Address.String(),
			)
		}

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
// the re-panic takes the process down. Display.Stop already restores
// the terminal; we just add a last-resort second call in case Stop
// fails.
func (a *App) shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if a.DisplayWrapper != nil {
		_ = a.DisplayWrapper.Stop(ctx)
	}
	_ = a.Stop(ctx)
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

