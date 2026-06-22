// pkg/metrics/equivocation.go               (WS-G)
package metrics

import ctypes "github.com/cometbft/cometbft/types"

type equivState struct{}

func newEquivState() equivState { return equivState{} }

func (b *Builder) observeForEquivocation(e ctypes.TMEventData)    {}
func enrichEquivocations(nh *NetworkHealth, b *Builder)            {}
func enrichEquivocationFlags(rows []ValidatorHealthRow, b *Builder) {}
