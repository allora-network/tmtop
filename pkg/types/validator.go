package types

import (
	"fmt"
	"main/pkg/utils"
	"math/big"
	"strconv"

	"github.com/cometbft/cometbft/p2p"
	ctypes "github.com/cometbft/cometbft/types"
)


// TMValidator represents a unified validator type based on CometBFT with tmtop extensions
type TMValidator struct {
	// Core CometBFT validator data
	ctypes.Validator

	// tmtop-specific fields
	Index              int              // Display index for UI
	VotingPowerPercent *big.Float       // Calculated percentage
	PeerID             p2p.ID           // P2P network ID

	// Optional chain-specific metadata
	ChainValidator *ChainValidator     // Moniker, assigned addresses, etc.

	// Current round vote state (optional)
	CurrentRoundVote *RoundVoteState   // Vote state for current round
}

// GetDisplayAddress returns the best available address for display
func (v TMValidator) GetDisplayAddress() string {
	return v.Address.String()
}

// GetDisplayName returns the best available name for display
func (v TMValidator) GetDisplayName() string {
	if v.ChainValidator != nil && v.ChainValidator.Moniker != "" {
		return v.ChainValidator.Moniker
	}
	// Truncate address for display
	addr := v.GetDisplayAddress()
	if len(addr) > 10 {
		return addr[:6] + "..." + addr[len(addr)-4:]
	}
	return addr
}

// HasAssignedKey returns true if validator has an assigned consensus key
func (v TMValidator) HasAssignedKey() bool {
	return v.ChainValidator != nil && v.ChainValidator.AssignedAddress != ""
}

// Serialize returns formatted string for display (replaces ValidatorWithInfo.Serialize)
func (v TMValidator) Serialize(disableEmojis bool) string {
	name := v.GetDisplayName()
	if v.HasAssignedKey() {
		emoji := "🔑"
		if disableEmojis {
			emoji = "[k[]"
		}
		name = emoji + " " + name
	}

	// If no current round vote state, show placeholders
	prevoteStr := "❌"
	precommitStr := "❌"
	if disableEmojis {
		prevoteStr = "[ []"
		precommitStr = "[ []"
	}

	if v.CurrentRoundVote != nil {
		prevoteStr = v.CurrentRoundVote.Prevote.Serialize(disableEmojis)
		precommitStr = v.CurrentRoundVote.Precommit.Serialize(disableEmojis)
	}

	// Format voting power percentage
	votingPowerStr := "0.00"
	if v.VotingPowerPercent != nil {
		votingPowerStr = v.VotingPowerPercent.Text('f', 2)
	}

	return fmt.Sprintf(
		" %s %s %s %s%% %s ",
		prevoteStr,
		precommitStr,
		utils.RightPadAndTrim(strconv.Itoa(v.Index+1), 3),
		utils.RightPadAndTrim(votingPowerStr, 6),
		utils.LeftPadAndTrim(name, 25),
	)
}

type TMValidators []TMValidator

// GetTotalVotingPower returns the sum of all validators' voting power
func (v TMValidators) GetTotalVotingPower() *big.Int {
	sum := big.NewInt(0)
	for _, validator := range v {
		sum = sum.Add(sum, big.NewInt(validator.VotingPower))
	}
	return sum
}

// GetTotalVotingPowerPrevotedPercent calculates percentage of voting power that prevoted
func (v TMValidators) GetTotalVotingPowerPrevotedPercent(countDisagreeing bool) *big.Float {
	prevoted := big.NewInt(0)
	totalVP := big.NewInt(0)

	for _, validator := range v {
		totalVP = totalVP.Add(totalVP, big.NewInt(validator.VotingPower))
		if validator.CurrentRoundVote != nil {
			if validator.CurrentRoundVote.Prevote == VoteStateForBlock || 
			   (countDisagreeing && validator.CurrentRoundVote.Prevote == VoteStateNone) {
				prevoted = prevoted.Add(prevoted, big.NewInt(validator.VotingPower))
			}
		}
	}

	votingPowerPercent := big.NewFloat(0).SetInt(prevoted)
	votingPowerPercent = votingPowerPercent.Quo(votingPowerPercent, big.NewFloat(0).SetInt(totalVP))
	votingPowerPercent = votingPowerPercent.Mul(votingPowerPercent, big.NewFloat(100))

	return votingPowerPercent
}

// GetTotalVotingPowerPrecommittedPercent calculates percentage of voting power that precommitted
func (v TMValidators) GetTotalVotingPowerPrecommittedPercent(countDisagreeing bool) *big.Float {
	precommitted := big.NewInt(0)
	totalVP := big.NewInt(0)

	for _, validator := range v {
		totalVP = totalVP.Add(totalVP, big.NewInt(validator.VotingPower))
		if validator.CurrentRoundVote != nil {
			if validator.CurrentRoundVote.Precommit == VoteStateForBlock || 
			   (countDisagreeing && validator.CurrentRoundVote.Precommit == VoteStateNone) {
				precommitted = precommitted.Add(precommitted, big.NewInt(validator.VotingPower))
			}
		}
	}

	votingPowerPercent := big.NewFloat(0).SetInt(precommitted)
	votingPowerPercent = votingPowerPercent.Quo(votingPowerPercent, big.NewFloat(0).SetInt(totalVP))
	votingPowerPercent = votingPowerPercent.Mul(votingPowerPercent, big.NewFloat(100))

	return votingPowerPercent
}


// RoundVoteState represents validator vote state for a specific round using new VoteState
type RoundVoteState struct {
	Address    string
	Prevote    VoteState
	Precommit  VoteState
	IsProposer bool
}

func (v RoundVoteState) Serialize(disableEmojis bool) string {
	return fmt.Sprintf(
		" %s %s",
		v.Prevote.Serialize(disableEmojis),
		v.Precommit.Serialize(disableEmojis),
	)
}


