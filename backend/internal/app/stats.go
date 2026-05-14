package app

import (
	"context"
	"fmt"
	"os"

	"horse.fit/scoop/internal/db"
)

type statsCommandConfig = noArgFormatCommandConfig

func runStats(args []string) int {
	return runParsedCommand(args, parseStatsCommand, executeStatsCommand)
}

func parseStatsCommand(args []string) (statsCommandConfig, int, bool) {
	return parseNoArgFormatCommand(args, "stats", outputFormatTable)
}

func executeStatsCommand(cfg statsCommandConfig) int {
	return runWithReadPool(cfg.timeout, cfg.envLoader, func(ctx context.Context, pool *db.Pool) int {
		dayStart := defaultUTCDay()
		_, dayEnd := utcDayBounds(dayStart)

		stats, err := pool.QueryPipelineStats(ctx, dayStart, dayEnd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to query pipeline stats: %v\n", err)
			return 1
		}
		return renderStats(stats, cfg.format)
	})
}

func renderStats(stats *db.PipelineStats, outputFormat string) int {
	return renderJSONOrTable(stats, outputFormat, func() error { return writeStatsTables(stats) })
}

func writeStatsTables(stats *db.PipelineStats) error {
	collectionRows := make([][]string, 0, len(stats.Collections)+1)
	for _, row := range stats.Collections {
		collectionRows = append(collectionRows, []string{
			row.Collection,
			fmt.Sprintf("%d", row.Articles),
			fmt.Sprintf("%d", row.Stories),
			fmt.Sprintf("%d", row.Embeddings),
		})
	}
	collectionRows = append(collectionRows, []string{
		"TOTAL",
		fmt.Sprintf("%d", stats.Totals.Articles),
		fmt.Sprintf("%d", stats.Totals.Stories),
		fmt.Sprintf("%d", stats.Totals.Embeddings),
	})

	if err := writeTable([]string{"collection", "articles", "stories", "embeddings"}, collectionRows); err != nil {
		return fmt.Errorf("failed to render collection table: %w", err)
	}

	fmt.Println()
	throughputRows := [][]string{
		{"articles_ingested_today", fmt.Sprintf("%d", stats.Throughput.ArticlesIngestedToday)},
		{"stories_created_today", fmt.Sprintf("%d", stats.Throughput.StoriesCreatedToday)},
		{"pending_not_embedded", fmt.Sprintf("%d", stats.Throughput.PendingNotEmbedded)},
		{"pending_not_deduped", fmt.Sprintf("%d", stats.Throughput.PendingNotDeduped)},
	}
	if err := writeTable([]string{"metric", "value"}, throughputRows); err != nil {
		return fmt.Errorf("failed to render throughput table: %w", err)
	}
	return nil
}
