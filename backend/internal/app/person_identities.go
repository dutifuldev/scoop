package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"horse.fit/scoop/internal/cli"
	"horse.fit/scoop/internal/db"
	"horse.fit/scoop/internal/globaltime"
)

func runPersonIdentities(args []string) int {
	return runSubcommands("person-identities", args, []subcommand{
		{names: []string{"list"}, run: runPersonIdentitiesList},
		{names: []string{"show"}, run: runPersonIdentitiesShow},
		{names: []string{"refresh-avatar"}, run: runPersonIdentitiesRefreshAvatar},
		{names: []string{"refresh-avatars"}, run: runPersonIdentitiesRefreshAvatars},
		{names: []string{"archive"}, run: func(args []string) int { return runPersonIdentitiesArchive(args, true) }},
		{names: []string{"unarchive"}, run: func(args []string) int { return runPersonIdentitiesArchive(args, false) }},
	}, printPersonIdentitiesUsage, nil)
}

func runPersonIdentitiesRefreshAvatar(args []string) int {
	return runParsedCommand(args, parseRefreshAvatarCommand, executeRefreshAvatarCommand)
}

type refreshAvatarCommandConfig struct {
	envLoader         *cli.EnvLoader
	timeout           time.Duration
	format            string
	identityRefOrUUID string
}

func parseRefreshAvatarCommand(args []string) (refreshAvatarCommandConfig, int, bool) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: scoop person-identities refresh-avatar <identity_ref-or-person_identity_uuid>")
		return refreshAvatarCommandConfig{}, 2, false
	}
	fs := newAppFlagSet("person-identities refresh-avatar")
	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 30*time.Second, "Command timeout")
	format := fs.String("format", outputFormatTable, "Output format: table or json")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return refreshAvatarCommandConfig{}, 0, false
		}
		return refreshAvatarCommandConfig{}, 2, false
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "too many positional arguments")
		return refreshAvatarCommandConfig{}, 2, false
	}
	outputFormat, err := parseOutputFormat(*format, outputFormatTable)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid format: %v\n", err)
		return refreshAvatarCommandConfig{}, 2, false
	}
	return refreshAvatarCommandConfig{
		envLoader:         envLoader,
		timeout:           *timeout,
		format:            outputFormat,
		identityRefOrUUID: args[0],
	}, 0, true
}

func executeRefreshAvatarCommand(cfg refreshAvatarCommandConfig) int {
	return runReadPoolValue(
		cfg.timeout,
		cfg.envLoader,
		func(ctx context.Context, pool *db.Pool) (*db.PersonIdentityRecord, error) {
			return refreshSelectedAvatar(ctx, pool, cfg.identityRefOrUUID)
		},
		func(updated *db.PersonIdentityRecord) int {
			return printPersonIdentityResult(updated, cfg.format)
		},
	)
}

func refreshSelectedAvatar(ctx context.Context, pool *db.Pool, identityRefOrUUID string) (*db.PersonIdentityRecord, error) {
	identity, err := pool.GetPersonIdentity(ctx, identityRefOrUUID)
	if err != nil {
		return nil, personIdentityLookupError(err)
	}
	avatarURL, err := resolvePersonIdentityAvatarURL(ctx, identity)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh avatar: %w", err)
	}
	updated, err := pool.SetPersonIdentityAvatarURL(ctx, identityRefOrUUID, avatarURL, globaltime.UTC())
	if err != nil {
		return nil, fmt.Errorf("failed to update avatar: %w", err)
	}
	return updated, nil
}

func personIdentityLookupError(err error) error {
	if errors.Is(err, db.ErrNoRows) {
		return errors.New("person identity not found")
	}
	return fmt.Errorf("failed to show person identity: %w", err)
}

func runPersonIdentitiesRefreshAvatars(args []string) int {
	return runParsedCommand(args, parseRefreshAvatarsCommand, executeRefreshAvatarsCommand)
}

type refreshAvatarsCommandConfig struct {
	envLoader       *cli.EnvLoader
	timeout         time.Duration
	format          string
	provider        string
	includeArchived bool
	limit           int
}

