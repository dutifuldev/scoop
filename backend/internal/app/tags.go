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
	"horse.fit/scoop/internal/globaltime"
)

func runTags(args []string) int {
	return runSubcommands("tags", args, []subcommand{
		{names: []string{"list"}, run: runTagsList},
		{names: []string{"create"}, run: runTagsCreate},
		{names: []string{"rename"}, run: runTagsRename},
		{names: []string{"update"}, run: runTagsUpdate},
		{names: []string{"archive"}, run: func(args []string) int { return runTagsArchive(args, true) }},
		{names: []string{"unarchive"}, run: func(args []string) int { return runTagsArchive(args, false) }},
		{names: []string{"delete"}, run: runTagsDelete},
		{names: []string{"add-article"}, run: runTagsAddArticle},
		{names: []string{"remove-article"}, run: runTagsRemoveArticle},
	}, printTagsUsage, nil)
}

func runTagsList(args []string) int {
	return runParsedCommand(args, parseTagsListCommand, executeTagsListCommand)
}

type tagsListCommandConfig struct {
	envLoader       *cli.EnvLoader
	timeout         time.Duration
	format          string
	includeArchived bool
}

func parseTagsListCommand(args []string) (tagsListCommandConfig, int, bool) {
	fs := newAppFlagSet("tags list")
	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 30*time.Second, "Command timeout")
	format := fs.String("format", outputFormatTable, "Output format: table or json")
	includeArchived := fs.Bool("include-archived", false, "Include archived tags")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return tagsListCommandConfig{}, 0, false
		}
		return tagsListCommandConfig{}, 2, false
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "tags list does not accept positional arguments")
		return tagsListCommandConfig{}, 2, false
	}
	outputFormat, err := parseOutputFormat(*format, outputFormatTable)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid format: %v\n", err)
		return tagsListCommandConfig{}, 2, false
	}
	return tagsListCommandConfig{
		envLoader:       envLoader,
		timeout:         *timeout,
		format:          outputFormat,
		includeArchived: *includeArchived,
	}, 0, true
}

func executeTagsListCommand(cfg tagsListCommandConfig) int {
	ctx, cancel, pool, err := connectReadPool(cfg.timeout, cfg.envLoader)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer cancel()
	defer pool.Close()

	tags, err := pool.ListTags(ctx, cfg.includeArchived)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list tags: %v\n", err)
		return 1
	}
	return renderTagsList(tags, cfg.format)
}

func renderTagsList(tags []db.TagRecord, outputFormat string) int {
	if outputFormat == outputFormatJSON {
		if err := printJSON(tags); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to encode JSON: %v\n", err)
			return 1
		}
		return 0
	}

	rows := make([][]string, 0, len(tags))
	for _, tag := range tags {
		rows = append(rows, []string{
			tag.Tag,
			pointerStringOrEmpty(tag.Color),
			pointerStringOrEmpty(tag.HighlightColor),
			formatUTCTimestampPtr(tag.ArchivedAt),
			formatUTCTimestamp(tag.CreatedAt),
		})
	}
	if err := writeTable([]string{"tag", "color", "highlight_color", "archived_at", "created_at"}, rows); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to render table: %v\n", err)
		return 1
	}
	return 0
}

func runTagsCreate(args []string) int {
	return runParsedCommand(args, parseTagsCreateCommand, executeTagsCreateCommand)
}

type tagsCreateCommandConfig struct {
	envLoader *cli.EnvLoader
	timeout   time.Duration
	format    string
	opts      db.UpsertTagOptions
}

func parseTagsCreateCommand(args []string) (tagsCreateCommandConfig, int, bool) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "tag is required")
		return tagsCreateCommandConfig{}, 2, false
	}
	slug := args[0]

	fs := newAppFlagSet("tags create")
	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 30*time.Second, "Command timeout")
	format := fs.String("format", outputFormatTable, "Output format: table or json")
	description := fs.String("description", "", "Description")
	color := fs.String("color", "", "Tag color as #RRGGBB")
	highlightColor := fs.String("highlight-color", "", "Article/story highlight color as #RRGGBB")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return tagsCreateCommandConfig{}, 0, false
		}
		return tagsCreateCommandConfig{}, 2, false
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "tags create accepts only one tag")
		return tagsCreateCommandConfig{}, 2, false
	}
	outputFormat, err := parseOutputFormat(*format, outputFormatTable)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid format: %v\n", err)
		return tagsCreateCommandConfig{}, 2, false
	}
	return tagsCreateCommandConfig{
		envLoader: envLoader,
		timeout:   *timeout,
		format:    outputFormat,
		opts: db.UpsertTagOptions{
			Slug:           slug,
			Description:    stringPtrFromFlag(description),
			Color:          stringPtrFromFlag(color),
			HighlightColor: stringPtrFromFlag(highlightColor),
		},
	}, 0, true
}

