package metrics

import (
	"testing"
	"time"
)

func TestBlockTimeStats(t *testing.T) {
	nh := &NetworkHealth{}
	b := &Builder{
		blockIntervals:    []time.Duration{2 * time.Second, 4 * time.Second, 6 * time.Second},
		maxRoundPerHeight: []int32{0, 0, 1, 0},
	}
	enrichBlockTime(nh, b)
	if nh.AvgBlockTime != 4*time.Second {
		t.Fatalf("avg = %v want 4s", nh.AvgBlockTime)
	}
	if nh.RoundZeroCommitPct != 75.0 { // 3 of 4 heights at round 0
		t.Fatalf("round0 = %v want 75", nh.RoundZeroCommitPct)
	}
}

func TestBlockTimeStats_Empty(t *testing.T) {
	nh := &NetworkHealth{}
	b := &Builder{}
	enrichBlockTime(nh, b) // must not panic / divide-by-zero
	if nh.AvgBlockTime != 0 {
		t.Fatalf("expected zero AvgBlockTime on empty builder, got %v", nh.AvgBlockTime)
	}
	if nh.RoundZeroCommitPct != 0 {
		t.Fatalf("expected zero RoundZeroCommitPct on empty builder, got %v", nh.RoundZeroCommitPct)
	}
}

func TestBlockTimeStats_StdDev(t *testing.T) {
	// intervals 2s, 4s, 6s → mean 4s
	// deviations: -2s, 0s, 2s → variance = (4+0+4)/3 = 8/3 s² → stddev ≈ 1.633s
	nh := &NetworkHealth{}
	b := &Builder{
		blockIntervals: []time.Duration{2 * time.Second, 4 * time.Second, 6 * time.Second},
	}
	enrichBlockTime(nh, b)
	// stddev should be roughly 1.633s; allow ±100ms tolerance for integer truncation
	want := time.Duration(1633333333) // ~1.633s in nanoseconds
	diff := nh.BlockTimeStdDev - want
	if diff < 0 {
		diff = -diff
	}
	if diff > 100*time.Millisecond {
		t.Fatalf("stddev = %v want ~1.633s", nh.BlockTimeStdDev)
	}
}

func TestBlockTimeStats_BlockIntervalsCloned(t *testing.T) {
	orig := []time.Duration{1 * time.Second, 2 * time.Second}
	b := &Builder{blockIntervals: orig}
	nh := &NetworkHealth{}
	enrichBlockTime(nh, b)
	// mutating nh.BlockIntervals must not affect builder's ring
	nh.BlockIntervals[0] = 99 * time.Second
	if b.blockIntervals[0] != 1*time.Second {
		t.Fatal("enrichBlockTime did not deep-copy BlockIntervals")
	}
}
