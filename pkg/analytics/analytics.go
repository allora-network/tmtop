package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"main/pkg/db"
	"main/pkg/db/sqlc"

	"github.com/rs/zerolog"
)

// Helper functions to safely convert interface{} types from SQLite.
func convertToInt64(val interface{}) int64 {
	if val == nil {
		return 0
	}

	switch v := val.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case string:
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			return i
		}
	}
	return 0
}

func convertToFloat64(val interface{}) float64 {
	if val == nil {
		return 0.0
	}

	switch v := val.(type) {
	case float64:
		return v
	case int64:
		return float64(v)
	case int:
		return float64(v)
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return 0.0
}

func convertToTime(val interface{}) time.Time {
	if val == nil {
		return time.Time{}
	}

	if timeStr, ok := val.(string); ok {
		if t, err := time.Parse("2006-01-02 15:04:05", timeStr); err == nil {
			return t
		}
	}
	return time.Time{}
}

// ValidatorAnalytics provides performance analysis for validators.
type ValidatorAnalytics struct {
	db     *db.DB
	logger zerolog.Logger
}

// TimeWindow represents a time range for analysis.
type TimeWindow struct {
	Start time.Time
	End   time.Time
}

// PerformanceMetrics contains comprehensive validator performance data.
type PerformanceMetrics struct {
	ValidatorHexAddress string
	ValidatorMoniker    string
	TimeWindow          TimeWindow

	// Block signing metrics
	TotalBlocks       int64
	BlocksSigned      int64
	BlocksMissed      int64
	SigningEfficiency float64 // Percentage

	// Consensus participation
	TotalRounds   int64
	PrevoteRate   float64 // Percentage
	PrecommitRate float64 // Percentage
	VotingPower   int64

	// Miss analysis
	LongestMissStreak int64

	// Time-based
	CalculatedAt time.Time
}

// ConsensusMetrics contains detailed consensus participation data.
type ConsensusMetrics struct {
	ValidatorHexAddress string
	TotalRounds         int64
	RoundsWithPrevote   int64
	RoundsWithPrecommit int64
	PrevoteRate         float64
	PrecommitRate       float64
}

// MissedBlockStreak represents a sequence of consecutive missed blocks.
type MissedBlockStreak struct {
	ConsecutiveMisses int64
	StreakStartHeight int64
	StreakEndHeight   int64
	StreakStartTime   time.Time
	StreakEndTime     time.Time
}

// UptimeMetrics contains uptime analysis for a validator.
type UptimeMetrics struct {
	ValidatorHexAddress string
	TotalBlocksInWindow int64
	BlocksParticipated  int64
	BlocksMissed        int64
	UptimePercentage    float64
	WindowStart         time.Time
	WindowEnd           time.Time
}

// ValidatorRanking contains ranking information with multiple metrics.
type ValidatorRanking struct {
	HexAddress        string
	Moniker           string
	TotalBlocks       int64
	BlocksSigned      int64
	BlocksMissed      int64
	SigningEfficiency float64
	PrevotesCast      int64
	PrecommitsCast    int64
	VotingPower       int64
	EfficiencyRank    int64
}

// TimeSeriesPoint represents performance metrics at a specific time.
type TimeSeriesPoint struct {
	TimeBucket        time.Time
	BlocksInBucket    int64
	BlocksSigned      int64
	BlocksMissed      int64
	SigningEfficiency float64
}

// NewValidatorAnalytics creates a new validator analytics service.
func NewValidatorAnalytics(database *db.DB, logger zerolog.Logger) *ValidatorAnalytics {
	return &ValidatorAnalytics{
		db:     database,
		logger: logger.With().Str("component", "validator_analytics").Logger(),
	}
}

// GetSigningEfficiency calculates signing efficiency for a validator over a time window.
func (va *ValidatorAnalytics) GetSigningEfficiency(ctx context.Context, validatorAddr string, window TimeWindow) (*PerformanceMetrics, error) {
	queries := va.db.Queries()

	// Get basic signing efficiency
	efficiency, err := queries.GetValidatorSigningEfficiency(ctx, sqlc.GetValidatorSigningEfficiencyParams{
		ValidatorHexAddress: validatorAddr,
		BlockTime:           sql.NullTime{Time: window.Start, Valid: true},
		BlockTime_2:         sql.NullTime{Time: window.End, Valid: true},
	})
	if err != nil {
		va.logger.Error().Err(err).Str("validator", validatorAddr).Msg("Failed to get signing efficiency")
		return nil, err
	}

	// Get consensus participation
	participation, err := queries.GetValidatorConsensusParticipation(ctx, sqlc.GetValidatorConsensusParticipationParams{
		ValidatorHexAddress: validatorAddr,
		BlockTime:           sql.NullTime{Time: window.Start, Valid: true},
		BlockTime_2:         sql.NullTime{Time: window.End, Valid: true},
	})
	if err != nil {
		va.logger.Error().Err(err).Str("validator", validatorAddr).Msg("Failed to get consensus participation")
		return nil, err
	}

	// Get miss streaks
	missStreaks, err := queries.GetValidatorMissedBlockStreaks(ctx, sqlc.GetValidatorMissedBlockStreaksParams{
		ValidatorHexAddress: validatorAddr,
		BlockTime:           sql.NullTime{Time: window.Start, Valid: true},
		BlockTime_2:         sql.NullTime{Time: window.End, Valid: true},
	})
	if err != nil {
		va.logger.Error().Err(err).Str("validator", validatorAddr).Msg("Failed to get missed block streaks")
		return nil, err
	}

	// Find longest miss streak
	longestMissStreak := int64(0)
	if len(missStreaks) > 0 {
		longestMissStreak = missStreaks[0].ConsecutiveMisses // Already sorted by consecutive_misses DESC
	}

	// Convert interface{} types safely
	totalBlocks := convertToInt64(efficiency.TotalBlocks)
	blocksSigned := convertToInt64(efficiency.BlocksSigned)
	blocksMissed := convertToInt64(efficiency.BlocksMissed)
	signingEfficiency := convertToFloat64(efficiency.SigningEfficiency)

	totalRounds := convertToInt64(participation.TotalRounds)
	prevoteRate := convertToFloat64(participation.PrevoteRate)
	precommitRate := convertToFloat64(participation.PrecommitRate)

	return &PerformanceMetrics{
		ValidatorHexAddress: validatorAddr,
		TimeWindow:          window,
		TotalBlocks:         totalBlocks,
		BlocksSigned:        blocksSigned,
		BlocksMissed:        blocksMissed,
		SigningEfficiency:   signingEfficiency,
		TotalRounds:         totalRounds,
		PrevoteRate:         prevoteRate,
		PrecommitRate:       precommitRate,
		LongestMissStreak:   longestMissStreak,
		CalculatedAt:        time.Now(),
	}, nil
}

// GetMissedBlockStreaks returns all consecutive missed block sequences for a validator.
func (va *ValidatorAnalytics) GetMissedBlockStreaks(ctx context.Context, validatorAddr string, window TimeWindow) ([]MissedBlockStreak, error) {
	queries := va.db.Queries()

	streaks, err := queries.GetValidatorMissedBlockStreaks(ctx, sqlc.GetValidatorMissedBlockStreaksParams{
		ValidatorHexAddress: validatorAddr,
		BlockTime:           sql.NullTime{Time: window.Start, Valid: true},
		BlockTime_2:         sql.NullTime{Time: window.End, Valid: true},
	})
	if err != nil {
		va.logger.Error().Err(err).Str("validator", validatorAddr).Msg("Failed to get missed block streaks")
		return nil, err
	}

	result := make([]MissedBlockStreak, len(streaks))
	for i, streak := range streaks {
		// Convert interface{} types safely using helper functions
		startHeight := convertToInt64(streak.StreakStartHeight)
		endHeight := convertToInt64(streak.StreakEndHeight)
		startTime := convertToTime(streak.StreakStartTime)
		endTime := convertToTime(streak.StreakEndTime)

		result[i] = MissedBlockStreak{
			ConsecutiveMisses: streak.ConsecutiveMisses,
			StreakStartHeight: startHeight,
			StreakEndHeight:   endHeight,
			StreakStartTime:   startTime,
			StreakEndTime:     endTime,
		}
	}

	return result, nil
}

// GetConsensusParticipation returns detailed consensus participation metrics.
func (va *ValidatorAnalytics) GetConsensusParticipation(ctx context.Context, validatorAddr string, window TimeWindow) (*ConsensusMetrics, error) {
	queries := va.db.Queries()

	participation, err := queries.GetValidatorConsensusParticipation(ctx, sqlc.GetValidatorConsensusParticipationParams{
		ValidatorHexAddress: validatorAddr,
		BlockTime:           sql.NullTime{Time: window.Start, Valid: true},
		BlockTime_2:         sql.NullTime{Time: window.End, Valid: true},
	})
	if err != nil {
		va.logger.Error().Err(err).Str("validator", validatorAddr).Msg("Failed to get consensus participation")
		return nil, err
	}

	// Convert interface{} types safely
	totalRounds := convertToInt64(participation.TotalRounds)
	roundsWithPrevote := convertToInt64(participation.RoundsWithPrevote)
	roundsWithPrecommit := convertToInt64(participation.RoundsWithPrecommit)
	prevoteRate := convertToFloat64(participation.PrevoteRate)
	precommitRate := convertToFloat64(participation.PrecommitRate)

	return &ConsensusMetrics{
		ValidatorHexAddress: validatorAddr,
		TotalRounds:         totalRounds,
		RoundsWithPrevote:   roundsWithPrevote,
		RoundsWithPrecommit: roundsWithPrecommit,
		PrevoteRate:         prevoteRate,
		PrecommitRate:       precommitRate,
	}, nil
}

// GetValidatorUptime returns uptime metrics for a validator.
func (va *ValidatorAnalytics) GetValidatorUptime(ctx context.Context, validatorAddr string, window TimeWindow) (*UptimeMetrics, error) {
	queries := va.db.Queries()

	uptime, err := queries.GetValidatorUptime(ctx, sqlc.GetValidatorUptimeParams{
		ValidatorHexAddress: validatorAddr,
		BlockTime:           sql.NullTime{Time: window.Start, Valid: true},
		BlockTime_2:         sql.NullTime{Time: window.End, Valid: true},
	})
	if err != nil {
		va.logger.Error().Err(err).Str("validator", validatorAddr).Msg("Failed to get validator uptime")
		return nil, err
	}

	// Convert interface{} types safely
	blocksMissed := convertToInt64(uptime.BlocksMissed)
	uptimePercentage := convertToFloat64(uptime.UptimePercentage)
	windowStart := convertToTime(uptime.WindowStart)
	windowEnd := convertToTime(uptime.WindowEnd)

	// Fallback to original window if conversion failed
	if windowStart.IsZero() {
		windowStart = window.Start
	}
	if windowEnd.IsZero() {
		windowEnd = window.End
	}

	return &UptimeMetrics{
		ValidatorHexAddress: validatorAddr,
		TotalBlocksInWindow: uptime.TotalBlocksInWindow,
		BlocksParticipated:  uptime.BlocksParticipated,
		BlocksMissed:        blocksMissed,
		UptimePercentage:    uptimePercentage,
		WindowStart:         windowStart,
		WindowEnd:           windowEnd,
	}, nil
}

// GetAllValidatorMetrics returns performance metrics for all validators.
func (va *ValidatorAnalytics) GetAllValidatorMetrics(ctx context.Context, window TimeWindow) ([]ValidatorRanking, error) {
	queries := va.db.Queries()

	rankings, err := queries.GetValidatorRanking(ctx, sqlc.GetValidatorRankingParams{
		BlockTime:   sql.NullTime{Time: window.Start, Valid: true},
		BlockTime_2: sql.NullTime{Time: window.End, Valid: true},
	})
	if err != nil {
		va.logger.Error().Err(err).Msg("Failed to get validator rankings")
		return nil, err
	}

	result := make([]ValidatorRanking, len(rankings))
	for i, ranking := range rankings {
		// Convert interface{} types safely
		votingPower := convertToInt64(ranking.VotingPower)
		efficiencyRank := convertToInt64(ranking.EfficiencyRank)

		result[i] = ValidatorRanking{
			HexAddress:        ranking.HexAddress,
			Moniker:           ranking.Moniker.String,
			TotalBlocks:       ranking.TotalBlocks,
			BlocksSigned:      ranking.BlocksSigned,
			BlocksMissed:      ranking.BlocksMissed,
			SigningEfficiency: convertToFloat64(ranking.SigningEfficiency),
			PrevotesCast:      ranking.PrevotesCast,
			PrecommitsCast:    ranking.PrecommitsCast,
			VotingPower:       votingPower,
			EfficiencyRank:    efficiencyRank,
		}
	}

	return result, nil
}

// GetPerformanceTimeSeries returns time-series performance data.
func (va *ValidatorAnalytics) GetPerformanceTimeSeries(ctx context.Context, validatorAddr string, window TimeWindow) ([]TimeSeriesPoint, error) {
	queries := va.db.Queries()

	timeSeries, err := queries.GetValidatorPerformanceTimeSeries(ctx, sqlc.GetValidatorPerformanceTimeSeriesParams{
		ValidatorHexAddress: validatorAddr,
		BlockTime:           sql.NullTime{Time: window.Start, Valid: true},
		BlockTime_2:         sql.NullTime{Time: window.End, Valid: true},
	})
	if err != nil {
		va.logger.Error().Err(err).Str("validator", validatorAddr).Msg("Failed to get performance time series")
		return nil, err
	}

	result := make([]TimeSeriesPoint, len(timeSeries))
	for i, point := range timeSeries {
		// Convert interface{} types safely
		timeBucket := convertToTime(point.TimeBucket)
		blocksMissed := convertToInt64(point.BlocksMissed)
		signingEfficiency := convertToFloat64(point.SigningEfficiency)

		result[i] = TimeSeriesPoint{
			TimeBucket:        timeBucket,
			BlocksInBucket:    point.BlocksInBucket,
			BlocksSigned:      int64(point.BlocksSigned),
			BlocksMissed:      blocksMissed,
			SigningEfficiency: signingEfficiency,
		}
	}

	return result, nil
}

// ParseTimeWindow parses a duration string and returns a TimeWindow.
func ParseTimeWindow(durationStr string) (TimeWindow, error) {
	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		return TimeWindow{}, fmt.Errorf("invalid duration format '%s': %w. Examples: 1h, 30m, 24h, 168h (7 days)", durationStr, err)
	}

	if duration <= 0 {
		return TimeWindow{}, fmt.Errorf("duration must be positive, got: %s", durationStr)
	}

	now := time.Now()
	return TimeWindow{
		Start: now.Add(-duration),
		End:   now,
	}, nil
}

// GetCommonTimeWindows returns commonly used time windows for analysis (for documentation/examples).
func GetCommonTimeWindows() map[string]TimeWindow {
	now := time.Now()
	return map[string]TimeWindow{
		"1h": {
			Start: now.Add(-1 * time.Hour),
			End:   now,
		},
		"6h": {
			Start: now.Add(-6 * time.Hour),
			End:   now,
		},
		"24h": {
			Start: now.Add(-24 * time.Hour),
			End:   now,
		},
		"168h": { // 7 days
			Start: now.Add(-7 * 24 * time.Hour),
			End:   now,
		},
		"720h": { // 30 days
			Start: now.Add(-30 * 24 * time.Hour),
			End:   now,
		},
	}
}
