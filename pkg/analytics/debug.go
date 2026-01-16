package analytics

import (
	"context"
	"database/sql"
	"fmt"

	"main/pkg/db"

	"github.com/rs/zerolog"
)

// DatabaseDebugger provides debugging capabilities for the analytics database.
type DatabaseDebugger struct {
	db     *db.DB
	logger zerolog.Logger
}

// NewDatabaseDebugger creates a new database debugger.
func NewDatabaseDebugger(database *db.DB, logger zerolog.Logger) *DatabaseDebugger {
	return &DatabaseDebugger{
		db:     database,
		logger: logger.With().Str("component", "db_debugger").Logger(),
	}
}

// PrintDatabaseSummary prints a summary of what's in the database.
func (d *DatabaseDebugger) PrintDatabaseSummary(ctx context.Context) error {
	fmt.Printf("\n=== Database Summary ===\n")

	// Check each table
	tables := []string{"validators", "heights", "rounds", "votes", "consensus_events", "validator_snapshots"}

	for _, table := range tables {
		count, err := d.getTableCount(ctx, table)
		if err != nil {
			fmt.Printf("❌ %s: ERROR - %v\n", table, err)
			continue
		}
		fmt.Printf("📊 %s: %d records\n", table, count)
	}

	// Get sample data from key tables
	fmt.Printf("\n=== Sample Data ===\n")

	// Sample validators
	if err := d.printSampleValidators(ctx); err != nil {
		fmt.Printf("❌ Error getting validators: %v\n", err)
	}

	// Sample heights
	if err := d.printSampleHeights(ctx); err != nil {
		fmt.Printf("❌ Error getting heights: %v\n", err)
	}

	// Sample votes
	if err := d.printSampleVotes(ctx); err != nil {
		fmt.Printf("❌ Error getting votes: %v\n", err)
	}

	return nil
}

// DiagnoseValidatorData checks if a specific validator has data.
func (d *DatabaseDebugger) DiagnoseValidatorData(ctx context.Context, validatorAddr string, timeWindow TimeWindow) error {
	fmt.Printf("\n=== Validator Diagnosis ===\n")
	fmt.Printf("Validator: %s\n", validatorAddr)
	fmt.Printf("Time Window: %s to %s\n", timeWindow.Start.Format("2006-01-02 15:04:05"), timeWindow.End.Format("2006-01-02 15:04:05"))
	fmt.Printf("==========================\n\n")

	// Check if validator exists in validators table
	validatorExists, err := d.checkValidatorExists(ctx, validatorAddr)
	if err != nil {
		return fmt.Errorf("error checking validator existence: %w", err)
	}

	if validatorExists {
		fmt.Printf("✅ Validator found in validators table\n")
	} else {
		fmt.Printf("❌ Validator NOT found in validators table\n")
		fmt.Printf("💡 Available validators:\n")
		if err := d.printAllValidators(ctx); err != nil {
			fmt.Printf("   Error listing validators: %v\n", err)
		}
	}

	// Check heights in time window
	heightCount, err := d.getHeightsInWindow(ctx, timeWindow)
	if err != nil {
		return fmt.Errorf("error checking heights: %w", err)
	}
	fmt.Printf("📊 Heights in time window: %d\n", heightCount)

	// Check votes for this validator in time window
	voteCount, err := d.getVotesForValidatorInWindow(ctx, validatorAddr, timeWindow)
	if err != nil {
		return fmt.Errorf("error checking votes: %w", err)
	}
	fmt.Printf("🗳️  Votes for validator in time window: %d\n", voteCount)

	// Sample some votes for this validator
	if err := d.printSampleValidatorVotes(ctx, validatorAddr, 5); err != nil {
		fmt.Printf("❌ Error getting sample votes: %v\n", err)
	}

	return nil
}

func (d *DatabaseDebugger) getTableCount(ctx context.Context, tableName string) (int, error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)
	row := d.db.DB().QueryRowContext(ctx, query)

	var count int
	err := row.Scan(&count)
	return count, err
}

