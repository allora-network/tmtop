package types

import (
	"fmt"
	"maps"
	"math/big"
	"strings"
	"sync"
	"time"

	"main/pkg/utils"

	butils "github.com/brynbellomy/go-utils"
	cptypes "github.com/cometbft/cometbft/proto/tendermint/types"
	ctypes "github.com/cometbft/cometbft/types"
	"github.com/rs/zerolog"
)

type State struct {
	logger zerolog.Logger

	Height    int64
	Round     int64
	Step      int64
	StartTime time.Time

	// Unified validator collection
	TMValidators TMValidators

	// Chain metadata
	ChainValidators *ChainValidators
	ChainInfo       *TendermintStatusResult
	Upgrade         *Upgrade
	BlockTime       time.Duration
	NetInfo         *NetInfo

	// Vote and proposal tracking
	VotesByRound     *RoundDataMap
	ProposalsByRound *butils.SortedMap[int64, *butils.SortedMap[int32, butils.Set[string]]]

	// Simplified validator tracking
	validatorsByPeerID map[string]TMValidator

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
		ChainValidators:    nil,
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

func (s *State) Clear() {
	s.Height = 0
	s.Round = 0
	s.Step = 0
	s.TMValidators = TMValidators{}
	s.ChainValidators = nil
	s.ChainInfo = nil
	s.StartTime = time.Now()
	s.Upgrade = nil
	s.BlockTime = time.Duration(0)
	s.NetInfo = nil
	s.ConsensusStateError = nil
	s.ValidatorsError = nil
	s.ChainValidatorsError = nil
	s.UpgradePlanError = nil
	s.ChainInfoError = nil
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
	s.logger.Error().Msg("AddCometBFTEvents")
	for _, event := range events {
		switch x := event.(type) {
		case ctypes.EventDataNewRound:
			s.VotesByRound.AddProposer(x.Height, x.Round, x.Proposer.Address.String())

		case ctypes.EventDataVote:
			s.VotesByRound.AddVote(x.Vote.Height, x.Vote.Round, x.Vote.ValidatorAddress.String(), x.Vote.Type, x.Vote.BlockID)
		}
	}
}

func (s *State) SetChainValidators(validators *ChainValidators) {
	s.ChainValidators = validators
}

func (s *State) SetChainInfo(info *TendermintStatusResult) {
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

func (s *State) SerializeConsensus(timezone *time.Location) string {
	if s.ConsensusStateError != nil {
		return fmt.Sprintf(" consensus state error: %s", s.ConsensusStateError)
	}

	if len(s.TMValidators) == 0 {
		return ""
	}

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(" height=%d round=%d step=%d\n", s.Height, s.Round, s.Step))
	sb.WriteString(fmt.Sprintf(
		" block time: %s (%s)\n",
		utils.ZeroOrPositiveDuration(utils.SerializeDuration(time.Since(s.StartTime))),
		utils.SerializeTime(s.StartTime.In(timezone)),
	))

	sb.WriteString(fmt.Sprintf(
		" prevote consensus (total/agreeing): %.2f / %.2f\n",
		s.GetTotalVotingPowerPrevotedPercent(true),
		s.GetTotalVotingPowerPrevotedPercent(false),
	))
	sb.WriteString(fmt.Sprintf(
		" precommit consensus (total/agreeing): %.2f / %.2f\n",
		s.GetTotalVotingPowerPrecommittedPercent(true),
		s.GetTotalVotingPowerPrecommittedPercent(false),
	))

	sb.WriteString(fmt.Sprintf(" last updated at: %s\n", utils.SerializeTime(time.Now().In(timezone))))

	return sb.String()
}

func (s *State) SerializeChainInfo(timezone *time.Location) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(" rpc: %v\n", s.CurrentRPC().URL))
	sb.WriteString(fmt.Sprintf(" (%v)\n\n", s.CurrentRPC().Moniker))

	if s.ChainInfoError != nil {
		sb.WriteString(fmt.Sprintf(" chain info fetch error: %s\n", s.ChainInfoError.Error()))
	} else if s.ChainInfo != nil {
		sb.WriteString(fmt.Sprintf(" chain name: %s\n", s.ChainInfo.NodeInfo.Network))
		sb.WriteString(fmt.Sprintf(" tendermint version: v%s\n", s.ChainInfo.NodeInfo.Version))

		if s.BlockTime != 0 {
			sb.WriteString(fmt.Sprintf(" avg block time: %s\n", utils.SerializeDuration(s.BlockTime)))
		}
	}

	if s.UpgradePlanError != nil {
		sb.WriteString(fmt.Sprintf(" upgrade plan fetch error: %s\n", s.UpgradePlanError))
	} else if s.Upgrade == nil {
		sb.WriteString(" no chain upgrade scheduled\n")
	} else {
		sb.WriteString(s.SerializeUpgradeInfo(timezone))
	}

	return sb.String()
}