func parseRefreshAvatarsCommand(args []string) (refreshAvatarsCommandConfig, int, bool) {
	fs := newAppFlagSet("person-identities refresh-avatars")
	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 60*time.Second, "Command timeout")
	format := fs.String("format", outputFormatTable, "Output format: table or json")
	provider := fs.String("provider", "discord", "Provider to refresh")
	includeArchived := fs.Bool("include-archived", false, "Include archived identities")
	limit := fs.Int("limit", 200, "Maximum identities to scan")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return refreshAvatarsCommandConfig{}, 0, false
		}
		return refreshAvatarsCommandConfig{}, 2, false
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "person-identities refresh-avatars does not accept positional arguments")
		return refreshAvatarsCommandConfig{}, 2, false
	}
	normalizedProvider := strings.ToLower(strings.TrimSpace(*provider))
	if normalizedProvider != "discord" && normalizedProvider != "github" {
		fmt.Fprintln(os.Stderr, "only discord and github avatar refresh are supported")
		return refreshAvatarsCommandConfig{}, 2, false
	}
	outputFormat, err := parseOutputFormat(*format, outputFormatTable)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid format: %v\n", err)
		return refreshAvatarsCommandConfig{}, 2, false
	}
	return refreshAvatarsCommandConfig{
		envLoader:       envLoader,
		timeout:         *timeout,
		format:          outputFormat,
		provider:        normalizedProvider,
		includeArchived: *includeArchived,
		limit:           *limit,
	}, 0, true
}

func executeRefreshAvatarsCommand(cfg refreshAvatarsCommandConfig) int {
	return runReadPoolValue(
		cfg.timeout,
		cfg.envLoader,
		func(ctx context.Context, pool *db.Pool) ([]db.PersonIdentityRecord, error) {
			return refreshAvatars(ctx, pool, cfg)
		},
		func(updated []db.PersonIdentityRecord) int { return renderPersonIdentities(updated, cfg.format) },
	)
}

func refreshAvatars(ctx context.Context, pool *db.Pool, cfg refreshAvatarsCommandConfig) ([]db.PersonIdentityRecord, error) {
	identities, err := pool.ListPersonIdentities(ctx, "", cfg.includeArchived, cfg.limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list person identities: %w", err)
	}
	updated := make([]db.PersonIdentityRecord, 0, len(identities))
	for _, identity := range identities {
		if strings.ToLower(identity.Provider) != cfg.provider {
			continue
		}
		nextIdentity, err := refreshOneAvatar(ctx, pool, identity)
		if err != nil {
			return nil, err
		}
		updated = append(updated, *nextIdentity)
	}
	return updated, nil
}

func refreshOneAvatar(ctx context.Context, pool *db.Pool, identity db.PersonIdentityRecord) (*db.PersonIdentityRecord, error) {
	avatarURL, err := resolvePersonIdentityAvatarURL(ctx, &identity)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh avatar for %s: %w", identity.IdentityRef, err)
	}
	nextIdentity, err := pool.SetPersonIdentityAvatarURL(ctx, identity.IdentityRef, avatarURL, globaltime.UTC())
	if err != nil {
		return nil, fmt.Errorf("failed to update avatar for %s: %w", identity.IdentityRef, err)
	}
	return nextIdentity, nil
}

func renderPersonIdentities(identities []db.PersonIdentityRecord, outputFormat string) int {
	if outputFormat == outputFormatJSON {
		if err := printJSON(identities); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to encode JSON: %v\n", err)
			return 1
		}
		return 0
	}
	return writePersonIdentityTable(identities)
}

type discordUserResponse struct {
	ID     string  `json:"id"`
	Avatar *string `json:"avatar"`
}

type githubUserResponse struct {
	AvatarURL string `json:"avatar_url"`
}

var githubHandlePattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)

type avatarResolver struct {
	httpClient        *http.Client
	discordAPIBaseURL string
	githubAPIBaseURL  string
}

var defaultAvatarResolver = avatarResolver{
	httpClient:        http.DefaultClient,
	discordAPIBaseURL: "https://discord.com/api/v10",
	githubAPIBaseURL:  "https://api.github.com",
}

