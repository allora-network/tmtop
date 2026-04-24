package types

import (
	"strings"

	cmtprotocrypto "github.com/cometbft/cometbft/proto/tendermint/crypto"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdkTypes "github.com/cosmos/cosmos-sdk/types"
)

type CosmosValidator struct {
	Moniker              string
	ConsensusPubkey      cryptotypes.PubKey
	CometConsensusPubkey cmtprotocrypto.PublicKey
	OperatorAddress      string
	ConsensusAddress     string
	AssignedAddress      string
	RawAssignedAddress   string
}

type CosmosValidators []CosmosValidator

func (c CosmosValidators) ToMap() map[string]CosmosValidator {
	valsMap := make(map[string]CosmosValidator, len(c))

	for _, validator := range c {
		// Index by operator address (bech32)
		valsMap[validator.OperatorAddress] = validator

		// Index by consensus address for TMValidator matching
		if validator.ConsensusAddress != "" {
			valsMap[validator.ConsensusAddress] = validator

			// Also index by hex format for CometBFT TMValidator matching
			if consAddr, err := sdkTypes.ConsAddressFromBech32(validator.ConsensusAddress); err == nil {
				hexAddr := strings.ToUpper(consAddr.String())
				valsMap[hexAddr] = validator
			}
		}

		// Index by assigned address if available
		if validator.RawAssignedAddress != "" {
			valsMap[validator.RawAssignedAddress] = validator
		}
	}

	return valsMap
}
