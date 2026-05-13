package app

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"horse.fit/scoop/internal/cli"
	"horse.fit/scoop/internal/db"
	"horse.fit/scoop/internal/globaltime"
)

func runDelete(args []string) int {
	if len(args) == 0 {
		printDeleteUsage()
		return 2
	}

	target := strings.ToLower(strings.TrimSpace(args[0]))
	switch target {
	case "story", "article", "collection", "before":
	default:
		fmt.Fprintf(os.Stderr, "Unknown delete target: %s\n\n", args[0])
		printDeleteUsage()
		return 2
	}

	fs := flag.NewFlagSet("delete "+target, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 30*time.Second, "Command timeout")
	dryRun := fs.Bool("dry-run", false, "Preview affected rows without applying changes")
	force := fs.Bool("force", false, "Skip confirmation prompt")

	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "delete requires one argument")
		printDeleteUsage()
		return 2
	}

	arg := strings.TrimSpace(fs.Arg(0))
	if arg == "" {
		fmt.Fprintln(os.Stderr, "delete argument must not be empty")
		return 2
	}

	if !*force {
		ok, err := confirmDangerousAction(fmt.Sprintf("Proceed with delete %s %q?", target, arg))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read confirmation: %v\n", err)
			return 1
		}
		if !ok {
			fmt.Fprintln(os.Stderr, "Cancelled")
			return 1
		}
	}

	ctx, cancel, pool, err := connectReadPool(*timeout, envLoader)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer cancel()
	defer pool.Close()

	now := globaltime.UTC()
	switch target {
	case "story":
		return runDeleteStory(ctx, pool, arg, now, *dryRun)
	case "article":
		return runDeleteArticle(ctx, pool, arg, now, *dryRun)
	case "collection":
		return runDeleteCollection(ctx, pool, arg, now, *dryRun)
	default:
		return runDeleteBefore(ctx, pool, arg, now, *dryRun)
	}
}

func runDeleteStory(ctx context.Context, pool *db.Pool, storyUUID string, now time.Time, dryRun bool) int {
	return runSingleRowChange(ctx, pool, storyUUID, now, dryRun, singleRowChangeOptions{
		affectedLabel: "stories_affected",
		previewError:  "Failed to preview story delete",
		applyError:    "Failed to soft delete story",
		preview:       previewStoryDeleteCount,
		apply: func(ctx context.Context, pool *db.Pool, id string, now time.Time) (int64, error) {
			return pool.SoftDeleteStory(ctx, id, now)
		},
	})
}

func runDeleteArticle(ctx context.Context, pool *db.Pool, articleUUID string, now time.Time, dryRun bool) int {
	return runSingleRowChange(ctx, pool, articleUUID, now, dryRun, singleRowChangeOptions{
		affectedLabel: "articles_affected",
		previewError:  "Failed to preview article delete",
		applyError:    "Failed to soft delete article",
		preview:       previewArticleDeleteCount,
		apply: func(ctx context.Context, pool *db.Pool, id string, now time.Time) (int64, error) {
			return pool.SoftDeleteArticle(ctx, id, now)
		},
	})
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
		fmt.Printf(
			"dry_run=true before=%s raw_arrivals_affected=%d articles_affected=%d stories_affected=%d\n",
			before.UTC().Format(time.RFC3339),
			preview.RawArrivals,
			preview.Articles,
			preview.Stories,
		)
		return 0
	}

	result, err := pool.SoftDeleteBefore(ctx, before, now)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to soft delete rows before cutoff: %v\n", err)
		return 1
	}
	fmt.Printf(
		"before=%s raw_arrivals_affected=%d articles_affected=%d stories_affected=%d\n",
		before.UTC().Format(time.RFC3339),
		result.RawArrivals,
		result.Articles,
		result.Stories,
	)
	return 0
}

func previewStoryDeleteCount(ctx context.Context, pool *db.Pool, storyUUID string) (int64, error) {
	const q = `
SELECT COUNT(*)
FROM news.stories
WHERE story_uuid = $1::uuid
  AND deleted_at IS NULL
`
	var count int64
	if err := pool.QueryRow(ctx, q, strings.TrimSpace(storyUUID)).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func previewArticleDeleteCount(ctx context.Context, pool *db.Pool, articleUUID string) (int64, error) {
	const q = `
SELECT COUNT(*)
FROM news.articles
WHERE article_uuid = $1::uuid
  AND deleted_at IS NULL
`
	var count int64
	if err := pool.QueryRow(ctx, q, strings.TrimSpace(articleUUID)).Scan(&count); err != nil {
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
	var result db.SoftDeleteCollectionResult

	const rawArrivalsQ = `
SELECT COUNT(*)
FROM news.raw_arrivals
WHERE collection = $1
  AND deleted_at IS NULL
`
	if err := pool.QueryRow(ctx, rawArrivalsQ, collection).Scan(&result.RawArrivals); err != nil {
		return db.SoftDeleteCollectionResult{}, err
	}

	const articlesQ = `
SELECT COUNT(*)
FROM news.articles
WHERE collection = $1
  AND deleted_at IS NULL
`
	if err := pool.QueryRow(ctx, articlesQ, collection).Scan(&result.Articles); err != nil {
		return db.SoftDeleteCollectionResult{}, err
	}

	const storiesQ = `
SELECT COUNT(*)
FROM news.stories
WHERE collection = $1
  AND deleted_at IS NULL
`
	if err := pool.QueryRow(ctx, storiesQ, collection).Scan(&result.Stories); err != nil {
		return db.SoftDeleteCollectionResult{}, err
	}

	return result, nil
}

func previewBeforeDeleteCounts(ctx context.Context, pool *db.Pool, before time.Time) (db.SoftDeleteBeforeResult, error) {
	var result db.SoftDeleteBeforeResult

	const rawArrivalsQ = `
SELECT COUNT(*)
FROM news.raw_arrivals
WHERE fetched_at < $1
  AND deleted_at IS NULL
`
	if err := pool.QueryRow(ctx, rawArrivalsQ, before.UTC()).Scan(&result.RawArrivals); err != nil {
		return db.SoftDeleteBeforeResult{}, err
	}

	const articlesQ = `
SELECT COUNT(*)
FROM news.articles
WHERE created_at < $1
  AND deleted_at IS NULL
`
	if err := pool.QueryRow(ctx, articlesQ, before.UTC()).Scan(&result.Articles); err != nil {
		return db.SoftDeleteBeforeResult{}, err
	}

	const storiesQ = `
SELECT COUNT(*)
FROM news.stories
WHERE last_seen_at < $1
  AND deleted_at IS NULL
`
	if err := pool.QueryRow(ctx, storiesQ, before.UTC()).Scan(&result.Stories); err != nil {
		return db.SoftDeleteBeforeResult{}, err
	}

	return result, nil
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
