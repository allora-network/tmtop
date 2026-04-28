package types

import (
	"math/big"

	"github.com/cometbft/cometbft/p2p"
	ctypes "github.com/cometbft/cometbft/types"
)

type TMValidator struct {
	CometValidator  *ctypes.Validator
	CosmosValidator *CosmosValidator

	Index              int        // Display index for UI
	VotingPowerPercent *big.Float // Calculated percentage
	PeerID             p2p.ID     // P2P network ID
}

// GetDisplayAddress returns the validator's hex consensus address —
// the canonical key used by CometBFT for votes, proposers, and
// peer-level identity. This is what goes into VotesByRound and the
// DB's hex_address column. Display *names* go through GetDisplayName
// (moniker / bech32 / truncated hex).
//
// Falls back to the bech32 operator address only if the comet
// validator info is missing (e.g. tendermint-only chains).
func (v TMValidator) GetDisplayAddress() string {
	if v.CometValidator != nil {
		return v.CometValidator.Address.String()
	}
	if v.CosmosValidator != nil {
		return v.CosmosValidator.OperatorAddress
	}
	return ""
}

// GetDisplayName returns the best available name for display.
func (v TMValidator) GetDisplayName() string {
	if v.CosmosValidator != nil && v.CosmosValidator.Moniker != "" {
		return v.CosmosValidator.Moniker
	}
	// Truncate address for display
	addr := v.GetDisplayAddress()
	if len(addr) > 10 {
		return addr[:6] + "..." + addr[len(addr)-4:]
	}
	return addr
}

// HasAssignedKey returns true if validator has an assigned consensus key.
func (v TMValidator) HasAssignedKey() bool {
	return v.CosmosValidator != nil && v.CosmosValidator.AssignedAddress != ""
}

type TMValidators []TMValidator
