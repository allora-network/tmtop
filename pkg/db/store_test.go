package db

import (
	"context"
	"testing"

	ctypes "github.com/cometbft/cometbft/types"
	"github.com/stretchr/testify/require"
)

// TestNopConsensusStoreReturnsNil locks down the contract: a
// disabled-persistence build must never bubble up errors from the
// store, no matter what callers throw at it.
func TestNopConsensusStoreReturnsNil(t *testing.T) {
	ctx := context.Background()
	var s ConsensusStorer = NopConsensusStore{}

	require.NoError(t, s.StoreRoundData(ctx, 1, 0, nil, nil))
	require.NoError(t, s.StoreValidators(ctx, 1, nil))
	require.NoError(t, s.StoreCometBFTEvents(ctx, []ctypes.TMEventData{}, nil))
	require.NoError(t, s.CleanupOldData(ctx))
	require.NoError(t, s.Close())
}
