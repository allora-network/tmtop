package db

import (
	"context"

	"main/pkg/types"

	ctypes "github.com/cometbft/cometbft/types"
)

// ConsensusStorer persists consensus progress and validator data.
//
// Implementations are expected to be safe for concurrent use. A nop
// implementation (NopConsensusStore) is provided for builds with
// persistence disabled, so callers never need to nil-check.
type ConsensusStorer interface {
	StoreRoundData(ctx context.Context, height int64, round int32, data *types.RoundData, validators types.TMValidators) error
	StoreValidators(ctx context.Context, height int64, validators types.TMValidators) error
	StoreCometBFTEvents(ctx context.Context, events []ctypes.TMEventData, validators types.TMValidators) error
	CleanupOldData(ctx context.Context) error
	Close() error
}

// NopConsensusStore is a no-op ConsensusStorer. Used when persistence
// is disabled so the app never needs to nil-check.
type NopConsensusStore struct{}

var (
	_ ConsensusStorer = (*ConsensusStore)(nil)
	_ ConsensusStorer = (*NopConsensusStore)(nil)
)

func (NopConsensusStore) StoreRoundData(context.Context, int64, int32, *types.RoundData, types.TMValidators) error {
	return nil
}

func (NopConsensusStore) StoreValidators(context.Context, int64, types.TMValidators) error {
	return nil
}

func (NopConsensusStore) StoreCometBFTEvents(context.Context, []ctypes.TMEventData, types.TMValidators) error {
	return nil
}

func (NopConsensusStore) CleanupOldData(context.Context) error { return nil }

func (NopConsensusStore) Close() error { return nil }
