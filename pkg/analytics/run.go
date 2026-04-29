package analytics

import (
	"context"
	"fmt"
	"os"

	configPkg "main/pkg/config"
	"main/pkg/db"

	"github.com/rs/zerolog"
)

// Run executes the CLI-only analytics command selected by
// cfg.AnalyticsCommand. It is invoked by the main entry point when
// the --analytics flag is set, and does NOT spin up the TUI. Exits
// the process on argument validation failure or command error.
func Run(cfg *configPkg.Config, database *db.DB, logger zerolog.Logger) {
	if database == nil {
		fmt.Fprintln(os.Stderr, "Error: analytics mode requires database to be enabled (--database-path or a non-zero retention).")
		os.Exit(1)
	}

	cli := NewCLIAnalytics(database, logger)
	ctx := context.Background()

	switch cfg.AnalyticsCommand {
	case "performance":
		requireValidator(cfg, "performance analysis")
		if err := cli.PrintValidatorPerformance(ctx, cfg.AnalyticsValidator, cfg.AnalyticsTimeWindow); err != nil {
			fatal("performance analysis", err)
		}

	case "rankings":
		if err := cli.PrintValidatorRankings(ctx, cfg.AnalyticsTimeWindow, 20); err != nil {
			fatal("rankings analysis", err)
		}

	case "timeseries":
		requireValidator(cfg, "time series analysis")
		if err := cli.PrintPerformanceTimeSeries(ctx, cfg.AnalyticsValidator, cfg.AnalyticsTimeWindow); err != nil {
			fatal("time series analysis", err)
		}

	case "debug":
		if err := cli.PrintDatabaseSummary(ctx); err != nil {
			fatal("database debug", err)
		}

	case "diagnose":
		requireValidator(cfg, "validator diagnosis")
		if err := cli.DiagnoseValidator(ctx, cfg.AnalyticsValidator, cfg.AnalyticsTimeWindow); err != nil {
			fatal("validator diagnosis", err)
		}

	case "search":
		requireValidator(cfg, "validator search (use --analytics-validator as search term)")
		if err := cli.SearchValidators(ctx, cfg.AnalyticsValidator); err != nil {
			fatal("validator search", err)
		}

	default:
		fmt.Fprintf(os.Stderr, "Error: unknown analytics command %q. Available: performance, rankings, timeseries, debug, diagnose, search\n", cfg.AnalyticsCommand)
		os.Exit(1)
	}
}

func requireValidator(cfg *configPkg.Config, what string) {
	if cfg.AnalyticsValidator == "" {
		fmt.Fprintf(os.Stderr, "Error: --analytics-validator is required for %s\n", what)
		os.Exit(1)
	}
}

func fatal(what string, err error) {
	fmt.Fprintf(os.Stderr, "Error running %s: %v\n", what, err)
	os.Exit(1)
}
