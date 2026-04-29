package analytics

import (
	"context"
	"fmt"
	"time"

	"main/pkg/db"

	"github.com/rs/zerolog"
)

// CLIAnalytics provides command-line interface for analytics.
type CLIAnalytics struct {
	analytics *ValidatorAnalytics
	debugger  *DatabaseDebugger
	logger    zerolog.Logger
}

// NewCLIAnalytics creates a new CLI analytics interface.
func NewCLIAnalytics(database *db.DB, logger zerolog.Logger) *CLIAnalytics {
	return &CLIAnalytics{
		analytics: NewValidatorAnalytics(database, logger),
		debugger:  NewDatabaseDebugger(database, logger),
		logger:    logger.With().Str("component", "cli_analytics").Logger(),
	}
}

// PrintValidatorPerformance displays comprehensive performance metrics for a validator.
func (cli *CLIAnalytics) PrintValidatorPerformance(ctx context.Context, validatorAddr string, durationStr string) error {
	window, err := ParseTimeWindow(durationStr)
	if err != nil {
		return fmt.Errorf("invalid time window: %w", err)
	}

	fmt.Printf("\n=== Validator Performance Analysis ===\n")
	fmt.Printf("Validator: %s\n", validatorAddr)
	fmt.Printf("Time Window: %s (%s to %s)\n", durationStr, window.Start.Format("2006-01-02 15:04:05"), window.End.Format("2006-01-02 15:04:05"))
	fmt.Printf("=======================================\n\n")

	// Get comprehensive performance metrics
	metrics, err := cli.analytics.GetSigningEfficiency(ctx, validatorAddr, window)
	if err != nil {
		return fmt.Errorf("failed to get performance metrics: %w", err)
	}

	// Check if we have any data
	if metrics.TotalBlocks == 0 {
		fmt.Printf("📊 No data found for validator %s in the specified time window.\n", validatorAddr)
		fmt.Printf("💡 This could mean:\n")
		fmt.Printf("   • The database is empty (tmtop hasn't been running long enough)\n")
		fmt.Printf("   • The validator address is incorrect\n")
		fmt.Printf("   • The time window doesn't contain any recorded blocks\n")
		fmt.Printf("\n💭 Try:\n")
		fmt.Printf("   • Run tmtop in normal mode first to collect data\n")
		fmt.Printf("   • Use a different time window (e.g., --analytics-time-window 1h)\n")
		fmt.Printf("   • Check if the validator address is correct\n")
		return nil
	}

	// Display block signing metrics
	fmt.Printf("📊 Block Signing Performance:\n")
	fmt.Printf("  Total Blocks:       %d\n", metrics.TotalBlocks)
	fmt.Printf("  Blocks Signed:      %d\n", metrics.BlocksSigned)
	fmt.Printf("  Blocks Missed:      %d\n", metrics.BlocksMissed)
	fmt.Printf("  Signing Efficiency: %.2f%%\n", metrics.SigningEfficiency)
	fmt.Printf("\n")

	// Display consensus participation
	fmt.Printf("🗳️  Consensus Participation:\n")
	fmt.Printf("  Total Rounds:       %d\n", metrics.TotalRounds)
	fmt.Printf("  Prevote Rate:       %.2f%%\n", metrics.PrevoteRate)
	fmt.Printf("  Precommit Rate:     %.2f%%\n", metrics.PrecommitRate)
	fmt.Printf("\n")

	// Display miss analysis
	fmt.Printf("⚠️  Miss Analysis:\n")
	fmt.Printf("  Longest Miss Streak: %d blocks\n", metrics.LongestMissStreak)

	// Get detailed miss streaks if any
	if metrics.LongestMissStreak > 0 {
		streaks, err := cli.analytics.GetMissedBlockStreaks(ctx, validatorAddr, window)
		if err == nil && len(streaks) > 0 {
			fmt.Printf("  Recent Miss Streaks:\n")
			for i, streak := range streaks {
				if i >= 5 { // Show only top 5 streaks
					break
				}
				fmt.Printf("    - %d blocks (heights %d-%d)\n",
					streak.ConsecutiveMisses,
					streak.StreakStartHeight,
					streak.StreakEndHeight)
			}
		}
	}
	fmt.Printf("\n")

	// Get uptime metrics
	uptime, err := cli.analytics.GetValidatorUptime(ctx, validatorAddr, window)
	if err == nil {
		fmt.Printf("⏱️  Uptime Metrics:\n")
		fmt.Printf("  Uptime Percentage:  %.2f%%\n", uptime.UptimePercentage)
		fmt.Printf("  Blocks Participated: %d/%d\n", uptime.BlocksParticipated, uptime.TotalBlocksInWindow)
	}

	return nil
}

