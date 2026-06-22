// pkg/metrics/equivocation.go               (WS-G)
package metrics

import (
	"fmt"

	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	ctypes "github.com/cometbft/cometbft/types"
)

// equivState accumulates per-slot first-seen BlockID hashes and any detected
// equivocation events across ObserveEvents calls.
//
// Memory is bounded: height buckets older than (currentHeight - windowBlocks)
// are evicted after each observation, and events is capped at maxEquivEvents.
type equivState struct {
	// seen maps height → slotKey → hex of first non-nil BlockID seen for that slot.
	// slotKey = validatorAddress + "|" + round + "|" + voteType
	seen   map[int64]map[string]string
	events []EquivocationEvent
}

const maxEquivEvents = 256

// defaultEvictionWindow is the fallback cap when Builder.windowBlocks == 0.
// Prevents unbounded growth even when no window is configured.
const defaultEvictionWindow = int64(500)

func newEquivState() equivState {
	return equivState{seen: map[int64]map[string]string{}}
}

// slotKey produces the inner map key for a vote slot within a single height.
// Format: validatorAddress + "|" + round + "|" + voteType-int
func slotKey(addr string, r int32, vt cmtproto.SignedMsgType) string {
	return fmt.Sprintf("%s|%d|%d", addr, r, vt)
}

// observeForEquivocation inspects a TMEventData value.
//
// Scope: conflicts are detected within the same (height, round, voteType).
// PartSetHeader differences are ignored. Only non-nil BlockID.Hash values
// constitute evidence — nil/empty votes indicate a validator chose not to
// vote for a specific block and are not equivocation evidence per the
// CometBFT spec.
//
// When the same (validator, height, round, voteType) slot is seen with two
// distinct non-nil block IDs, exactly one EquivocationEvent is emitted.
func (b *Builder) observeForEquivocation(e ctypes.TMEventData) {
	vd, ok := e.(ctypes.EventDataVote)
	if !ok || vd.Vote == nil {
		return
	}
	v := vd.Vote
	if len(v.BlockID.Hash) == 0 {
		// Nil/empty BlockID — not equivocation evidence per Tendermint spec.
		return
	}
	blockIDHex := v.BlockID.Hash.String()
	height := v.Height
	sk := slotKey(v.ValidatorAddress.String(), v.Round, v.Type)

	// Ensure height bucket exists.
	if b.equiv.seen[height] == nil {
		b.equiv.seen[height] = map[string]string{}
	}
	prev, exists := b.equiv.seen[height][sk]
	if !exists {
		b.equiv.seen[height][sk] = blockIDHex
	} else if prev != blockIDHex {
		ev := EquivocationEvent{
			ValidatorAddress: v.ValidatorAddress.String(),
			Height:           height,
			Round:            v.Round,
			VoteType:         voteTypeName(v.Type),
			BlockIDA:         prev,
			BlockIDB:         blockIDHex,
			DetectedAt:       b.now(),
		}
		b.equiv.events = append(b.equiv.events, ev)
		// Cap events to prevent unbounded growth from a pathological stream.
		if len(b.equiv.events) > maxEquivEvents {
			b.equiv.events = b.equiv.events[len(b.equiv.events)-maxEquivEvents:]
		}
	}

	// Evict height buckets outside the retention window.
	window := b.windowBlocks
	if window <= 0 {
		window = defaultEvictionWindow
	}
	cutoff := height - window
	for h := range b.equiv.seen {
		if h < cutoff {
			delete(b.equiv.seen, h)
		}
	}
}

// voteTypeName converts a SignedMsgType to the canonical string used in
// EquivocationEvent.VoteType.
func voteTypeName(t cmtproto.SignedMsgType) string {
	if t == cmtproto.PrecommitType {
		return "precommit"
	}
	return "prevote"
}

// enrichEquivocations copies the accumulated equivocation events into nh.
// Monikers are left empty here; they can be resolved by the renderer from
// state.GetTMValidators() if desired (see brief Step 5).
func enrichEquivocations(nh *NetworkHealth, b *Builder) {
	nh.Equivocations = append([]EquivocationEvent(nil), b.equiv.events...)
}

// enrichEquivocationFlags sets Equivocated=true on any ValidatorHealthRow
// whose Address matches an equivocating validator.
func enrichEquivocationFlags(rows []ValidatorHealthRow, b *Builder) {
	if len(b.equiv.events) == 0 {
		return
	}
	flagged := make(map[string]bool, len(b.equiv.events))
	for _, e := range b.equiv.events {
		flagged[e.ValidatorAddress] = true
	}
	for i := range rows {
		if flagged[rows[i].Address] {
			rows[i].Equivocated = true
		}
	}
}
