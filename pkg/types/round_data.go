package types

import (
	"sync"

	butils "github.com/brynbellomy/go-utils"
	cptypes "github.com/cometbft/cometbft/proto/tendermint/types"
	ctypes "github.com/cometbft/cometbft/types"
)

// RoundDataMap indexes vote and proposer data by (height, round).
// Safe for concurrent use.
type RoundDataMap struct {
	mu      *sync.RWMutex
	heights *butils.SortedMap[int64, *butils.SortedMap[int32, *RoundData]]
}

// RoundData captures what we know about one consensus round at one height.
type RoundData struct {
	Proposers butils.Set[string]
	Votes     map[string]map[cptypes.SignedMsgType]ctypes.BlockID
}

// HeightAndRound is the composite key exposed by RoundDataMap.Iter.
type HeightAndRound struct {
	Height int64
	Round  int32
}

func NewRoundDataMap() *RoundDataMap {
	return &RoundDataMap{
		mu:      &sync.RWMutex{},
		heights: butils.NewSortedMap[int64, *butils.SortedMap[int32, *RoundData]](),
	}
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
