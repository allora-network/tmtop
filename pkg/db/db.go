package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"main/pkg/db/sqlc"

	_ "modernc.org/sqlite"
)

type DB struct {
	db      *sql.DB
	queries *sqlc.Queries
}

type Config struct {
	DatabasePath    string
	MaxRetainDays   int   // How many days of data to retain
	MaxRetainBlocks int64 // How many blocks of data to retain (alternative to days)
}

func New(config Config) (*DB, error) {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(config.DatabasePath), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open SQLite database
	db, err := sql.Open("sqlite", config.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure SQLite for better performance
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("failed to set WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA synchronous=NORMAL"); err != nil {
		return nil, fmt.Errorf("failed to set synchronous mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	service := &DB{
		db:      db,
		queries: sqlc.New(db),
	}

	// Run migrations
	if err := service.migrate(); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return service, nil
}

func (d *DB) Close() error {
	return d.db.Close()
}

func (d *DB) Queries() *sqlc.Queries {
	return d.queries
}

func (d *DB) DB() *sql.DB {
	return d.db
}

// WithTx executes a function within a database transaction.
func (d *DB) WithTx(ctx context.Context, fn func(*sqlc.Queries) error) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	q := d.queries.WithTx(tx)
	if err := fn(q); err != nil {
		return err
	}

	return tx.Commit()
}

// migrate runs database migrations.
func (d *DB) migrate() error {
	// Create migrations table if it doesn't exist
	createMigrationsTable := `
		CREATE TABLE IF NOT EXISTS migrations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			version TEXT NOT NULL UNIQUE,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`

	if _, err := d.db.Exec(createMigrationsTable); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Check if migration 001_initial has been applied
	var count int
	err := d.db.QueryRow("SELECT COUNT(*) FROM migrations WHERE version = '001_initial'").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check migration status: %w", err)
	}

	// If migration already applied, skip it
	if count > 0 {
		return nil
	}

	// Read migration file
	migrationSQL := `-- Initial schema for tmtop database

-- Validators table to store validator information
CREATE TABLE validators (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    operator_address TEXT NOT NULL UNIQUE,
    hex_address TEXT NOT NULL UNIQUE,
    public_key TEXT NOT NULL,
    voting_power INTEGER NOT NULL,
    moniker TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Index for fast validator lookups
CREATE INDEX idx_validators_hex_address ON validators(hex_address);
CREATE INDEX idx_validators_operator_address ON validators(operator_address);

-- Heights table to store block height information
CREATE TABLE heights (
    height INTEGER PRIMARY KEY,
    block_hash TEXT,
    block_time DATETIME,
    proposer_address TEXT,
    total_validators INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (proposer_address) REFERENCES validators(hex_address)
);

-- Rounds table to store consensus round information
CREATE TABLE rounds (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    height INTEGER NOT NULL,
    round_number INTEGER NOT NULL,
    step INTEGER,
    start_time DATETIME,
    proposer_address TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(height, round_number),
    FOREIGN KEY (height) REFERENCES heights(height),
    FOREIGN KEY (proposer_address) REFERENCES validators(hex_address)
);

-- Index for efficient round queries
CREATE INDEX idx_rounds_height_round ON rounds(height, round_number);
CREATE INDEX idx_rounds_height_desc ON rounds(height DESC);

-- Votes table to store individual validator votes
CREATE TABLE votes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    height INTEGER NOT NULL,
    round_number INTEGER NOT NULL,
    validator_hex_address TEXT NOT NULL,
    vote_type INTEGER NOT NULL, -- 1 = prevote, 2 = precommit
    block_hash TEXT, -- NULL for nil votes
    signature TEXT,
    timestamp DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(height, round_number, validator_hex_address, vote_type),
    FOREIGN KEY (height) REFERENCES heights(height),
    FOREIGN KEY (validator_hex_address) REFERENCES validators(hex_address)
);

-- Indexes for efficient vote queries
CREATE INDEX idx_votes_height_round ON votes(height, round_number);
CREATE INDEX idx_votes_validator ON votes(validator_hex_address);
CREATE INDEX idx_votes_height_round_type ON votes(height, round_number, vote_type);

-- Consensus events table for tracking important consensus milestones
CREATE TABLE consensus_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    height INTEGER NOT NULL,
    round_number INTEGER NOT NULL,
    event_type TEXT NOT NULL, -- 'new_round', 'proposal', 'prevote_majority', 'precommit_majority', 'block_commit'
    event_data TEXT, -- JSON data for additional context
    timestamp DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (height) REFERENCES heights(height)
);

-- Index for consensus event queries
CREATE INDEX idx_consensus_events_height_round ON consensus_events(height, round_number);
CREATE INDEX idx_consensus_events_type ON consensus_events(event_type);
CREATE INDEX idx_consensus_events_timestamp ON consensus_events(timestamp);

-- Validator snapshots table to track voting power changes over time
CREATE TABLE validator_snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    height INTEGER NOT NULL,
    validator_hex_address TEXT NOT NULL,
    voting_power INTEGER NOT NULL,
    voting_power_percent REAL,
    is_proposer BOOLEAN DEFAULT FALSE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(height, validator_hex_address),
    FOREIGN KEY (height) REFERENCES heights(height),
    FOREIGN KEY (validator_hex_address) REFERENCES validators(hex_address)
);

-- Index for efficient validator snapshot queries
CREATE INDEX idx_validator_snapshots_height ON validator_snapshots(height);
CREATE INDEX idx_validator_snapshots_validator ON validator_snapshots(validator_hex_address);`

	// Execute migration
	if _, err := d.db.Exec(migrationSQL); err != nil {
		return fmt.Errorf("failed to execute migration: %w", err)
	}

	// Record that migration was applied
	_, err = d.db.Exec("INSERT INTO migrations (version) VALUES ('001_initial')")
	if err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	return nil
}

