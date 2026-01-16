package types

import "time"

type CometConsensusStateResponse struct {
	Result *CometConsensusStateResult `json:"result"`
}

type CometConsensusStateResult struct {
	RoundState *CometConsensusStateRoundState `json:"round_state"`
}

type CometConsensusStateRoundState struct {
	HeightRoundStep string                        `json:"height/round/step"`
	StartTime       time.Time                     `json:"start_time"`
	HeightVoteSet   []CometConsensusHeightVoteSet `json:"height_vote_set"`
	Proposer        CometConsensusStateProposer   `json:"proposer"`
}

type CometConsensusHeightVoteSet struct {
	Round              int                        `json:"round"`
	Prevotes           []CometConsensusVote       `json:"prevotes"`
	Precommits         []CometConsensusVote       `json:"precommits"`
	PrevotesBitArray   CometConsensusVoteBitArray `json:"prevotes_bit_array"`
	PrecommitsBitArray CometConsensusVoteBitArray `json:"precommits_bit_array"`
}

type CometConsensusStateProposer struct {
	Address string `json:"address"`
	Index   int    `json:"index"`
}

type (
	CometConsensusVote         string
	CometConsensusVoteBitArray string
)

type CometDumpConsensusStateResponse struct {
	Result *CometDumpConsensusStateResult `json:"result"`
}

type CometDumpConsensusStateResult struct {
	RoundState *CometDumpConsensusStateRoundState `json:"round_state"`
}

type CometDumpConsensusStateRoundState struct {
	Validators CometDumpConsensusStateRoundStateValidators `json:"validators"`
}

type CometDumpConsensusStateRoundStateValidators struct {
	Validators []CometValidator `json:"validators"`
}
