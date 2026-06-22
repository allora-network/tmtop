package metrics

import (
	"context"
	"testing"
)

// TestEnrichPerformanceNilAnalytics verifies the nil-analytics early-return path:
// rows must keep HasHistory=false when the DB is disabled.
func TestEnrichPerformanceNilAnalytics(t *testing.T) {
	rows := []ValidatorHealthRow{
		{Address: "AABBCC", Moniker: "val-1", VotingPowerPct: 5.0},
		{Address: "DDEEFF", Moniker: "val-2", VotingPowerPct: 3.0},
	}
	b := &Builder{analytics: nil, windowBlocks: 100}
	enrichPerformance(context.Background(), rows, b)
	for _, r := range rows {
		if r.HasHistory {
			t.Errorf("row %s: HasHistory should be false when analytics is nil", r.Address)
		}
		if r.SigningRatePct != 0 || r.BlocksMissed != 0 {
			t.Errorf("row %s: performance fields should be zero when analytics is nil", r.Address)
		}
	}
}
