package metrics

import (
	"testing"
	"time"

	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	ctypes "github.com/cometbft/cometbft/types"
)

var fixedNow = func() time.Time { return time.Unix(1700000000, 0) }

// makeAddr returns a valid ctypes.Address (HexBytes) from a short seed string.
func makeAddr(t *testing.T) ctypes.Address {
	t.Helper()
	// Address is an alias for cmtbytes.HexBytes which is []byte
	return ctypes.Address("validator-address-seed-1")
}

// voteEvent constructs a ctypes.EventDataVote carrying a Vote with the given
// fields. hashHex is a short distinguishing string used as the BlockID.Hash.
func voteEvent(addr ctypes.Address, height int64, round int32, voteType cmtproto.SignedMsgType, hashHex string) ctypes.TMEventData {
	return ctypes.EventDataVote{
		Vote: &ctypes.Vote{
			Type:             voteType,
			Height:           height,
			Round:            round,
			ValidatorAddress: addr,
			BlockID: ctypes.BlockID{
				Hash: []byte(hashHex),
			},
		},
	}
}

// nilBlockVoteEvent constructs a vote with a nil/empty BlockID.Hash (nil vote).
func nilBlockVoteEvent(addr ctypes.Address, height int64, round int32, voteType cmtproto.SignedMsgType) ctypes.TMEventData {
	return ctypes.EventDataVote{
		Vote: &ctypes.Vote{
			Type:             voteType,
			Height:           height,
			Round:            round,
			ValidatorAddress: addr,
			BlockID:          ctypes.BlockID{Hash: nil},
		},
	}
}

func TestEquivocationDetected(t *testing.T) {
	b := &Builder{equiv: newEquivState(), now: fixedNow}
	addr := makeAddr(t)
	b.observeForEquivocation(voteEvent(addr, 100, 0, cmtproto.PrecommitType, "AAAA"))
	b.observeForEquivocation(voteEvent(addr, 100, 0, cmtproto.PrecommitType, "BBBB"))
	nh := &NetworkHealth{}
	enrichEquivocations(nh, b)
	if len(nh.Equivocations) != 1 {
		t.Fatalf("want 1 equivocation, got %d", len(nh.Equivocations))
	}
}

func TestEquivocationNilBlockNotEvidence(t *testing.T) {
	b := &Builder{equiv: newEquivState(), now: fixedNow}
	addr := makeAddr(t)
	// Nil-block votes must not be treated as equivocation evidence
	b.observeForEquivocation(nilBlockVoteEvent(addr, 100, 0, cmtproto.PrevoteType))
	b.observeForEquivocation(voteEvent(addr, 100, 0, cmtproto.PrevoteType, "AAAA"))
	nh := &NetworkHealth{}
	enrichEquivocations(nh, b)
	if len(nh.Equivocations) != 0 {
		t.Fatalf("want 0 equivocations (nil vote not evidence), got %d", len(nh.Equivocations))
	}
}

func TestEquivocationFirstVoteNoFlag(t *testing.T) {
	b := &Builder{equiv: newEquivState(), now: fixedNow}
	addr := makeAddr(t)
	// Only one vote — no conflict
	b.observeForEquivocation(voteEvent(addr, 100, 0, cmtproto.PrevoteType, "CCCC"))
	nh := &NetworkHealth{}
	enrichEquivocations(nh, b)
	if len(nh.Equivocations) != 0 {
		t.Fatalf("want 0 equivocations for single vote, got %d", len(nh.Equivocations))
	}
}

func TestEquivocationSameBlockNoFlag(t *testing.T) {
	b := &Builder{equiv: newEquivState(), now: fixedNow}
	addr := makeAddr(t)
	// Same block twice — not equivocation
	b.observeForEquivocation(voteEvent(addr, 100, 0, cmtproto.PrecommitType, "DDDD"))
	b.observeForEquivocation(voteEvent(addr, 100, 0, cmtproto.PrecommitType, "DDDD"))
	nh := &NetworkHealth{}
	enrichEquivocations(nh, b)
	if len(nh.Equivocations) != 0 {
		t.Fatalf("want 0 equivocations for repeated identical vote, got %d", len(nh.Equivocations))
	}
}

func TestEquivocationDifferentSlotNoConflict(t *testing.T) {
	b := &Builder{equiv: newEquivState(), now: fixedNow}
	addr := makeAddr(t)
	// Different heights — each is its own slot, no conflict
	b.observeForEquivocation(voteEvent(addr, 100, 0, cmtproto.PrecommitType, "AAAA"))
	b.observeForEquivocation(voteEvent(addr, 101, 0, cmtproto.PrecommitType, "BBBB"))
	nh := &NetworkHealth{}
	enrichEquivocations(nh, b)
	if len(nh.Equivocations) != 0 {
		t.Fatalf("want 0 equivocations for votes at different heights, got %d", len(nh.Equivocations))
	}
}

func TestEquivocationFlagsRows(t *testing.T) {
	b := &Builder{equiv: newEquivState(), now: fixedNow}
	addr := makeAddr(t)
	b.observeForEquivocation(voteEvent(addr, 100, 0, cmtproto.PrecommitType, "AAAA"))
	b.observeForEquivocation(voteEvent(addr, 100, 0, cmtproto.PrecommitType, "BBBB"))

	rows := []ValidatorHealthRow{
		{Address: addr.String()},
		{Address: "other-validator"},
	}
	enrichEquivocationFlags(rows, b)

	if !rows[0].Equivocated {
		t.Errorf("want rows[0].Equivocated=true for equivocating validator")
	}
	if rows[1].Equivocated {
		t.Errorf("want rows[1].Equivocated=false for innocent validator")
	}
}

