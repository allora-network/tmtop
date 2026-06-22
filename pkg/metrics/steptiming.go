// pkg/metrics/steptiming.go                 (WS-H)
package metrics

import (
	"time"

	ctypes "github.com/cometbft/cometbft/types"
)

type stepAccumulator struct {
	lastStep  string
	lastAt    time.Time
	propose   durSamples
	prevote   durSamples
	precommit durSamples
}

type durSamples struct {
	total time.Duration
	count int
}

func (d *durSamples) add(v time.Duration) { d.total += v; d.count++ }
func (d durSamples) avg() time.Duration {
	if d.count == 0 {
		return 0
	}
	return d.total / time.Duration(d.count)
}

func newStepAccumulator() stepAccumulator { return stepAccumulator{} }

// observeForStepTiming records wall-clock duration spent in each consensus step.
// It consumes EventDataRoundState events delivered for the NewRoundStep subscription.
// Non-RoundState events are ignored. The very first event sets the baseline without
// recording a duration (no prior step to measure).
func (b *Builder) observeForStepTiming(e ctypes.TMEventData) {
	rs, ok := e.(ctypes.EventDataRoundState)
	if !ok {
		return
	}
	now := b.now()
	if b.step.lastStep != "" && !b.step.lastAt.IsZero() {
		dur := now.Sub(b.step.lastAt)
		switch b.step.lastStep {
		case "RoundStepPropose":
			b.step.propose.add(dur)
		case "RoundStepPrevote", "RoundStepPrevoteWait":
			b.step.prevote.add(dur)
		case "RoundStepPrecommit", "RoundStepPrecommitWait":
			b.step.precommit.add(dur)
		}
	}
	b.step.lastStep = rs.Step
	b.step.lastAt = now
}

// enrichStepTiming sets the per-step average durations and sample count on nh.
// Averages guard divide-by-zero via durSamples.avg().
func enrichStepTiming(nh *NetworkHealth, b *Builder) {
	nh.AvgProposeTime = b.step.propose.avg()
	nh.AvgPrevoteTime = b.step.prevote.avg()
	nh.AvgPrecommitTime = b.step.precommit.avg()
	nh.StepTimingSample = b.step.precommit.count
}
