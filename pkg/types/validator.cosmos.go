package types

import (
	cmtprotocrypto "github.com/cometbft/cometbft/proto/tendermint/crypto"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
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
