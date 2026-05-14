package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"horse.fit/scoop/internal/cli"
	"horse.fit/scoop/internal/db"
)

type digestOutput struct {
	Date       string            `json:"date"`
	Collection string            `json:"collection"`
	Today      []db.StorySummary `json:"today"`
	Yesterday  []db.StorySummary `json:"yesterday"`
}

type digestCommandConfig struct {
	envLoader  *cli.EnvLoader
	timeout    time.Duration
	collection string
	date       time.Time
	format     string
}

func runDigest(args []string) int {
	return runParsedCommand(args, parseDigestCommand, executeDigestCommand)
}

func parseDigestCommand(args []string) (digestCommandConfig, int, bool) {
	fs := newAppFlagSet("digest")

	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 30*time.Second, "Command timeout")
	collection := fs.String("collection", "", "Target collection (required)")
	date := fs.String("date", defaultUTCDayString(), "Target date in YYYY-MM-DD (UTC)")
	format := fs.String("format", outputFormatJSON, "Output format: table or json")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return digestCommandConfig{}, 0, false
		}
		return digestCommandConfig{}, 2, false
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "digest does not accept positional arguments")
		return digestCommandConfig{}, 2, false
	}

	targetCollection := normalizeCollectionFlag(*collection)
	if targetCollection == "" {
		fmt.Fprintln(os.Stderr, "--collection is required")
		return digestCommandConfig{}, 2, false
	}

	outputFormat, err := parseOutputFormat(*format, outputFormatJSON)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid format: %v\n", err)
		return digestCommandConfig{}, 2, false
	}

	targetDay, err := parseUTCDate(*date)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid --date: %v\n", err)
		return digestCommandConfig{}, 2, false
	}
	return digestCommandConfig{
		envLoader:  envLoader,
		timeout:    *timeout,
		collection: targetCollection,
		date:       targetDay,
		format:     outputFormat,
	}, 0, true
}

func executeDigestCommand(cfg digestCommandConfig) int {
	return runReadPoolValue(
		cfg.timeout,
		cfg.envLoader,
		func(ctx context.Context, pool *db.Pool) (digestOutput, error) { return queryDigest(ctx, pool, cfg) },
		func(result digestOutput) int { return renderDigest(result, cfg.format) },
	)
}

func queryDigest(ctx context.Context, pool *db.Pool, cfg digestCommandConfig) (digestOutput, error) {
	dayStart, dayEnd := utcDayBounds(cfg.date)
	yesterdayStart, yesterdayEnd := utcDayBounds(cfg.date.AddDate(0, 0, -1))
	todayStories, err := pool.ListDigestStories(ctx, cfg.collection, dayStart, dayEnd)
	if err != nil {
		return digestOutput{}, fmt.Errorf("failed to query today's digest stories: %w", err)
	}
	yesterdayStories, err := pool.ListDigestStories(ctx, cfg.collection, yesterdayStart, yesterdayEnd)
	if err != nil {
		return digestOutput{}, fmt.Errorf("failed to query yesterday's digest stories: %w", err)
	}
	return digestOutput{
		Date:       cfg.date.Format("2006-01-02"),
		Collection: cfg.collection,
		Today:      todayStories,
		Yesterday:  yesterdayStories,
	}, nil
}

func renderDigest(result digestOutput, outputFormat string) int {
	return renderJSONOrTable(result, outputFormat, func() error { return writeDigestTables(result) })
}

func writeDigestTables(result digestOutput) error {
	fmt.Printf("date: %s\n", result.Date)
	fmt.Printf("collection: %s\n\n", result.Collection)

	fmt.Println("today")
	if err := writeStorySummaryTable(result.Today); err != nil {
		return fmt.Errorf("failed to render today table: %w", err)
	}

	fmt.Println()
	fmt.Println("yesterday")
	if err := writeStorySummaryTable(result.Yesterday); err != nil {
		return fmt.Errorf("failed to render yesterday table: %w", err)
	}
	return nil
}
