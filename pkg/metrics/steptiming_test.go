package metrics

import (
	"testing"
	"time"

	ctypes "github.com/cometbft/cometbft/types"
)

// makeRoundState returns an EventDataRoundState with the given step string.
func makeRoundState(step string) ctypes.EventDataRoundState {
	return ctypes.EventDataRoundState{Height: 1, Round: 0, Step: step}
}

// TestStepTimingBasicPropose feeds a Propose→Prevote transition and asserts
// that the propose duration lands in the propose bucket.
func TestStepTimingBasicPropose(t *testing.T) {
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ticks := []time.Time{
		t0,
		t0.Add(500 * time.Millisecond), // propose lasted 500 ms
	}
	idx := 0
	b := &Builder{
		now: func() time.Time {
			v := ticks[idx]
			if idx < len(ticks)-1 {
				idx++
			}
			return v
		},
		step: newStepAccumulator(),
	}

	// First event: enter Propose at t0
	b.observeForStepTiming(makeRoundState("RoundStepPropose"))
	// Second event: enter Prevote at t0+500ms → records 500ms in propose bucket
	b.observeForStepTiming(makeRoundState("RoundStepPrevote"))

	nh := &NetworkHealth{}
	enrichStepTiming(nh, b)

	want := 500 * time.Millisecond
	if nh.AvgProposeTime != want {
		t.Errorf("AvgProposeTime: got %v, want %v", nh.AvgProposeTime, want)
	}
	if nh.AvgPrevoteTime != 0 {
		t.Errorf("AvgPrevoteTime: got %v, want 0", nh.AvgPrevoteTime)
	}
	if nh.StepTimingSample != 0 {
		t.Errorf("StepTimingSample: got %v, want 0 (no precommit observed)", nh.StepTimingSample)
	}
}

// TestStepTimingNoBogusOnFirst verifies that the very first event never records
// a spurious duration (no prior step = no recording).
func TestStepTimingNoBogusOnFirst(t *testing.T) {
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	b := &Builder{
		now:  func() time.Time { return t0 },
		step: newStepAccumulator(),
	}

	b.observeForStepTiming(makeRoundState("RoundStepPropose"))

	nh := &NetworkHealth{}
	enrichStepTiming(nh, b)

	if nh.AvgProposeTime != 0 {
		t.Errorf("expected no duration on first event, got %v", nh.AvgProposeTime)
	}
}

// TestStepTimingIgnoresUnknownEventType ensures non-RoundState events are
// dropped silently.
func TestStepTimingIgnoresUnknownEventType(t *testing.T) {
	b := &Builder{
		now:  time.Now,
		step: newStepAccumulator(),
	}

	// Feed something that is not EventDataRoundState
	b.observeForStepTiming(ctypes.EventDataNewRound{Height: 1, Round: 0, Step: "RoundStepNewRound"})

	nh := &NetworkHealth{}
	enrichStepTiming(nh, b)

	if nh.AvgProposeTime != 0 || nh.AvgPrevoteTime != 0 || nh.AvgPrecommitTime != 0 {
		t.Error("unexpected duration from non-RoundState event")
	}
}

// TestStepTimingPrevoteAndPrecommit exercises prevote + precommit buckets.
func TestStepTimingPrevoteAndPrecommit(t *testing.T) {
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	// Steps: Propose(t0) → Prevote(t0+200ms) → Precommit(t0+500ms) → NewRound(t0+800ms)
	calls := []time.Time{
		t0,
		t0.Add(200 * time.Millisecond),
		t0.Add(500 * time.Millisecond),
		t0.Add(800 * time.Millisecond),
	}
	idx := 0
	b := &Builder{
		now: func() time.Time {
			v := calls[idx]
			if idx < len(calls)-1 {
				idx++
			}
			return v
		},
		step: newStepAccumulator(),
	}

	b.observeForStepTiming(makeRoundState("RoundStepPropose"))   // t0, no record
	b.observeForStepTiming(makeRoundState("RoundStepPrevote"))   // t0+200ms → propose=200ms
	b.observeForStepTiming(makeRoundState("RoundStepPrecommit")) // t0+500ms → prevote=300ms
	b.observeForStepTiming(makeRoundState("RoundStepNewRound"))  // t0+800ms → precommit=300ms

	nh := &NetworkHealth{}
	enrichStepTiming(nh, b)

	if nh.AvgProposeTime != 200*time.Millisecond {
		t.Errorf("AvgProposeTime: got %v, want 200ms", nh.AvgProposeTime)
	}
	if nh.AvgPrevoteTime != 300*time.Millisecond {
		t.Errorf("AvgPrevoteTime: got %v, want 300ms", nh.AvgPrevoteTime)
	}
	if nh.AvgPrecommitTime != 300*time.Millisecond {
		t.Errorf("AvgPrecommitTime: got %v, want 300ms", nh.AvgPrecommitTime)
	}
	if nh.StepTimingSample != 1 {
		t.Errorf("StepTimingSample: got %v, want 1", nh.StepTimingSample)
	}
}

// TestStepTimingAverage verifies multi-block averaging.
func TestStepTimingAverage(t *testing.T) {
	// Simulate two propose durations: 200ms, 400ms → avg 300ms
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	calls := []time.Time{
		t0,
		t0.Add(200 * time.Millisecond), // propose1 = 200ms
		t0.Add(200 * time.Millisecond), // start propose2 (same time as prevote arrival)
		t0.Add(600 * time.Millisecond), // propose2 = 400ms
		t0.Add(600 * time.Millisecond),
	}
	idx := 0
	b := &Builder{
		now: func() time.Time {
			v := calls[idx]
			if idx < len(calls)-1 {
				idx++
			}
			return v
		},
		step: newStepAccumulator(),
	}

	// Round 1
	b.observeForStepTiming(makeRoundState("RoundStepPropose")) // t0
	b.observeForStepTiming(makeRoundState("RoundStepPrevote")) // t0+200ms → propose+=200ms

	// Round 2
	b.observeForStepTiming(makeRoundState("RoundStepPropose")) // t0+200ms
	b.observeForStepTiming(makeRoundState("RoundStepPrevote")) // t0+600ms → propose+=400ms

	nh := &NetworkHealth{}
	enrichStepTiming(nh, b)

	want := 300 * time.Millisecond
	if nh.AvgProposeTime != want {
		t.Errorf("AvgProposeTime avg: got %v, want %v", nh.AvgProposeTime, want)
	}
}