func (s *State) SerializeUpgradeInfo(timezone *time.Location) string {
	var sb strings.Builder

	if s.Upgrade.Height+1 == s.Height {
		sb.WriteString(" upgrade in progress...\n")
		return sb.String()
	}

	if s.Upgrade.Height+1 < s.Height {
		sb.WriteString(fmt.Sprintf(
			" chain upgrade %s applied at block %d\n",
			s.Upgrade.Name,
			s.Upgrade.Height,
		))

		sb.WriteString(fmt.Sprintf(
			" blocks since upgrade: %d\n",
			s.Height-s.Upgrade.Height,
		))

		if s.BlockTime == 0 {
			return sb.String()
		}

		upgradeTime := utils.CalculateTimeTillBlock(s.Height, s.Upgrade.Height, s.BlockTime)
		sb.WriteString(fmt.Sprintf(
			" time since upgrade: %s\n",
			utils.SerializeDuration(time.Since(upgradeTime)),
		))

		sb.WriteString(fmt.Sprintf(" upgrade approximate time: %s\n", utils.SerializeTime(upgradeTime.In(timezone))))
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf(
		" chain upgrade %s scheduled at block %d\n",
		s.Upgrade.Name,
		s.Upgrade.Height,
	))

	sb.WriteString(fmt.Sprintf(
		" blocks till upgrade: %d\n",
		s.Upgrade.Height-s.Height,
	))

	if s.BlockTime == 0 {
		return sb.String()
	}

	upgradeTime := utils.CalculateTimeTillBlock(s.Height, s.Upgrade.Height, s.BlockTime)

	sb.WriteString(fmt.Sprintf(
		" time till upgrade: %s\n",
		utils.SerializeDuration(time.Until(upgradeTime)),
	))

	sb.WriteString(fmt.Sprintf(" upgrade estimated time: %s\n", utils.SerializeTime(upgradeTime.In(timezone))))

	return sb.String()
}

func (s *State) SerializeProgressbar(width int, height int, prefix string, progress int) string {
	progressBar := ProgressBar{
		Width:    width,
		Height:   height,
		Progress: progress,
		Prefix:   prefix,
	}

	return progressBar.Serialize()
}

func (s *State) SerializePrevotesProgressbar(width int, height int) string {
	if len(s.TMValidators) == 0 {
		return ""
	}

	prevotePercent := s.GetTotalVotingPowerPrevotedPercent(true)
	prevotePercentFloat, _ := prevotePercent.Float64()
	prevotePercentInt := int(prevotePercentFloat)

	return s.SerializeProgressbar(width, height, "Prevotes: ", prevotePercentInt)
}

func (s *State) SerializePrecommitsProgressbar(width int, height int) string {
	if len(s.TMValidators) == 0 {
		return ""
	}

	precommitPercent := s.GetTotalVotingPowerPrecommittedPercent(true)
	precommitPercentFloat, _ := precommitPercent.Float64()
	precommitPercentInt := int(precommitPercentFloat)

	return s.SerializeProgressbar(width, height, "Precommits: ", precommitPercentInt)
}

// TMValidator-based methods

// GetTMValidators returns the unified validator collection
func (s *State) GetTMValidators() TMValidators {
	return s.TMValidators
}

// SetTMValidators sets the unified validator collection
func (s *State) SetTMValidators(validators TMValidators) {
	s.TMValidators = validators
}

// UpdateTMValidatorsWithRoundVotes updates current round vote state for all validators
func (s *State) UpdateTMValidatorsWithRoundVotes(height int64, round int32) {
	for i := range s.TMValidators {
		validator := &s.TMValidators[i]

		// Create current round vote state from VotesByRound data
		roundVoteState := &RoundVoteState{
			Address:    validator.GetDisplayAddress(),
			Prevote:    s.VotesByRound.GetVote(height, round, validator.GetDisplayAddress(), cptypes.PrevoteType),
			Precommit:  s.VotesByRound.GetVote(height, round, validator.GetDisplayAddress(), cptypes.PrecommitType),
			IsProposer: s.VotesByRound.GetProposers(height, round).Has(validator.GetDisplayAddress()),
		}

		validator.CurrentRoundVote = roundVoteState
	}
}

// SyncTMValidatorsWithChainValidators updates TMValidators with chain validator metadata
func (s *State) SyncTMValidatorsWithChainValidators() {
	if s.ChainValidators == nil {
		return
	}

	chainValidatorsMap := s.ChainValidators.ToMap()
	for i := range s.TMValidators {
		validator := &s.TMValidators[i]
		if chainValidator, ok := chainValidatorsMap[validator.GetDisplayAddress()]; ok {
			validator.ChainValidator = &chainValidator
		}
	}
}

type RoundDataMap struct {
	mu      *sync.RWMutex
	heights *butils.SortedMap[int64, *butils.SortedMap[int32, *RoundData]]
}

type RoundData struct {
	Proposers butils.Set[string]
	Votes     map[string]map[cptypes.SignedMsgType]ctypes.BlockID
}

func NewRoundDataMap() *RoundDataMap {
	return &RoundDataMap{
		mu:      &sync.RWMutex{},
		heights: butils.NewSortedMap[int64, *butils.SortedMap[int32, *RoundData]](),
	}
}

type HeightAndRound struct {
	Height int64
	Round  int32
}