func TestEquivocationEventFields(t *testing.T) {
	b := &Builder{equiv: newEquivState(), now: fixedNow}
	addr := makeAddr(t)
	b.observeForEquivocation(voteEvent(addr, 100, 2, cmtproto.PrecommitType, "AAAA"))
	b.observeForEquivocation(voteEvent(addr, 100, 2, cmtproto.PrecommitType, "BBBB"))
	nh := &NetworkHealth{}
	enrichEquivocations(nh, b)
	if len(nh.Equivocations) != 1 {
		t.Fatalf("want 1 equivocation, got %d", len(nh.Equivocations))
	}
	ev := nh.Equivocations[0]
	if ev.Height != 100 {
		t.Errorf("want Height=100, got %d", ev.Height)
	}
	if ev.Round != 2 {
		t.Errorf("want Round=2, got %d", ev.Round)
	}
	if ev.VoteType != "precommit" {
		t.Errorf("want VoteType=precommit, got %q", ev.VoteType)
	}
	if ev.ValidatorAddress != addr.String() {
		t.Errorf("want ValidatorAddress=%q, got %q", addr.String(), ev.ValidatorAddress)
	}
	if ev.DetectedAt.IsZero() {
		t.Errorf("DetectedAt must not be zero")
	}
	// BlockIDA and BlockIDB are the core payload: the two conflicting hashes.
	// []byte("AAAA") hex-encodes to "41414141"; []byte("BBBB") to "42424242".
	const wantA = "41414141"
	const wantB = "42424242"
	if ev.BlockIDA != wantA {
		t.Errorf("want BlockIDA=%q, got %q", wantA, ev.BlockIDA)
	}
	if ev.BlockIDB != wantB {
		t.Errorf("want BlockIDB=%q, got %q", wantB, ev.BlockIDB)
	}
	if ev.BlockIDA == ev.BlockIDB {
		t.Errorf("BlockIDA and BlockIDB must differ; both are %q", ev.BlockIDA)
	}
}

// TestEquivocationEviction verifies that height buckets outside the window
// are evicted from seen, so memory does not grow unboundedly.
func TestEquivocationEviction(t *testing.T) {
	// windowBlocks = 10; after observing at height 50, heights < 40 must be gone.
	b := &Builder{equiv: newEquivState(), now: fixedNow, windowBlocks: 10}
	addr := makeAddr(t)

	// Seed votes at heights 1 and 2 — both will be outside window once we reach 50.
	b.observeForEquivocation(voteEvent(addr, 1, 0, cmtproto.PrevoteType, "AAAA"))
	b.observeForEquivocation(voteEvent(addr, 2, 0, cmtproto.PrevoteType, "AAAA"))

	// Observe a vote at height 50 — triggers eviction of heights < 40.
	b.observeForEquivocation(voteEvent(addr, 50, 0, cmtproto.PrevoteType, "AAAA"))

	// Heights 1 and 2 must no longer be in seen.
	if _, ok := b.equiv.seen[1]; ok {
		t.Errorf("height 1 should have been evicted but is still in seen")
	}
	if _, ok := b.equiv.seen[2]; ok {
		t.Errorf("height 2 should have been evicted but is still in seen")
	}
	// Height 50 must still be present (it is within the window).
	if _, ok := b.equiv.seen[50]; !ok {
		t.Errorf("height 50 should still be in seen but was evicted")
	}
}

// TestEquivocationDefaultWindowEviction verifies that when windowBlocks == 0,
// the defaultEvictionWindow is used instead of growing without bound.
func TestEquivocationDefaultWindowEviction(t *testing.T) {
	// windowBlocks = 0 → falls back to defaultEvictionWindow (500).
	b := &Builder{equiv: newEquivState(), now: fixedNow, windowBlocks: 0}
	addr := makeAddr(t)

	// Seed a vote at height 1.
	b.observeForEquivocation(voteEvent(addr, 1, 0, cmtproto.PrevoteType, "AAAA"))

	// Observe a vote at height = defaultEvictionWindow + 10.
	// cutoff = (500+10) - 500 = 10 → height 1 should be evicted.
	triggerHeight := defaultEvictionWindow + 10
	b.observeForEquivocation(voteEvent(addr, triggerHeight, 0, cmtproto.PrevoteType, "AAAA"))

	if _, ok := b.equiv.seen[1]; ok {
		t.Errorf("height 1 should have been evicted by default window but is still in seen")
	}
}

// TestEquivocationEventsCapped verifies that the events slice never exceeds
// maxEquivEvents, dropping oldest entries.
func TestEquivocationEventsCapped(t *testing.T) {
	b := &Builder{equiv: newEquivState(), now: fixedNow, windowBlocks: 0}
	addr := makeAddr(t)

	// Generate maxEquivEvents+10 distinct equivocations, each at a unique height
	// so they don't share a slot (preventing de-duplication logic from interfering).
	total := maxEquivEvents + 10
	for i := 0; i < total; i++ {
		h := int64(i + 1)
		b.observeForEquivocation(voteEvent(addr, h, 0, cmtproto.PrecommitType, "AAAA"))
		b.observeForEquivocation(voteEvent(addr, h, 0, cmtproto.PrecommitType, "BBBB"))
	}

	if len(b.equiv.events) > maxEquivEvents {
		t.Errorf("events slice grew to %d, want at most %d", len(b.equiv.events), maxEquivEvents)
	}
}
