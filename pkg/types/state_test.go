package types

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// TestStateConcurrentAccess hits every setter and getter from many
// goroutines at once. With race detector enabled this is the
// regression test for the data races that motivated Phase 3.2.
func TestStateConcurrentAccess(t *testing.T) {
	s := NewState("http://localhost:26657", zerolog.Nop())

	const goroutines = 10
	const iterations = 200

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// writers
				s.SetConsensusHeight(int64(j), int64(j%5), int64(j%3), time.Now())
				s.SetBlockTime(time.Duration(j) * time.Millisecond)
				s.SetUpgrade(&Upgrade{Name: "test", Height: int64(j)})
				s.SetConsensusStateError(errors.New("test"))
				s.SetTMValidators(TMValidators{})

				// readers
				_, _, _, _ = s.GetConsensusHeight()
				_ = s.GetBlockTime()
				_ = s.GetUpgrade()
				_ = s.GetConsensusStateError()
				_ = s.GetTMValidators()
			}
		}(i)
	}

	wg.Wait()
}

// TestStateRoundDataMapConcurrentAccess exercises the sub-map's own
// internal mutex via State's interface. AddCometBFTEvents goes
// straight through to RoundDataMap so we can test it without setting
// up CometBFT events directly.
func TestStateRoundDataMapConcurrentAccess(t *testing.T) {
	s := NewState("http://localhost:26657", zerolog.Nop())

	const goroutines = 8
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				s.VotesByRound.AddProposer(int64(j), int32(id), "validator-x")

				// Read back, no assertions — just race detector.
				_ = s.VotesByRound.GetProposers(int64(j), int32(id))
			}
		}(i)
	}

	wg.Wait()
}

// TestRPCManagementCurrentURL covers the RPC setter/getter pair —
// the simplest end-to-end shape used by every refresh goroutine.
func TestRPCManagementCurrentURL(t *testing.T) {
	const seedURL = "http://seed:26657"
	s := NewState(seedURL, zerolog.Nop())

	rpc := s.CurrentRPC()
	require.Equal(t, seedURL, rpc.URL, "seed URL should be reachable as the current RPC even before AddKnownRPC")

	s.AddKnownRPC(RPC{URL: "http://other:26657", Moniker: "other"})
	s.SetCurrentRPCURL("http://other:26657")

	got := s.CurrentRPC()
	require.Equal(t, "http://other:26657", got.URL)
	require.Equal(t, "other", got.Moniker, "switching current RPC to a known URL should resolve its full record")
}
