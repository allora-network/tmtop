package types

import (
	cptypes "github.com/cometbft/cometbft/proto/tendermint/types"
	ctypes "github.com/cometbft/cometbft/types"
)

// VoteState represents the state of a validator's vote using CometBFT types directly
type VoteState int

const (
	VoteStateNone VoteState = iota
	VoteStateNil
	VoteStateForBlock
)

// VoteStateFromCometBFT determines vote state from CometBFT types
func VoteStateFromCometBFT(voteExists bool, blockID ctypes.BlockID) VoteState {
	if !voteExists {
		return VoteStateNone
	}
	if blockID.IsZero() {
		return VoteStateNil
	}
	return VoteStateForBlock
}

// VoteStateFromVotesMap determines vote state from votes map
func VoteStateFromVotesMap(votesMap map[cptypes.SignedMsgType]ctypes.BlockID, msgType cptypes.SignedMsgType) VoteState {
	blockID, exists := votesMap[msgType]
	return VoteStateFromCometBFT(exists, blockID)
}

// Serialize returns UI representation of vote state
func (v VoteState) Serialize(disableEmojis bool) string {
	if disableEmojis {
		switch v {
		case VoteStateForBlock:
			return "[X[]"
		case VoteStateNil:
			return "[0[]"
		case VoteStateNone:
			return "[ []"
		default:
			return ""
		}
	}

	switch v {
	case VoteStateForBlock:
		return "✅"
	case VoteStateNil:
		return "🤷"
	case VoteStateNone:
		return "❌"
	default:
		return ""
	}
}