func executeTagsCreateCommand(cfg tagsCreateCommandConfig) int {
	ctx, cancel, pool, err := connectReadPool(cfg.timeout, cfg.envLoader)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer cancel()
	defer pool.Close()

	tag, err := pool.CreateTag(ctx, cfg.opts, globaltime.UTC())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create tag: %v\n", err)
		return 1
	}
	return printTagResult(tag, cfg.format)
}

func stringPtrFromFlag(value *string) *string {
	if value == nil || *value == "" {
		return nil
	}
	return value
}

func runTagsRename(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: scoop tags rename <old-tag> <new-tag>")
		return 2
	}
	fs := newAppFlagSet("tags rename")
	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 30*time.Second, "Command timeout")
	format := fs.String("format", outputFormatTable, "Output format: table or json")
	if err := fs.Parse(args[2:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "too many positional arguments")
		return 2
	}
	return updateTag(args[0], db.UpdateTagOptions{NewSlug: &args[1]}, *timeout, envLoader, *format)
}

func runTagsUpdate(args []string) int {
	slug, exitCode, ok := parseRequiredTagSlugArg(args)
	if !ok {
		return exitCode
	}
	fs := newAppFlagSet("tags update")
	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 30*time.Second, "Command timeout")
	format := fs.String("format", outputFormatTable, "Output format: table or json")
	description := fs.String("description", "", "Description")
	color := fs.String("color", "", "Tag color as #RRGGBB")
	highlightColor := fs.String("highlight-color", "", "Article/story highlight color as #RRGGBB")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "too many positional arguments")
		return 2
	}
	return updateTag(slug, tagUpdateOptionsFromFlags(description, color, highlightColor), *timeout, envLoader, *format)
}

func parseRequiredTagSlugArg(args []string) (string, int, bool) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "tag is required")
		return "", 2, false
	}
	return args[0], 0, true
}

func tagUpdateOptionsFromFlags(description *string, color *string, highlightColor *string) db.UpdateTagOptions {
	opts := db.UpdateTagOptions{}
	if *description != "" {
		opts.Description = description
	}
	if *color != "" {
		opts.Color = color
	}
	if *highlightColor != "" {
		opts.HighlightColor = highlightColor
	}
	return opts
}

func updateTag(slug string, opts db.UpdateTagOptions, timeout time.Duration, envLoader *cli.EnvLoader, format string) int {
	if _, err := parseOutputFormat(format, outputFormatTable); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid format: %v\n", err)
		return 2
	}
	ctx, cancel, pool, err := connectReadPool(timeout, envLoader)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer cancel()
	defer pool.Close()

	tag, err := pool.UpdateTag(ctx, slug, opts, globaltime.UTC())
	if err != nil {
		if errors.Is(err, db.ErrNoRows) {
			fmt.Fprintln(os.Stderr, "Tag not found")
			return 1
		}
		fmt.Fprintf(os.Stderr, "Failed to update tag: %v\n", err)
		return 1
	}
	return printTagResult(tag, format)
}

func runTagsArchive(args []string, archived bool) int {
	cfg, exitCode, ok := parseSingleTagCommand(args, "tags archive", true)
	if !ok {
		return exitCode
	}
	ctx, cancel, pool, err := connectReadPool(cfg.timeout, cfg.envLoader)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer cancel()
	defer pool.Close()

	tag, err := pool.SetTagArchived(ctx, cfg.slug, archived, globaltime.UTC())
	if err != nil {
		if errors.Is(err, db.ErrNoRows) {
			fmt.Fprintln(os.Stderr, "Tag not found")
			return 1
		}
		fmt.Fprintf(os.Stderr, "Failed to update tag: %v\n", err)
		return 1
	}
	return printTagResult(tag, cfg.format)
}

type singleTagCommandConfig struct {
	envLoader *cli.EnvLoader
	timeout   time.Duration
	format    string
	slug      string
}

