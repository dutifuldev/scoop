package app

import (
	"context"
	"time"

	"horse.fit/scoop/internal/db"
)

type restoreCommandConfig = targetActionCommandConfig

var restoreCommandSpec = targetActionCommandSpec{
	commandName:  "restore",
	validTarget:  isRestoreTarget,
	usage:        printRestoreUsage,
	emptyMessage: "UUID must not be empty",
}

func runRestore(args []string) int {
	return runParsedCommand(args, parseRestoreCommand, executeRestoreCommand)
}

func parseRestoreCommand(args []string) (restoreCommandConfig, int, bool) {
	return parseTargetActionCommand(args, restoreCommandSpec)
}

func isRestoreTarget(target string) bool {
	return target == "story" || target == "article"
}

func executeRestoreCommand(cfg restoreCommandConfig) int {
	return executeConfirmedTargetAction(cfg, "restore", runRestoreTarget)
}

func runRestoreTarget(ctx context.Context, pool *db.Pool, cfg restoreCommandConfig, now time.Time) int {
	switch cfg.target {
	case "story":
		return runRestoreStory(ctx, pool, cfg.value, now, cfg.dryRun)
	default:
		return runRestoreArticle(ctx, pool, cfg.value, now, cfg.dryRun)
	}
}

func runRestoreStory(ctx context.Context, pool *db.Pool, storyUUID string, now time.Time, dryRun bool) int {
	return runSingleRowChange(ctx, pool, storyUUID, now, dryRun, storyRestoreChangeOptions())
}

func runRestoreArticle(ctx context.Context, pool *db.Pool, articleUUID string, now time.Time, dryRun bool) int {
	return runSingleRowChange(ctx, pool, articleUUID, now, dryRun, articleRestoreChangeOptions())
}

func previewStoryRestoreCount(ctx context.Context, pool *db.Pool, storyUUID string) (int64, error) {
	return previewDeletedStateCount(ctx, pool, "news.stories", "story_uuid", storyUUID, true)
}

func previewArticleRestoreCount(ctx context.Context, pool *db.Pool, articleUUID string) (int64, error) {
	return previewDeletedStateCount(ctx, pool, "news.articles", "article_uuid", articleUUID, true)
}

func storyRestoreChangeOptions() singleRowChangeOptions {
	return newSingleRowChangeOptions("stories_affected", "story restore", "restore story", previewStoryRestoreCount, restoreStory)
}

func articleRestoreChangeOptions() singleRowChangeOptions {
	return newSingleRowChangeOptions("articles_affected", "article restore", "restore article", previewArticleRestoreCount, restoreArticle)
}

func restoreStory(ctx context.Context, pool *db.Pool, id string, now time.Time) (int64, error) {
	return pool.RestoreStory(ctx, id, now)
}

func restoreArticle(ctx context.Context, pool *db.Pool, id string, now time.Time) (int64, error) {
	return pool.RestoreArticle(ctx, id, now)
}

func printRestoreUsage() {
	printUsageLines(
		"Usage:",
		"  scoop restore story <story_uuid> [--dry-run] [--force] [--env .env] [--timeout 30s]",
		"  scoop restore article <article_uuid> [--dry-run] [--force] [--env .env] [--timeout 30s]",
	)
}
