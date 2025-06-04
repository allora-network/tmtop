package pkg

import (
	"fmt"
	"math/big"
	configPkg "main/pkg/config"
	"main/pkg/display"
	"main/pkg/fetcher"
	tmhttp "main/pkg/http"
	loggerPkg "main/pkg/logger"
	"main/pkg/topology"
	"main/pkg/types"
	"main/pkg/utils"
	"strconv"
	"strings"
	"sync"
	"time"

	butils "github.com/brynbellomy/go-utils"
	"github.com/cometbft/cometbft/crypto"
	"github.com/cometbft/cometbft/crypto/ed25519"
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

	// Direct fetcher instances instead of Aggregator
	TendermintClient  *fetcher.TendermintRPC
	DataFetcher       fetcher.DataFetcher
	CometRPCWebsocket *fetcher.CometRPCWebsocket

	mbRPCURLs        *butils.Mailbox[string]
	rpcURLsLastFetch map[string]time.Time

	PauseChannel chan bool
	IsPaused     bool

	// Terminal state management
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

	return &App{
		Logger:            logger,
		Version:           version,
		Config:            config,
		DisplayWrapper:    display.NewWrapper(config, state, logger, pauseChannel, version),
		State:             state,
		LogChannel:        logChannel,
		TendermintClient:  fetcher.NewTendermintRPC(config, state, logger),
		DataFetcher:       fetcher.GetDataFetcher(config, state, logger),
		CometRPCWebsocket: fetcher.NewCometRPCWebsocket(config.RPCHost, logger),
		mbRPCURLs:         butils.NewMailbox[string](1000),
		rpcURLsLastFetch:  make(map[string]time.Time),
		PauseChannel:      pauseChannel,
		IsPaused:          false,
		cleanupFuncs:      make([]func(), 0),
	}
}

func (a *App) Start() {
	// Set up terminal cleanup on exit
	defer a.restoreTerminal()

	if a.Config.WithTopologyAPI {
		go a.ServeTopology()
		topology.LogChannel = a.LogChannel
	}

	go a.CrawlRPCURLs()

	go a.GoRefreshConsensus()
	go a.GoRefreshValidators()
	go a.GoRefreshChainInfo()
	go a.GoRefreshUpgrade()
	go a.GoRefreshBlockTime()
	go a.GoRefreshNetInfo()
	go a.SubscribeCometBFT()
	go a.DisplayLogs()
	go a.ListenForPause()

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
				if lastFetch, ok := a.rpcURLsLastFetch[url]; ok && time.Now().Sub(lastFetch) < 15*time.Second {
					continue
				}
				a.rpcURLsLastFetch[url] = time.Now()

				url := url
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
		a.LogChannel <- fmt.Sprintf("error getting /net_info from %s: %v", rpcURL, err)
		return
	}

	status, err := a.TendermintClient.GetStatus(rpcURL)
	if err != nil {
		a.LogChannel <- fmt.Sprintf("error getting /status from %s: %v", rpcURL, err)
		return
	}

	var rpc types.RPC
	if known, ok := a.State.KnownRPCByURL(rpcURL); ok {
		rpc = known
	}
	rpc.ID = status.Result.NodeInfo.ID
	rpc.URL = rpcURL
	rpc.Moniker = status.Result.NodeInfo.Moniker
	rpc.ValidatorAddress = status.Result.ValidatorInfo.Address

	if status.Result.ValidatorInfo.Address != "" && a.State.ChainValidators != nil {
		for _, cv := range *a.State.ChainValidators {
			if strings.ToLower(cv.Address) == strings.ToLower(rpc.ValidatorAddress) {
				rpc.ValidatorMoniker = cv.Moniker
				break
			}
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
	var consensusError error
	var validatorsError error
	var validators []types.TendermintValidator
	var consensus *types.ConsensusStateResponse

	wg.Add(1)
	go func() {
		defer wg.Done()
		consensus, consensusError = a.TendermintClient.GetConsensusState()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		validators, validatorsError = a.TendermintClient.GetValidators()
	}()

	wg.Wait()

	if consensusError != nil {
		a.Logger.Error().Err(consensusError).Msg("Could not fetch consensus data")
		a.State.SetConsensusStateError(consensusError)
		a.DisplayWrapper.SetState(a.State)
		return
	}

	if validatorsError != nil {
		a.Logger.Error().Err(validatorsError).Msg("Could not fetch validators")
		a.State.SetConsensusStateError(validatorsError)
		a.DisplayWrapper.SetState(a.State)
		return
	}

	// Convert TendermintValidator data to TMValidators
	tmValidators := a.convertToTMValidators(validators, consensus)
	a.State.SetTMValidators(tmValidators)
	a.State.SetConsensusStateError(nil)
	a.DisplayWrapper.SetState(a.State)
}

func (a *App) GoRefreshValidators() {
	defer a.HandlePanic()

	a.RefreshValidators()

	ticker := time.NewTicker(a.Config.ValidatorsRefreshRate)
	done := make(chan bool)

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			a.RefreshValidators()
		}
	}
}

