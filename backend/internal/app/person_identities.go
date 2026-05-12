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

func runPersonIdentities(args []string) int {
	if len(args) == 0 {
		printPersonIdentitiesUsage()
		return 2
	}
	switch args[0] {
	case "list":
		return runPersonIdentitiesList(args[1:])
	case "show":
		return runPersonIdentitiesShow(args[1:])
	case "archive":
		return runPersonIdentitiesArchive(args[1:], true)
	case "unarchive":
		return runPersonIdentitiesArchive(args[1:], false)
	case "help", "--help", "-h":
		printPersonIdentitiesUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown person-identities command: %s\n\n", args[0])
		printPersonIdentitiesUsage()
		return 2
	}
}

func runPersonIdentitiesList(args []string) int {
	fs := flag.NewFlagSet("person-identities list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 30*time.Second, "Command timeout")
	format := fs.String("format", outputFormatTable, "Output format: table or json")
	includeArchived := fs.Bool("include-archived", false, "Include archived identities")
	query := fs.String("q", "", "Search query")
	limit := fs.Int("limit", 50, "Maximum identities to return")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "person-identities list does not accept positional arguments")
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
	identities, err := pool.ListPersonIdentities(ctx, *query, *includeArchived, *limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list person identities: %v\n", err)
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

func runPersonIdentitiesShow(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: scoop person-identities show <identity_ref-or-person_identity_uuid>")
		return 2
	}
	fs := flag.NewFlagSet("person-identities show", flag.ContinueOnError)
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
	ctx, cancel, pool, err := connectReadPool(*timeout, envLoader)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer cancel()
	defer pool.Close()
	identity, err := pool.GetPersonIdentity(ctx, args[0])
	if err != nil {
		if errors.Is(err, db.ErrNoRows) {
			fmt.Fprintln(os.Stderr, "Person identity not found")
			return 1
		}
		fmt.Fprintf(os.Stderr, "Failed to show person identity: %v\n", err)
		return 1
	}
	return printPersonIdentityResult(identity, *format)
}

func runPersonIdentitiesArchive(args []string, archived bool) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: scoop person-identities archive <identity_ref-or-person_identity_uuid>")
		return 2
	}
	fs := flag.NewFlagSet("person-identities archive", flag.ContinueOnError)
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
	ctx, cancel, pool, err := connectReadPool(*timeout, envLoader)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer cancel()
	defer pool.Close()
	identity, err := pool.SetPersonIdentityArchived(ctx, args[0], archived, globaltime.UTC())
	if err != nil {
		if errors.Is(err, db.ErrNoRows) {
			fmt.Fprintln(os.Stderr, "Person identity not found")
			return 1
		}
		fmt.Fprintf(os.Stderr, "Failed to update person identity: %v\n", err)
		return 1
	}
	return printPersonIdentityResult(identity, *format)
}

func printPersonIdentityResult(identity *db.PersonIdentityRecord, rawFormat string) int {
	outputFormat, err := parseOutputFormat(rawFormat, outputFormatTable)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid format: %v\n", err)
		return 2
	}
	if outputFormat == outputFormatJSON {
		if err := printJSON(identity); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to encode JSON: %v\n", err)
			return 1
		}
		return 0
	}
	return writePersonIdentityTable([]db.PersonIdentityRecord{*identity})
}

func writePersonIdentityTable(identities []db.PersonIdentityRecord) int {
	rows := make([][]string, 0, len(identities))
	for _, identity := range identities {
		rows = append(rows, []string{
			identity.Provider,
			pointerStringOrEmpty(identity.Handle),
			pointerStringOrEmpty(identity.ProviderUserID),
			identity.IdentityRef,
			formatUTCTimestampPtr(identity.ArchivedAt),
		})
	}
	if err := writeTable([]string{"provider", "handle", "provider_user_id", "identity_ref", "archived_at"}, rows); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to render table: %v\n", err)
		return 1
	}
	return 0
}

func printPersonIdentitiesUsage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  scoop person-identities list [--include-archived] [--q <query>] [--limit 50] [--format table|json]")
	fmt.Fprintln(os.Stderr, "  scoop person-identities show <identity_ref-or-person_identity_uuid> [--format table|json]")
	fmt.Fprintln(os.Stderr, "  scoop person-identities archive <identity_ref-or-person_identity_uuid> [--format table|json]")
	fmt.Fprintln(os.Stderr, "  scoop person-identities unarchive <identity_ref-or-person_identity_uuid> [--format table|json]")
}
