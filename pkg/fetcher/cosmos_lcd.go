package fetcher

import (
	"main/pkg/config"
	"main/pkg/http"
	"main/pkg/types"

	upgradeTypes "cosmossdk.io/x/upgrade/types"
	"github.com/cosmos/cosmos-sdk/codec"
	codecTypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/std"
	sdkTypes "github.com/cosmos/cosmos-sdk/types"
	stakingTypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/rs/zerolog"
)

type CosmosLCDDataFetcher struct {
	client      *http.Client
	parseCodec  *codec.ProtoCodec
	LegacyAmino *codec.LegacyAmino
}

var _ cosmosRPCFetcher = (*CosmosLCDDataFetcher)(nil)

func NewCosmosLCDDataFetcher(config *config.Config, logger zerolog.Logger) *CosmosLCDDataFetcher {
	interfaceRegistry := codecTypes.NewInterfaceRegistry()
	std.RegisterInterfaces(interfaceRegistry)

	return &CosmosLCDDataFetcher{
		client:     http.NewClient(logger, "cosmos_lcd_data_fetcher", config.LCDHost),
		parseCodec: codec.NewProtoCodec(interfaceRegistry),
	}
}

func (f *CosmosLCDDataFetcher) GetValidators() (types.CosmosValidators, error) {
	bytes, err := f.client.GetPlain("/cosmos/staking/v1beta1/validators?status=BOND_STATUS_BONDED&pagination.limit=1000")
	if err != nil {
		return nil, err
	}

	var validatorsResponse stakingTypes.QueryValidatorsResponse
	if err := f.parseCodec.UnmarshalJSON(bytes, &validatorsResponse); err != nil {
		return nil, err
	}

	validators := make(types.CosmosValidators, len(validatorsResponse.Validators))

	for i, validator := range validatorsResponse.Validators {
		if err := validator.UnpackInterfaces(f.parseCodec); err != nil {
			return nil, err
		}

		addr, err := validator.GetConsAddr()
		if err != nil {
			return nil, err
		}

		consPubKey, err := validator.ConsPubKey()
		cometConsPubKey, err := validator.CmtConsPublicKey()

		validators[i] = types.CosmosValidator{
			Moniker:              validator.GetMoniker(),
			OperatorAddress:      validator.GetOperator(),
			ConsensusAddress:     sdkTypes.ConsAddress(addr).String(),
			ConsensusPubkey:      consPubKey,
			CometConsensusPubkey: cometConsPubKey,
		}
	}

	return validators, nil
}

func (f *CosmosLCDDataFetcher) GetUpgradePlan() (*types.Upgrade, error) {
	var response upgradeTypes.QueryCurrentPlanResponse
	err := f.client.Get("/cosmos/upgrade/v1beta1/current_plan", &response)
	if err != nil {
		return nil, err
	} else if response.Plan == nil {
		return nil, nil
	}

	return &types.Upgrade{
		Name:   response.Plan.Name,
		Height: response.Plan.Height,
	}, nil
}
