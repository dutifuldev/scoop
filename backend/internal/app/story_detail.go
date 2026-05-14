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

type storyDetailCommandConfig struct {
	envLoader *cli.EnvLoader
	timeout   time.Duration
	format    string
	storyUUID string
}

func runStoryDetail(args []string) int {
	return runParsedCommand(args, parseStoryDetailCommand, executeStoryDetailCommand)
}

func parseStoryDetailCommand(args []string) (storyDetailCommandConfig, int, bool) {
	fs := newAppFlagSet("story")

	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 30*time.Second, "Command timeout")
	format := fs.String("format", outputFormatTable, "Output format: table or json")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return storyDetailCommandConfig{}, 0, false
		}
		return storyDetailCommandConfig{}, 2, false
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "Usage: scoop story <story_uuid> [--format table|json]")
		return storyDetailCommandConfig{}, 2, false
	}

	outputFormat, err := parseOutputFormat(*format, outputFormatTable)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid format: %v\n", err)
		return storyDetailCommandConfig{}, 2, false
	}

	storyUUID := strings.TrimSpace(fs.Arg(0))
	if storyUUID == "" {
		fmt.Fprintln(os.Stderr, "story_uuid is required")
		return storyDetailCommandConfig{}, 2, false
	}
	return storyDetailCommandConfig{
		envLoader: envLoader,
		timeout:   *timeout,
		format:    outputFormat,
		storyUUID: storyUUID,
	}, 0, true
}

func executeStoryDetailCommand(cfg storyDetailCommandConfig) int {
	return runWithReadPool(cfg.timeout, cfg.envLoader, func(ctx context.Context, pool *db.Pool) int {
		detail, err := pool.GetStoryDetail(ctx, cfg.storyUUID)
		if err != nil {
			if errors.Is(err, db.ErrNoRows) {
				fmt.Fprintf(os.Stderr, "Story not found: %s\n", cfg.storyUUID)
				return 1
			}
			fmt.Fprintf(os.Stderr, "Failed to load story detail: %v\n", err)
			return 1
		}
		return renderStoryDetail(detail, cfg.format)
	})
}

func renderStoryDetail(detail *db.StoryDetail, outputFormat string) int {
	return renderJSONOrTable(detail, outputFormat, func() error { return writeStoryDetailTable(detail) })
}

func writeStoryDetailTable(detail *db.StoryDetail) error {
	if detail == nil {
		return fmt.Errorf("story detail is nil")
	}

	fmt.Println("story")
	storyRows := [][]string{
		{"story_uuid", detail.Story.StoryUUID},
		{"title", detail.Story.CanonicalTitle},
		{"url", pointerStringOrEmpty(detail.Story.CanonicalURL)},
		{"source_count", fmt.Sprintf("%d", detail.Story.SourceCount)},
		{"article_count", fmt.Sprintf("%d", detail.Story.ArticleCount)},
		{"created_at", formatUTCTimestamp(detail.Story.CreatedAt)},
	}
	if err := writeTable([]string{"field", "value"}, storyRows); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("articles")
	articleRows := make([][]string, 0, len(detail.Articles))
	for _, article := range detail.Articles {
		articleRows = append(articleRows, []string{
			truncateForTable(article.Title, 80),
			pointerStringOrEmpty(article.URL),
			article.Source,
			formatUTCTimestampPtr(article.PublishedAt),
		})
	}
	return writeTable([]string{"title", "url", "source", "published_at"}, articleRows)
}
