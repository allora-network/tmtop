package fetcher

import (
	"context"
	"math/big"
	"time"

	configPkg "main/pkg/config"
	"main/pkg/types"

	butils "github.com/brynbellomy/go-utils"
	cnstypes "github.com/cometbft/cometbft/consensus/types"
	rpctypes "github.com/cometbft/cometbft/rpc/core/types"
	ctypes "github.com/cometbft/cometbft/types"
	"github.com/rs/zerolog"
)

type DataFetcher struct {
	logger        zerolog.Logger
	cosmosFetcher cosmosRPCFetcher
	cometFetcher  *CometRPC
	cometWS       *CometRPCWebsocket
}

type cosmosRPCFetcher interface {
	GetValidators() (types.CosmosValidators, error)
	GetUpgradePlan() (*types.Upgrade, error)
}

func NewDataFetcher(config *configPkg.Config, state *types.State, logger zerolog.Logger) *DataFetcher {
	getCurrentURL := func() string { return state.CurrentRPC().URL }

	var cosmosFetcher cosmosRPCFetcher
	if config.ChainType == "tendermint" {
		cosmosFetcher = newNoopDataFetcher()
	} else if config.ChainType == "cosmos-lcd" {
		cosmosFetcher = NewCosmosLCDDataFetcher(config, logger)
	} else {
		cosmosFetcher = newCosmosRPCDataFetcher(config, getCurrentURL, logger)
	}

	return &DataFetcher{
		logger:        logger,
		cosmosFetcher: cosmosFetcher,
		cometFetcher:  NewCometRPC(config, getCurrentURL, logger),
		cometWS:       NewCometRPCWebsocket(config.RPCHost, logger),
	}
}

func (f *DataFetcher) GetConsensusState() (*cnstypes.RoundState, error) {
	return f.cometFetcher.GetConsensusState()
}

func (f *DataFetcher) GetCometNodeStatus(rpcURL string) (*rpctypes.ResultStatus, error) {
	return f.cometFetcher.WithEndpoint(rpcURL).GetCometNodeStatus()
}

func (f *DataFetcher) Block(height int64) (*rpctypes.ResultBlock, error) {
	return f.cometFetcher.Block(height)
}

func (f *DataFetcher) GetBlockTime() (time.Duration, error) {
	return f.cometFetcher.GetBlockTime()
}

func (f *DataFetcher) GetNetInfo(rpcURL string) (*types.NetInfo, error) {
	return f.cometFetcher.WithEndpoint(rpcURL).GetNetInfo()
}

func (f *DataFetcher) GetUpgradePlan() (*types.Upgrade, error) {
	return f.cosmosFetcher.GetUpgradePlan()
}

func (f *DataFetcher) Subscribe(mb *butils.Mailbox[ctypes.TMEventData], events ...string) {
	f.cometWS.Subscribe(mb, events...)
}

// Close stops the websocket subscription and releases fetcher resources.
// Safe to call multiple times. The context governs how long to wait for
// the websocket goroutine to exit.
func (f *DataFetcher) Close(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		f.cometWS.Close()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (f *DataFetcher) GetValidators() ([]types.TMValidator, error) {
	cometVals, err := f.cometFetcher.GetValidators()
	if err != nil {
		return nil, err
	}

	cosmosVals, err := f.cosmosFetcher.GetValidators()
	if err != nil {
		return nil, err
	}

	// Calculate total voting power first
	totalVotingPower := int64(0)
	for _, validator := range cometVals {
		totalVotingPower += validator.VotingPower
	}

	cosmValMap := make(map[string]types.CosmosValidator, len(cosmosVals))
	for _, cosmosVal := range cosmosVals {
		// Skip Cosmos validators we can't index by consensus address —
		// e.g. partial fallback responses that omit ConsensusPubkey.
		// Without this guard the .Address() call panics.
		if cosmosVal.ConsensusPubkey == nil {
			continue
		}
		cosmValMap[cosmosVal.ConsensusPubkey.Address().String()] = cosmosVal
	}

	var vals []types.TMValidator
	for i, cometVal := range cometVals {
		// Only attach a CosmosValidator pointer when we actually matched
		// one. Previously this took &cosmosVal of the zero-value returned
		// from a missed map lookup, which surfaces downstream as a non-nil
		// pointer with a nil ConsensusPubkey — and panics in the crawler
		// (and anywhere else doing CosmosValidator.ConsensusPubkey.Address()).
		var cosmosValPtr *types.CosmosValidator
		if cosmosVal, ok := cosmValMap[cometVal.PubKey.Address().String()]; ok {
			cosmosValCopy := cosmosVal
			cosmosValPtr = &cosmosValCopy
		}
		votingPowerPercent := big.NewFloat(0)
		if totalVotingPower > 0 {
			votingPowerPercent = big.NewFloat(float64(cometVal.VotingPower) / float64(totalVotingPower) * 100)
		}
		vals = append(vals, types.TMValidator{
			CometValidator:     cometVal,
			CosmosValidator:    cosmosValPtr,
			Index:              i,
			VotingPowerPercent: votingPowerPercent,
		})
	}
	return vals, nil
}
