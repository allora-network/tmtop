package types

import (
	"maps"
	"math/big"
	"strings"
	"sync"
	"time"

	butils "github.com/brynbellomy/go-utils"
	cptypes "github.com/cometbft/cometbft/proto/tendermint/types"
	rpctypes "github.com/cometbft/cometbft/rpc/core/types"
	ctypes "github.com/cometbft/cometbft/types"
	"github.com/rs/zerolog"
)

type State struct {
	logger zerolog.Logger

	Height    int64
	Round     int64
	Step      int64
	StartTime time.Time

	TMValidators       TMValidators
	validatorsByPeerID map[string]TMValidator

	ChainInfo *rpctypes.ResultStatus
	Upgrade   *Upgrade
	BlockTime time.Duration
	NetInfo   *NetInfo

	// Vote and proposal tracking
	VotesByRound     *RoundDataMap
	ProposalsByRound *butils.SortedMap[int64, *butils.SortedMap[int32, butils.Set[string]]]

	// RPC management
	currentRPC string
	knownRPCs  *butils.OrderedMap[string, RPC]
	rpcPeers   *butils.OrderedMap[string, []Peer]
	muRPCs     *sync.RWMutex

	// Error tracking
	ConsensusStateError  error
	ValidatorsError      error
	ChainValidatorsError error
	UpgradePlanError     error
	ChainInfoError       error
}

type RPC struct {
	ID               string `json:"id"`
	IP               string `json:"ip"`
	URL              string `json:"url"`
	Moniker          string `json:"moniker"`
	ValidatorAddress string `json:"validatorAddress"`
	ValidatorMoniker string `json:"validatorMoniker"`
}

func NewRPCFromPeer(peer Peer) RPC {
	return RPC{
		ID:      string(peer.NodeInfo.DefaultNodeID),
		IP:      peer.RemoteIP,
		URL:     peer.URL(),
		Moniker: peer.NodeInfo.Moniker,
	}
}

func NewState(firstRPC string, logger zerolog.Logger) *State {
	return &State{
		logger:             logger.With().Str("component", "state").Logger(),
		Height:             0,
		Round:              0,
		Step:               0,
		StartTime:          time.Now(),
		TMValidators:       TMValidators{},
		VotesByRound:       NewRoundDataMap(),
		ProposalsByRound:   butils.NewSortedMap[int64, *butils.SortedMap[int32, butils.Set[string]]](),
		BlockTime:          0,
		validatorsByPeerID: make(map[string]TMValidator),
		currentRPC:         firstRPC,
		knownRPCs:          butils.NewOrderedMap[string, RPC](),
		rpcPeers:           butils.NewOrderedMap[string, []Peer](),
		muRPCs:             &sync.RWMutex{},
	}
}

func (s *State) CurrentRPC() RPC {
	s.muRPCs.RLock()
	defer s.muRPCs.RUnlock()

	rpc, ok := s.knownRPCs.Get(s.currentRPC)
	if !ok {
		return RPC{URL: s.currentRPC}
	}
	return rpc
}

func (s *State) SetCurrentRPCURL(rpcURL string) {
	s.muRPCs.Lock()
	defer s.muRPCs.Unlock()

	s.currentRPC = rpcURL
}

func (s *State) KnownRPCByURL(url string) (RPC, bool) {
	s.muRPCs.RLock()
	defer s.muRPCs.RUnlock()

	rpc, ok := s.knownRPCs.Get(url)
	return rpc, ok
}

func (s *State) KnownRPCs() *butils.OrderedMap[string, RPC] {
	s.muRPCs.RLock()
	defer s.muRPCs.RUnlock()

	return s.knownRPCs.Copy()
}

func (s *State) AddKnownRPC(rpc RPC) {
	s.muRPCs.Lock()
	defer s.muRPCs.Unlock()

	s.knownRPCs.Set(rpc.URL, rpc)
}

func (s *State) IsKnownRPC(rpcURL string) bool {
	s.muRPCs.RLock()
	defer s.muRPCs.RUnlock()

	_, ok := s.knownRPCs.Get(rpcURL)
	return ok
}

func (s *State) RPCAtIndex(index int) (RPC, bool) {
	s.muRPCs.RLock()
	defer s.muRPCs.RUnlock()

	_, rpc, ok := s.knownRPCs.GetByIndex(index)
	return rpc, ok
}

func (s *State) AddRPCPeers(rpcURL string, peers []Peer) {
	s.muRPCs.Lock()
	defer s.muRPCs.Unlock()

	s.rpcPeers.Set(rpcURL, peers)
}

func (s *State) RPCPeers(rpcURL string) []Peer {
	s.muRPCs.RLock()
	defer s.muRPCs.RUnlock()

	peers, _ := s.rpcPeers.Get(rpcURL)
	return peers
}