// PrintValidatorRankings displays performance rankings for all validators.
func (cli *CLIAnalytics) PrintValidatorRankings(ctx context.Context, durationStr string, limit int) error {
	window, err := ParseTimeWindow(durationStr)
	if err != nil {
		return fmt.Errorf("invalid time window: %w", err)
	}

	fmt.Printf("\n=== Validator Performance Rankings ===\n")
	fmt.Printf("Time Window: %s (%s to %s)\n", durationStr, window.Start.Format("2006-01-02 15:04:05"), window.End.Format("2006-01-02 15:04:05"))
	fmt.Printf("======================================\n\n")

	rankings, err := cli.analytics.GetAllValidatorMetrics(ctx, window)
	if err != nil {
		return fmt.Errorf("failed to get validator rankings: %w", err)
	}

	if len(rankings) == 0 {
		fmt.Printf("No validator data found for the specified time window.\n")
		return nil
	}

	// Header
	fmt.Printf("%-4s %-20s %-12s %-12s %-12s %-8s %-12s\n",
		"Rank", "Moniker", "Blocks Signed", "Efficiency", "Voting Power", "Prevotes", "Precommits")
	fmt.Printf("%-4s %-20s %-12s %-12s %-12s %-8s %-12s\n",
		"----", "--------------------", "------------", "------------", "------------", "--------", "------------")

	// Display top validators (limited)
	displayCount := limit
	if len(rankings) < displayCount {
		displayCount = len(rankings)
	}

	for i := 0; i < displayCount; i++ {
		v := rankings[i]
		moniker := v.Moniker
		if len(moniker) > 20 {
			moniker = moniker[:17] + "..."
		}
		if moniker == "" {
			if len(v.HexAddress) > 12 {
				moniker = v.HexAddress[:12] + "..."
			} else {
				moniker = v.HexAddress
			}
		}

		fmt.Printf("%-4d %-20s %-12s %-12s %-12d %-8d %-12d\n",
			v.EfficiencyRank,
			moniker,
			fmt.Sprintf("%d/%d", v.BlocksSigned, v.TotalBlocks),
			fmt.Sprintf("%.2f%%", v.SigningEfficiency),
			v.VotingPower,
			v.PrevotesCast,
			v.PrecommitsCast)
	}

	if len(rankings) > limit {
		fmt.Printf("\n... and %d more validators\n", len(rankings)-limit)
	}

	return nil
}

// PrintPerformanceTimeSeries displays hourly performance trend for a validator.
func (cli *CLIAnalytics) PrintPerformanceTimeSeries(ctx context.Context, validatorAddr string, durationStr string) error {
	window, err := ParseTimeWindow(durationStr)
	if err != nil {
		return fmt.Errorf("invalid time window: %w", err)
	}

	fmt.Printf("\n=== Validator Performance Time Series ===\n")
	fmt.Printf("Validator: %s\n", validatorAddr)
	fmt.Printf("Time Window: %s (hourly buckets)\n", durationStr)
	fmt.Printf("=========================================\n\n")

	series, err := cli.analytics.GetPerformanceTimeSeries(ctx, validatorAddr, window)
	if err != nil {
		return fmt.Errorf("failed to get performance time series: %w", err)
	}

	if len(series) == 0 {
		fmt.Printf("No time series data found for the specified time window.\n")
		return nil
	}

	// Header
	fmt.Printf("%-20s %-12s %-12s %-12s\n", "Time", "Blocks", "Signed", "Efficiency")
	fmt.Printf("%-20s %-12s %-12s %-12s\n", "--------------------", "------------", "------------", "------------")

	// Display time series data
	for _, point := range series {
		fmt.Printf("%-20s %-12d %-12d %-12s\n",
			point.TimeBucket.Format("2006-01-02 15:04"),
			point.BlocksInBucket,
			point.BlocksSigned,
			fmt.Sprintf("%.2f%%", point.SigningEfficiency))
	}

	return nil
}

// PrintDatabaseSummary prints a summary of the database contents.
func (cli *CLIAnalytics) PrintDatabaseSummary(ctx context.Context) error {
	return cli.debugger.PrintDatabaseSummary(ctx)
}

// DiagnoseValidator performs detailed diagnosis for a specific validator.
func (cli *CLIAnalytics) DiagnoseValidator(ctx context.Context, validatorAddr string, durationStr string) error {
	window, err := ParseTimeWindow(durationStr)
	if err != nil {
		return fmt.Errorf("invalid time window: %w", err)
	}

	return cli.debugger.DiagnoseValidatorData(ctx, validatorAddr, window)
}

// SearchValidators searches for validators by partial address or moniker.
func (cli *CLIAnalytics) SearchValidators(ctx context.Context, searchTerm string) error {
	return cli.debugger.SearchValidators(ctx, searchTerm)
}

// FormatDuration formats a duration in a human-readable way.
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%.1fh", d.Hours())
	}
	return fmt.Sprintf("%.1fd", d.Hours()/24)
}
