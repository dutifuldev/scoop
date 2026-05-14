package app

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

type rootCommand struct {
	names []string
	run   func([]string) int
}

type subcommand struct {
	names []string
	run   func([]string) int
}

func runParsedCommand[T any](args []string, parse func([]string) (T, int, bool), execute func(T) int) int {
	cfg, exitCode, ok := parse(args)
	if !ok {
		return exitCode
	}
	return execute(cfg)
}

func newAppFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func parseAppFlagSet(fs *flag.FlagSet, args []string) (int, bool) {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, false
		}
		return 2, false
	}
	return 0, true
}

var rootCommands = []rootCommand{
	{names: []string{"stories"}, run: runStories},
	{names: []string{"stats"}, run: runStats},
	{names: []string{"story"}, run: runStoryDetail},
	{names: []string{"delete"}, run: runDelete},
	{names: []string{"update"}, run: runUpdate},
	{names: []string{"restore"}, run: runRestore},
	{names: []string{"collections"}, run: runCollections},
	{names: []string{"search"}, run: runSearch},
	{names: []string{"articles"}, run: runArticles},
	{names: []string{"tags"}, run: runTags},
	{names: []string{"person-identities"}, run: runPersonIdentities},
	{names: []string{"digest"}, run: runDigest},
	{names: []string{"health"}, run: runHealth},
	{names: []string{"ingest"}, run: runIngest},
	{names: []string{"validate"}, run: runValidate},
	{names: []string{"normalize"}, run: runNormalize},
	{names: []string{"embed"}, run: runEmbed},
	{names: []string{"translate"}, run: runTranslate},
	{names: []string{"dedup"}, run: runDedup},
	{names: []string{"process", "run-once"}, run: runProcess},
	{names: []string{"serve"}, run: runServe},
	{names: []string{"daemon"}, run: runDaemon},
}

// Run executes the CLI command and returns a process exit code.
func Run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 2
	}

	commandName := normalizeCommandName(args[0])
	if isHelpCommand(commandName) {
		printUsage()
		return 0
	}

	command, ok := findRootCommand(commandName)
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		printUsage()
		return 2
	}
	return command.run(args[1:])
}

func normalizeCommandName(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func isHelpCommand(name string) bool {
	return stringSliceContains([]string{"help", "--help", "-h"}, name)
}

func findRootCommand(name string) (rootCommand, bool) {
	for _, command := range rootCommands {
		if stringSliceContains(command.names, name) {
			return command, true
		}
	}
	return rootCommand{}, false
}

func runSubcommands(commandName string, args []string, commands []subcommand, usage func(), defaultRun func([]string) int) int {
	if len(args) == 0 {
		return runDefaultSubcommand(args, usage, defaultRun)
	}

	name := normalizeCommandName(args[0])
	if strings.HasPrefix(args[0], "-") {
		return runFlagDefaultSubcommand(args, commandName, usage, defaultRun)
	}
	if isHelpCommand(name) {
		usage()
		return 0
	}
	if command, ok := findSubcommand(commands, name); ok {
		return command.run(args[1:])
	}
	fmt.Fprintf(os.Stderr, "unknown %s command: %s\n\n", commandName, args[0])
	usage()
	return 2
}

func runDefaultSubcommand(args []string, usage func(), defaultRun func([]string) int) int {
	if defaultRun != nil {
		return defaultRun(args)
	}
	usage()
	return 2
}

func runFlagDefaultSubcommand(args []string, commandName string, usage func(), defaultRun func([]string) int) int {
	if defaultRun != nil {
		return defaultRun(args)
	}
	fmt.Fprintf(os.Stderr, "%s requires a subcommand\n\n", commandName)
	usage()
	return 2
}

func findSubcommand(commands []subcommand, name string) (subcommand, bool) {
	for _, command := range commands {
		if stringSliceContains(command.names, name) {
			return command, true
		}
	}
	return subcommand{}, false
}

func stringSliceContains(candidates []string, value string) bool {
	for _, candidate := range candidates {
		if candidate == value {
			return true
		}
	}
	return false
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