func (v *RoundDataMap) Iter() func(yield func(HeightAndRound, *RoundData) bool) {
	return func(yield func(hr HeightAndRound, rd *RoundData) bool) {
		v.mu.RLock()
		defer v.mu.RUnlock()

		for height, heightMap := range v.heights.Iter() {
			for round, roundData := range heightMap.Iter() {
				if !yield(HeightAndRound{height, round}, roundData) {
					return
				}
			}
		}
	}
}

func (v *RoundDataMap) ReverseIter() func(yield func(HeightAndRound, *RoundData) bool) {
	return func(yield func(hr HeightAndRound, rd *RoundData) bool) {
		v.mu.RLock()
		defer v.mu.RUnlock()

		for height, heightMap := range v.heights.ReverseIter() {
			for round, roundData := range heightMap.ReverseIter() {
				if !yield(HeightAndRound{height, round}, roundData) {
					return
				}
			}
		}
	}
}

func (v *RoundDataMap) AddProposer(height int64, round int32, proposer string) {
	v.mu.Lock()
	defer v.mu.Unlock()

	roundData := v.upsertRoundData(height, round)
	roundData.Proposers.Add(proposer)
}

func (v *RoundDataMap) GetProposers(height int64, round int32) butils.Set[string] {
	v.mu.RLock()
	defer v.mu.RUnlock()

	roundMap, ok := v.heights.Get(height)
	if !ok {
		return nil
	}

	roundData, ok := roundMap.Get(round)
	if !ok {
		return nil
	}

	return roundData.Proposers.Copy()
}

func (v *RoundDataMap) AddVote(height int64, round int32, validator string, msgType cptypes.SignedMsgType, blockID ctypes.BlockID) {
	v.mu.Lock()
	defer v.mu.Unlock()

	roundData := v.upsertRoundData(height, round)

	votesMap, ok := roundData.Votes[validator]
	if !ok {
		votesMap = map[cptypes.SignedMsgType]ctypes.BlockID{}
		roundData.Votes[validator] = votesMap
	}

	votesMap[msgType] = blockID
}

func (v *RoundDataMap) GetVote(height int64, round int32, validator string, msgType cptypes.SignedMsgType) VoteState {
	v.mu.RLock()
	defer v.mu.RUnlock()

	roundMap, ok := v.heights.Get(height)
	if !ok {
		return VoteStateNone
	}

	roundData, ok := roundMap.Get(round)
	if !ok {
		return VoteStateNone
	}

	votesMap, ok := roundData.Votes[validator]
	if !ok {
		return VoteStateNone
	}

	blockID, ok := votesMap[msgType]
	if !ok {
		return VoteStateNone
	} else if blockID.IsZero() {
		return VoteStateNil
	}
	return VoteStateForBlock
}

func (v *RoundDataMap) upsertRoundData(height int64, round int32) *RoundData {
	roundMap, ok := v.heights.Get(height)
	if !ok {
		roundMap = butils.NewSortedMap[int32, *RoundData]()
		v.heights.Insert(height, roundMap)
	}

	roundData, ok := roundMap.Get(round)
	if !ok {
		roundData = &RoundData{
			Proposers: butils.NewSet[string](),
			Votes:     make(map[string]map[cptypes.SignedMsgType]ctypes.BlockID),
		}
		roundMap.Insert(round, roundData)
	}

	return roundData
}

// GetTotalVotingPowerPrevotedPercent calculates percentage using RoundDataMap
func (s *State) GetTotalVotingPowerPrevotedPercent(countDisagreeing bool) *big.Float {
	if len(s.TMValidators) == 0 {
		return big.NewFloat(0)
	}

	prevoted := big.NewInt(0)
	totalVP := big.NewInt(0)

	for _, validator := range s.TMValidators {
		totalVP = totalVP.Add(totalVP, big.NewInt(validator.VotingPower))

		// Query RoundDataMap for current vote state
		prevoteState := s.VotesByRound.GetVote(s.Height, int32(s.Round), validator.GetDisplayAddress(), cptypes.PrevoteType)
		if prevoteState == VoteStateForBlock || (countDisagreeing && prevoteState == VoteStateNil) {
			prevoted = prevoted.Add(prevoted, big.NewInt(validator.VotingPower))
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

// GetTotalVotingPowerPrecommittedPercent calculates percentage using RoundDataMap
func (s *State) GetTotalVotingPowerPrecommittedPercent(countDisagreeing bool) *big.Float {
	if len(s.TMValidators) == 0 {
		return big.NewFloat(0)
	}

	precommitted := big.NewInt(0)
	totalVP := big.NewInt(0)

	for _, validator := range s.TMValidators {
		totalVP = totalVP.Add(totalVP, big.NewInt(validator.VotingPower))

		// Query RoundDataMap for current vote state
		precommitState := s.VotesByRound.GetVote(s.Height, int32(s.Round), validator.GetDisplayAddress(), cptypes.PrecommitType)
		if precommitState == VoteStateForBlock || (countDisagreeing && precommitState == VoteStateNil) {
			precommitted = precommitted.Add(precommitted, big.NewInt(validator.VotingPower))
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
