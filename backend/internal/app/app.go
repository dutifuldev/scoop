package app

import (
	"fmt"
	"os"
	"strings"
)

// Run executes the CLI command and returns a process exit code.
func Run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 2
	}

	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "help", "--help", "-h":
		printUsage()
		return 0
	case "stories":
		return runStories(args[1:])
	case "stats":
		return runStats(args[1:])
	case "story":
		return runStoryDetail(args[1:])
	case "delete":
		return runDelete(args[1:])
	case "update":
		return runUpdate(args[1:])
	case "restore":
		return runRestore(args[1:])
	case "collections":
		return runCollections(args[1:])
	case "search":
		return runSearch(args[1:])
	case "articles":
		return runArticles(args[1:])
	case "tags":
		return runTags(args[1:])
	case "person-identities":
		return runPersonIdentities(args[1:])
	case "digest":
		return runDigest(args[1:])
	case "health":
		return runHealth(args[1:])
	case "ingest":
		return runIngest(args[1:])
	case "validate":
		return runValidate(args[1:])
	case "normalize":
		return runNormalize(args[1:])
	case "embed":
		return runEmbed(args[1:])
	case "translate":
		return runTranslate(args[1:])
	case "dedup":
		return runDedup(args[1:])
	case "process", "run-once":
		return runProcess(args[1:])
	case "serve":
		return runServe(args[1:])
	case "daemon":
		return runDaemon(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		printUsage()
		return 2
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "scoop CLI")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  scoop <command> [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  stories    List stories by dedup event date window")
	fmt.Fprintln(os.Stderr, "  stats      Show per-collection and pipeline throughput counts")
	fmt.Fprintln(os.Stderr, "  story      Show detail for one story UUID")
	fmt.Fprintln(os.Stderr, "  delete     Soft delete stories/articles/collections or rows before a date")
	fmt.Fprintln(os.Stderr, "  update     Update stories/articles by UUID")
	fmt.Fprintln(os.Stderr, "  restore    Restore soft-deleted stories/articles by UUID")
	fmt.Fprintln(os.Stderr, "  collections  List collections with article/story counts and ranges")
	fmt.Fprintln(os.Stderr, "  search     Search story titles")
	fmt.Fprintln(os.Stderr, "  articles   List normalized articles")
	fmt.Fprintln(os.Stderr, "  tags       Manage allowed article tags and article tag assignments")
	fmt.Fprintln(os.Stderr, "  person-identities  Manage external person identities")
	fmt.Fprintln(os.Stderr, "  digest     Build today/yesterday digest story sets")
	fmt.Fprintln(os.Stderr, "  health     Verify database connectivity")
	fmt.Fprintln(os.Stderr, "  ingest     Insert one article into ingest ledger tables")
	fmt.Fprintln(os.Stderr, "  validate   Validate news article JSON files against v1 schema")
	fmt.Fprintln(os.Stderr, "  normalize  Convert pending raw arrivals into normalized articles")
	fmt.Fprintln(os.Stderr, "  embed      Generate embeddings for normalized articles")
	fmt.Fprintln(os.Stderr, "  translate  Translate stories/articles/collections with cached outputs")
	fmt.Fprintln(os.Stderr, "  dedup      Assign pending articles into canonical stories")
	fmt.Fprintln(os.Stderr, "  process    Run normalize + embed + dedup in sequence")
	fmt.Fprintln(os.Stderr, "  run-once   Alias for process")
	fmt.Fprintln(os.Stderr, "  serve      Start Echo API server")
	fmt.Fprintln(os.Stderr, "  daemon     Manage systemd services for backend + frontend")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Use \"scoop <command> -h\" for command-specific flags.")
}
