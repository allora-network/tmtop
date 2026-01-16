package fetcher

import (
	"main/pkg/types"
)

type noopDataFetcher struct{}

var _ cosmosRPCFetcher = (*noopDataFetcher)(nil)

func newNoopDataFetcher() *noopDataFetcher {
	return &noopDataFetcher{}
}

func (f *noopDataFetcher) GetValidators() (types.ChainValidators, error) {
	return types.ChainValidators{}, nil
}

func (f *noopDataFetcher) GetUpgradePlan() (*types.Upgrade, error) {
	return nil, nil
}
