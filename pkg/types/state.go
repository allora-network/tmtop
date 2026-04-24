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

// State is the central mutable store for everything tmtop renders.
// All access is serialized through mu. Setters Lock; readers either
// use accessor methods that RLock, or interact with already-concurrent
// sub-structures (VotesByRound, ProposalsByRound) that have their own
// internal synchronization.
//
// Fields are left exported because rendering touches many of them and
// unexporting everything would be churn; discipline lives in the
// accessor conventions below.
type State struct {
	mu     sync.RWMutex
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

	// Vote and proposal tracking. These have their own internal
	// mutexes so they can be accessed without holding State.mu.
	VotesByRound     *RoundDataMap
	ProposalsByRound *butils.SortedMap[int64, *butils.SortedMap[int32, butils.Set[string]]]

	// RPC management. Guarded by the main mu.
	currentRPC string
	knownRPCs  *butils.OrderedMap[string, RPC]
	rpcPeers   *butils.OrderedMap[string, []Peer]

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
	}
}

func (s *State) CurrentRPC() RPC {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rpc, ok := s.knownRPCs.Get(s.currentRPC)
	if !ok {
		return RPC{URL: s.currentRPC}
	}
	return rpc
}

func (s *State) SetCurrentRPCURL(rpcURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.currentRPC = rpcURL
}

func (s *State) KnownRPCByURL(url string) (RPC, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rpc, ok := s.knownRPCs.Get(url)
	return rpc, ok
}

func (s *State) KnownRPCs() *butils.OrderedMap[string, RPC] {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.knownRPCs.Copy()
}

func (s *State) AddKnownRPC(rpc RPC) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.knownRPCs.Set(rpc.URL, rpc)
}

func (s *State) IsKnownRPC(rpcURL string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.knownRPCs.Get(rpcURL)
	return ok
}

func (s *State) RPCAtIndex(index int) (RPC, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, rpc, ok := s.knownRPCs.GetByIndex(index)
	return rpc, ok
}

func (s *State) AddRPCPeers(rpcURL string, peers []Peer) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.rpcPeers.Set(rpcURL, peers)
}

func (s *State) RPCPeers(rpcURL string) []Peer {
	s.mu.RLock()
	defer s.mu.RUnlock()

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
	// VotesByRound carries its own mutex, so no State.mu needed here.
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
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ChainInfo = info
}

func (s *State) GetChainInfo() *rpctypes.ResultStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ChainInfo
}

func (s *State) SetUpgrade(upgrade *Upgrade) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Upgrade = upgrade
}

func (s *State) GetUpgrade() *Upgrade {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Upgrade
}

func (s *State) SetBlockTime(blockTime time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.BlockTime = blockTime
}

func (s *State) GetBlockTime() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.BlockTime
}

func (s *State) SetNetInfo(info *NetInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.NetInfo = info
}

func (s *State) GetNetInfo() *NetInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.NetInfo
}

func (s *State) SetConsensusStateError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ConsensusStateError = err
}

func (s *State) GetConsensusStateError() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ConsensusStateError
}

func (s *State) SetValidatorsError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ValidatorsError = err
}

func (s *State) SetUpgradePlanError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.UpgradePlanError = err
}

func (s *State) GetUpgradePlanError() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.UpgradePlanError
}

func (s *State) SetChainInfoError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ChainInfoError = err
}

func (s *State) GetChainInfoError() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ChainInfoError
}

// SetConsensusHeight records the current height/round/step/start time
// in one locked write.
func (s *State) SetConsensusHeight(height, round, step int64, startTime time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Height = height
	s.Round = round
	s.Step = step
	s.StartTime = startTime
}

// GetConsensusHeight returns the current height/round/step/start time.
func (s *State) GetConsensusHeight() (height, round, step int64, startTime time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Height, s.Round, s.Step, s.StartTime
}

// GetTMValidators returns the unified validator collection.
func (s *State) GetTMValidators() TMValidators {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.TMValidators
}

// SetTMValidators sets the unified validator collection.
func (s *State) SetTMValidators(validators TMValidators) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TMValidators = validators
}

// GetTotalVotingPowerPrevotedPercent calculates percentage using RoundDataMap.
func (s *State) GetTotalVotingPowerPrevotedPercent(countDisagreeing bool) *big.Float {
	s.mu.RLock()
	validators := s.TMValidators
	height, round := s.Height, int32(s.Round)
	s.mu.RUnlock()
	return votingPowerPercent(validators, s.VotesByRound, height, round, cptypes.PrevoteType, countDisagreeing)
}

// GetTotalVotingPowerPrecommittedPercent calculates percentage using RoundDataMap.
func (s *State) GetTotalVotingPowerPrecommittedPercent(countDisagreeing bool) *big.Float {
	s.mu.RLock()
	validators := s.TMValidators
	height, round := s.Height, int32(s.Round)
	s.mu.RUnlock()
	return votingPowerPercent(validators, s.VotesByRound, height, round, cptypes.PrecommitType, countDisagreeing)
}

func votingPowerPercent(
	validators TMValidators,
	votes *RoundDataMap,
	height int64,
	round int32,
	msgType cptypes.SignedMsgType,
	countDisagreeing bool,
) *big.Float {
	if len(validators) == 0 {
		return big.NewFloat(0)
	}

	voted := big.NewInt(0)
	totalVP := big.NewInt(0)

	for _, validator := range validators {
		totalVP = totalVP.Add(totalVP, big.NewInt(validator.CometValidator.VotingPower))

		state := votes.GetVote(height, round, validator.GetDisplayAddress(), msgType)
		if state == VoteStateForBlock || (countDisagreeing && state == VoteStateNil) {
			voted = voted.Add(voted, big.NewInt(validator.CometValidator.VotingPower))
		}
	}

	if totalVP.Cmp(big.NewInt(0)) == 0 {
		return big.NewFloat(0)
	}

	percent := big.NewFloat(0).SetInt(voted)
	percent = percent.Quo(percent, big.NewFloat(0).SetInt(totalVP))
	percent = percent.Mul(percent, big.NewFloat(100))
	return percent
}
