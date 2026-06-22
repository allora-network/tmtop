// pkg/metrics/equivocation.go               (WS-G)
package metrics

import (
	"fmt"

	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	ctypes "github.com/cometbft/cometbft/types"
)

// equivState accumulates per-slot first-seen BlockID hashes and any detected
// equivocation events across ObserveEvents calls.
type equivState struct {
	// seen maps equivKey → hex representation of the first non-nil BlockID seen
	// for that (validatorAddress, height, round, voteType) slot.
	seen   map[string]string
	events []EquivocationEvent
}

func newEquivState() equivState {
	return equivState{seen: map[string]string{}}
}

// equivKey produces the map key for a vote slot.
func equivKey(addr string, h int64, r int32, vt cmtproto.SignedMsgType) string {
	return fmt.Sprintf("%s|%d|%d|%d", addr, h, r, vt)
}

// observeForEquivocation inspects a TMEventData value. When the event is an
// EventDataVote carrying a non-nil BlockID and the same slot has already been
// seen with a *different* BlockID, an EquivocationEvent is appended.
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
	key := equivKey(v.ValidatorAddress.String(), v.Height, v.Round, v.Type)

	prev, exists := b.equiv.seen[key]
	if !exists {
		b.equiv.seen[key] = blockIDHex
		return
	}
	if prev != blockIDHex {
		b.equiv.events = append(b.equiv.events, EquivocationEvent{
			ValidatorAddress: v.ValidatorAddress.String(),
			Height:           v.Height,
			Round:            v.Round,
			VoteType:         voteTypeName(v.Type),
			BlockIDA:         prev,
			BlockIDB:         blockIDHex,
			DetectedAt:       b.now(),
		})
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
