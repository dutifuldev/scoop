package app

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"horse.fit/scoop/internal/cli"
	"horse.fit/scoop/internal/db"
	"horse.fit/scoop/internal/globaltime"
)

func runArticles(args []string) int {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "list":
			return runArticlesList(args[1:])
		case "add-person":
			return runArticlesAddPerson(args[1:])
		case "remove-person":
			return runArticlesRemovePerson(args[1:])
		case "list-people":
			return runArticlesListPeople(args[1:])
		case "help", "--help", "-h":
			printArticlesUsage()
			return 0
		default:
			fmt.Fprintf(os.Stderr, "unknown articles command: %s\n\n", args[0])
			printArticlesUsage()
			return 2
		}
	}
	return runArticlesList(args)
}

func runArticlesList(args []string) int {
	fs := flag.NewFlagSet("articles", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 30*time.Second, "Command timeout")
	collection := fs.String("collection", "", "Filter by collection")
	from := fs.String("from", defaultUTCDayString(), "Start date in YYYY-MM-DD (UTC)")
	to := fs.String("to", defaultUTCDayString(), "End date in YYYY-MM-DD (UTC)")
	limit := fs.Int("limit", 50, "Maximum articles to return")
	format := fs.String("format", outputFormatTable, "Output format: table or json")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "articles does not accept positional arguments")
		return 2
	}
	if *limit <= 0 {
		fmt.Fprintln(os.Stderr, "--limit must be > 0")
		return 2
	}

	outputFormat, err := parseOutputFormat(*format, outputFormatTable)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid format: %v\n", err)
		return 2
	}

	fromStart, toEnd, err := parseUTCDateRange(*from, *to)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid date range: %v\n", err)
		return 2
	}

	ctx, cancel, pool, err := connectReadPool(*timeout, envLoader)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer cancel()
	defer pool.Close()

	articles, err := pool.ListArticles(ctx, db.ArticleListOptions{
		Collection: normalizeCollectionFlag(*collection),
		From:       fromStart,
		To:         toEnd,
		Limit:      *limit,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to query articles: %v\n", err)
		return 1
	}

	if outputFormat == outputFormatJSON {
		if err := printJSON(articles); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to encode JSON: %v\n", err)
			return 1
		}
		return 0
	}

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
		fmt.Fprintf(os.Stderr, "Failed to render table: %v\n", err)
		return 1
	}

	return 0
}

func runArticlesAddPerson(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: scoop articles add-person <article_uuid> <identity_ref>")
		return 2
	}
	articleUUID := args[0]
	identityRef := args[1]
	fs := flag.NewFlagSet("articles add-person", flag.ContinueOnError)
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
	identity, err := pool.AddArticlePersonIdentity(ctx, articleUUID, identityRef, nil, globaltime.UTC())
	if err != nil {
		if errors.Is(err, db.ErrNoRows) {
			fmt.Fprintln(os.Stderr, "Article or person identity not found")
			return 1
		}
		fmt.Fprintf(os.Stderr, "Failed to add article person identity: %v\n", err)
		return 1
	}
	return printPersonIdentityResult(identity, *format)
}

func runArticlesRemovePerson(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: scoop articles remove-person <article_uuid> <identity_ref-or-person_identity_uuid>")
		return 2
	}
	articleUUID := args[0]
	identityRefOrUUID := args[1]
	fs := flag.NewFlagSet("articles remove-person", flag.ContinueOnError)
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
	if err := pool.RemoveArticlePersonIdentity(ctx, articleUUID, identityRefOrUUID, nil); err != nil {
		if errors.Is(err, db.ErrNoRows) {
			fmt.Fprintln(os.Stderr, "Article or person identity not found")
			return 1
		}
		fmt.Fprintf(os.Stderr, "Failed to remove article person identity: %v\n", err)
		return 1
	}
	fmt.Printf("removed person identity from article %s\n", articleUUID)
	return 0
}

func runArticlesListPeople(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: scoop articles list-people <article_uuid>")
		return 2
	}
	articleUUID := args[0]
	fs := flag.NewFlagSet("articles list-people", flag.ContinueOnError)
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
	identities, err := pool.ListPersonIdentitiesForArticleUUID(ctx, articleUUID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list article person identities: %v\n", err)
		return 1
	}
	if outputFormat == outputFormatJSON {
		if err := printJSON(identities); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to encode JSON: %v\n", err)
			return 1
		}
		return 0
	}
	return writePersonIdentityTable(identities)
}

func printArticlesUsage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  scoop articles list [--collection ...] [--from ...] [--to ...] [--limit ...] [--format table|json]")
	fmt.Fprintln(os.Stderr, "  scoop articles [--collection ...] [--from ...] [--to ...] [--limit ...] [--format table|json]")
	fmt.Fprintln(os.Stderr, "  scoop articles add-person <article_uuid> <identity_ref> [--format table|json]")
	fmt.Fprintln(os.Stderr, "  scoop articles remove-person <article_uuid> <identity_ref-or-person_identity_uuid>")
	fmt.Fprintln(os.Stderr, "  scoop articles list-people <article_uuid> [--format table|json]")
}
