package fetcher

import (
	"encoding/json"
	"fmt"
	configPkg "main/pkg/config"
	"main/pkg/types"
	"net/http"
	"net/url"
	"time"

	"github.com/cosmos/cosmos-sdk/std"
	"github.com/rs/zerolog"

	"github.com/cosmos/cosmos-sdk/codec"
	codecTypes "github.com/cosmos/cosmos-sdk/codec/types"
	queryTypes "github.com/cosmos/cosmos-sdk/types/query"
	stakingTypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	upgradeTypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
)

type CosmosDataFetcher struct {
	Config configPkg.Config
	Logger zerolog.Logger

	Registry   codecTypes.InterfaceRegistry
	ParseCodec *codec.ProtoCodec
}

func NewCosmosDataFetcher(config configPkg.Config, logger zerolog.Logger) *CosmosDataFetcher {
	interfaceRegistry := codecTypes.NewInterfaceRegistry()
	std.RegisterInterfaces(interfaceRegistry)
	parseCodec := codec.NewProtoCodec(interfaceRegistry)

	return &CosmosDataFetcher{
		Config:     config,
		Logger:     logger.With().Str("component", "cosmos_data_fetcher").Logger(),
		Registry:   interfaceRegistry,
		ParseCodec: parseCodec,
	}
}

func (f *CosmosDataFetcher) AbciQuery(
	method string,
	message codec.ProtoMarshaler,
	output codec.ProtoMarshaler,
	host string,
) error {
	dataBytes, err := message.Marshal()
	if err != nil {
		return err
	}

	methodName := fmt.Sprintf("\"%s\"", method)
	queryURL := fmt.Sprintf(
		"/abci_query?path=%s&data=0x%x",
		url.QueryEscape(methodName),
		dataBytes,
	)

	var response types.AbciQueryResponse
	if err := f.Get(queryURL, &response, host); err != nil {
		return err
	}

	if response.Result.Response.Code != 0 {
		return fmt.Errorf(
			"error in Tendermint response: expected code 0, but got %d, error: %s",
			response.Result.Response.Code,
			response.Result.Response.Log,
		)
	}

	return output.Unmarshal(response.Result.Response.Value)
}

func (f *CosmosDataFetcher) GetValidators() (*types.ChainValidators, error) {
	query := stakingTypes.QueryValidatorsRequest{
		Pagination: &queryTypes.PageRequest{
			Limit: 1000,
		},
	}

	var validatorsResponse stakingTypes.QueryValidatorsResponse
	if err := f.AbciQuery(
		"/cosmos.staking.v1beta1.Query/Validators",
		&query,
		&validatorsResponse,
		f.Config.GetProviderOrConsumerHost(),
	); err != nil {
		return nil, err
	}

	validators := make(types.ChainValidators, len(validatorsResponse.Validators))

	for index, validator := range validatorsResponse.Validators {
		if err := validator.UnpackInterfaces(f.ParseCodec); err != nil {
			return nil, err
		}

		addr, err := validator.GetConsAddr()
		if err != nil {
			return nil, err
		}

		validators[index] = types.ChainValidator{
			Moniker: validator.GetMoniker(),
			Address: fmt.Sprintf("%x", addr),
		}
	}

	return &validators, nil
}

func (f *CosmosDataFetcher) GetUpgradePlan() (*types.Upgrade, error) {
	query := upgradeTypes.QueryCurrentPlanRequest{}

	var response upgradeTypes.QueryCurrentPlanResponse
	if err := f.AbciQuery(
		"/cosmos.upgrade.v1beta1.Query/CurrentPlan",
		&query,
		&response,
		f.Config.RPCHost,
	); err != nil {
		return nil, err
	}

	if response.Plan == nil {
		return nil, nil
	}

	return &types.Upgrade{
		Name:   response.Plan.Name,
		Height: response.Plan.Height,
	}, nil
}

func (f *CosmosDataFetcher) Get(relativeURL string, target interface{}, host string) error {
	client := &http.Client{Timeout: 300 * time.Second}
	start := time.Now()

	fullURL := fmt.Sprintf("%s%s", host, relativeURL)

	req, err := http.NewRequest(http.MethodGet, fullURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", "tmtop")

	f.Logger.Debug().Str("url", fullURL).Msg("Doing a query...")

	res, err := client.Do(req)
	if err != nil {
		f.Logger.Warn().Str("url", fullURL).Err(err).Msg("Query failed")
		return err
	}
	defer res.Body.Close()

	f.Logger.Debug().Str("url", fullURL).Dur("duration", time.Since(start)).Msg("Query is finished")

	return json.NewDecoder(res.Body).Decode(target)
}