func (d *DatabaseDebugger) printSampleValidators(ctx context.Context) error {
	query := "SELECT address, moniker, voting_power FROM validators LIMIT 5"
	rows, err := d.db.DB().QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	fmt.Printf("👥 Sample Validators:\n")
	for rows.Next() {
		var address, moniker string
		var votingPower int64
		if err := rows.Scan(&address, &moniker, &votingPower); err != nil {
			return err
		}
		fmt.Printf("   %s (%s) - Power: %d\n", address, moniker, votingPower)
	}

	return rows.Err()
}

func (d *DatabaseDebugger) printSampleHeights(ctx context.Context) error {
	query := "SELECT height, block_time, proposer_address FROM heights ORDER BY height DESC LIMIT 5"
	rows, err := d.db.DB().QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	fmt.Printf("📏 Sample Heights (latest):\n")
	for rows.Next() {
		var height int64
		var blockTime sql.NullTime
		var proposer sql.NullString
		if err := rows.Scan(&height, &blockTime, &proposer); err != nil {
			return err
		}

		blockTimeStr := "NULL"
		if blockTime.Valid {
			blockTimeStr = blockTime.Time.Format("2006-01-02 15:04:05")
		}

		proposerStr := "NULL"
		if proposer.Valid {
			proposerStr = proposer.String
		}

		fmt.Printf("   Height: %d, Time: %s, Proposer: %s\n", height, blockTimeStr, proposerStr)
	}

	return rows.Err()
}

func (d *DatabaseDebugger) printSampleVotes(ctx context.Context) error {
	query := "SELECT height, round_number, validator_address, vote_type, timestamp FROM votes LIMIT 5"
	rows, err := d.db.DB().QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	fmt.Printf("🗳️  Sample Votes:\n")
	for rows.Next() {
		var height, round, voteType int64
		var validatorAddr string
		var timestamp sql.NullTime
		if err := rows.Scan(&height, &round, &validatorAddr, &voteType, &timestamp); err != nil {
			return err
		}

		timestampStr := "NULL"
		if timestamp.Valid {
			timestampStr = timestamp.Time.Format("15:04:05")
		}

		fmt.Printf("   H:%d R:%d Validator:%s Type:%d Time:%s\n", height, round, validatorAddr, voteType, timestampStr)
	}

	return rows.Err()
}

func (d *DatabaseDebugger) checkValidatorExists(ctx context.Context, validatorAddr string) (bool, error) {
	query := "SELECT COUNT(*) FROM validators WHERE address = ?"
	row := d.db.DB().QueryRowContext(ctx, query, validatorAddr)

	var count int
	err := row.Scan(&count)
	return count > 0, err
}

func (d *DatabaseDebugger) printAllValidators(ctx context.Context) error {
	// First get the total count
	countQuery := "SELECT COUNT(*) FROM validators"
	row := d.db.DB().QueryRowContext(ctx, countQuery)
	var totalCount int
	if err := row.Scan(&totalCount); err != nil {
		return err
	}

	fmt.Printf("   Total validators in database: %d\n", totalCount)

	// Show all validators (no limit)
	query := "SELECT address, moniker FROM validators ORDER BY address"
	rows, err := d.db.DB().QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	fmt.Printf("   All validators:\n")
	count := 0
	for rows.Next() {
		var address, moniker string
		if err := rows.Scan(&address, &moniker); err != nil {
			return err
		}
		count++
		if moniker != "" {
			fmt.Printf("   %d. %s (%s)\n", count, address, moniker)
		} else {
			fmt.Printf("   %d. %s\n", count, address)
		}
	}

	return rows.Err()
}

func (d *DatabaseDebugger) getHeightsInWindow(ctx context.Context, window TimeWindow) (int, error) {
	query := "SELECT COUNT(*) FROM heights WHERE block_time >= ? AND block_time <= ?"
	row := d.db.DB().QueryRowContext(ctx, query, window.Start, window.End)

	var count int
	err := row.Scan(&count)
	return count, err
}

