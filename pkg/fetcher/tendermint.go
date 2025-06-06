package fetcher

import (
	"errors"
	"fmt"
	configPkg "main/pkg/config"
	"main/pkg/http"
	"strconv"
	"strings"
	"time"

	"main/pkg/types"

	"github.com/rs/zerolog"
)

type TendermintRPC struct {
	Config     *configPkg.Config
	State      *types.State
	Logger     zerolog.Logger
	Client     *http.Client
	LogChannel chan string
}

func NewTendermintRPC(config *configPkg.Config, state *types.State, logger zerolog.Logger) *TendermintRPC {
	return &TendermintRPC{
		Config: config,
		State:  state,
		Logger: logger.With().Str("component", "tendermint_rpc").Logger(),
		Client: http.NewClient(logger, "tendermint_rpc", config.RPCHost),
	}
}

func (rpc *TendermintRPC) client() *http.Client {
	return http.NewClient(rpc.Logger, "tendermint_rpc", rpc.State.CurrentRPC().URL)
}

func (rpc *TendermintRPC) GetConsensusState() (*types.ConsensusStateResponse, error) {
	var response types.ConsensusStateResponse
	if err := rpc.client().Get("/consensus_state", &response); err != nil {
		return nil, err
	}

	return &response, nil
}

func (rpc *TendermintRPC) GetValidators() ([]types.TendermintValidator, error) {
	page := 1

	validators := make([]types.TendermintValidator, 0)

	for {
		response, err := rpc.GetValidatorsAtPage(page)
		if err != nil {
			return nil, err
		}

		if response == nil {
			return nil, errors.New("malformed response from node: no response")
		}

		if response.Error != nil {
			// on genesis, /validators is not working
			if strings.Contains(response.Error.Data, "could not find validator set for height") {
				return rpc.GetValidatorsViaDumpConsensusState()
			}

			return nil, fmt.Errorf("malformed response from node: %s: %s", response.Error.Message, response.Error.Data)
		}

		if response.Result == nil || response.Result.Total == "" {
			return nil, errors.New("malformed response from node")
		}

		total, err := strconv.ParseInt(response.Result.Total, 10, 64)
		if err != nil {
			return nil, err
		}

		validators = append(validators, response.Result.Validators...)
		if int64(len(validators)) >= total {
			break
		}

		page++
	}

	return validators, nil
}

func (rpc *TendermintRPC) GetValidatorsViaDumpConsensusState() ([]types.TendermintValidator, error) {
	var response types.DumpConsensusStateResponse
	if err := rpc.client().Get("/dump_consensus_state", &response); err != nil {
		return nil, err
	}

	if response.Result == nil ||
		response.Result.RoundState == nil ||
		len(response.Result.RoundState.Validators.Validators) == 0 {
		return nil, fmt.Errorf("malformed response from /dump_consensus_state")
	}

	return response.Result.RoundState.Validators.Validators, nil
}

func (rpc *TendermintRPC) GetStatus(rpcURL string) (*types.TendermintStatusResponse, error) {
	client := http.NewClient(rpc.Logger, "tendermint_rpc", rpcURL)

	var response types.TendermintStatusResponse
	if err := client.Get("/status", &response); err != nil {
		return nil, err
	}

	return &response, nil
}

func (rpc *TendermintRPC) GetValidatorsAtPage(page int) (*types.ValidatorsResponse, error) {
	var response types.ValidatorsResponse
	if err := rpc.client().Get(fmt.Sprintf("/validators?page=%d&per_page=100", page), &response); err != nil {
		return nil, err
	}

	return &response, nil
}

func (rpc *TendermintRPC) Block(height int64) (types.TendermintBlockResponse, error) {
	blockURL := "/block"
	if height != 0 {
		blockURL = fmt.Sprintf("/block?height=%d", height)
	}

	res := types.TendermintBlockResponse{}
	err := rpc.client().Get(blockURL, &res)
	return res, err
}

func (rpc *TendermintRPC) GetBlockTime() (time.Duration, error) {
	latestBlock, err := rpc.Block(0)
	if err != nil {
		rpc.Logger.Error().Err(err).Msg("Could not fetch current block")
		return 0, err
	}

	if latestBlock.Result.Block == nil {
		return 0, fmt.Errorf("no current block present")
	}

	latestBlockHeight, err := strconv.ParseInt(latestBlock.Result.Block.Header.Height, 10, 64)
	if err != nil {
		rpc.Logger.Error().
			Err(err).
			Msg("Error converting latest block height to int64, which should never happen.")
		return 0, err
	}
	olderBlockHeight := latestBlockHeight - int64(rpc.Config.BlocksBehind)
	if olderBlockHeight <= 0 {
		olderBlockHeight = 1
	}

	blocksDiff := latestBlockHeight - olderBlockHeight
	if blocksDiff <= 0 {
		return 0, fmt.Errorf("cannot calculate block time with the negative blocks counter")
	}

	olderBlock, err := rpc.Block(olderBlockHeight)
	if err != nil {
		rpc.Logger.Error().Err(err).Msg("Could not fetch older block")
		return 0, err
	}

	if olderBlock.Result.Block == nil {
		return 0, fmt.Errorf("no older block present")
	}

	blocksDiffTime := latestBlock.Result.Block.Header.Time.Sub(olderBlock.Result.Block.Header.Time)
	blockTime := blocksDiffTime.Seconds() / float64(blocksDiff)

	duration := time.Duration(int64(blockTime * float64(time.Second)))
	return duration, nil
}
