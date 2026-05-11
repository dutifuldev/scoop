package app

import (
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
	if len(args) == 0 {
		printTagsUsage()
		return 2
	}

	switch args[0] {
	case "list":
		return runTagsList(args[1:])
	case "create":
		return runTagsCreate(args[1:])
	case "rename":
		return runTagsRename(args[1:])
	case "update":
		return runTagsUpdate(args[1:])
	case "archive":
		return runTagsArchive(args[1:], true)
	case "unarchive":
		return runTagsArchive(args[1:], false)
	case "delete":
		return runTagsDelete(args[1:])
	case "add-article":
		return runTagsAddArticle(args[1:])
	case "remove-article":
		return runTagsRemoveArticle(args[1:])
	case "help", "--help", "-h":
		printTagsUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown tags command: %s\n\n", args[0])
		printTagsUsage()
		return 2
	}
}

func runTagsList(args []string) int {
	fs := flag.NewFlagSet("tags list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 30*time.Second, "Command timeout")
	format := fs.String("format", outputFormatTable, "Output format: table or json")
	includeArchived := fs.Bool("include-archived", false, "Include archived tags")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "tags list does not accept positional arguments")
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

	tags, err := pool.ListTags(ctx, *includeArchived)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list tags: %v\n", err)
		return 1
	}
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
			formatUTCTimestampPtr(tag.ArchivedAt),
			formatUTCTimestamp(tag.CreatedAt),
		})
	}
	if err := writeTable([]string{"tag", "color", "archived_at", "created_at"}, rows); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to render table: %v\n", err)
		return 1
	}
	return 0
}

func runTagsCreate(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "tag is required")
		return 2
	}
	slug := args[0]

	fs := flag.NewFlagSet("tags create", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 30*time.Second, "Command timeout")
	format := fs.String("format", outputFormatTable, "Output format: table or json")
	description := fs.String("description", "", "Description")
	color := fs.String("color", "", "Tag color as #RRGGBB")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "tags create accepts only one tag")
		return 2
	}
	if _, err := parseOutputFormat(*format, outputFormatTable); err != nil {
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

	var descriptionPtr *string
	if *description != "" {
		descriptionPtr = description
	}
	var colorPtr *string
	if *color != "" {
		colorPtr = color
	}
	tag, err := pool.CreateTag(ctx, db.UpsertTagOptions{
		Slug:        slug,
		Description: descriptionPtr,
		Color:       colorPtr,
	}, globaltime.UTC())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create tag: %v\n", err)
		return 1
	}
	return printTagResult(tag, *format)
}

func runTagsRename(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: scoop tags rename <old-tag> <new-tag>")
		return 2
	}
	fs := flag.NewFlagSet("tags rename", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
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
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "tag is required")
		return 2
	}
	slug := args[0]
	fs := flag.NewFlagSet("tags update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 30*time.Second, "Command timeout")
	format := fs.String("format", outputFormatTable, "Output format: table or json")
	description := fs.String("description", "", "Description")
	color := fs.String("color", "", "Tag color as #RRGGBB")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	opts := db.UpdateTagOptions{}
	if *description != "" {
		opts.Description = description
	}
	if *color != "" {
		opts.Color = color
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "too many positional arguments")
		return 2
	}
	return updateTag(slug, opts, *timeout, envLoader, *format)
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
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "tag is required")
		return 2
	}
	slug := args[0]
	fs := flag.NewFlagSet("tags archive", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 30*time.Second, "Command timeout")
	format := fs.String("format", outputFormatTable, "Output format: table or json")
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
	if _, err := parseOutputFormat(*format, outputFormatTable); err != nil {
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

	tag, err := pool.SetTagArchived(ctx, slug, archived, globaltime.UTC())
	if err != nil {
		if errors.Is(err, db.ErrNoRows) {
			fmt.Fprintln(os.Stderr, "Tag not found")
			return 1
		}
		fmt.Fprintf(os.Stderr, "Failed to update tag: %v\n", err)
		return 1
	}
	return printTagResult(tag, *format)
}

func runTagsDelete(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "tag is required")
		return 2
	}
	slug := args[0]
	fs := flag.NewFlagSet("tags delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 30*time.Second, "Command timeout")
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
	ctx, cancel, pool, err := connectReadPool(*timeout, envLoader)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer cancel()
	defer pool.Close()
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
}

func runTagsAddArticle(args []string) int {
	return runTagsArticleMutation(args, true)
}

func runTagsRemoveArticle(args []string) int {
	return runTagsArticleMutation(args, false)
}

func runTagsArticleMutation(args []string, add bool) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: scoop tags add-article <article_uuid> <tag>")
		return 2
	}
	articleUUID := args[0]
	slug := args[1]
	fs := flag.NewFlagSet("tags article", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 30*time.Second, "Command timeout")
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
	ctx, cancel, pool, err := connectReadPool(*timeout, envLoader)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer cancel()
	defer pool.Close()
	if add {
		err = pool.AddArticleTag(ctx, articleUUID, slug, nil, globaltime.UTC())
	} else {
		err = pool.RemoveArticleTag(ctx, articleUUID, slug, nil)
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
	fmt.Printf("%s tag %s on article %s\n", action, db.NormalizeTagSlug(slug), articleUUID)
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
		formatUTCTimestampPtr(tag.ArchivedAt),
	}}
	if err := writeTable([]string{"tag", "color", "archived_at"}, rows); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to render table: %v\n", err)
		return 1
	}
	return 0
}

func printTagsUsage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  scoop tags list [--include-archived] [--format table|json]")
	fmt.Fprintln(os.Stderr, "  scoop tags create <tag> [--description <text>] [--color <hex>]")
	fmt.Fprintln(os.Stderr, "  scoop tags rename <old-tag> <new-tag>")
	fmt.Fprintln(os.Stderr, "  scoop tags update <tag> [--description <text>] [--color <hex>]")
	fmt.Fprintln(os.Stderr, "  scoop tags archive <tag>")
	fmt.Fprintln(os.Stderr, "  scoop tags unarchive <tag>")
	fmt.Fprintln(os.Stderr, "  scoop tags delete <tag>")
	fmt.Fprintln(os.Stderr, "  scoop tags add-article <article_uuid> <tag>")
	fmt.Fprintln(os.Stderr, "  scoop tags remove-article <article_uuid> <tag>")
}