func (d *DatabaseDebugger) getVotesForValidatorInWindow(ctx context.Context, validatorAddr string, window TimeWindow) (int, error) {
	query := `
		SELECT COUNT(*) 
		FROM votes v 
		JOIN heights h ON v.height = h.height 
		WHERE v.validator_address = ? AND h.block_time >= ? AND h.block_time <= ?
	`
	row := d.db.DB().QueryRowContext(ctx, query, validatorAddr, window.Start, window.End)

	var count int
	err := row.Scan(&count)
	return count, err
}

func (d *DatabaseDebugger) printSampleValidatorVotes(ctx context.Context, validatorAddr string, limit int) error {
	query := `
		SELECT v.height, v.round_number, v.vote_type, v.timestamp, h.block_time 
		FROM votes v 
		JOIN heights h ON v.height = h.height 
		WHERE v.validator_address = ? 
		ORDER BY v.height DESC 
		LIMIT ?
	`
	rows, err := d.db.DB().QueryContext(ctx, query, validatorAddr, limit)
	if err != nil {
		return err
	}
	defer rows.Close()

	fmt.Printf("🔍 Sample votes for validator %s:\n", validatorAddr)
	count := 0
	for rows.Next() {
		var height, round, voteType int64
		var voteTime, blockTime sql.NullTime
		if err := rows.Scan(&height, &round, &voteType, &voteTime, &blockTime); err != nil {
			return err
		}

		voteTimeStr := "NULL"
		if voteTime.Valid {
			voteTimeStr = voteTime.Time.Format("15:04:05")
		}

		blockTimeStr := "NULL"
		if blockTime.Valid {
			blockTimeStr = blockTime.Time.Format("15:04:05")
		}

		fmt.Printf("   H:%d R:%d Type:%d VoteTime:%s BlockTime:%s\n",
			height, round, voteType, voteTimeStr, blockTimeStr)
		count++
	}

	if count == 0 {
		fmt.Printf("   No votes found for this validator\n")
	}

	return rows.Err()
}

// SearchValidators searches for validators by partial address or moniker.
func (d *DatabaseDebugger) SearchValidators(ctx context.Context, searchTerm string) error {
	fmt.Printf("\n=== Validator Search ===\n")
	fmt.Printf("Search term: %s\n", searchTerm)
	fmt.Printf("=======================\n\n")

	// Search by address (partial match)
	query := `
		SELECT address, moniker, voting_power 
		FROM validators 
		WHERE address LIKE ? OR LOWER(moniker) LIKE LOWER(?) 
		ORDER BY address
	`

	searchPattern := "%" + searchTerm + "%"
	rows, err := d.db.DB().QueryContext(ctx, query, searchPattern, searchPattern)
	if err != nil {
		return err
	}
	defer rows.Close()

	matches := 0
	fmt.Printf("🔍 Matching validators:\n")
	for rows.Next() {
		var address, moniker string
		var votingPower int64
		if err := rows.Scan(&address, &moniker, &votingPower); err != nil {
			return err
		}
		matches++
		if moniker != "" {
			fmt.Printf("   %d. %s (%s) - Power: %d\n", matches, address, moniker, votingPower)
		} else {
			fmt.Printf("   %d. %s - Power: %d\n", matches, address, votingPower)
		}
	}

	if matches == 0 {
		fmt.Printf("   No validators found matching '%s'\n", searchTerm)
		fmt.Printf("\n💡 Try:\n")
		fmt.Printf("   - Use a shorter search term\n")
		fmt.Printf("   - Check the exact validator address format\n")
		fmt.Printf("   - Run the 'debug' command to see all available validators\n")
	} else {
		fmt.Printf("\nFound %d matching validator(s)\n", matches)
	}

	return rows.Err()
}
