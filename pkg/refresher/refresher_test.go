package refresher

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// TestRunInvokesRefreshImmediately verifies the "fetch now then poll"
// shape — refresh is called once before the first tick.
func TestRunInvokesRefreshImmediately(t *testing.T) {
	var calls atomic.Int64
	called := make(chan struct{}, 1)
	chStop := make(chan struct{})

	r := New("test", time.Hour, func() {
		calls.Add(1)
		select {
		case called <- struct{}{}:
		default:
		}
	}, chStop, zerolog.Nop())

	done := make(chan struct{})
	go func() {
		r.Run()
		close(done)
	}()

	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("refresh was not called immediately")
	}

	close(chStop)
	<-done

	require.EqualValues(t, 1, calls.Load(), "refresh should fire once when no ticks elapse")
}

// TestRunTicks verifies refresh is called on each tick after the
// initial invocation.
func TestRunTicks(t *testing.T) {
	var calls atomic.Int64
	chStop := make(chan struct{})

	r := New("test", 50*time.Millisecond, func() {
		calls.Add(1)
	}, chStop, zerolog.Nop())

	done := make(chan struct{})
	go func() {
		r.Run()
		close(done)
	}()

	// 250ms should give us the initial call plus 4-5 ticks. Allow
	// jitter and just assert "more than one call".
	time.Sleep(250 * time.Millisecond)
	close(chStop)
	<-done

	require.Greater(t, calls.Load(), int64(1), "refresh should fire on ticks, got %d calls", calls.Load())
}

// TestRunExitsOnStop verifies Run returns promptly when chStop closes
// — even mid-tick.
func TestRunExitsOnStop(t *testing.T) {
	chStop := make(chan struct{})

	r := New("test", time.Hour, func() {}, chStop, zerolog.Nop())

	done := make(chan struct{})
	go func() {
		r.Run()
		close(done)
	}()

	close(chStop)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not exit within 1s of chStop closing")
	}
}
