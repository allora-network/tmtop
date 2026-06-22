// pkg/metrics/enrich_liveness.go            (WS-B)
package metrics

import (
	cptypes "github.com/cometbft/cometbft/proto/tendermint/types"
	"main/pkg/types"
)

// haltThreshold: if no new height for this long, flag the chain as stalled.
// Tunable; deliberately a few× a normal block time.
const haltThresholdSecs = 15.0

func enrichLiveness(nh *NetworkHealth, b *Builder, s *types.State) {
	setHaltFields(nh, b)
	height, round, _, _ := s.GetConsensusHeight()
	nh.CurrentMaxRound = int32(round)

	validators := s.GetTMValidators()
	var offline int
	var offlinePower, totalPower float64
	for _, v := range validators {
		if v.CometValidator == nil {
			continue
		}
		power := float64(v.CometValidator.VotingPower)
		totalPower += power
		if !votedThisHeight(s, height, int32(round), v.GetDisplayAddress()) {
			offline++
			offlinePower += power
		}
	}
	nh.OfflineCount = offline
	if totalPower > 0 {
		nh.OfflinePowerPct = 100.0 * offlinePower / totalPower
	}
}

// setHaltFields updates ChainHalted and SecondsSinceHeight based on how long
// the chain has been stuck at the same height. A zero lastHeightChangeAt means
// we have not yet observed any height transition, so no determination is made.
func setHaltFields(nh *NetworkHealth, b *Builder) {
	if b.lastHeightChangeAt.IsZero() {
		return
	}
	secs := b.now().Sub(b.lastHeightChangeAt).Seconds()
	nh.SecondsSinceHeight = secs
	nh.ChainHalted = secs > haltThresholdSecs
}

// votedThisHeight returns true if the validator cast any vote (prevote or
// precommit, for a block or nil) at the given height and round.
// A validator is considered online if we received at least one signed message
// from them — receiving a nil vote still demonstrates liveness.
func votedThisHeight(s *types.State, height int64, round int32, addr string) bool {
	pre := s.VotesByRound.GetVote(height, round, addr, cptypes.PrevoteType)
	pc := s.VotesByRound.GetVote(height, round, addr, cptypes.PrecommitType)
	return pre == types.VoteStateForBlock || pre == types.VoteStateNil ||
		pc == types.VoteStateForBlock || pc == types.VoteStateNil
}

func enrichLivenessRows(rows []ValidatorHealthRow, b *Builder, s *types.State) {
	height, round, _, _ := s.GetConsensusHeight()
	for i := range rows {
		rows[i].Online = votedThisHeight(s, height, int32(round), rows[i].Address)
	}
}
