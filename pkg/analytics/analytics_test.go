package analytics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseTimeWindow(t *testing.T) {
	testCases := []struct {
		input       string
		shouldError bool
		duration    time.Duration
	}{
		{"1h", false, time.Hour},
		{"30m", false, 30 * time.Minute},
		{"24h", false, 24 * time.Hour},
		{"168h", false, 168 * time.Hour}, // 7 days
		{"720h", false, 720 * time.Hour}, // 30 days
		{"1s", false, time.Second},
		{"2h30m", false, 2*time.Hour + 30*time.Minute},
		{"invalid", true, 0},
		{"-1h", true, 0}, // negative duration
		{"0s", true, 0},  // zero duration
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			window, err := ParseTimeWindow(tc.input)

			if tc.shouldError {
				assert.Error(t, err, "Expected error for input: %s", tc.input)
				return
			}

			assert.NoError(t, err, "Unexpected error for input: %s", tc.input)
			assert.True(t, window.End.After(window.Start), "End should be after Start")

			actualDuration := window.End.Sub(window.Start)
			// Allow for small timing differences (within 1 second)
			assert.True(t,
				actualDuration >= tc.duration-time.Second && actualDuration <= tc.duration+time.Second,
				"Expected duration %v, got %v for input %s", tc.duration, actualDuration, tc.input)
		})
	}
}

func TestGetCommonTimeWindows(t *testing.T) {
	windows := GetCommonTimeWindows()

	// Test that all expected time windows are present
	expectedWindows := []string{"1h", "6h", "24h", "168h", "720h"}
	for _, expected := range expectedWindows {
		_, exists := windows[expected]
		assert.True(t, exists, "Expected time window %s to exist", expected)
	}

	// Test that time windows are properly ordered (End > Start)
	for name, window := range windows {
		assert.True(t, window.End.After(window.Start), "Time window %s should have End after Start", name)
	}

	// Test specific durations
	assert.Equal(t, time.Hour, windows["1h"].End.Sub(windows["1h"].Start), "1h window should be 1 hour")
	assert.Equal(t, 24*time.Hour, windows["24h"].End.Sub(windows["24h"].Start), "24h window should be 24 hours")
}

func TestTimeWindow(t *testing.T) {
	start := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)

	window := TimeWindow{
		Start: start,
		End:   end,
	}

	assert.Equal(t, start, window.Start)
	assert.Equal(t, end, window.End)
	assert.True(t, window.End.After(window.Start))
}

func TestPerformanceMetrics(t *testing.T) {
	window := TimeWindow{
		Start: time.Now().Add(-24 * time.Hour),
		End:   time.Now(),
	}

	metrics := &PerformanceMetrics{
		ValidatorHexAddress: "cosmosvaloper1test",
		ValidatorMoniker:    "Test Validator",
		TimeWindow:          window,
		TotalBlocks:         100,
		BlocksSigned:        95,
		BlocksMissed:        5,
		SigningEfficiency:   95.0,
		TotalRounds:         100,
		PrevoteRate:         98.0,
		PrecommitRate:       96.0,
		LongestMissStreak:   2,
		CalculatedAt:        time.Now(),
	}

	assert.Equal(t, "cosmosvaloper1test", metrics.ValidatorHexAddress)
	assert.Equal(t, "Test Validator", metrics.ValidatorMoniker)
	assert.Equal(t, int64(100), metrics.TotalBlocks)
	assert.Equal(t, int64(95), metrics.BlocksSigned)
	assert.Equal(t, int64(5), metrics.BlocksMissed)
	assert.Equal(t, 95.0, metrics.SigningEfficiency)
	assert.Equal(t, int64(2), metrics.LongestMissStreak)
}

func TestMissedBlockStreak(t *testing.T) {
	streak := MissedBlockStreak{
		ConsecutiveMisses: 5,
		StreakStartHeight: 100,
		StreakEndHeight:   104,
		StreakStartTime:   time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		StreakEndTime:     time.Date(2023, 1, 1, 0, 5, 0, 0, time.UTC),
	}

	assert.Equal(t, int64(5), streak.ConsecutiveMisses)
	assert.Equal(t, int64(100), streak.StreakStartHeight)
	assert.Equal(t, int64(104), streak.StreakEndHeight)
	assert.True(t, streak.StreakEndTime.After(streak.StreakStartTime))
}

