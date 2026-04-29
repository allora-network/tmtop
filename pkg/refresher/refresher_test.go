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

	// Wait until at least one tick has fired beyond the initial call.
	// time.Sleep with a fixed duration is timing-flaky on busy CI.
	require.Eventually(t, func() bool { return calls.Load() > 1 },
		time.Second, 10*time.Millisecond,
		"refresh should fire on ticks")
	close(chStop)
	<-done
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