func parseSingleTagCommand(args []string, name string, hasFormat bool) (singleTagCommandConfig, int, bool) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "tag is required")
		return singleTagCommandConfig{}, 2, false
	}
	fs := newAppFlagSet(name)
	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 30*time.Second, "Command timeout")
	format := outputFormatTable
	if hasFormat {
		fs.StringVar(&format, "format", outputFormatTable, "Output format: table or json")
	}
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return singleTagCommandConfig{}, 0, false
		}
		return singleTagCommandConfig{}, 2, false
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "too many positional arguments")
		return singleTagCommandConfig{}, 2, false
	}
	outputFormat, err := parseOutputFormat(format, outputFormatTable)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid format: %v\n", err)
		return singleTagCommandConfig{}, 2, false
	}
	return singleTagCommandConfig{envLoader: envLoader, timeout: *timeout, format: outputFormat, slug: args[0]}, 0, true
}

func runTagsDelete(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "tag is required")
		return 2
	}
	slug := args[0]
	fs := newAppFlagSet("tags delete")
	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 30*time.Second, "Command timeout")
	if exitCode, ok := parseAppFlagSet(fs, args[1:]); !ok {
		return exitCode
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "too many positional arguments")
		return 2
	}
	return runWithReadPool(*timeout, envLoader, func(ctx context.Context, pool *db.Pool) int {
		if err := pool.DeleteTag(ctx, slug); err != nil {
			if errors.Is(err, db.ErrNoRows) {
				fmt.Fprintln(os.Stderr, "Tag not found")
				return 1
			}
			fmt.Fprintf(os.Stderr, "Failed to delete tag: %v\n", err)
			return 1
		}
		fmt.Printf("deleted tag %s\n", db.NormalizeTagSlug(slug))
		return 0
	})
}

func runTagsAddArticle(args []string) int {
	return runTagsArticleMutation(args, true)
}

func runTagsRemoveArticle(args []string) int {
	return runTagsArticleMutation(args, false)
}

func runTagsArticleMutation(args []string, add bool) int {
	cfg, exitCode, ok := parseArticleValueCommand(args, "tags article", "usage: scoop tags add-article <article_uuid> <tag>", false)
	if !ok {
		return exitCode
	}
	ctx, cancel, pool, err := connectReadPool(cfg.timeout, cfg.envLoader)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer cancel()
	defer pool.Close()
	if add {
		err = pool.AddArticleTag(ctx, cfg.articleUUID, cfg.value, nil, globaltime.UTC())
	} else {
		err = pool.RemoveArticleTag(ctx, cfg.articleUUID, cfg.value, nil)
	}
	if err != nil {
		if errors.Is(err, db.ErrNoRows) {
			fmt.Fprintln(os.Stderr, "Article or tag not found")
			return 1
		}
		fmt.Fprintf(os.Stderr, "Failed to update article tag: %v\n", err)
		return 1
	}
	action := "added"
	if !add {
		action = "removed"
	}
	fmt.Printf("%s tag %s on article %s\n", action, db.NormalizeTagSlug(cfg.value), cfg.articleUUID)
	return 0
}

func printTagResult(tag *db.TagRecord, rawFormat string) int {
	outputFormat, err := parseOutputFormat(rawFormat, outputFormatTable)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid format: %v\n", err)
		return 2
	}
	if outputFormat == outputFormatJSON {
		if err := printJSON(tag); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to encode JSON: %v\n", err)
			return 1
		}
		return 0
	}
	rows := [][]string{{
		tag.Tag,
		pointerStringOrEmpty(tag.Color),
		pointerStringOrEmpty(tag.HighlightColor),
		formatUTCTimestampPtr(tag.ArchivedAt),
	}}
	if err := writeTable([]string{"tag", "color", "highlight_color", "archived_at"}, rows); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to render table: %v\n", err)
		return 1
	}
	return 0
}

func printTagsUsage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  scoop tags list [--include-archived] [--format table|json]")
	fmt.Fprintln(os.Stderr, "  scoop tags create <tag> [--description <text>] [--color <hex>] [--highlight-color <hex>]")
	fmt.Fprintln(os.Stderr, "  scoop tags rename <old-tag> <new-tag>")
	fmt.Fprintln(os.Stderr, "  scoop tags update <tag> [--description <text>] [--color <hex>] [--highlight-color <hex>]")
	fmt.Fprintln(os.Stderr, "  scoop tags archive <tag>")
	fmt.Fprintln(os.Stderr, "  scoop tags unarchive <tag>")
	fmt.Fprintln(os.Stderr, "  scoop tags delete <tag>")
	fmt.Fprintln(os.Stderr, "  scoop tags add-article <article_uuid> <tag>")
	fmt.Fprintln(os.Stderr, "  scoop tags remove-article <article_uuid> <tag>")
}
