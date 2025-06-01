package types

type VoteType int

const (
	NoVote VoteType = iota
	VotedNil
	VotedForBlock
)

func (v VoteType) Serialize(disableEmojis bool) string {
	if disableEmojis {
		switch v {
		case VotedForBlock:
			return "[X[]"
		case VotedNil:
			return "[0[]"
		case NoVote:
			return "[ []"
		default:
			return ""
		}
	}

	switch v {
	case VotedForBlock:
		return "✅"
	case VotedNil:
		return "🤷"
	case NoVote:
		return "❌"
	default:
		return ""
	}
}
