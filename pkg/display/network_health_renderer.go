package display

import (
	"fmt"
	"strings"
	"time"

	"main/pkg/metrics"
	"main/pkg/types"
)

// SerializeNetworkHealth renders the full-width Network Health panel.
// It mirrors the strings.Builder style of SerializeConsensus / SerializeChainInfo.
func SerializeNetworkHealth(s *types.State, disableEmojis bool) string {
	raw := s.GetNetworkHealth()
	if raw == nil {
		return " network health: computing…\n"
	}
	nh, ok := raw.(*metrics.NetworkHealth)
	if !ok || nh == nil {
		return " network health: computing…\n"
	}

	var sb strings.Builder

	// ── LIVENESS ────────────────────────────────────────────────────────────
	fmt.Fprintf(&sb, " LIVENESS\n")

	heightStatus := "yes"
	if nh.ChainHalted {
		if disableEmojis {
			heightStatus = fmt.Sprintf("STALLED (%.1fs since last height)", nh.SecondsSinceHeight)
		} else {
			heightStatus = fmt.Sprintf("⚠ STALLED (%.1fs since last height)", nh.SecondsSinceHeight)
		}
	}
	fmt.Fprintf(&sb, "   height advancing: %-30s", heightStatus)

	offlineStr := dash(nh.OfflineCount)
	offlinePctStr := dash(nh.OfflinePowerPct)
	if nh.OfflineCount > 0 {
		offlineStr = fmt.Sprintf("%d vals / %.1f%% power", nh.OfflineCount, nh.OfflinePowerPct)
		offlinePctStr = ""
	}
	if offlinePctStr == "" {
		fmt.Fprintf(&sb, "offline: %s\n", offlineStr)
	} else {
		fmt.Fprintf(&sb, "offline: %s\n", offlinePctStr)
	}

	fmt.Fprintf(&sb, "   current round: %-33s\n", dash(nh.CurrentMaxRound))

	// ── CONSENSUS QUALITY ───────────────────────────────────────────────────
	fmt.Fprintf(&sb, " CONSENSUS QUALITY\n")

	rpb := dash(nh.RoundsPerBlockAvg)
	if nh.RoundsPerBlockAvg != 0 {
		rpb = fmt.Sprintf("%.2f", nh.RoundsPerBlockAvg)
	}
	r0c := dash(nh.RoundZeroCommitPct)
	if nh.RoundZeroCommitPct != 0 {
		r0c = fmt.Sprintf("%.1f%%", nh.RoundZeroCommitPct)
	}
	fmt.Fprintf(&sb, "   rounds/block (avg): %-22s round-0 commits: %s\n", rpb, r0c)

	avgBT := dash(nh.AvgBlockTime)
	stdBT := dash(nh.BlockTimeStdDev)
	if nh.AvgBlockTime != 0 {
		avgBT = fmtDuration(nh.AvgBlockTime)
	}
	if nh.BlockTimeStdDev != 0 {
		stdBT = fmtDuration(nh.BlockTimeStdDev)
	}
	spark := ""
	if len(nh.BlockIntervals) > 0 {
		spark = " [" + sparkline(nh.BlockIntervals, disableEmojis) + "]"
	}
	fmt.Fprintf(&sb, "   block time: %s ± %s%s\n", avgBT, stdBT, spark)

	propT := dash(nh.AvgProposeTime)
	prevT := dash(nh.AvgPrevoteTime)
	preT := dash(nh.AvgPrecommitTime)
	if nh.AvgProposeTime != 0 {
		propT = fmtDuration(nh.AvgProposeTime)
	}
	if nh.AvgPrevoteTime != 0 {
		prevT = fmtDuration(nh.AvgPrevoteTime)
	}
	if nh.AvgPrecommitTime != 0 {
		preT = fmtDuration(nh.AvgPrecommitTime)
	}
	fmt.Fprintf(&sb, "   step timing (avg): propose %s · prevote %s · precommit %s\n", propT, prevT, preT)

	// ── MEMPOOL ─────────────────────────────────────────────────────────────
	fmt.Fprintf(&sb, " MEMPOOL\n")
	if !nh.MempoolKnown {
		fmt.Fprintf(&sb, "   unconfirmed txs: —\n")
	} else {
		kb := float64(nh.MempoolBytes) / 1024.0
		fmt.Fprintf(&sb, "   unconfirmed txs: %d (%.0f KB)\n", nh.MempoolTxs, kb)
	}

	// ── DECENTRALIZATION ────────────────────────────────────────────────────
	fmt.Fprintf(&sb, " DECENTRALIZATION\n")

	n33 := dash(nh.Nakamoto33)
	n66 := dash(nh.Nakamoto66)
	if nh.Nakamoto33 != 0 {
		n33 = fmt.Sprintf("%d", nh.Nakamoto33)
	}
	if nh.Nakamoto66 != 0 {
		n66 = fmt.Sprintf("%d", nh.Nakamoto66)
	}
	fmt.Fprintf(&sb, "   nakamoto: %s to halt (>⅓) · %s to control (>⅔)\n", n33, n66)

	gini := dash(nh.Gini)
	top10 := dash(nh.Top10Pct)
	if nh.Gini != 0 {
		gini = fmt.Sprintf("%.2f", nh.Gini)
	}
	if nh.Top10Pct != 0 {
		top10 = fmt.Sprintf("%.1f%%", nh.Top10Pct)
	}
	fmt.Fprintf(&sb, "   gini: %-12s top-10 power: %s\n", gini, top10)

	if len(nh.TopASNs) > 0 {
		sb.WriteString("   top hosting: ")
		for i, asn := range nh.TopASNs {
			if i > 0 {
				sb.WriteString(" · ")
			}
			if i >= 3 {
				sb.WriteString("…")
				break
			}
			fmt.Fprintf(&sb, "AS%d %s %.1f%%", asn.ASN, asn.Description, asn.PowerPct)
		}
		sb.WriteString("\n")
	}

	// ── SECURITY ────────────────────────────────────────────────────────────
	fmt.Fprintf(&sb, " SECURITY\n")
	if len(nh.Equivocations) == 0 {
		fmt.Fprintf(&sb, "   no equivocations detected (same-round only)\n")
	} else {
		for _, ev := range nh.Equivocations {
			addr := ev.ValidatorAddress
			if len(addr) > 8 {
				addr = addr[:8] + "…"
			}
			warn := "⚠"
			if disableEmojis {
				warn = "!"
			}
			fmt.Fprintf(&sb, "   %s equivocation detected (val %s h=%d r=%d %s)\n",
				warn, addr, ev.Height, ev.Round, ev.VoteType)
		}
	}

	return sb.String()
}

