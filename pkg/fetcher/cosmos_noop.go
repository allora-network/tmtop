package fetcher

import (
	"main/pkg/types"
)

type noopDataFetcher struct{}

var _ cosmosRPCFetcher = (*noopDataFetcher)(nil)

func newNoopDataFetcher() *noopDataFetcher {
	return &noopDataFetcher{}
}

func (f *noopDataFetcher) GetValidators() (types.CosmosValidators, error) {
	return types.CosmosValidators{}, nil
}

func (f *noopDataFetcher) GetUpgradePlan() (*types.Upgrade, error) {
	return nil, nil
}