func (s *State) ValidatorByPeerID(peerID string) (TMValidator, bool) {
	val, ok := s.validatorsByPeerID[strings.ToLower(peerID)]
	return val, ok
}

func (s *State) ValidatorsByPeerID() map[string]TMValidator {
	return maps.Clone(s.validatorsByPeerID)
}

func (s *State) AddCometBFTEvents(events []ctypes.TMEventData) {
	for _, event := range events {
		switch x := event.(type) {
		case ctypes.EventDataNewRound:
			s.VotesByRound.AddProposer(x.Height, x.Round, x.Proposer.Address.String())

		case ctypes.EventDataVote:
			s.VotesByRound.AddVote(x.Vote.Height, x.Vote.Round, x.Vote.ValidatorAddress.String(), x.Vote.Type, x.Vote.BlockID)
		}
	}
}

func (s *State) SetChainInfo(info *rpctypes.ResultStatus) {
	s.ChainInfo = info
}

func (s *State) SetUpgrade(upgrade *Upgrade) {
	s.Upgrade = upgrade
}

func (s *State) SetBlockTime(blockTime time.Duration) {
	s.BlockTime = blockTime
}

func (s *State) SetNetInfo(info *NetInfo) {
	s.NetInfo = info
}

func (s *State) SetConsensusStateError(err error) {
	s.ConsensusStateError = err
}

func (s *State) SetValidatorsError(err error) {
	s.ValidatorsError = err
}

func (s *State) SetUpgradePlanError(err error) {
	s.UpgradePlanError = err
}

func (s *State) SetChainInfoError(err error) {
	s.ChainInfoError = err
}

// GetTMValidators returns the unified validator collection.
func (s *State) GetTMValidators() TMValidators {
	return s.TMValidators
}

// SetTMValidators sets the unified validator collection.
func (s *State) SetTMValidators(validators TMValidators) {
	s.TMValidators = validators
}

// GetTotalVotingPowerPrevotedPercent calculates percentage using RoundDataMap.
func (s *State) GetTotalVotingPowerPrevotedPercent(countDisagreeing bool) *big.Float {
	if len(s.TMValidators) == 0 {
		return big.NewFloat(0)
	}

	prevoted := big.NewInt(0)
	totalVP := big.NewInt(0)

	for _, validator := range s.TMValidators {
		totalVP = totalVP.Add(totalVP, big.NewInt(validator.CometValidator.VotingPower))

		// Query RoundDataMap for current vote state
		prevoteState := s.VotesByRound.GetVote(s.Height, int32(s.Round), validator.GetDisplayAddress(), cptypes.PrevoteType)
		if prevoteState == VoteStateForBlock || (countDisagreeing && prevoteState == VoteStateNil) {
			prevoted = prevoted.Add(prevoted, big.NewInt(validator.CometValidator.VotingPower))
		}
	}

	if totalVP.Cmp(big.NewInt(0)) == 0 {
		return big.NewFloat(0)
	}

	votingPowerPercent := big.NewFloat(0).SetInt(prevoted)
	votingPowerPercent = votingPowerPercent.Quo(votingPowerPercent, big.NewFloat(0).SetInt(totalVP))
	votingPowerPercent = votingPowerPercent.Mul(votingPowerPercent, big.NewFloat(100))

	return votingPowerPercent
}

// GetTotalVotingPowerPrecommittedPercent calculates percentage using RoundDataMap.
func (s *State) GetTotalVotingPowerPrecommittedPercent(countDisagreeing bool) *big.Float {
	if len(s.TMValidators) == 0 {
		return big.NewFloat(0)
	}

	precommitted := big.NewInt(0)
	totalVP := big.NewInt(0)

	for _, validator := range s.TMValidators {
		totalVP = totalVP.Add(totalVP, big.NewInt(validator.CometValidator.VotingPower))

		// Query RoundDataMap for current vote state
		precommitState := s.VotesByRound.GetVote(s.Height, int32(s.Round), validator.GetDisplayAddress(), cptypes.PrecommitType)
		if precommitState == VoteStateForBlock || (countDisagreeing && precommitState == VoteStateNil) {
			precommitted = precommitted.Add(precommitted, big.NewInt(validator.CometValidator.VotingPower))
		}
	}

	if totalVP.Cmp(big.NewInt(0)) == 0 {
		return big.NewFloat(0)
	}

	votingPowerPercent := big.NewFloat(0).SetInt(precommitted)
	votingPowerPercent = votingPowerPercent.Quo(votingPowerPercent, big.NewFloat(0).SetInt(totalVP))
	votingPowerPercent = votingPowerPercent.Mul(votingPowerPercent, big.NewFloat(100))

	return votingPowerPercent
}
