package analytics

import (
	"context"
	"database/sql"
	"fmt"

	"main/pkg/db"
	"main/pkg/db/sqlc"

	"github.com/rs/zerolog"
)

// DatabaseDebugger provides debugging capabilities for the analytics database.
// All queries go through sqlc — no raw SQL.
type DatabaseDebugger struct {
	q      *sqlc.Queries
	logger zerolog.Logger
}

// NewDatabaseDebugger creates a new database debugger.
func NewDatabaseDebugger(database *db.DB, logger zerolog.Logger) *DatabaseDebugger {
	return &DatabaseDebugger{
		q:      database.Queries(),
		logger: logger.With().Str("component", "db_debugger").Logger(),
	}
}

// PrintDatabaseSummary prints a summary of what's in the database.
func (d *DatabaseDebugger) PrintDatabaseSummary(ctx context.Context) error {
	fmt.Printf("\n=== Database Summary ===\n")

	type counter struct {
		name string
		fn   func(context.Context) (int64, error)
	}
	counters := []counter{
		{"validators", d.q.CountValidators},
		{"heights", d.q.CountHeights},
		{"rounds", d.q.CountRounds},
		{"votes", d.q.CountVotes},
		{"consensus_events", d.q.CountConsensusEvents},
		{"validator_snapshots", d.q.CountValidatorSnapshots},
	}

	for _, c := range counters {
		count, err := c.fn(ctx)
		if err != nil {
			fmt.Printf("❌ %s: ERROR - %v\n", c.name, err)
			continue
		}
		fmt.Printf("📊 %s: %d records\n", c.name, count)
	}

	fmt.Printf("\n=== Sample Data ===\n")

	if err := d.printSampleValidators(ctx); err != nil {
		fmt.Printf("❌ Error getting validators: %v\n", err)
	}
	if err := d.printSampleHeights(ctx); err != nil {
		fmt.Printf("❌ Error getting heights: %v\n", err)
	}
	if err := d.printSampleVotes(ctx); err != nil {
		fmt.Printf("❌ Error getting votes: %v\n", err)
	}

	return nil
}

