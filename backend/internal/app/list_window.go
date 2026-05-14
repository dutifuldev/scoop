package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"horse.fit/scoop/internal/cli"
	"horse.fit/scoop/internal/db"
)

type windowListCommandConfig struct {
	envLoader  *cli.EnvLoader
	timeout    time.Duration
	collection string
	from       time.Time
	to         time.Time
	limit      int
	format     string
}

func parseWindowListCommand(args []string, name string, limitHelp string) (windowListCommandConfig, int, bool) {
	fs := newAppFlagSet(name)

	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 30*time.Second, "Command timeout")
	collection := fs.String("collection", "", "Filter by collection")
	from := fs.String("from", defaultUTCDayString(), "Start date in YYYY-MM-DD (UTC)")
	to := fs.String("to", defaultUTCDayString(), "End date in YYYY-MM-DD (UTC)")
	limit := fs.Int("limit", 50, limitHelp)
	format := fs.String("format", outputFormatTable, "Output format: table or json")

	if exitCode, ok := parseAppFlagSet(fs, args); !ok {
		return windowListCommandConfig{}, exitCode, false
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "%s does not accept positional arguments\n", name)
		return windowListCommandConfig{}, 2, false
	}
	if *limit <= 0 {
		fmt.Fprintln(os.Stderr, "--limit must be > 0")
		return windowListCommandConfig{}, 2, false
	}

	outputFormat, err := parseOutputFormat(*format, outputFormatTable)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid format: %v\n", err)
		return windowListCommandConfig{}, 2, false
	}

	fromStart, toEnd, err := parseUTCDateRange(*from, *to)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid date range: %v\n", err)
		return windowListCommandConfig{}, 2, false
	}
	return windowListCommandConfig{
		envLoader:  envLoader,
		timeout:    *timeout,
		collection: normalizeCollectionFlag(*collection),
		from:       fromStart,
		to:         toEnd,
		limit:      *limit,
		format:     outputFormat,
	}, 0, true
}

func runWindowList[T any](
	cfg windowListCommandConfig,
	load func(context.Context, *db.Pool, windowListCommandConfig) ([]T, error),
	failureMessage string,
	render func([]T, string) int,
) int {
	return runReadPoolList(
		cfg.timeout,
		cfg.envLoader,
		func(ctx context.Context, pool *db.Pool) ([]T, error) {
			return load(ctx, pool, cfg)
		},
		failureMessage,
		render,
		cfg.format,
	)
}
