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

// GetDisplayAddress returns the best available address for display.
func (v TMValidator) GetDisplayAddress() string {
	return v.CosmosValidator.OperatorAddress
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
