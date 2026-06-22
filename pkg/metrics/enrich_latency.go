// pkg/metrics/enrich_latency.go             (WS-F)
package metrics

import "context"

// enrichLatency populates arrival-latency columns (HasLatency, AvgPrevoteArrival,
// AvgPrecommitArrival, MaxPrecommitArrival) on each ValidatorHealthRow.
//
// Latency is measured as the elapsed time between a round's start_time and
// the locally-observed timestamp of each vote — single-vantage semantics:
// values reflect both validator timing behaviour and this node's network
// distance from its peers. Rows without DB data retain HasLatency=false.
func enrichLatency(ctx context.Context, rows []ValidatorHealthRow, b *Builder) {
	if b.analytics == nil {
		return
	}
	maxH, err := maxStoredHeight(ctx, b)
	if err != nil || maxH == 0 {
		return
	}
	minH := maxH - b.windowBlocks
	if minH < 1 {
		minH = 1
	}
	stats, err := b.analytics.GetVoteArrivalByHeight(ctx, minH, maxH)
	if err != nil {
		b.logger.Error().Err(err).Msg("vote-arrival query failed")
		return
	}
	byAddr := make(map[string]int, len(rows))
	for i := range rows {
		byAddr[rows[i].Address] = i
	}
	for addr, s := range stats {
		i, ok := byAddr[addr]
		if !ok {
			continue
		}
		rows[i].HasLatency = true
		rows[i].AvgPrevoteArrival = s.AvgPrevote
		rows[i].AvgPrecommitArrival = s.AvgPrecommit
		rows[i].MaxPrecommitArrival = s.MaxPrecommit
	}
}
