package db

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"main/pkg/db/sqlc"

	_ "modernc.org/sqlite"
)

//go:embed migrations/001_initial.sql
var migration001Initial string

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

	if _, err := d.db.Exec(migration001Initial); err != nil {
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