// DiagnoseValidatorData checks if a specific validator (by operator
// address) has data in the requested time window.
func (d *DatabaseDebugger) DiagnoseValidatorData(ctx context.Context, validatorAddr string, timeWindow TimeWindow) error {
	fmt.Printf("\n=== Validator Diagnosis ===\n")
	fmt.Printf("Validator: %s\n", validatorAddr)
	fmt.Printf("Time Window: %s to %s\n", timeWindow.Start.Format("2006-01-02 15:04:05"), timeWindow.End.Format("2006-01-02 15:04:05"))
	fmt.Printf("==========================\n\n")

	count, err := d.q.ValidatorExistsByOperator(ctx, validatorAddr)
	if err != nil {
		return fmt.Errorf("checking validator existence: %w", err)
	}
	if count > 0 {
		fmt.Printf("✅ Validator found in validators table\n")
	} else {
		fmt.Printf("❌ Validator NOT found in validators table\n")
		fmt.Printf("💡 Available validators:\n")
		if err := d.printAllValidators(ctx); err != nil {
			fmt.Printf("   Error listing validators: %v\n", err)
		}
	}

	heightCount, err := d.q.CountHeightsInTimeWindow(ctx, sqlc.CountHeightsInTimeWindowParams{
		BlockTime:   sql.NullTime{Time: timeWindow.Start, Valid: true},
		BlockTime_2: sql.NullTime{Time: timeWindow.End, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("counting heights in window: %w", err)
	}
	fmt.Printf("📊 Heights in time window: %d\n", heightCount)

	voteCount, err := d.q.CountVotesForValidatorInTimeWindow(ctx, sqlc.CountVotesForValidatorInTimeWindowParams{
		OperatorAddress: validatorAddr,
		BlockTime:       sql.NullTime{Time: timeWindow.Start, Valid: true},
		BlockTime_2:     sql.NullTime{Time: timeWindow.End, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("counting votes for validator in window: %w", err)
	}
	fmt.Printf("🗳️  Votes for validator in time window: %d\n", voteCount)

	if err := d.printSampleValidatorVotes(ctx, validatorAddr, 5); err != nil {
		fmt.Printf("❌ Error getting sample votes: %v\n", err)
	}

	return nil
}

func (d *DatabaseDebugger) printSampleValidators(ctx context.Context) error {
	rows, err := d.q.SampleValidators(ctx)
	if err != nil {
		return fmt.Errorf("sampling validators: %w", err)
	}

	fmt.Printf("👥 Sample Validators:\n")
	for _, row := range rows {
		moniker := ""
		if row.Moniker.Valid {
			moniker = row.Moniker.String
		}
		fmt.Printf("   %s (%s) - Power: %d\n", row.OperatorAddress, moniker, row.VotingPower)
	}
	return nil
}

func (d *DatabaseDebugger) printSampleHeights(ctx context.Context) error {
	rows, err := d.q.SampleHeights(ctx)
	if err != nil {
		return fmt.Errorf("sampling heights: %w", err)
	}

	fmt.Printf("📏 Sample Heights (latest):\n")
	for _, row := range rows {
		blockTimeStr := nullTimeStr(row.BlockTime, "2006-01-02 15:04:05")
		proposerStr := nullStringStr(row.ProposerAddress)
		fmt.Printf("   Height: %d, Time: %s, Proposer: %s\n", row.Height, blockTimeStr, proposerStr)
	}
	return nil
}

func (d *DatabaseDebugger) printSampleVotes(ctx context.Context) error {
	rows, err := d.q.SampleVotes(ctx)
	if err != nil {
		return fmt.Errorf("sampling votes: %w", err)
	}

	fmt.Printf("🗳️  Sample Votes:\n")
	for _, row := range rows {
		timestampStr := nullTimeStr(row.Timestamp, "15:04:05")
		fmt.Printf("   H:%d R:%d Validator:%s Type:%d Time:%s\n",
			row.Height, row.RoundNumber, row.ValidatorHexAddress, row.VoteType, timestampStr)
	}
	return nil
}

func (d *DatabaseDebugger) printAllValidators(ctx context.Context) error {
	totalCount, err := d.q.CountValidators(ctx)
	if err != nil {
		return fmt.Errorf("counting validators: %w", err)
	}
	fmt.Printf("   Total validators in database: %d\n", totalCount)

	rows, err := d.q.ListAllValidators(ctx)
	if err != nil {
		return fmt.Errorf("listing validators: %w", err)
	}

	fmt.Printf("   All validators:\n")
	for i, row := range rows {
		moniker := ""
		if row.Moniker.Valid {
			moniker = row.Moniker.String
		}
		if moniker != "" {
			fmt.Printf("   %d. %s (%s)\n", i+1, row.OperatorAddress, moniker)
		} else {
			fmt.Printf("   %d. %s\n", i+1, row.OperatorAddress)
		}
	}
	return nil
}

func (d *DatabaseDebugger) printSampleValidatorVotes(ctx context.Context, validatorAddr string, limit int) error {
	rows, err := d.q.SampleVotesForValidator(ctx, sqlc.SampleVotesForValidatorParams{
		OperatorAddress: validatorAddr,
		Limit:           int64(limit),
	})
	if err != nil {
		return fmt.Errorf("sampling votes for validator: %w", err)
	}

	fmt.Printf("🔍 Sample votes for validator %s:\n", validatorAddr)
	for _, row := range rows {
		voteTimeStr := nullTimeStr(row.Timestamp, "15:04:05")
		blockTimeStr := nullTimeStr(row.BlockTime, "15:04:05")
		fmt.Printf("   H:%d R:%d Type:%d VoteTime:%s BlockTime:%s\n",
			row.Height, row.RoundNumber, row.VoteType, voteTimeStr, blockTimeStr)
	}
	if len(rows) == 0 {
		fmt.Printf("   No votes found for this validator\n")
	}
	return nil
}

// SearchValidators searches for validators by partial operator
// address or moniker (case-insensitive).
func (d *DatabaseDebugger) SearchValidators(ctx context.Context, searchTerm string) error {
	fmt.Printf("\n=== Validator Search ===\n")
	fmt.Printf("Search term: %s\n", searchTerm)
	fmt.Printf("=======================\n\n")

	pattern := "%" + searchTerm + "%"
	rows, err := d.q.SearchValidatorsByOperatorOrMoniker(ctx, sqlc.SearchValidatorsByOperatorOrMonikerParams{
		OperatorAddress: pattern,
		LOWER:           pattern,
	})
	if err != nil {
		return fmt.Errorf("searching validators: %w", err)
	}

	fmt.Printf("🔍 Matching validators:\n")
	for i, row := range rows {
		moniker := ""
		if row.Moniker.Valid {
			moniker = row.Moniker.String
		}
		if moniker != "" {
			fmt.Printf("   %d. %s (%s) - Power: %d\n", i+1, row.OperatorAddress, moniker, row.VotingPower)
		} else {
			fmt.Printf("   %d. %s - Power: %d\n", i+1, row.OperatorAddress, row.VotingPower)
		}
	}

	if len(rows) == 0 {
		fmt.Printf("   No validators found matching '%s'\n", searchTerm)
		fmt.Printf("\n💡 Try:\n")
		fmt.Printf("   - Use a shorter search term\n")
		fmt.Printf("   - Check the exact validator address format\n")
		fmt.Printf("   - Run the 'debug' command to see all available validators\n")
	} else {
		fmt.Printf("\nFound %d matching validator(s)\n", len(rows))
	}

	return nil
}

func nullTimeStr(t sql.NullTime, layout string) string {
	if !t.Valid {
		return "NULL"
	}
	return t.Time.Format(layout)
}

func nullStringStr(s sql.NullString) string {
	if !s.Valid {
		return "NULL"
	}
	return s.String
}