func resolvePersonIdentityAvatarURL(ctx context.Context, identity *db.PersonIdentityRecord) (*string, error) {
	return defaultAvatarResolver.resolve(ctx, identity)
}

func (r avatarResolver) resolve(ctx context.Context, identity *db.PersonIdentityRecord) (*string, error) {
	if identity == nil {
		return nil, fmt.Errorf("person identity is required")
	}
	switch strings.ToLower(strings.TrimSpace(identity.Provider)) {
	case "discord":
		return r.resolveDiscord(ctx, identity)
	case "github":
		return r.resolveGitHub(ctx, identity)
	default:
		return nil, fmt.Errorf("avatar refresh is not supported for provider %q", identity.Provider)
	}
}

func (r avatarResolver) client() *http.Client {
	if r.httpClient != nil {
		return r.httpClient
	}
	return http.DefaultClient
}

func (r avatarResolver) resolveDiscord(ctx context.Context, identity *db.PersonIdentityRecord) (*string, error) {
	userID, token, err := discordAvatarRequestIdentity(identity)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.discordUsersURL(userID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bot "+token)
	req.Header.Set("User-Agent", "scoop-avatar-refresh/1.0")
	resp, err := r.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch Discord user: %w", err)
	}
	defer resp.Body.Close()
	return discordAvatarURLFromResponse(resp, userID)
}

func discordAvatarURLFromResponse(resp *http.Response, userID string) (*string, error) {
	if err := requireHTTPSuccess(resp.StatusCode, "fetch Discord user"); err != nil {
		return nil, err
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

func discordAvatarRequestIdentity(identity *db.PersonIdentityRecord) (string, string, error) {
	if identity == nil {
		return "", "", fmt.Errorf("person identity is required")
	}
	if strings.ToLower(identity.Provider) != "discord" {
		return "", "", fmt.Errorf("identity provider must be discord")
	}
	if identity.ProviderUserID == nil || strings.TrimSpace(*identity.ProviderUserID) == "" {
		return "", "", fmt.Errorf("discord identity must include provider_user_id")
	}
	token := strings.TrimSpace(os.Getenv("DISCORD_BOT_TOKEN"))
	if token == "" {
		return "", "", fmt.Errorf("DISCORD_BOT_TOKEN is required")
	}
	return strings.TrimSpace(*identity.ProviderUserID), token, nil
}

func (r avatarResolver) discordUsersURL(userID string) string {
	return providerUsersURL(r.discordAPIBaseURL, "https://discord.com/api/v10", userID)
}

func (r avatarResolver) resolveGitHub(ctx context.Context, identity *db.PersonIdentityRecord) (*string, error) {
	handle, err := githubAvatarRequestHandle(identity)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.githubUsersURL(handle), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "scoop-avatar-refresh/1.0")
	if token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := r.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch GitHub user: %w", err)
	}
	defer resp.Body.Close()
	return githubAvatarURLFromResponse(resp)
}

func githubAvatarURLFromResponse(resp *http.Response) (*string, error) {
	if err := requireHTTPSuccess(resp.StatusCode, "fetch GitHub user"); err != nil {
		return nil, err
	}
	var user githubUserResponse
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("decode GitHub user: %w", err)
	}
	avatarURL := strings.TrimSpace(user.AvatarURL)
	if avatarURL == "" {
		return nil, nil
	}
	return &avatarURL, nil
}

func requireHTTPSuccess(statusCode int, label string) error {
	if statusCode >= 200 && statusCode < 300 {
		return nil
	}
	return fmt.Errorf("%s returned HTTP %d", label, statusCode)
}

func githubAvatarRequestHandle(identity *db.PersonIdentityRecord) (string, error) {
	if identity == nil {
		return "", fmt.Errorf("person identity is required")
	}
	if strings.ToLower(identity.Provider) != "github" {
		return "", fmt.Errorf("identity provider must be github")
	}
	if identity.Handle == nil || strings.TrimSpace(*identity.Handle) == "" {
		return "", fmt.Errorf("github identity must include handle")
	}
	handle := strings.TrimSpace(*identity.Handle)
	if !isValidGitHubHandle(handle) {
		return "", fmt.Errorf("invalid github handle %q", handle)
	}
	return handle, nil
}

