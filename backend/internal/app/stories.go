package app

import (
	"context"
	"fmt"

	"horse.fit/scoop/internal/db"
)

type storiesCommandConfig = windowListCommandConfig

func runStories(args []string) int {
	return runParsedCommand(args, parseStoriesCommand, executeStoriesCommand)
}

func parseStoriesCommand(args []string) (storiesCommandConfig, int, bool) {
	return parseWindowListCommand(args, "stories", "Maximum stories to return")
}

func executeStoriesCommand(cfg storiesCommandConfig) int {
	return runWindowList(cfg, loadStorySummaries, "Failed to query stories", renderStorySummaries)
}

func loadStorySummaries(ctx context.Context, pool *db.Pool, cfg windowListCommandConfig) ([]db.StorySummary, error) {
	return pool.ListStoriesByDedupEventWindow(ctx, db.StoryEventListOptions{
		Collection: cfg.collection,
		From:       cfg.from,
		To:         cfg.to,
		Limit:      cfg.limit,
	})
}

func writeStorySummaryTable(items []db.StorySummary) error {
	rows := make([][]string, 0, len(items))
	for _, item := range items {
		createdAt := item.CreatedAt
		if item.EventCreatedAt != nil {
			createdAt = item.EventCreatedAt.UTC()
		}
		rows = append(rows, []string{
			fmt.Sprintf("%d", item.StoryID),
			truncateForTable(item.CanonicalTitle, 80),
			pointerStringOrEmpty(item.SourceDomain),
			fmt.Sprintf("%d", item.ArticleCount),
			formatUTCDate(createdAt),
		})
	}

	return writeTable(
		[]string{"story_id", "canonical_title", "source_domain", "article_count", "created_date"},
		rows,
	)
}