// dash returns "—" when the given value is zero (its type's zero value).
// Supported: int, int32, int64, float64, time.Duration.
func dash[T int | int32 | int64 | float64 | time.Duration](v T) string {
	if v == 0 {
		return "—"
	}
	return fmt.Sprintf("%v", v)
}

// fmtDuration formats a duration as seconds with one decimal place (e.g. "6.2s").
func fmtDuration(d time.Duration) string {
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// sparkline renders a slice of durations as a Unicode block-bar string.
// When disableEmojis is true, ASCII characters are used instead.
func sparkline(intervals []time.Duration, disableEmojis bool) string {
	if len(intervals) == 0 {
		return ""
	}

	// Find min/max to normalise each bar.
	minV := intervals[0]
	maxV := intervals[0]
	for _, d := range intervals[1:] {
		if d < minV {
			minV = d
		}
		if d > maxV {
			maxV = d
		}
	}

	var bars string
	if disableEmojis {
		bars = ".:-=+*#@"
	} else {
		bars = "▁▂▃▄▅▆▇█"
	}
	nBars := len([]rune(bars)) // 8 levels

	var sb strings.Builder
	runes := []rune(bars)
	for _, d := range intervals {
		idx := 0
		if maxV > minV {
			ratio := float64(d-minV) / float64(maxV-minV)
			idx = int(ratio * float64(nBars-1))
			if idx >= nBars {
				idx = nBars - 1
			}
		}
		sb.WriteRune(runes[idx])
	}
	return sb.String()
}
