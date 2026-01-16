package fetcher

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"main/pkg/config"
	"main/pkg/http"
	"main/pkg/types"

	cnstypes "github.com/cometbft/cometbft/consensus/types"
	cmtjson "github.com/cometbft/cometbft/libs/json"
	rpctypes "github.com/cometbft/cometbft/rpc/core/types"
	jsonrpctypes "github.com/cometbft/cometbft/rpc/jsonrpc/types"
	ctypes "github.com/cometbft/cometbft/types"
	"github.com/rs/zerolog"
)

type CometRPC struct {
	url          string
	logger       zerolog.Logger
	blocksBehind uint64
}

func NewCometRPC(config *config.Config, state *types.State, logger zerolog.Logger) *CometRPC {
	return &CometRPC{
		url:          state.CurrentRPC().URL,
		logger:       logger.With().Str("component", "comet_rpc").Logger(),
		blocksBehind: config.BlocksBehind,
	}
}

func (rpc *CometRPC) WithEndpoint(url string) *CometRPC {
	return &CometRPC{
		url:          url,
		logger:       rpc.logger,
		blocksBehind: rpc.blocksBehind,
	}
}

func (rpc *CometRPC) client() *http.Client {
	return http.NewClient(rpc.logger, "comet_rpc", rpc.url)
}

func (rpc *CometRPC) request(path string, target any) error {
	bs, err := rpc.client().GetPlain(path)
	if err != nil {
		return err
	}

	var response jsonrpctypes.RPCResponse
	err = cmtjson.Unmarshal(bs, &response)
	if err != nil {
		return err
	} else if response.Error != nil {
		return response.Error
	}

	err = cmtjson.Unmarshal(response.Result, &target)
	return err
}

func (rpc *CometRPC) GetConsensusState() (*cnstypes.RoundState, error) {
	var response rpctypes.ResultConsensusState
	if err := rpc.request("/consensus_state", &response); err != nil {
		return nil, err
	}

	var state cnstypes.RoundState
	if err := cmtjson.Unmarshal(response.RoundState, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal round state: %w", err)
	}

	return &state, nil
}

func (rpc *CometRPC) GetValidators() ([]*ctypes.Validator, error) {
	page := 1

	validators := make([]*ctypes.Validator, 0)

	for {
		response, err := rpc.getValidatorsAtPage(page)
		if err != nil && strings.Contains(err.Error(), "could not find validator set for height") {
			// on genesis, /validators is not working
			return rpc.getValidatorsViaDumpConsensusState()
		} else if err != nil {
			return nil, err
		} else if response == nil {
			return nil, errors.New("malformed response from node: no response")
		}

		validators = append(validators, response.Validators...)
		if len(validators) >= response.Total {
			break
		}
		page++
	}

	return validators, nil
}

func (rpc *CometRPC) getValidatorsAtPage(page int) (*rpctypes.ResultValidators, error) {
	var resp rpctypes.ResultValidators
	err := rpc.request(fmt.Sprintf("/validators?page=%d&per_page=100", page), &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (rpc *CometRPC) getValidatorsViaDumpConsensusState() ([]*ctypes.Validator, error) {
	var response rpctypes.ResultDumpConsensusState
	err := rpc.request("/dump_consensus_state", &response)
	if err != nil {
		return nil, err
	}

	if response.RoundState == nil {
		return nil, fmt.Errorf("malformed response from /dump_consensus_state")
	}

	var state cnstypes.RoundState
	if err := cmtjson.Unmarshal(response.RoundState, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal round state: %w", err)
	}

	return state.Validators.Validators, nil
}

func (rpc *CometRPC) GetCometNodeStatus() (*rpctypes.ResultStatus, error) {
	var resp rpctypes.ResultStatus
	err := rpc.request("/status", &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (rpc *CometRPC) Block(height int64) (*rpctypes.ResultBlock, error) {
	blockURL := "/block"
	if height != 0 {
		blockURL = fmt.Sprintf("/block?height=%d", height)
	}

	var res rpctypes.ResultBlock
	err := rpc.request(blockURL, &res)
	return &res, err
}

func (rpc *CometRPC) GetBlockTime() (time.Duration, error) {
	latestBlock, err := rpc.Block(0)
	if err != nil {
		rpc.logger.Error().Err(err).Msg("Could not fetch current block")
		return 0, err
	}

	if latestBlock.Block == nil {
		return 0, fmt.Errorf("no current block present")
	}

	latestBlockHeight := latestBlock.Block.Header.Height
	olderBlockHeight := latestBlockHeight - int64(rpc.blocksBehind)
	if olderBlockHeight <= 0 {
		olderBlockHeight = 1
	}

	blocksDiff := latestBlockHeight - olderBlockHeight
	if blocksDiff <= 0 {
		return 0, fmt.Errorf("cannot calculate block time with the negative blocks counter")
	}

	olderBlock, err := rpc.Block(olderBlockHeight)
	if err != nil {
		rpc.logger.Error().Err(err).Msg("Could not fetch older block")
		return 0, err
	}

	if olderBlock.Block == nil {
		return 0, fmt.Errorf("no older block present")
	}

	blocksDiffTime := latestBlock.Block.Header.Time.Sub(olderBlock.Block.Header.Time)
	blockTime := blocksDiffTime.Seconds() / float64(blocksDiff)

	duration := time.Duration(int64(blockTime * float64(time.Second)))
	return duration, nil
}

func (rpc *CometRPC) GetNetInfo() (*types.NetInfo, error) {
	var result types.NetInfo
	err := rpc.request("/net_info", &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}
