// pkg/metrics/enrich_performance.go         (WS-E)
package metrics

import "context"

// maxStoredHeight returns the highest block height stored in the database,
// or 0 if none (or the DB is unavailable).
func maxStoredHeight(ctx context.Context, b *Builder) (int64, error) {
	return b.analytics.GetMaxStoredHeight(ctx)
}

func enrichPerformance(ctx context.Context, rows []ValidatorHealthRow, b *Builder) {
	if b.analytics == nil {
		return // DB disabled; rows keep HasHistory=false → render "—"
	}
	maxH, err := maxStoredHeight(ctx, b)
	if err != nil || maxH == 0 {
		return
	}
	minH := maxH - b.windowBlocks
	if minH < 1 {
		minH = 1
	}

	ranking, err := b.analytics.GetRankingByHeight(ctx, minH, maxH)
	if err != nil {
		b.logger.Error().Err(err).Msg("ranking-by-height failed")
		return
	}
	share, _ := b.analytics.GetProposerShareByHeight(ctx, minH, maxH)

	byAddr := make(map[string]int, len(rows))
	for i := range rows {
		byAddr[rows[i].Address] = i
	}
	for _, r := range ranking {
		i, ok := byAddr[r.HexAddress]
		if !ok {
			continue
		}
		rows[i].HasHistory = true
		rows[i].SigningRatePct = r.SigningEfficiency
		rows[i].BlocksMissed = r.BlocksMissed
		if r.TotalBlocks > 0 {
			rows[i].PrevoteRatePct = 100.0 * float64(r.PrevotesCast) / float64(r.TotalBlocks)
			rows[i].PrecommitRatePct = 100.0 * float64(r.PrecommitsCast) / float64(r.TotalBlocks)
		}
		rows[i].ProposerSharePct = share[r.HexAddress]
	}
}
