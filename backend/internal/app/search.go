package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"horse.fit/scoop/internal/cli"
	"horse.fit/scoop/internal/db"
)

type searchCommandConfig struct {
	envLoader  *cli.EnvLoader
	timeout    time.Duration
	query      string
	collection string
	limit      int
	format     string
}

func runSearch(args []string) int {
	return runParsedCommand(args, parseSearchCommand, executeSearchCommand)
}

func parseSearchCommand(args []string) (searchCommandConfig, int, bool) {
	fs := newAppFlagSet("search")

	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 30*time.Second, "Command timeout")
	query := fs.String("query", "", "Query text for story title search")
	collection := fs.String("collection", "", "Optional collection filter")
	limit := fs.Int("limit", 20, "Maximum stories to return")
	format := fs.String("format", outputFormatTable, "Output format: table or json")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return searchCommandConfig{}, 0, false
		}
		return searchCommandConfig{}, 2, false
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "search does not accept positional arguments")
		return searchCommandConfig{}, 2, false
	}

	trimmedQuery := strings.TrimSpace(*query)
	if trimmedQuery == "" {
		fmt.Fprintln(os.Stderr, "--query is required")
		return searchCommandConfig{}, 2, false
	}
	if *limit <= 0 {
		fmt.Fprintln(os.Stderr, "--limit must be > 0")
		return searchCommandConfig{}, 2, false
	}

	outputFormat, err := parseOutputFormat(*format, outputFormatTable)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid format: %v\n", err)
		return searchCommandConfig{}, 2, false
	}
	return searchCommandConfig{
		envLoader:  envLoader,
		timeout:    *timeout,
		query:      trimmedQuery,
		collection: normalizeCollectionFlag(*collection),
		limit:      *limit,
		format:     outputFormat,
	}, 0, true
}

func executeSearchCommand(cfg searchCommandConfig) int {
	load := func(ctx context.Context, pool *db.Pool) ([]db.StorySummary, error) {
		return pool.SearchStoriesByTitle(ctx, cfg.query, cfg.collection, cfg.limit)
	}
	return runReadPoolList(cfg.timeout, cfg.envLoader, load, "Failed to search stories", renderStorySummaries, cfg.format)
}

func renderStorySummaries(stories []db.StorySummary, outputFormat string) int {
	return renderList(stories, outputFormat, writeStorySummaryTable)
}
