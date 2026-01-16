package fetcher

import (
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
	var cosmosFetcher cosmosRPCFetcher
	if config.ChainType == "tendermint" {
		cosmosFetcher = newNoopDataFetcher()
	} else if config.ChainType == "cosmos-lcd" {
		cosmosFetcher = NewCosmosLCDDataFetcher(config, logger)
	} else {
		cosmosFetcher = newCosmosRPCDataFetcher(config, state, logger)
	}

	return &DataFetcher{
		logger:        logger,
		cosmosFetcher: cosmosFetcher,
		cometFetcher:  NewCometRPC(config, state, logger),
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
		cosmValMap[cosmosVal.ConsensusPubkey.Address().String()] = cosmosVal
	}

	var vals []types.TMValidator
	for i, cometVal := range cometVals {
		// Find matching Cosmos validator by consensus address
		cosmosVal := cosmValMap[cometVal.PubKey.Address().String()]
		vals = append(vals, types.TMValidator{
			CometValidator:     cometVal,
			CosmosValidator:    &cosmosVal,
			Index:              i,
			VotingPowerPercent: big.NewFloat(float64(cometVal.VotingPower / totalVotingPower)),
		})
	}
	return vals, nil
}
