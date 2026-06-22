// pkg/metrics/steptiming.go                 (WS-H)
package metrics

import ctypes "github.com/cometbft/cometbft/types"

type stepAccumulator struct{}

func newStepAccumulator() stepAccumulator { return stepAccumulator{} }

func (b *Builder) observeForStepTiming(e ctypes.TMEventData) {}
func enrichStepTiming(nh *NetworkHealth, b *Builder)         {}