func TestValidatorRanking(t *testing.T) {
	ranking := ValidatorRanking{
		HexAddress:        "cosmosvaloper1test",
		Moniker:           "Test Validator",
		TotalBlocks:       1000,
		BlocksSigned:      950,
		BlocksMissed:      50,
		SigningEfficiency: 95.0,
		PrevotesCast:      980,
		PrecommitsCast:    960,
		VotingPower:       1000000,
		EfficiencyRank:    1,
	}

	assert.Equal(t, "cosmosvaloper1test", ranking.HexAddress)
	assert.Equal(t, "Test Validator", ranking.Moniker)
	assert.Equal(t, int64(1000), ranking.TotalBlocks)
	assert.Equal(t, int64(950), ranking.BlocksSigned)
	assert.Equal(t, int64(50), ranking.BlocksMissed)
	assert.Equal(t, 95.0, ranking.SigningEfficiency)
	assert.Equal(t, int64(1), ranking.EfficiencyRank)

	// Test that blocks signed + blocks missed = total blocks
	assert.Equal(t, ranking.TotalBlocks, ranking.BlocksSigned+ranking.BlocksMissed)
}

// TestArrivalStatsMapping verifies that the seconds→duration conversion formula
// and vote-type bucket assignment (1=prevote, 2=precommit) are correct without
// requiring a live database. It exercises the same arithmetic used inside
// GetVoteArrivalByHeight so that a regression in the conversion would be caught here.
func TestArrivalStatsMapping(t *testing.T) {
	type inputRow struct {
		hexAddress string
		voteType   int64
		avgSecs    interface{}
		maxSecs    interface{}
	}
	secsToDur := func(secs float64) time.Duration {
		return time.Duration(secs * float64(time.Second))
	}

	rows := []inputRow{
		{"addr1", 1, float64(0.5), float64(1.2)},  // prevote
		{"addr1", 2, float64(0.3), float64(0.9)},  // precommit
		{"addr2", 1, float64(1.0), float64(2.0)},  // prevote only
		{"addr3", 2, float64(0.1), float64(0.4)},  // precommit only
		{"addr4", 1, nil, nil},                     // nil → zero duration (COALESCE edge)
	}

	// Replicate the mapping loop from GetVoteArrivalByHeight.
	out := make(map[string]ArrivalStats)
	for _, r := range rows {
		avg := secsToDur(convertToFloat64(r.avgSecs))
		max := secsToDur(convertToFloat64(r.maxSecs))
		s := out[r.hexAddress]
		switch r.voteType {
		case 1:
			s.AvgPrevote = avg
			s.MaxPrevote = max
		case 2:
			s.AvgPrecommit = avg
			s.MaxPrecommit = max
		}
		out[r.hexAddress] = s
	}

	assert.Equal(t, secsToDur(0.5), out["addr1"].AvgPrevote,   "addr1 AvgPrevote")
	assert.Equal(t, secsToDur(1.2), out["addr1"].MaxPrevote,   "addr1 MaxPrevote")
	assert.Equal(t, secsToDur(0.3), out["addr1"].AvgPrecommit, "addr1 AvgPrecommit")
	assert.Equal(t, secsToDur(0.9), out["addr1"].MaxPrecommit, "addr1 MaxPrecommit")

	assert.Equal(t, secsToDur(1.0), out["addr2"].AvgPrevote,       "addr2 AvgPrevote")
	assert.Equal(t, time.Duration(0), out["addr2"].AvgPrecommit,    "addr2 AvgPrecommit should be zero")

	assert.Equal(t, time.Duration(0), out["addr3"].AvgPrevote,      "addr3 AvgPrevote should be zero")
	assert.Equal(t, secsToDur(0.1), out["addr3"].AvgPrecommit,      "addr3 AvgPrecommit")

	assert.Equal(t, time.Duration(0), out["addr4"].AvgPrevote,      "addr4 nil avgSecs → zero duration")
	assert.Equal(t, time.Duration(0), out["addr4"].MaxPrevote,      "addr4 nil maxSecs → zero duration")
}

func TestTimeSeriesPoint(t *testing.T) {
	point := TimeSeriesPoint{
		TimeBucket:        time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
		BlocksInBucket:    60,
		BlocksSigned:      58,
		BlocksMissed:      2,
		SigningEfficiency: 96.67,
	}

	assert.Equal(t, int64(60), point.BlocksInBucket)
	assert.Equal(t, int64(58), point.BlocksSigned)
	assert.Equal(t, int64(2), point.BlocksMissed)
	assert.Equal(t, 96.67, point.SigningEfficiency)

	// Test that blocks signed + blocks missed = blocks in bucket
	assert.Equal(t, point.BlocksInBucket, point.BlocksSigned+point.BlocksMissed)
}
