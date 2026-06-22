package metrics

import (
	"context"
	"sync"
	"time"

	"main/pkg/analytics"
	"main/pkg/asn"
	"main/pkg/types"

	ctypes "github.com/cometbft/cometbft/types"
	"github.com/rs/zerolog"
)

const ringCapacity = 200 // recent blocks retained for trend/jitter/rounds

// Builder is the single integration seam for health metrics. It is the only
// stateful object: it remembers recent block timing and accumulates security
// events between periodic Build() calls.
type Builder struct {
	mu     sync.Mutex
	logger zerolog.Logger
	now    func() time.Time

	// dependencies (nil ⇒ that family of metrics is unavailable)
	analytics *analytics.ValidatorAnalytics
	asn       *asn.Lookup

	windowBlocks int64

	// cross-call timing state (WS-B/WS-C feed/read these via observe())
	lastHeight         int64
	lastHeightChangeAt time.Time
	blockIntervals     []time.Duration // ring, oldest→newest
	maxRoundPerHeight  []int32         // ring, parallel to heights observed
	curHeightMaxRound  int32

	// step timing accumulators (WS-H)
	step stepAccumulator

	// equivocation accumulators (WS-G)
	equiv equivState
}

func NewBuilder(
	logger zerolog.Logger,
	now func() time.Time,
	an *analytics.ValidatorAnalytics,
	asnLookup *asn.Lookup,
	windowBlocks int64,
) *Builder {
	return &Builder{
		logger:       logger.With().Str("component", "metrics_builder").Logger(),
		now:          now,
		analytics:    an,
		asn:          asnLookup,
		windowBlocks: windowBlocks,
		step:         newStepAccumulator(),
		equiv:        newEquivState(),
	}
}

// Build computes a full health snapshot from current state + accumulated history.
func (b *Builder) Build(state *types.State) (*NetworkHealth, []ValidatorHealthRow) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.observe(state)

	validators := state.GetTMValidators()
	nh := &NetworkHealth{}

	// network-wide enrichers. Each lives in its own file with a no-op body
	// (created in Step 2); its workstream fills the body later. builder.go is
	// NEVER edited by a workstream — this keeps the 9 workstreams conflict-free.
	enrichDecentralization(nh, validators)
	enrichLiveness(nh, b, state)
	enrichBlockTime(nh, b)
	enrichStepTiming(nh, b)
	enrichMempool(nh, state)
	enrichASN(nh, state, b.asn)
	enrichEquivocations(nh, b)

	// per-validator rows
	rows := buildBaseRows(validators)
	ctx := context.Background()
	enrichLivenessRows(rows, b, state)
	enrichPerformance(ctx, rows, b)
	enrichLatency(ctx, rows, b)
	enrichASNRows(rows, state, b.asn)
	enrichEquivocationFlags(rows, b)

	return nh, rows
}

// ObserveEvents is called from the websocket handler with each batch of events.
func (b *Builder) ObserveEvents(events []ctypes.TMEventData) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, e := range events {
		b.observeForEquivocation(e)
		b.observeForStepTiming(e)
	}
}

// observe updates cross-call timing buffers on height change. Block intervals
// use locally observed wall-clock between new heights (single-vantage), which
// is self-consistent with the halt timer.
func (b *Builder) observe(state *types.State) {
	height, round, _, _ := state.GetConsensusHeight()
	b.curHeightMaxRound = int32(round)
	if height == b.lastHeight {
		return
	}
	now := b.now()
	if b.lastHeight != 0 && height > b.lastHeight {
		if !b.lastHeightChangeAt.IsZero() {
			b.blockIntervals = appendRing(b.blockIntervals, now.Sub(b.lastHeightChangeAt))
		}
		// record the max round reached for the height we just left
		b.maxRoundPerHeight = appendRing32(b.maxRoundPerHeight, b.curHeightMaxRound)
	}
	b.lastHeight = height
	b.lastHeightChangeAt = now
}

func buildBaseRows(validators types.TMValidators) []ValidatorHealthRow {
	rows := make([]ValidatorHealthRow, 0, len(validators))
	for _, v := range validators {
		pct := 0.0
		if v.VotingPowerPercent != nil {
			pct, _ = v.VotingPowerPercent.Float64()
		}
		rows = append(rows, ValidatorHealthRow{
			Address:        v.GetDisplayAddress(),
			Moniker:        v.GetDisplayName(),
			VotingPowerPct: pct,
		})
	}
	return rows
}

func appendRing(s []time.Duration, v time.Duration) []time.Duration {
	s = append(s, v)
	if len(s) > ringCapacity {
		s = s[len(s)-ringCapacity:]
	}
	return s
}

func appendRing32(s []int32, v int32) []int32 {
	s = append(s, v)
	if len(s) > ringCapacity {
		s = s[len(s)-ringCapacity:]
	}
	return s
}

// NOTE: every enrich* function, plus the stepAccumulator/equivState types and
// the observeFor* methods referenced above, are defined in sibling files
// created in Step 2 with no-op bodies. builder.go itself declares NONE of them,
// so each workstream edits only its own file.
