package display

import (
	"strings"
	"testing"
	"time"

	"main/pkg/metrics"
	"main/pkg/types"

	"github.com/rs/zerolog"
)

func newTestState() *types.State {
	return types.NewState("http://localhost:26657", zerolog.Nop())
}

// TestSerializeNetworkHealth_NilSnapshot verifies that a nil NetworkHealth
// value (state not yet computed) returns the "computing…" placeholder.
func TestSerializeNetworkHealth_NilSnapshot(t *testing.T) {
	s := newTestState()
	// NetworkHealth not set → GetNetworkHealth() returns nil.
	out := SerializeNetworkHealth(s, false)
	if !strings.Contains(out, "computing") {
		t.Errorf("expected 'computing' in nil-snapshot output, got: %q", out)
	}
}

// TestSerializeNetworkHealth_PopulatedSnapshot verifies section headers
// appear and that zero fields render as "—".
func TestSerializeNetworkHealth_PopulatedSnapshot(t *testing.T) {
	s := newTestState()

	nh := &metrics.NetworkHealth{
		// Leave most fields zero to exercise the dash helper.
		// Fill a few to verify non-zero paths.
		ChainHalted:        false,
		SecondsSinceHeight: 0,
		CurrentMaxRound:    0, // zero → dash
		OfflineCount:       0, // zero → dash
		RoundsPerBlockAvg:  0.04,
		RoundZeroCommitPct: 96.1,
		AvgBlockTime:       time.Duration(6.2 * float64(time.Second)),
		BlockTimeStdDev:    time.Duration(0.8 * float64(time.Second)),
		BlockIntervals: []time.Duration{
			1 * time.Second,
			2 * time.Second,
			5 * time.Second,
			3 * time.Second,
		},
		AvgProposeTime:   time.Duration(1.1 * float64(time.Second)),
		AvgPrevoteTime:   time.Duration(0.4 * float64(time.Second)),
		AvgPrecommitTime: time.Duration(0.5 * float64(time.Second)),
		MempoolKnown:     true,
		MempoolTxs:       142,
		MempoolBytes:     90112, // ~88 KB
		Nakamoto33:       4,
		Nakamoto66:       11,
		Gini:             0.71,
		Top10Pct:         62.3,
		TopASNs: []metrics.ASNShare{
			{ASN: 24940, Description: "Hetzner", PowerPct: 18.1},
			{ASN: 16509, Description: "AWS", PowerPct: 12.0},
		},
	}
	s.SetNetworkHealth(nh)

	out := SerializeNetworkHealth(s, false)

	wantSections := []string{"LIVENESS", "CONSENSUS QUALITY", "MEMPOOL", "DECENTRALIZATION", "SECURITY"}
	for _, section := range wantSections {
		if !strings.Contains(out, section) {
			t.Errorf("missing section %q in output:\n%s", section, out)
		}
	}

	// Zero field (CurrentMaxRound == 0) should render as dash.
	if !strings.Contains(out, "—") {
		t.Errorf("expected em-dash for zero CurrentMaxRound, output:\n%s", out)
	}

	// Sparkline should be present (non-empty intervals).
	if !strings.Contains(out, "[") {
		t.Errorf("expected sparkline brackets in output:\n%s", out)
	}

	// Non-zero fields should appear.
	if !strings.Contains(out, "96.1%") {
		t.Errorf("expected round-0 commit pct '96.1%%' in output:\n%s", out)
	}
	if !strings.Contains(out, "142") {
		t.Errorf("expected mempool tx count 142 in output:\n%s", out)
	}
	if !strings.Contains(out, "Hetzner") {
		t.Errorf("expected ASN description 'Hetzner' in output:\n%s", out)
	}
	if !strings.Contains(out, "no equivocations") {
		t.Errorf("expected 'no equivocations' when list is empty:\n%s", out)
	}
}

// TestSerializeNetworkHealth_DisableEmojis verifies ASCII sparkline
// and ASCII warning prefix when disableEmojis is true.
func TestSerializeNetworkHealth_DisableEmojis(t *testing.T) {
	s := newTestState()

	nh := &metrics.NetworkHealth{
		ChainHalted:        true,
		SecondsSinceHeight: 12.4,
		BlockIntervals:     []time.Duration{1 * time.Second, 3 * time.Second, 2 * time.Second},
		Equivocations: []metrics.EquivocationEvent{
			{ValidatorAddress: "abcdef1234567890", Height: 812345, Round: 0, VoteType: "precommit"},
		},
	}
	s.SetNetworkHealth(nh)

	out := SerializeNetworkHealth(s, true /* disableEmojis */)

	// ASCII sparkline characters should appear (no Unicode bars).
	asciiChars := ".:-=+*#@"
	found := false
	for _, c := range asciiChars {
		if strings.ContainsRune(out, c) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected ASCII sparkline chars (%q) when DisableEmojis=true, output:\n%s", asciiChars, out)
	}

	// ASCII "!" prefix for equivocations (not "⚠").
	if !strings.Contains(out, "! equivocation") {
		t.Errorf("expected ASCII '!' equivocation prefix when DisableEmojis=true, output:\n%s", out)
	}

	// Stalled message should use ASCII prefix too.
	if !strings.Contains(out, "STALLED") {
		t.Errorf("expected STALLED in halted output:\n%s", out)
	}
}

// TestSparkline_Unicode verifies the sparkline helper produces Unicode bars.
func TestSparkline_Unicode(t *testing.T) {
	intervals := []time.Duration{1 * time.Second, 2 * time.Second, 3 * time.Second}
	out := sparkline(intervals, false)
	if out == "" {
		t.Error("expected non-empty sparkline")
	}
	// Should start with the lowest bar (▁) and end with the highest (█).
	runes := []rune(out)
	if runes[0] != '▁' {
		t.Errorf("first bar should be ▁, got %q", string(runes[0]))
	}
	if runes[len(runes)-1] != '█' {
		t.Errorf("last bar should be █, got %q", string(runes[len(runes)-1]))
	}
}

// TestSparkline_ASCII verifies the sparkline helper falls back to ASCII.
func TestSparkline_ASCII(t *testing.T) {
	intervals := []time.Duration{1 * time.Second, 3 * time.Second}
	out := sparkline(intervals, true)
	if out == "" {
		t.Error("expected non-empty sparkline")
	}
	// With only two distinct values the first rune should be '.' (lowest).
	runes := []rune(out)
	if runes[0] != '.' {
		t.Errorf("first ASCII bar should be '.', got %q", string(runes[0]))
	}
}

// TestDash verifies that the dash helper returns "—" for zero values and
// the formatted value otherwise.
func TestDash(t *testing.T) {
	if got := dash(0); got != "—" {
		t.Errorf("dash(0) = %q, want —", got)
	}
	if got := dash(int32(0)); got != "—" {
		t.Errorf("dash(int32(0)) = %q, want —", got)
	}
	if got := dash(float64(0)); got != "—" {
		t.Errorf("dash(float64(0)) = %q, want —", got)
	}
	if got := dash(time.Duration(0)); got != "—" {
		t.Errorf("dash(Duration(0)) = %q, want —", got)
	}
	if got := dash(5); got == "—" {
		t.Errorf("dash(5) should not return — ")
	}
}
