package app

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"horse.fit/scoop/internal/cli"
)

func runCollections(args []string) int {
	fs := flag.NewFlagSet("collections", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 30*time.Second, "Command timeout")
	format := fs.String("format", outputFormatTable, "Output format: table or json")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "collections does not accept positional arguments")
		return 2
	}

	outputFormat, err := parseOutputFormat(*format, outputFormatTable)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid format: %v\n", err)
		return 2
	}

	ctx, cancel, pool, err := connectReadPool(*timeout, envLoader)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer cancel()
	defer pool.Close()

	rows, err := pool.ListCollectionsWithCounts(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to query collections: %v\n", err)
		return 1
	}

	if outputFormat == outputFormatJSON {
		if err := printJSON(rows); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to encode JSON: %v\n", err)
			return 1
		}
		return 0
	}

	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		tableRows = append(tableRows, []string{
			row.Collection,
			row.TranslationMode,
			fmt.Sprintf("%d", row.ArticleCount),
			fmt.Sprintf("%d", row.StoryCount),
			formatUTCTimestampPtr(row.EarliestArticleAt),
			formatUTCTimestampPtr(row.LatestArticleAt),
		})
	}

	if err := writeTable(
		[]string{"collection", "translation_mode", "article_count", "story_count", "earliest_article", "latest_article"},
		tableRows,
	); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to render table: %v\n", err)
		return 1
	}

	return 0
}