func (r avatarResolver) githubUsersURL(handle string) string {
	return providerUsersURL(r.githubAPIBaseURL, "https://api.github.com", handle)
}

func providerUsersURL(apiBaseURL, fallbackBaseURL, identity string) string {
	baseURL := strings.TrimRight(strings.TrimSpace(apiBaseURL), "/")
	if baseURL == "" {
		baseURL = fallbackBaseURL
	}
	return baseURL + "/users/" + identity
}

func isValidGitHubHandle(handle string) bool {
	return githubHandlePattern.MatchString(handle)
}

func runPersonIdentitiesList(args []string) int {
	return runParsedCommand(args, parsePersonIdentitiesListCommand, executePersonIdentitiesListCommand)
}

type personIdentitiesListCommandConfig struct {
	envLoader       *cli.EnvLoader
	timeout         time.Duration
	format          string
	includeArchived bool
	query           string
	limit           int
}

func parsePersonIdentitiesListCommand(args []string) (personIdentitiesListCommandConfig, int, bool) {
	fs := newAppFlagSet("person-identities list")
	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 30*time.Second, "Command timeout")
	format := fs.String("format", outputFormatTable, "Output format: table or json")
	includeArchived := fs.Bool("include-archived", false, "Include archived identities")
	query := fs.String("q", "", "Search query")
	limit := fs.Int("limit", 50, "Maximum identities to return")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return personIdentitiesListCommandConfig{}, 0, false
		}
		return personIdentitiesListCommandConfig{}, 2, false
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "person-identities list does not accept positional arguments")
		return personIdentitiesListCommandConfig{}, 2, false
	}
	outputFormat, err := parseOutputFormat(*format, outputFormatTable)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid format: %v\n", err)
		return personIdentitiesListCommandConfig{}, 2, false
	}
	return personIdentitiesListCommandConfig{
		envLoader:       envLoader,
		timeout:         *timeout,
		format:          outputFormat,
		includeArchived: *includeArchived,
		query:           *query,
		limit:           *limit,
	}, 0, true
}

func executePersonIdentitiesListCommand(cfg personIdentitiesListCommandConfig) int {
	ctx, cancel, pool, err := connectReadPool(cfg.timeout, cfg.envLoader)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer cancel()
	defer pool.Close()
	identities, err := pool.ListPersonIdentities(ctx, cfg.query, cfg.includeArchived, cfg.limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list person identities: %v\n", err)
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

func runPersonIdentitiesShow(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: scoop person-identities show <identity_ref-or-person_identity_uuid>")
		return 2
	}
	fs := newAppFlagSet("person-identities show")
	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 30*time.Second, "Command timeout")
	format := fs.String("format", outputFormatTable, "Output format: table or json")
	if exitCode, ok := parseAppFlagSet(fs, args[1:]); !ok {
		return exitCode
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "too many positional arguments")
		return 2
	}
	return runWithReadPool(*timeout, envLoader, func(ctx context.Context, pool *db.Pool) int {
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
	})
}

func runPersonIdentitiesArchive(args []string, archived bool) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: scoop person-identities archive <identity_ref-or-person_identity_uuid>")
		return 2
	}
	fs := newAppFlagSet("person-identities archive")
	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 30*time.Second, "Command timeout")
	format := fs.String("format", outputFormatTable, "Output format: table or json")
	if exitCode, ok := parseAppFlagSet(fs, args[1:]); !ok {
		return exitCode
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "too many positional arguments")
		return 2
	}
	return runWithReadPool(*timeout, envLoader, func(ctx context.Context, pool *db.Pool) int {
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
	})
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
	fmt.Fprintln(os.Stderr, "  scoop person-identities refresh-avatars [--provider discord|github] [--format table|json]")
	fmt.Fprintln(os.Stderr, "  scoop person-identities archive <identity_ref-or-person_identity_uuid> [--format table|json]")
	fmt.Fprintln(os.Stderr, "  scoop person-identities unarchive <identity_ref-or-person_identity_uuid> [--format table|json]")
}
