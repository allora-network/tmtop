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
	DatabasePath   string
	MaxRetainDays  int // How many days of data to retain
	MaxRetainBlocks int64 // How many blocks of data to retain (alternative to days)
}

func New(config Config) (*DB, error) {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(config.DatabasePath), 0755); err != nil {
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

// WithTx executes a function within a database transaction
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

// migrate runs database migrations
func (d *DB) migrate() error {
	// Read migration file
	migrationSQL := `-- Initial schema for tmtop database

-- Validators table to store validator information
CREATE TABLE IF NOT EXISTS validators (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    address TEXT NOT NULL UNIQUE,
    public_key TEXT NOT NULL,
    voting_power INTEGER NOT NULL,
    moniker TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Index for fast validator lookups
CREATE INDEX IF NOT EXISTS idx_validators_address ON validators(address);

-- Heights table to store block height information
CREATE TABLE IF NOT EXISTS heights (
    height INTEGER PRIMARY KEY,
    block_hash TEXT,
    block_time DATETIME,
    proposer_address TEXT,
    total_validators INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (proposer_address) REFERENCES validators(address)
);

-- Rounds table to store consensus round information
CREATE TABLE IF NOT EXISTS rounds (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    height INTEGER NOT NULL,
    round_number INTEGER NOT NULL,
    step INTEGER,
    start_time DATETIME,
    proposer_address TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(height, round_number),
    FOREIGN KEY (height) REFERENCES heights(height),
    FOREIGN KEY (proposer_address) REFERENCES validators(address)
);

-- Index for efficient round queries
CREATE INDEX IF NOT EXISTS idx_rounds_height_round ON rounds(height, round_number);
CREATE INDEX IF NOT EXISTS idx_rounds_height_desc ON rounds(height DESC);

-- Votes table to store individual validator votes
CREATE TABLE IF NOT EXISTS votes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    height INTEGER NOT NULL,
    round_number INTEGER NOT NULL,
    validator_address TEXT NOT NULL,
    vote_type INTEGER NOT NULL, -- 1 = prevote, 2 = precommit
    block_hash TEXT, -- NULL for nil votes
    signature TEXT,
    timestamp DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(height, round_number, validator_address, vote_type),
    FOREIGN KEY (height) REFERENCES heights(height),
    FOREIGN KEY (validator_address) REFERENCES validators(address)
);

-- Indexes for efficient vote queries
CREATE INDEX IF NOT EXISTS idx_votes_height_round ON votes(height, round_number);
CREATE INDEX IF NOT EXISTS idx_votes_validator ON votes(validator_address);
CREATE INDEX IF NOT EXISTS idx_votes_height_round_type ON votes(height, round_number, vote_type);

-- Consensus events table for tracking important consensus milestones
CREATE TABLE IF NOT EXISTS consensus_events (
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
CREATE INDEX IF NOT EXISTS idx_consensus_events_height_round ON consensus_events(height, round_number);
CREATE INDEX IF NOT EXISTS idx_consensus_events_type ON consensus_events(event_type);
CREATE INDEX IF NOT EXISTS idx_consensus_events_timestamp ON consensus_events(timestamp);

-- Validator snapshots table to track voting power changes over time
CREATE TABLE IF NOT EXISTS validator_snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    height INTEGER NOT NULL,
    validator_address TEXT NOT NULL,
    voting_power INTEGER NOT NULL,
    voting_power_percent REAL,
    is_proposer BOOLEAN DEFAULT FALSE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(height, validator_address),
    FOREIGN KEY (height) REFERENCES heights(height),
    FOREIGN KEY (validator_address) REFERENCES validators(address)
);

-- Index for efficient validator snapshot queries
CREATE INDEX IF NOT EXISTS idx_validator_snapshots_height ON validator_snapshots(height);
CREATE INDEX IF NOT EXISTS idx_validator_snapshots_validator ON validator_snapshots(validator_address);`

	// Execute migration
	_, err := d.db.Exec(migrationSQL)
	return err
}

// CleanupOldData removes data older than the configured retention period
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