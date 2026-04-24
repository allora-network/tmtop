package fetcher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"main/pkg/config"
	"main/pkg/http"
	"main/pkg/types"
	"main/pkg/utils"

	upgradeTypes "cosmossdk.io/x/upgrade/types"
	"github.com/cosmos/cosmos-sdk/codec"
	codecTypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptoTypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/cosmos/cosmos-sdk/std"
	sdkTypes "github.com/cosmos/cosmos-sdk/types"
	queryTypes "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/cosmos/cosmos-sdk/x/auth/tx"
	genutilTypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	stakingTypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	providerTypes "github.com/cosmos/interchain-security/v6/x/ccv/provider/types"
	"github.com/rs/zerolog"
)

type CosmosRPCDataFetcher struct {
	config         *config.Config
	logger         zerolog.Logger
	getURL         func() string
	providerClient *http.Client

	registry   codecTypes.InterfaceRegistry
	parseCodec *codec.ProtoCodec
	txDecoder  sdkTypes.TxDecoder
}

var _ cosmosRPCFetcher = (*CosmosRPCDataFetcher)(nil)

func newCosmosRPCDataFetcher(config *config.Config, getURL func() string, logger zerolog.Logger) *CosmosRPCDataFetcher {
	interfaceRegistry := codecTypes.NewInterfaceRegistry()
	std.RegisterInterfaces(interfaceRegistry)
	stakingTypes.RegisterInterfaces(interfaceRegistry) // for MsgCreateValidator for gentx parsing

	parseCodec := codec.NewProtoCodec(interfaceRegistry)

	return &CosmosRPCDataFetcher{
		config:         config,
		getURL:         getURL,
		logger:         logger.With().Str("component", "cosmos_data_fetcher").Logger(),
		providerClient: http.NewClient(logger, "cosmos_data_fetcher", config.ProviderRPCHost),
		registry:       interfaceRegistry,
		parseCodec:     parseCodec,
		txDecoder:      tx.NewTxConfig(parseCodec, tx.DefaultSignModes).TxJSONDecoder(),
	}
}

func (f *CosmosRPCDataFetcher) getProviderOrConsumerClient() *http.Client {
	if f.config.ProviderRPCHost != "" {
		return f.providerClient
	}
	return f.client()
}

func (f *CosmosRPCDataFetcher) client() *http.Client {
	return http.NewClient(f.logger, "cosmos_data_fetcher", f.getURL())
}