// CleanupOldData removes data older than the configured retention period.
func (d *DB) CleanupOldData(ctx context.Context, config Config) error {
	var cutoffHeight int64

	if config.MaxRetainBlocks > 0 {
		// Get latest height and calculate cutoff
		latest, err := d.queries.GetLatestHeight(ctx)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil // No data to clean up
			}
			return fmt.Errorf("failed to get latest height: %w", err)
		}
		cutoffHeight = latest.Height - config.MaxRetainBlocks
	} else if config.MaxRetainDays > 0 {
		// Calculate cutoff based on days
		_ = time.Now().AddDate(0, 0, -config.MaxRetainDays)
		// This is a simplified approach - in a real implementation you'd want to
		// find the height that corresponds to the cutoff time
		cutoffHeight = 0 // For now, we'll rely on MaxRetainBlocks
	} else {
		return nil // No cleanup configured
	}

	if cutoffHeight <= 0 {
		return nil
	}

	// Clean up old data in order (due to foreign key constraints)
	if err := d.queries.DeleteConsensusEventsOlderThan(ctx, cutoffHeight); err != nil {
		return fmt.Errorf("failed to delete old consensus events: %w", err)
	}

	if err := d.queries.DeleteVotesOlderThan(ctx, cutoffHeight); err != nil {
		return fmt.Errorf("failed to delete old votes: %w", err)
	}

	if err := d.queries.DeleteValidatorSnapshotsOlderThan(ctx, cutoffHeight); err != nil {
		return fmt.Errorf("failed to delete old validator snapshots: %w", err)
	}

	if err := d.queries.DeleteRoundsOlderThan(ctx, cutoffHeight); err != nil {
		return fmt.Errorf("failed to delete old rounds: %w", err)
	}

	if err := d.queries.DeleteHeightsOlderThan(ctx, cutoffHeight); err != nil {
		return fmt.Errorf("failed to delete old heights: %w", err)
	}

	return nil
}
