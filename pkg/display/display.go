package display

import (
	"context"

	"main/pkg/types"
)

// Display is the TUI boundary. Implementations block on Start until
// the user quits, then Stop cleans up. SetState and DebugText are
// called from any goroutine; implementations must marshal onto the
// UI thread internally.
type Display interface {
	Start() error
	Stop(ctx context.Context) error
	SetState(state *types.State)
	DebugText(line string)
}

var _ Display = (*Wrapper)(nil)