func (f *CosmosRPCDataFetcher) abciQuery(
	method string,
	message codec.ProtoMarshaler, //nolint:staticcheck
	output codec.ProtoMarshaler, //nolint:staticcheck
	client *http.Client,
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
	if err := client.Get(queryURL, &response); err != nil {
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

func (f *CosmosRPCDataFetcher) GetValidators() (types.CosmosValidators, error) {
	query := stakingTypes.QueryValidatorsRequest{
		Pagination: &queryTypes.PageRequest{
			Limit: 1000,
		},
	}

	var validatorsResponse stakingTypes.QueryValidatorsResponse
	if err := f.abciQuery(
		"/cosmos.staking.v1beta1.Query/Validators",
		&query,
		&validatorsResponse,
		f.getProviderOrConsumerClient(),
	); err != nil {
		if strings.Contains(err.Error(), " please wait for first block") {
			return f.getGenesisValidators()
		}
		return nil, err
	}

	validators := make(types.CosmosValidators, len(validatorsResponse.Validators))

	for i, validator := range validatorsResponse.Validators {
		v, err := f.parseValidator(validator)
		if err != nil {
			return nil, err
		}
		validators[i] = v
	}

	if !f.config.IsConsumer() {
		return validators, nil
	}

	var assignedKeysResponse providerTypes.QueryAllPairsValConsAddrByConsumerResponse
	err := f.abciQuery(
		"/interchain_security.ccv.provider.v1.Query/QueryAllPairsValConsAddrByConsumer",
		&providerTypes.QueryAllPairsValConsAddrByConsumerRequest{ConsumerId: f.config.ConsumerID},
		&assignedKeysResponse,
		f.providerClient,
	)
	if err != nil {
		return nil, err
	}

	for i, validator := range validators {
		assignedConsensusAddr, found := utils.Find(
			assignedKeysResponse.PairValConAddr,
			func(i *providerTypes.PairValConAddrProviderAndConsumer) bool {
				equal, compareErr := utils.CompareTwoBech32(i.ProviderAddress, validator.ConsensusAddress)
				if compareErr != nil {
					f.logger.Error().
						Str("operator_address", validator.OperatorAddress).
						Str("first", i.ProviderAddress).
						Str("second", validator.ConsensusAddress).
						Msg("Error converting bech32 address")
					return false
				}

				return equal
			},
		)

		if found {
			addr, _ := sdkTypes.ConsAddressFromBech32(assignedConsensusAddr.ConsumerAddress)

			validators[i].AssignedAddress = addr.String()
			validators[i].RawAssignedAddress = fmt.Sprintf("%X", addr)
		}
	}

	return validators, nil
}

func (f *CosmosRPCDataFetcher) getGenesisValidators() (types.CosmosValidators, error) {
	f.logger.Info().Msg("Fetching genesis validators...")

	genesisChunks := make([][]byte, 0)
	var chunk int64 = 0

	for {
		f.logger.Info().Int64("chunk", chunk).Msg("Fetching genesis chunk...")
		genesisChunk, total, err := f.getGenesisChunk(chunk)
		f.logger.Info().Int64("chunk", chunk).Int64("total", total).Msg("Fetched genesis chunk...")
		if err != nil {
			return nil, err
		}

		genesisChunks = append(genesisChunks, genesisChunk)

		if chunk >= total-1 {
			break
		}

		chunk++
	}

	genesisBytes := bytes.Join(genesisChunks, []byte{})
	f.logger.Info().Int("length", len(genesisBytes)).Msg("Fetched genesis")

	var genesisStruct types.Genesis

	if err := json.Unmarshal(genesisBytes, &genesisStruct); err != nil {
		f.logger.Error().Err(err).Msg("Error unmarshalling genesis")
		return nil, err
	}

	var stakingGenesisState stakingTypes.GenesisState
	err := f.parseCodec.UnmarshalJSON(genesisStruct.AppState.Staking, &stakingGenesisState)
	if err != nil {
		f.logger.Error().Err(err).Msg("Error unmarshalling staking genesis state")
		return nil, err
	}

	f.logger.Info().Int("validators", len(stakingGenesisState.Validators)).Msg("Genesis unmarshalled")

	// 1. Trying to fetch validators from staking module. Works for chain which did not start
	// from the first block but had their genesis as an export from older chain.
	if len(stakingGenesisState.Validators) > 0 {
		validators := make(types.CosmosValidators, len(stakingGenesisState.Validators))
		for index, validator := range stakingGenesisState.Validators {
			if chainValidator, err := f.parseValidator(validator); err != nil {
				return nil, err
			} else {
				validators[index] = chainValidator
			}
		}

		return validators, nil
	}

	// 2. If there's 0 validators in staking module, then we parse genutil module
	// and converting validators from their gentxs.
	var genutilGenesisState genutilTypes.GenesisState
	if err := f.parseCodec.UnmarshalJSON(genesisStruct.AppState.Genutil, &genutilGenesisState); err != nil {
		f.logger.Error().Err(err).Msg("Error unmarshalling genutil genesis state")
		return nil, err
	}

	validators := make(types.CosmosValidators, len(genutilGenesisState.GenTxs))
	for index, gentx := range genutilGenesisState.GenTxs {
		decodedTx, err := f.txDecoder(gentx)
		if err != nil {
			f.logger.Error().Err(err).Msg("Error decoding gentx")
			return nil, err
		}

		if len(decodedTx.GetMsgs()) != 1 {
			f.logger.Error().
				Int("length", len(decodedTx.GetMsgs())).
				Msg("Error decoding gentx: expected 1 message")
			return nil, err
		}

		msg := decodedTx.GetMsgs()[0]
		msgCreateValidator, ok := msg.(*stakingTypes.MsgCreateValidator)
		if !ok {
			f.logger.Error().Msg("gentx msg is not MsgCreateValidator")
			return nil, err
		}

		var pubkey cryptoTypes.PubKey
		if err := f.parseCodec.UnpackAny(msgCreateValidator.Pubkey, &pubkey); err != nil {
			f.logger.Error().Err(err).Msg("Error unpacking pubkey")
			return nil, err
		}

		addr := sdkTypes.ConsAddress(pubkey.Address())

		validators[index] = types.CosmosValidator{
			Moniker:          msgCreateValidator.Description.Moniker,
			OperatorAddress:  msgCreateValidator.ValidatorAddress, // Bech32 operator address
			ConsensusAddress: addr.String(),
		}
	}

	return validators, nil
}

func (f *CosmosRPCDataFetcher) getGenesisChunk(chunk int64) ([]byte, int64, error) {
	var response types.CometGenesisChunkResponse
	err := f.client().Get(fmt.Sprintf("/genesis_chunked?chunk=%d", chunk), &response)
	if err != nil {
		return nil, 0, err
	}

	if response.Result == nil {
		return nil, 0, fmt.Errorf("malformed response from node")
	}

	total, err := strconv.ParseInt(response.Result.Total, 10, 64)
	if err != nil {
		return nil, 0, err
	}

	return response.Result.Data, total, nil
}

func (f *CosmosRPCDataFetcher) parseValidator(validator stakingTypes.Validator) (types.CosmosValidator, error) {
	if err := validator.UnpackInterfaces(f.parseCodec); err != nil {
		return types.CosmosValidator{}, err
	}

	consAddr, err := validator.GetConsAddr()
	if err != nil {
		return types.CosmosValidator{}, err
	}

	consPubKey, err := validator.ConsPubKey()
	if err != nil {
		return types.CosmosValidator{}, fmt.Errorf("getting consensus pubkey: %w", err)
	}

	cometConsPubKey, err := validator.CmtConsPublicKey()
	if err != nil {
		return types.CosmosValidator{}, fmt.Errorf("getting comet consensus pubkey: %w", err)
	}

	return types.CosmosValidator{
		Moniker:              validator.GetMoniker(),
		OperatorAddress:      validator.GetOperator(),
		ConsensusAddress:     sdkTypes.ConsAddress(consAddr).String(),
		ConsensusPubkey:      consPubKey,
		CometConsensusPubkey: cometConsPubKey,
	}, nil
}

func (f *CosmosRPCDataFetcher) GetUpgradePlan() (*types.Upgrade, error) {
	var response upgradeTypes.QueryCurrentPlanResponse
	err := f.abciQuery(
		"/cosmos.upgrade.v1beta1.Query/CurrentPlan",
		&upgradeTypes.QueryCurrentPlanRequest{},
		&response,
		f.client(),
	)
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
