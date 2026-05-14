package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"horse.fit/scoop/internal/db"
)

type deleteCommandConfig = targetActionCommandConfig

var deleteCommandSpec = targetActionCommandSpec{
	commandName:  "delete",
	validTarget:  isDeleteTarget,
	usage:        printDeleteUsage,
	emptyMessage: "delete argument must not be empty",
}

func runDelete(args []string) int {
	return runParsedCommand(args, parseDeleteCommand, executeDeleteCommand)
}

func parseDeleteCommand(args []string) (deleteCommandConfig, int, bool) {
	return parseTargetActionCommand(args, deleteCommandSpec)
}

func isDeleteTarget(target string) bool {
	return stringSliceContains([]string{"story", "article", "collection", "before"}, target)
}

func executeDeleteCommand(cfg deleteCommandConfig) int {
	return executeConfirmedTargetAction(cfg, "delete", runDeleteTarget)
}

func runDeleteTarget(ctx context.Context, pool *db.Pool, cfg deleteCommandConfig, now time.Time) int {
	switch cfg.target {
	case "story":
		return runDeleteStory(ctx, pool, cfg.value, now, cfg.dryRun)
	case "article":
		return runDeleteArticle(ctx, pool, cfg.value, now, cfg.dryRun)
	case "collection":
		return runDeleteCollection(ctx, pool, cfg.value, now, cfg.dryRun)
	default:
		return runDeleteBefore(ctx, pool, cfg.value, now, cfg.dryRun)
	}
}

func runDeleteStory(ctx context.Context, pool *db.Pool, storyUUID string, now time.Time, dryRun bool) int {
	return runSingleRowChange(ctx, pool, storyUUID, now, dryRun, storyDeleteChangeOptions())
}

func runDeleteArticle(ctx context.Context, pool *db.Pool, articleUUID string, now time.Time, dryRun bool) int {
	return runSingleRowChange(ctx, pool, articleUUID, now, dryRun, articleDeleteChangeOptions())
}

func runDeleteCollection(ctx context.Context, pool *db.Pool, collection string, now time.Time, dryRun bool) int {
	normalizedCollection := normalizeCollectionFlag(collection)
	if normalizedCollection == "" {
		fmt.Fprintln(os.Stderr, "collection must not be empty")
		return 2
	}

	preview, err := previewCollectionDeleteCounts(ctx, pool, normalizedCollection)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to preview collection delete: %v\n", err)
		return 1
	}
	if dryRun {
		fmt.Printf("dry_run=true raw_arrivals_affected=%d articles_affected=%d stories_affected=%d\n", preview.RawArrivals, preview.Articles, preview.Stories)
		return 0
	}

	result, err := pool.SoftDeleteCollection(ctx, normalizedCollection, now)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to soft delete collection: %v\n", err)
		return 1
	}
	fmt.Printf("raw_arrivals_affected=%d articles_affected=%d stories_affected=%d\n", result.RawArrivals, result.Articles, result.Stories)
	return 0
}

func runDeleteBefore(ctx context.Context, pool *db.Pool, beforeArg string, now time.Time, dryRun bool) int {
	before, err := parseDeleteBeforeArgument(beforeArg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid before value: %v\n", err)
		return 2
	}
	preview, err := previewBeforeDeleteCounts(ctx, pool, before)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to preview before delete: %v\n", err)
		return 1
	}
	if dryRun {
		return printDeleteBeforePreview(before, preview)
	}
	return applyDeleteBefore(ctx, pool, before, now)
}

func applyDeleteBefore(ctx context.Context, pool *db.Pool, before time.Time, now time.Time) int {
	result, err := pool.SoftDeleteBefore(ctx, before, now)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to soft delete rows before cutoff: %v\n", err)
		return 1
	}
	return printDeleteBeforeResult(before, result)
}

func printDeleteBeforePreview(before time.Time, preview db.SoftDeleteBeforeResult) int {
	return printDeleteBeforeCounts("dry_run=true ", before, preview)
}

func printDeleteBeforeResult(before time.Time, result db.SoftDeleteBeforeResult) int {
	return printDeleteBeforeCounts("", before, result)
}

func printDeleteBeforeCounts(prefix string, before time.Time, result db.SoftDeleteBeforeResult) int {
	fmt.Printf(
		"%sbefore=%s raw_arrivals_affected=%d articles_affected=%d stories_affected=%d\n",
		prefix,
		before.UTC().Format(time.RFC3339),
		result.RawArrivals,
		result.Articles,
		result.Stories,
	)
	return 0
}

func previewStoryDeleteCount(ctx context.Context, pool *db.Pool, storyUUID string) (int64, error) {
	return previewDeletedStateCount(ctx, pool, "news.stories", "story_uuid", storyUUID, false)
}

func previewArticleDeleteCount(ctx context.Context, pool *db.Pool, articleUUID string) (int64, error) {
	return previewDeletedStateCount(ctx, pool, "news.articles", "article_uuid", articleUUID, false)
}

