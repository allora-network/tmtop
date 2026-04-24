package refresher

import (
	"time"

	"github.com/rs/zerolog"
)

// Refresher runs a function on an interval until its stop channel
// closes. The function is invoked once immediately and then on each
// tick — matching the "fetch now then poll" pattern every refresh
// loop in the app used to hand-roll.
type Refresher struct {
	name     string
	interval time.Duration
	refresh  func()
	chStop   <-chan struct{}
	logger   zerolog.Logger
}

// New builds a Refresher. chStop is the parent's stop channel —
// Refresher doesn't own it. The caller is responsible for panic
// recovery around refresh if that matters.
func New(
	name string,
	interval time.Duration,
	refresh func(),
	chStop <-chan struct{},
	logger zerolog.Logger,
) *Refresher {
	return &Refresher{
		name:     name,
		interval: interval,
		refresh:  refresh,
		chStop:   chStop,
		logger:   logger.With().Str("component", "refresher").Str("name", name).Logger(),
	}
}

// Run blocks until chStop closes. Call it inside a tracked goroutine.
func (r *Refresher) Run() {
	r.refresh()

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-r.chStop:
			return
		case <-ticker.C:
			r.refresh()
		}
	}
}
