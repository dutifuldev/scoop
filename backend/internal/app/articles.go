package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"horse.fit/scoop/internal/cli"
	"horse.fit/scoop/internal/db"
	"horse.fit/scoop/internal/globaltime"
)

type articlesListCommandConfig = windowListCommandConfig

type articleValueCommandConfig struct {
	envLoader   *cli.EnvLoader
	timeout     time.Duration
	format      string
	articleUUID string
	value       string
}

func runArticles(args []string) int {
	return runSubcommands("articles", args, []subcommand{
		{names: []string{"list"}, run: runArticlesList},
		{names: []string{"add-person"}, run: runArticlesAddPerson},
		{names: []string{"remove-person"}, run: runArticlesRemovePerson},
		{names: []string{"list-people"}, run: runArticlesListPeople},
	}, printArticlesUsage, runArticlesList)
}

func runArticlesList(args []string) int {
	return runParsedCommand(args, parseArticlesListCommand, executeArticlesListCommand)
}

func parseArticlesListCommand(args []string) (articlesListCommandConfig, int, bool) {
	return parseWindowListCommand(args, "articles", "Maximum articles to return")
}

func executeArticlesListCommand(cfg articlesListCommandConfig) int {
	return runWindowList(cfg, loadArticleList, "Failed to query articles", renderArticlesList)
}

func loadArticleList(ctx context.Context, pool *db.Pool, cfg windowListCommandConfig) ([]db.ArticleListItem, error) {
	opts := db.ArticleListOptions{
		Collection: cfg.collection,
		From:       cfg.from,
		To:         cfg.to,
		Limit:      cfg.limit,
	}
	return pool.ListArticles(ctx, opts)
}

func renderArticlesList(articles []db.ArticleListItem, outputFormat string) int {
	return renderList(articles, outputFormat, writeArticlesListTable)
}

func writeArticlesListTable(articles []db.ArticleListItem) error {
	tableRows := make([][]string, 0, len(articles))
	for _, article := range articles {
		tableRows = append(tableRows, []string{
			fmt.Sprintf("%d", article.ArticleID),
			truncateForTable(article.Title, 80),
			article.Source,
			pointerStringOrEmpty(article.SourceDomain),
			formatUTCTimestampPtr(article.PublishedAt),
			article.Collection,
			formatUTCTimestamp(article.CreatedAt),
		})
	}

	if err := writeTable(
		[]string{"article_id", "title", "source", "source_domain", "published_at", "collection", "created_at"},
		tableRows,
	); err != nil {
		return err
	}
	return nil
}

func runArticlesAddPerson(args []string) int {
	cfg, exitCode, ok := parseArticleValueCommand(
		args,
		"articles add-person",
		"usage: scoop articles add-person <article_uuid> <identity_ref>",
		true,
	)
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
	identity, err := pool.AddArticlePersonIdentity(ctx, cfg.articleUUID, cfg.value, nil, globaltime.UTC())
	if err != nil {
		if errors.Is(err, db.ErrNoRows) {
			fmt.Fprintln(os.Stderr, "Article or person identity not found")
			return 1
		}
		fmt.Fprintf(os.Stderr, "Failed to add article person identity: %v\n", err)
		return 1
	}
	return printPersonIdentityResult(identity, cfg.format)
}

func runArticlesRemovePerson(args []string) int {
	cfg, exitCode, ok := parseArticleValueCommand(
		args,
		"articles remove-person",
		"usage: scoop articles remove-person <article_uuid> <identity_ref-or-person_identity_uuid>",
		false,
	)
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
	if err := pool.RemoveArticlePersonIdentity(ctx, cfg.articleUUID, cfg.value, nil); err != nil {
		if errors.Is(err, db.ErrNoRows) {
			fmt.Fprintln(os.Stderr, "Article or person identity not found")
			return 1
		}
		fmt.Fprintf(os.Stderr, "Failed to remove article person identity: %v\n", err)
		return 1
	}
	fmt.Printf("removed person identity from article %s\n", cfg.articleUUID)
	return 0
}

func runArticlesListPeople(args []string) int {
	cfg, exitCode, ok := parseArticleOnlyCommand(args, "articles list-people", "usage: scoop articles list-people <article_uuid>", true)
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
	identities, err := pool.ListPersonIdentitiesForArticleUUID(ctx, cfg.articleUUID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list article person identities: %v\n", err)
		return 1
	}
	if cfg.format == outputFormatJSON {
		if err := printJSON(identities); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to encode JSON: %v\n", err)
			return 1
		}
		return 0
	}
	return writePersonIdentityTable(identities)
}

func parseArticleValueCommand(args []string, name string, usage string, hasFormat bool) (articleValueCommandConfig, int, bool) {
	return parseArticleCommand(args, name, usage, hasFormat, 2)
}

func parseArticleOnlyCommand(args []string, name string, usage string, hasFormat bool) (articleValueCommandConfig, int, bool) {
	return parseArticleCommand(args, name, usage, hasFormat, 1)
}

func parseArticleCommand(args []string, name string, usage string, hasFormat bool, requiredArgs int) (articleValueCommandConfig, int, bool) {
	if len(args) < requiredArgs {
		fmt.Fprintln(os.Stderr, usage)
		return articleValueCommandConfig{}, 2, false
	}
	cfg, exitCode, ok := parseArticleCommandFlags(name, args[requiredArgs:], hasFormat)
	if !ok {
		return articleValueCommandConfig{}, exitCode, false
	}
	cfg.articleUUID = args[0]
	if requiredArgs > 1 {
		cfg.value = args[1]
	}
	return cfg, 0, true
}

func parseArticleCommandFlags(name string, args []string, hasFormat bool) (articleValueCommandConfig, int, bool) {
	fs := newAppFlagSet(name)
	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 30*time.Second, "Command timeout")
	format := outputFormatTable
	if hasFormat {
		fs.StringVar(&format, "format", outputFormatTable, "Output format: table or json")
	}
	if exitCode, ok := parseAppFlagSet(fs, args); !ok {
		return articleValueCommandConfig{}, exitCode, false
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "too many positional arguments")
		return articleValueCommandConfig{}, 2, false
	}
	outputFormat, err := parseOutputFormat(format, outputFormatTable)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid format: %v\n", err)
		return articleValueCommandConfig{}, 2, false
	}
	return articleValueCommandConfig{envLoader: envLoader, timeout: *timeout, format: outputFormat}, 0, true
}

func printArticlesUsage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  scoop articles list [--collection ...] [--from ...] [--to ...] [--limit ...] [--format table|json]")
	fmt.Fprintln(os.Stderr, "  scoop articles [--collection ...] [--from ...] [--to ...] [--limit ...] [--format table|json]")
	fmt.Fprintln(os.Stderr, "  scoop articles add-person <article_uuid> <identity_ref> [--format table|json]")
	fmt.Fprintln(os.Stderr, "  scoop articles remove-person <article_uuid> <identity_ref-or-person_identity_uuid>")
	fmt.Fprintln(os.Stderr, "  scoop articles list-people <article_uuid> [--format table|json]")
}
