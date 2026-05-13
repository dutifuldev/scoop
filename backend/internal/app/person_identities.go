package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
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
	case "refresh-avatar":
		return runPersonIdentitiesRefreshAvatar(args[1:])
	case "refresh-avatars":
		return runPersonIdentitiesRefreshAvatars(args[1:])
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

func runPersonIdentitiesRefreshAvatar(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: scoop person-identities refresh-avatar <identity_ref-or-person_identity_uuid>")
		return 2
	}
	fs := flag.NewFlagSet("person-identities refresh-avatar", flag.ContinueOnError)
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
	avatarURL, err := resolveDiscordAvatarURL(ctx, identity)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to refresh avatar: %v\n", err)
		return 1
	}
	updated, err := pool.SetPersonIdentityAvatarURL(ctx, args[0], avatarURL, globaltime.UTC())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to update avatar: %v\n", err)
		return 1
	}
	return printPersonIdentityResult(updated, *format)
}

func runPersonIdentitiesRefreshAvatars(args []string) int {
	fs := flag.NewFlagSet("person-identities refresh-avatars", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 60*time.Second, "Command timeout")
	format := fs.String("format", outputFormatTable, "Output format: table or json")
	provider := fs.String("provider", "discord", "Provider to refresh")
	includeArchived := fs.Bool("include-archived", false, "Include archived identities")
	limit := fs.Int("limit", 200, "Maximum identities to scan")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "person-identities refresh-avatars does not accept positional arguments")
		return 2
	}
	if strings.ToLower(strings.TrimSpace(*provider)) != "discord" {
		fmt.Fprintln(os.Stderr, "only discord avatar refresh is supported")
		return 2
	}
	ctx, cancel, pool, err := connectReadPool(*timeout, envLoader)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer cancel()
	defer pool.Close()
	identities, err := pool.ListPersonIdentities(ctx, "", *includeArchived, *limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list person identities: %v\n", err)
		return 1
	}
	updated := make([]db.PersonIdentityRecord, 0, len(identities))
	for _, identity := range identities {
		if strings.ToLower(identity.Provider) != "discord" {
			continue
		}
		avatarURL, err := resolveDiscordAvatarURL(ctx, &identity)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to refresh avatar for %s: %v\n", identity.IdentityRef, err)
			return 1
		}
		nextIdentity, err := pool.SetPersonIdentityAvatarURL(ctx, identity.IdentityRef, avatarURL, globaltime.UTC())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to update avatar for %s: %v\n", identity.IdentityRef, err)
			return 1
		}
		updated = append(updated, *nextIdentity)
	}
	if outputFormat, err := parseOutputFormat(*format, outputFormatTable); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid format: %v\n", err)
		return 2
	} else if outputFormat == outputFormatJSON {
		if err := printJSON(updated); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to encode JSON: %v\n", err)
			return 1
		}
		return 0
	}
	return writePersonIdentityTable(updated)
}

type discordUserResponse struct {
	ID     string  `json:"id"`
	Avatar *string `json:"avatar"`
}

func resolveDiscordAvatarURL(ctx context.Context, identity *db.PersonIdentityRecord) (*string, error) {
	if identity == nil {
		return nil, fmt.Errorf("person identity is required")
	}
	if strings.ToLower(identity.Provider) != "discord" {
		return nil, fmt.Errorf("identity provider must be discord")
	}
	if identity.ProviderUserID == nil || strings.TrimSpace(*identity.ProviderUserID) == "" {
		return nil, fmt.Errorf("discord identity must include provider_user_id")
	}
	token := strings.TrimSpace(os.Getenv("DISCORD_BOT_TOKEN"))
	if token == "" {
		return nil, fmt.Errorf("DISCORD_BOT_TOKEN is required")
	}
	userID := strings.TrimSpace(*identity.ProviderUserID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://discord.com/api/v10/users/"+userID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bot "+token)
	req.Header.Set("User-Agent", "scoop-avatar-refresh/1.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch Discord user: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch Discord user returned HTTP %d", resp.StatusCode)
	}
	var user discordUserResponse
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("decode Discord user: %w", err)
	}
	if user.Avatar == nil || strings.TrimSpace(*user.Avatar) == "" {
		return nil, nil
	}
	avatarHash := strings.TrimSpace(*user.Avatar)
	avatarURL := fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.webp?size=128", userID, avatarHash)
	return &avatarURL, nil
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
			pointerStringOrEmpty(identity.AvatarURL),
			identity.IdentityRef,
			formatUTCTimestampPtr(identity.ArchivedAt),
		})
	}
	if err := writeTable([]string{"provider", "handle", "provider_user_id", "avatar_url", "identity_ref", "archived_at"}, rows); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to render table: %v\n", err)
		return 1
	}
	return 0
}

func printPersonIdentitiesUsage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  scoop person-identities list [--include-archived] [--q <query>] [--limit 50] [--format table|json]")
	fmt.Fprintln(os.Stderr, "  scoop person-identities show <identity_ref-or-person_identity_uuid> [--format table|json]")
	fmt.Fprintln(os.Stderr, "  scoop person-identities refresh-avatar <identity_ref-or-person_identity_uuid> [--format table|json]")
	fmt.Fprintln(os.Stderr, "  scoop person-identities refresh-avatars [--provider discord] [--format table|json]")
	fmt.Fprintln(os.Stderr, "  scoop person-identities archive <identity_ref-or-person_identity_uuid> [--format table|json]")
	fmt.Fprintln(os.Stderr, "  scoop person-identities unarchive <identity_ref-or-person_identity_uuid> [--format table|json]")
}