func (a *App) RefreshValidators() {
	if a.IsPaused {
		return
	}

	chainValidators, err := a.DataFetcher.GetValidators()
	if err != nil {
		a.DisplayWrapper.SetState(a.State)
		a.Logger.Error().Err(err).Msg("Error getting chain validators")
		return
	}

	a.State.SetChainValidators(chainValidators)
	a.DisplayWrapper.SetState(a.State)
}

func (a *App) GoRefreshChainInfo() {
	defer a.HandlePanic()

	a.RefreshChainInfo()

	ticker := time.NewTicker(a.Config.ChainInfoRefreshRate)
	done := make(chan bool)

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			a.RefreshChainInfo()
		}
	}
}

func (a *App) RefreshChainInfo() {
	if a.IsPaused {
		return
	}

	chainInfo, err := a.TendermintClient.GetStatus(a.State.CurrentRPC().URL)
	if err != nil {
		a.Logger.Error().Err(err).Msg("Error getting chain info")
		a.State.SetChainInfoError(err)
		a.DisplayWrapper.SetState(a.State)
		return
	}

	a.State.SetChainInfo(&chainInfo.Result)
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

	blockTime, err := a.TendermintClient.GetBlockTime()
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
	a.CometRPCWebsocket.Subscribe(mbEvents, "Vote")
	a.CometRPCWebsocket.Subscribe(mbEvents, "NewRound")

	for {
		select {
		case <-mbEvents.Notify():
			events := mbEvents.RetrieveAll()
			a.State.AddCometBFTEvents(events)
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

// shutdown performs graceful shutdown with terminal cleanup
func (a *App) shutdown() {
	// Stop the tview application first
	if a.DisplayWrapper != nil && a.DisplayWrapper.App != nil {
		a.DisplayWrapper.App.Stop()
	}

	// Run all registered cleanup functions
	a.restoreTerminal()
}

// restoreTerminal restores terminal state and cleans up
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

// addCleanupFunc registers a function to be called during shutdown
func (a *App) addCleanupFunc(fn func()) {
	a.cleanupFuncs = append(a.cleanupFuncs, fn)
}

// convertToTMValidators converts TendermintValidator slice to TMValidators
func (a *App) convertToTMValidators(validators []types.TendermintValidator, consensus *types.ConsensusStateResponse) types.TMValidators {
	tmValidators := make(types.TMValidators, 0, len(validators))
	
	// Calculate total voting power first
	totalVotingPower := int64(0)
	for _, validator := range validators {
		votingPower, err := strconv.ParseInt(validator.VotingPower, 10, 64)
		if err != nil {
			a.Logger.Error().Err(err).Str("address", validator.Address).Msg("Failed to parse voting power")
			continue
		}
		totalVotingPower += votingPower
	}
	
	for i, validator := range validators {
		// Convert TendermintValidator to CometBFT Validator
		cometValidator, err := a.convertToCometBFTValidator(validator)
		if err != nil {
			a.Logger.Error().Err(err).Str("address", validator.Address).Msg("Failed to convert validator")
			continue
		}
		
		// Calculate voting power percentage
		votingPowerPercent := big.NewFloat(0)
		if totalVotingPower > 0 {
			validatorPower := big.NewFloat(float64(cometValidator.VotingPower))
			total := big.NewFloat(float64(totalVotingPower))
			votingPowerPercent.Quo(validatorPower, total)
			votingPowerPercent.Mul(votingPowerPercent, big.NewFloat(100))
		}
		
		tmValidator := types.TMValidator{
			Validator:          *cometValidator,
			Index:              i,
			VotingPowerPercent: votingPowerPercent,
		}
		
		// Add chain validator info if available
		if a.State.ChainValidators != nil {
			for _, cv := range *a.State.ChainValidators {
				if cv.Address == validator.Address {
					tmValidator.ChainValidator = &cv
					break
				}
			}
		}
		
		tmValidators = append(tmValidators, tmValidator)
	}
	
	return tmValidators
}

// convertToCometBFTValidator converts a TendermintValidator to CometBFT Validator
func (a *App) convertToCometBFTValidator(validator types.TendermintValidator) (*ctypes.Validator, error) {
	// Parse voting power
	votingPower, err := strconv.ParseInt(validator.VotingPower, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid voting power %s: %w", validator.VotingPower, err)
	}
	
	// Parse public key - for now, create a simple ed25519 key
	// This is a simplified approach since we primarily need the address and voting power for display
	pubKeyBytes, err := utils.Base64ToBytes(validator.PubKey.PubKeyBase64)
	if err != nil {
		return nil, fmt.Errorf("invalid public key %s: %w", validator.PubKey.PubKeyBase64, err)
	}
	
	// Create ed25519 public key (most common type)
	var pubKey crypto.PubKey
	if len(pubKeyBytes) == 32 { // ed25519 key length
		pubKey = ed25519.PubKey(pubKeyBytes)
	} else {
		// Fallback: create a minimal implementation that just handles address generation
		return nil, fmt.Errorf("unsupported public key type or length: %d", len(pubKeyBytes))
	}
	
	// Create CometBFT validator using the constructor
	cometValidator := ctypes.NewValidator(pubKey, votingPower)
	
	return cometValidator, nil
}
