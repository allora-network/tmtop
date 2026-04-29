package crawler

import (
	"errors"
	"sync"
	"testing"
	"time"

	"main/pkg/fetcher/mocks"
	"main/pkg/types"

	bytes0 "github.com/cometbft/cometbft/libs/bytes"
	rpctypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// fakeStatus returns a minimally-populated *rpctypes.ResultStatus —
// just enough for the crawler to extract NodeInfo.ID(), Moniker, and
// ValidatorInfo.Address.
func fakeStatus(id, moniker string) *rpctypes.ResultStatus {
	s := &rpctypes.ResultStatus{}
	s.NodeInfo.DefaultNodeID = "abcdef0123456789abcdef0123456789abcdef01"
	s.NodeInfo.Moniker = moniker
	s.ValidatorInfo.Address = bytes0.HexBytes(id)
	return s
}

// TestCrawlerSeedURL verifies the seed URL is fetched and recorded
// in State on first iteration.
func TestCrawlerSeedURL(t *testing.T) {
	const seed = "http://seed:26657"

	mockFetcher := mocks.NewMockFetcher(t)
	mockFetcher.EXPECT().GetNetInfo(seed).Return(&types.NetInfo{Peers: nil}, nil).Once()
	mockFetcher.EXPECT().GetCometNodeStatus(seed).Return(fakeStatus("addr", "seed-node"), nil).Once()

	state := types.NewState(seed, zerolog.Nop())
	chStop := make(chan struct{})

	c := New(seed, mockFetcher, state, zerolog.Nop(), chStop)

	done := make(chan struct{})
	go func() {
		c.Run()
		close(done)
	}()

	require.Eventually(t, func() bool {
		_, ok := state.KnownRPCByURL(seed)
		return ok
	}, time.Second, 10*time.Millisecond, "seed URL should land in KnownRPCs")

	got, _ := state.KnownRPCByURL(seed)
	require.Equal(t, "seed-node", got.Moniker)

	close(chStop)
	<-done
}

// TestCrawlerExitsOnStop verifies the crawler returns promptly when
// chStop closes, even if the fetcher is slow.
func TestCrawlerExitsOnStop(t *testing.T) {
	const seed = "http://seed:26657"

	mockFetcher := mocks.NewMockFetcher(t)
	// Allow zero-or-more calls — we close chStop right away.
	mockFetcher.EXPECT().GetNetInfo(mock.Anything).Return(&types.NetInfo{}, nil).Maybe()
	mockFetcher.EXPECT().GetCometNodeStatus(mock.Anything).Return(fakeStatus("a", "n"), nil).Maybe()

	state := types.NewState(seed, zerolog.Nop())
	chStop := make(chan struct{})

	c := New(seed, mockFetcher, state, zerolog.Nop(), chStop)

	done := make(chan struct{})
	go func() {
		c.Run()
		close(done)
	}()

	close(chStop)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("crawler did not exit within 1s of chStop closing")
	}
}

// TestCrawlerFetchError verifies fetch errors don't kill the crawler
// — it logs and moves on.
func TestCrawlerFetchError(t *testing.T) {
	const seed = "http://seed:26657"

	// Signal from inside the mock so we can synchronize on the fetch
	// attempt rather than guessing with time.Sleep.
	fetched := make(chan struct{})
	mockFetcher := mocks.NewMockFetcher(t)
	mockFetcher.EXPECT().GetNetInfo(seed).
		Run(func(string) { close(fetched) }).
		Return(nil, errors.New("connection refused")).
		Once()
	// GetCometNodeStatus is NOT expected — fetchOne returns early after
	// the GetNetInfo failure.

	state := types.NewState(seed, zerolog.Nop())
	chStop := make(chan struct{})

	c := New(seed, mockFetcher, state, zerolog.Nop(), chStop)

	done := make(chan struct{})
	go func() {
		c.Run()
		close(done)
	}()

	select {
	case <-fetched:
	case <-time.After(time.Second):
		t.Fatal("crawler never attempted the seed fetch")
	}

	close(chStop)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("crawler did not exit cleanly after fetch error")
	}

	// Seed URL should NOT be in known RPCs since the fetch failed
	// before AddKnownRPC.
	_, ok := state.KnownRPCByURL(seed)
	require.False(t, ok, "fetch error should not result in a recorded RPC")
}

// TestCrawlerRescanDeduplication verifies the lastSeen map prevents a
// URL from being re-fetched within rescanInterval.
//
// We seed the crawler, wait for the seed fetch to complete, then send
// (a) the same URL — which should be deduped — and (b) a probe URL
// right after. Once the probe completes we know the deduped URL has
// already been pulled out of the mailbox and skipped. mockery's strict
// .Once() on the seed asserts the dedup actually happened.
func TestCrawlerRescanDeduplication(t *testing.T) {
	const seed = "http://seed:26657"
	const probe = "http://probe:26657"

	mockFetcher := mocks.NewMockFetcher(t)
	// Exactly one call expected for the seed — mockery fails the test
	// if it's called twice. Probe is the synchronization handle for
	// "the post-dup batch has finished processing."
	probeFetched := make(chan struct{})
	mockFetcher.EXPECT().GetNetInfo(seed).Return(&types.NetInfo{}, nil).Once()
	mockFetcher.EXPECT().GetCometNodeStatus(seed).Return(fakeStatus("a", "n"), nil).Once()
	mockFetcher.EXPECT().GetNetInfo(probe).Return(&types.NetInfo{}, nil).Once()
	mockFetcher.EXPECT().GetCometNodeStatus(probe).
		Run(func(string) { close(probeFetched) }).
		Return(fakeStatus("b", "p"), nil).
		Once()

	state := types.NewState(seed, zerolog.Nop())
	chStop := make(chan struct{})

	c := New(seed, mockFetcher, state, zerolog.Nop(), chStop)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.Run()
	}()

	require.Eventually(t, func() bool {
		_, ok := state.KnownRPCByURL(seed)
		return ok
	}, time.Second, 10*time.Millisecond)

	// Re-deliver the seed URL — should be deduped — and immediately
	// queue the probe behind it. The mailbox is FIFO; once the probe
	// fetch fires the duplicate has already been seen and skipped.
	c.mb.Deliver(seed)
	c.mb.Deliver(probe)

	select {
	case <-probeFetched:
	case <-time.After(time.Second):
		t.Fatal("probe URL never fetched — crawler may be wedged")
	}

	close(chStop)
	wg.Wait()
}
