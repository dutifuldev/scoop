package app

import (
	"context"
	"fmt"
	"os"

	"horse.fit/scoop/internal/db"
)

type collectionsCommandConfig = noArgFormatCommandConfig

func runCollections(args []string) int {
	return runParsedCommand(args, parseCollectionsCommand, executeCollectionsCommand)
}

func parseCollectionsCommand(args []string) (collectionsCommandConfig, int, bool) {
	return parseNoArgFormatCommand(args, "collections", outputFormatTable)
}

func executeCollectionsCommand(cfg collectionsCommandConfig) int {
	return runWithReadPool(cfg.timeout, cfg.envLoader, func(ctx context.Context, pool *db.Pool) int {
		rows, err := pool.ListCollectionsWithCounts(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to query collections: %v\n", err)
			return 1
		}
		return renderCollections(rows, cfg.format)
	})
}

func renderCollections(rows []db.CollectionCount, outputFormat string) int {
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

	if err := writeCollectionsTable(tableRows); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to render table: %v\n", err)
		return 1
	}

	return 0
}

func writeCollectionsTable(rows [][]string) error {
	return writeTable(
		[]string{"collection", "translation_mode", "article_count", "story_count", "earliest_article", "latest_article"},
		rows,
	)
}
