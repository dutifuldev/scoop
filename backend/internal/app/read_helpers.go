package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"
	"unicode/utf8"

	"horse.fit/scoop/internal/cli"
	"horse.fit/scoop/internal/config"
	"horse.fit/scoop/internal/db"
	"horse.fit/scoop/internal/globaltime"
)

const (
	outputFormatTable = "table"
	outputFormatJSON  = "json"
)

func defaultUTCDay() time.Time {
	now := globaltime.UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

func defaultUTCDayString() string {
	return defaultUTCDay().Format("2006-01-02")
}

func parseOutputFormat(raw, defaultFormat string) (string, error) {
	format := strings.TrimSpace(strings.ToLower(raw))
	if format == "" {
		format = strings.TrimSpace(strings.ToLower(defaultFormat))
	}
	switch format {
	case outputFormatTable, outputFormatJSON:
		return format, nil
	default:
		return "", fmt.Errorf("--format must be table or json")
	}
}

func parseUTCDate(raw string) (time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("date is required")
	}
	day, err := time.Parse("2006-01-02", trimmed)
	if err != nil {
		return time.Time{}, fmt.Errorf("must be YYYY-MM-DD")
	}
	return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC), nil
}

func parseUTCDateRange(fromRaw, toRaw string) (time.Time, time.Time, error) {
	fromDay, err := parseUTCDate(fromRaw)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid from date: %w", err)
	}
	toDay, err := parseUTCDate(toRaw)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid to date: %w", err)
	}
	if toDay.Before(fromDay) {
		return time.Time{}, time.Time{}, fmt.Errorf("--from must be <= --to")
	}

	fromStart, _ := utcDayBounds(fromDay)
	_, toEnd := utcDayBounds(toDay)
	return fromStart, toEnd, nil
}

func utcDayBounds(day time.Time) (time.Time, time.Time) {
	start := time.Date(day.UTC().Year(), day.UTC().Month(), day.UTC().Day(), 0, 0, 0, 0, time.UTC)
	return start, start.Add(24 * time.Hour)
}

func normalizeCollectionFlag(raw string) string {
	return strings.TrimSpace(strings.ToLower(raw))
}

func truncateForTable(value string, maxLen int) string {
	trimmed := strings.TrimSpace(value)
	if maxLen <= 0 {
		return trimmed
	}
	if utf8.RuneCountInString(trimmed) <= maxLen {
		return trimmed
	}

	runes := []rune(trimmed)
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}

func pointerStringOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func formatUTCDate(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format("2006-01-02")
}

func formatUTCTimestamp(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func formatUTCTimestampPtr(value *time.Time) string {
	if value == nil || value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func printJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func renderJSONOrTable(value any, outputFormat string, writeRows func() error) int {
	if outputFormat == outputFormatJSON {
		if err := printJSON(value); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to encode JSON: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeRows(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	return 0
}

func writeTable(headers []string, rows [][]string) error {
	writer := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, strings.Join(headers, "\t")); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := fmt.Fprintln(writer, strings.Join(row, "\t")); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func printUsageLines(lines ...string) {
	for _, line := range lines {
		fmt.Fprintln(os.Stderr, line)
	}
}

func connectReadPool(timeout time.Duration, envLoader *cli.EnvLoader) (context.Context, context.CancelFunc, *db.Pool, error) {
	if envLoader != nil {
		if _, err := envLoader.Load(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load config: %w", err)
	}

	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	pool, err := db.NewPool(ctx, cfg)
	if err != nil {
		cancel()
		return nil, nil, nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return ctx, cancel, pool, nil
}
