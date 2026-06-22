// pkg/metrics/enrich_blocktime.go           (WS-C)
package metrics

import (
	"math"
	"time"
)

func enrichBlockTime(nh *NetworkHealth, b *Builder) {
	iv := b.blockIntervals
	if len(iv) > 0 {
		var sum time.Duration
		for _, d := range iv {
			sum += d
		}
		mean := sum / time.Duration(len(iv))
		nh.AvgBlockTime = mean

		var varAcc float64
		for _, d := range iv {
			diff := float64(d - mean)
			varAcc += diff * diff
		}
		nh.BlockTimeStdDev = time.Duration(math.Sqrt(varAcc / float64(len(iv))))
		nh.BlockIntervals = append([]time.Duration(nil), iv...)
	}

	mr := b.maxRoundPerHeight
	if len(mr) > 0 {
		var roundSum, zero int
		for _, r := range mr {
			roundSum += int(r)
			if r == 0 {
				zero++
			}
		}
		nh.RoundsPerBlockAvg = float64(roundSum) / float64(len(mr))
		nh.RoundZeroCommitPct = 100.0 * float64(zero) / float64(len(mr))
	}
}
