package fetcher

import (
	"context"
	"time"

	"main/pkg/types"

	butils "github.com/brynbellomy/go-utils"
	cnstypes "github.com/cometbft/cometbft/consensus/types"
	rpctypes "github.com/cometbft/cometbft/rpc/core/types"
	ctypes "github.com/cometbft/cometbft/types"
)

// Fetcher is the boundary between the app and all chain-data sources
// (CometBFT RPC + Cosmos SDK queries + websocket subscriptions).
//
// Implementations are expected to be safe for concurrent use. Close
// must be safe to call multiple times.
type Fetcher interface {
	GetConsensusState() (*cnstypes.RoundState, error)
	GetCometNodeStatus(rpcURL string) (*rpctypes.ResultStatus, error)
	Block(height int64) (*rpctypes.ResultBlock, error)
	GetBlockTime() (time.Duration, error)
	GetNetInfo(rpcURL string) (*types.NetInfo, error)
	GetUpgradePlan() (*types.Upgrade, error)
	GetValidators() ([]types.TMValidator, error)
	Subscribe(mb *butils.Mailbox[ctypes.TMEventData], events ...string)
	Close(ctx context.Context) error
}

var _ Fetcher = (*DataFetcher)(nil)
