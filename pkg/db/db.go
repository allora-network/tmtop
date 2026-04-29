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

//go:embed migrations/002_drop_operator_unique.sql
var migration002DropOperatorUnique string

type migration struct {
	version string
	sql     string
}

// migrations is the ordered list of schema migrations. Each entry's
// SQL is executed only once; the version name is recorded in the
// migrations table.
var migrations = []migration{
	{"001_initial", migration001Initial},
	{"002_drop_operator_unique", migration002DropOperatorUnique},
}

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
	// From here on any error path needs to close the *sql.DB; Open
	// allocates a connection pool that won't be reclaimed by GC alone.
	closeOnErr := func(wrapped error) error {
		_ = db.Close()
		return wrapped
	}

	// Configure SQLite for better performance
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, closeOnErr(fmt.Errorf("failed to set WAL mode: %w", err))
	}
	if _, err := db.Exec("PRAGMA synchronous=NORMAL"); err != nil {
		return nil, closeOnErr(fmt.Errorf("failed to set synchronous mode: %w", err))
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return nil, closeOnErr(fmt.Errorf("failed to enable foreign keys: %w", err))
	}

	service := &DB{
		db:      db,
		queries: sqlc.New(db),
	}

	// Run migrations
	if err := service.migrate(); err != nil {
		return nil, closeOnErr(fmt.Errorf("failed to run migrations: %w", err))
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

// WithTx executes a function within a database transaction. The
// transaction is rolled back if fn returns an error or the commit
// fails. The deferred rollback after a successful commit is a no-op.
func (d *DB) WithTx(ctx context.Context, fn func(*sqlc.Queries) error) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := fn(d.queries.WithTx(tx)); err != nil {
		return fmt.Errorf("running tx body: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing tx: %w", err)
	}
	return nil
}

// migrate runs each migration once, in order, recording its version
// in the migrations table.
func (d *DB) migrate() error {
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

	for _, m := range migrations {
		var count int
		if err := d.db.QueryRow("SELECT COUNT(*) FROM migrations WHERE version = ?", m.version).Scan(&count); err != nil {
			return fmt.Errorf("checking migration %s: %w", m.version, err)
		}
		if count > 0 {
			continue
		}

		// Apply the migration and the version-record insert in a single
		// transaction. Without this, a successful migration that fails to
		// record its version would leave the schema half-applied: the next
		// startup would re-run the migration and trip a "table already
		// exists" / index-already-exists error, requiring manual recovery.
		if err := d.applyMigration(m); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) applyMigration(m migration) error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("starting migration %s tx: %w", m.version, err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(m.sql); err != nil {
		return fmt.Errorf("applying migration %s: %w", m.version, err)
	}
	if _, err := tx.Exec("INSERT INTO migrations (version) VALUES (?)", m.version); err != nil {
		return fmt.Errorf("recording migration %s: %w", m.version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing migration %s: %w", m.version, err)
	}
	return nil
}

// CleanupOldData removes data older than the configured retention
// period. Both MaxRetainBlocks and MaxRetainDays are honored — when
// both are set, the more aggressive (higher) cutoff wins so retention
// is bounded by the shorter window.
func (d *DB) CleanupOldData(ctx context.Context, config Config) error {
	cutoffHeight, err := d.computeCutoffHeight(ctx, config)
	if err != nil {
		return err
	}
	if cutoffHeight <= 0 {
		return nil
	}

	// Order matters here — foreign keys reference heights/validators.
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

func (d *DB) computeCutoffHeight(ctx context.Context, config Config) (int64, error) {
	var cutoffs []int64

	if config.MaxRetainBlocks > 0 {
		latest, err := d.queries.GetLatestHeight(ctx)
		if err != nil {
			if err == sql.ErrNoRows {
				return 0, nil
			}
			return 0, fmt.Errorf("failed to get latest height: %w", err)
		}
		cutoffs = append(cutoffs, latest.Height-config.MaxRetainBlocks)
	}

	if config.MaxRetainDays > 0 {
		cutoffTime := time.Now().AddDate(0, 0, -config.MaxRetainDays)
		// MIN(height) returns NULL when no row matches; sqlc surfaces
		// that as interface{}.
		raw, err := d.queries.GetMinHeightAfterTime(ctx, sql.NullTime{Time: cutoffTime, Valid: true})
		if err != nil && err != sql.ErrNoRows {
			return 0, fmt.Errorf("failed to get height for time cutoff: %w", err)
		}
		if h, ok := raw.(int64); ok && h > 0 {
			cutoffs = append(cutoffs, h-1)
		}
	}

	if len(cutoffs) == 0 {
		return 0, nil
	}

	// Most aggressive cutoff wins so retention is bounded by the
	// tighter of the two configured windows.
	cutoff := cutoffs[0]
	for _, c := range cutoffs[1:] {
		if c > cutoff {
			cutoff = c
		}
	}
	return cutoff, nil
}