func previewDeletedStateCount(ctx context.Context, pool *db.Pool, tableName, uuidColumn, uuid string, deleted bool) (int64, error) {
	deletedCondition := "deleted_at IS NULL"
	if deleted {
		deletedCondition = "deleted_at IS NOT NULL"
	}
	q := fmt.Sprintf(`
SELECT COUNT(*)
FROM %s
WHERE %s = $1::uuid
  AND %s
`, tableName, uuidColumn, deletedCondition)
	var count int64
	if err := pool.QueryRow(ctx, q, strings.TrimSpace(uuid)).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

type singleRowChangeOptions struct {
	affectedLabel string
	previewError  string
	applyError    string
	preview       func(context.Context, *db.Pool, string) (int64, error)
	apply         func(context.Context, *db.Pool, string, time.Time) (int64, error)
}

func storyDeleteChangeOptions() singleRowChangeOptions {
	return newSingleRowChangeOptions("stories_affected", "story delete", "soft delete story", previewStoryDeleteCount, softDeleteStory)
}

func articleDeleteChangeOptions() singleRowChangeOptions {
	return newSingleRowChangeOptions("articles_affected", "article delete", "soft delete article", previewArticleDeleteCount, softDeleteArticle)
}

func newSingleRowChangeOptions(
	affectedLabel string,
	previewAction string,
	applyAction string,
	preview func(context.Context, *db.Pool, string) (int64, error),
	apply func(context.Context, *db.Pool, string, time.Time) (int64, error),
) singleRowChangeOptions {
	return singleRowChangeOptions{
		affectedLabel: affectedLabel,
		previewError:  fmt.Sprintf("Failed to preview %s", previewAction),
		applyError:    fmt.Sprintf("Failed to %s", applyAction),
		preview:       preview,
		apply:         apply,
	}
}

func softDeleteStory(ctx context.Context, pool *db.Pool, id string, now time.Time) (int64, error) {
	return pool.SoftDeleteStory(ctx, id, now)
}

func softDeleteArticle(ctx context.Context, pool *db.Pool, id string, now time.Time) (int64, error) {
	return pool.SoftDeleteArticle(ctx, id, now)
}

func runSingleRowChange(ctx context.Context, pool *db.Pool, id string, now time.Time, dryRun bool, opts singleRowChangeOptions) int {
	previewCount, err := opts.preview(ctx, pool, id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", opts.previewError, err)
		return 1
	}
	if dryRun {
		fmt.Printf("dry_run=true %s=%d\n", opts.affectedLabel, previewCount)
		return 0
	}

	affected, err := opts.apply(ctx, pool, id, now)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", opts.applyError, err)
		return 1
	}
	fmt.Printf("%s=%d\n", opts.affectedLabel, affected)
	return 0
}

func previewCollectionDeleteCounts(ctx context.Context, pool *db.Pool, collection string) (db.SoftDeleteCollectionResult, error) {
	const rawArrivalsQ = `
SELECT COUNT(*)
FROM news.raw_arrivals
WHERE collection = $1
  AND deleted_at IS NULL
`
	const articlesQ = `
SELECT COUNT(*)
FROM news.articles
WHERE collection = $1
  AND deleted_at IS NULL
`
	const storiesQ = `
SELECT COUNT(*)
FROM news.stories
WHERE collection = $1
  AND deleted_at IS NULL
`
	counts, err := previewDeleteCounts(ctx, pool, collection, []string{rawArrivalsQ, articlesQ, storiesQ})
	return db.SoftDeleteCollectionResult{RawArrivals: counts[0], Articles: counts[1], Stories: counts[2]}, err
}

func previewBeforeDeleteCounts(ctx context.Context, pool *db.Pool, before time.Time) (db.SoftDeleteBeforeResult, error) {
	const rawArrivalsQ = `
SELECT COUNT(*)
FROM news.raw_arrivals
WHERE fetched_at < $1
  AND deleted_at IS NULL
`
	const articlesQ = `
SELECT COUNT(*)
FROM news.articles
WHERE created_at < $1
  AND deleted_at IS NULL
`
	const storiesQ = `
SELECT COUNT(*)
FROM news.stories
WHERE last_seen_at < $1
  AND deleted_at IS NULL
`
	counts, err := previewDeleteCounts(ctx, pool, before.UTC(), []string{rawArrivalsQ, articlesQ, storiesQ})
	return db.SoftDeleteBeforeResult{RawArrivals: counts[0], Articles: counts[1], Stories: counts[2]}, err
}

func previewDeleteCounts(ctx context.Context, pool *db.Pool, arg any, queries []string) ([3]int64, error) {
	var counts [3]int64
	for idx, query := range queries {
		if err := pool.QueryRow(ctx, query, arg).Scan(&counts[idx]); err != nil {
			return [3]int64{}, err
		}
	}
	return counts, nil
}

func parseDeleteBeforeArgument(raw string) (time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("date/time is required")
	}

	if ts, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return ts.UTC(), nil
	}

	day, err := parseUTCDate(trimmed)
	if err != nil {
		return time.Time{}, fmt.Errorf("must be RFC3339 or YYYY-MM-DD")
	}
	return day.UTC(), nil
}

func confirmDangerousAction(prompt string) (bool, error) {
	fmt.Fprintf(os.Stderr, "%s [y/N]: ", strings.TrimSpace(prompt))
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}

	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func printDeleteUsage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  scoop delete story <story_uuid> [--dry-run] [--force] [--env .env] [--timeout 30s]")
	fmt.Fprintln(os.Stderr, "  scoop delete article <article_uuid> [--dry-run] [--force] [--env .env] [--timeout 30s]")
	fmt.Fprintln(os.Stderr, "  scoop delete collection <collection> [--dry-run] [--force] [--env .env] [--timeout 30s]")
	fmt.Fprintln(os.Stderr, "  scoop delete before <RFC3339|YYYY-MM-DD> [--dry-run] [--force] [--env .env] [--timeout 30s]")
}
