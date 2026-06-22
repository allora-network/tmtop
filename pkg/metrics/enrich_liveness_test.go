package metrics

import (
	"testing"
	"time"

	"github.com/cometbft/cometbft/crypto/ed25519"
	cptypes "github.com/cometbft/cometbft/proto/tendermint/types"
	ctypes "github.com/cometbft/cometbft/types"
	"github.com/rs/zerolog"
	"main/pkg/types"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// makeValidator creates a TMValidator with a freshly-generated keypair.
// Returns the validator and its canonical display address.
func makeValidator(power int64) (types.TMValidator, string) {
	pk := ed25519.GenPrivKey().PubKey()
	cv := &ctypes.Validator{
		Address:     pk.Address(),
		PubKey:      pk,
		VotingPower: power,
	}
	v := types.TMValidator{CometValidator: cv}
	return v, v.GetDisplayAddress()
}

func newTestState(height, round int64) *types.State {
	s := types.NewState("http://localhost:26657", zerolog.Nop())
	s.SetConsensusHeight(height, round, 0, time.Time{})
	return s
}

func addVote(s *types.State, height int64, round int32, addr string, msgType cptypes.SignedMsgType, forBlock bool) {
	blockID := ctypes.BlockID{}
	if forBlock {
		blockID.Hash = []byte("block_hash_placeholder")
	}
	s.VotesByRound.AddVote(height, round, addr, msgType, blockID)
}

// ---------------------------------------------------------------------------
// TestHaltDetection — halt flag fires when height is stale long enough
// ---------------------------------------------------------------------------

func TestHaltDetection(t *testing.T) {
	nh := &NetworkHealth{}
	base := time.Unix(1700000000, 0)
	b := &Builder{now: func() time.Time { return base.Add(30 * time.Second) }}
	b.lastHeightChangeAt = base
	setHaltFields(nh, b)
	if !nh.ChainHalted || nh.SecondsSinceHeight < 29 {
		t.Fatalf("expected halt, got %+v", nh)
	}
}

// ---------------------------------------------------------------------------
// TestNoHaltWhenFresh — no halt below the threshold
// ---------------------------------------------------------------------------

func TestNoHaltWhenFresh(t *testing.T) {
	nh := &NetworkHealth{}
	base := time.Unix(1700000000, 0)
	b := &Builder{now: func() time.Time { return base.Add(5 * time.Second) }}
	b.lastHeightChangeAt = base
	setHaltFields(nh, b)
	if nh.ChainHalted {
		t.Fatalf("unexpected halt for 5s stall, got %+v", nh)
	}
}

// ---------------------------------------------------------------------------
// TestNoHaltWhenZeroTime — zero lastHeightChangeAt ⇒ no panic, no halt
// ---------------------------------------------------------------------------

func TestNoHaltWhenZeroTime(t *testing.T) {
	nh := &NetworkHealth{}
	b := &Builder{now: func() time.Time { return time.Now() }}
	// lastHeightChangeAt is zero value — should skip the halt check
	setHaltFields(nh, b)
	if nh.ChainHalted {
		t.Fatalf("expected no halt for zero lastHeightChangeAt, got %+v", nh)
	}
}

// ---------------------------------------------------------------------------
// TestVotedThisHeight — validators with any vote (prevote or precommit) are online
// ---------------------------------------------------------------------------

func TestVotedThisHeight(t *testing.T) {
	s := newTestState(10, 0)

	const height int64 = 10
	const round int32 = 0

	_, addrA := makeValidator(100)
	_, addrB := makeValidator(100)
	_, addrC := makeValidator(100)

	// addrA has prevote for a block
	addVote(s, height, round, addrA, cptypes.PrevoteType, true)
	// addrB has precommit nil
	addVote(s, height, round, addrB, cptypes.PrecommitType, false)
	// addrC has no votes

	if !votedThisHeight(s, height, round, addrA) {
		t.Error("addrA should be online (has prevote for block)")
	}
	if !votedThisHeight(s, height, round, addrB) {
		t.Error("addrB should be online (has precommit nil)")
	}
	if votedThisHeight(s, height, round, addrC) {
		t.Error("addrC should be offline (no votes at all)")
	}
}

// ---------------------------------------------------------------------------
// TestEnrichLivenessRows — Online flag set correctly per row
// ---------------------------------------------------------------------------

func TestEnrichLivenessRows(t *testing.T) {
	s := newTestState(5, 1)

	const height int64 = 5
	const round int32 = 1

	_, addrC := makeValidator(100)
	_, addrD := makeValidator(100)

	// addrC has precommit nil — still online (any vote counts)
	addVote(s, height, round, addrC, cptypes.PrecommitType, false)

	rows := []ValidatorHealthRow{
		{Address: addrC},
		{Address: addrD},
	}

	b := &Builder{now: func() time.Time { return time.Now() }}
	b.lastHeightChangeAt = time.Now().Add(-2 * time.Second)

	enrichLivenessRows(rows, b, s)

	if !rows[0].Online {
		t.Errorf("addrC should be Online (has precommit nil vote)")
	}
	if rows[1].Online {
		t.Errorf("addrD should be offline (no votes)")
	}
}

// ---------------------------------------------------------------------------
// TestEnrichLivenessOfflineCounts — OfflineCount and OfflinePowerPct
// ---------------------------------------------------------------------------

func TestEnrichLivenessOfflineCounts(t *testing.T) {
	s := newTestState(7, 0)

	const height int64 = 7
	const round int32 = 0

	v1, addr1 := makeValidator(100)
	v2, _ := makeValidator(200) // v2 has no votes → offline

	// v1 voted prevote
	addVote(s, height, round, addr1, cptypes.PrevoteType, true)

	s.SetTMValidators(types.TMValidators{v1, v2})

	b := &Builder{now: func() time.Time { return time.Now() }}
	b.lastHeightChangeAt = time.Now().Add(-5 * time.Second)

	nh := &NetworkHealth{}
	enrichLiveness(nh, b, s)

	if nh.OfflineCount != 1 {
		t.Errorf("expected OfflineCount=1, got %d", nh.OfflineCount)
	}
	// v2 has 200 power out of 300 total → 66.6...%
	expectedPct := 100.0 * 200.0 / 300.0
	if nh.OfflinePowerPct < expectedPct-0.01 || nh.OfflinePowerPct > expectedPct+0.01 {
		t.Errorf("expected OfflinePowerPct≈%.2f, got %.2f", expectedPct, nh.OfflinePowerPct)
	}
}

// ---------------------------------------------------------------------------
// TestCurrentMaxRound — enrichLiveness sets CurrentMaxRound from state
// ---------------------------------------------------------------------------

func TestCurrentMaxRound(t *testing.T) {
	s := newTestState(3, 2) // height=3, round=2

	b := &Builder{now: func() time.Time { return time.Now() }}
	b.lastHeightChangeAt = time.Now().Add(-5 * time.Second)

	nh := &NetworkHealth{}
	enrichLiveness(nh, b, s)

	if nh.CurrentMaxRound != 2 {
		t.Errorf("expected CurrentMaxRound=2, got %d", nh.CurrentMaxRound)
	}
}
