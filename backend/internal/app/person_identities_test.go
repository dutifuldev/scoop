package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"horse.fit/scoop/internal/db"
)

func TestAvatarResolverResolvesGitHubAvatar(t *testing.T) {
	var gotPath string
	var gotUserAgent string
	var gotAuthorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotUserAgent = r.Header.Get("User-Agent")
		gotAuthorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"avatar_url":"https://avatars.githubusercontent.com/u/123?v=4"}`))
	}))
	defer server.Close()
	t.Setenv("GITHUB_TOKEN", "test-token")

	handle := "joshavant"
	resolver := avatarResolver{
		httpClient:       server.Client(),
		githubAPIBaseURL: server.URL,
	}
	got, err := resolver.resolve(context.Background(), &db.PersonIdentityRecord{
		Provider:    "github",
		Handle:      &handle,
		IdentityRef: "id://github/handle/joshavant",
	})
	if err != nil {
		t.Fatalf("resolve() error = %v", err)
	}
	if gotPath != "/users/joshavant" {
		t.Fatalf("request path = %q, want /users/joshavant", gotPath)
	}
	if got == nil || *got != "https://avatars.githubusercontent.com/u/123?v=4" {
		t.Fatalf("resolve() = %v, want GitHub avatar URL", got)
	}
	if gotUserAgent != "scoop-avatar-refresh/1.0" {
		t.Fatalf("User-Agent = %q, want scoop-avatar-refresh/1.0", gotUserAgent)
	}
	if gotAuthorization != "Bearer test-token" {
		t.Fatalf("Authorization = %q, want bearer token", gotAuthorization)
	}
}

func TestAvatarResolverResolvesDiscordAvatar(t *testing.T) {
	var gotPath string
	var gotUserAgent string
	var gotAuthorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotUserAgent = r.Header.Get("User-Agent")
		gotAuthorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"42","avatar":"abc123"}`))
	}))
	defer server.Close()
	t.Setenv("DISCORD_BOT_TOKEN", "discord-token")

	userID := "42"
	resolver := avatarResolver{
		httpClient:        server.Client(),
		discordAPIBaseURL: server.URL,
	}
	got, err := resolver.resolve(context.Background(), &db.PersonIdentityRecord{
		Provider:       "discord",
		ProviderUserID: &userID,
		IdentityRef:    "id://discord/id/42",
	})
	if err != nil {
		t.Fatalf("resolve() error = %v", err)
	}
	if gotPath != "/users/42" {
		t.Fatalf("request path = %q, want /users/42", gotPath)
	}
	if got == nil || *got != "https://cdn.discordapp.com/avatars/42/abc123.webp?size=128" {
		t.Fatalf("resolve() = %v, want Discord CDN avatar URL", got)
	}
	if gotUserAgent != "scoop-avatar-refresh/1.0" {
		t.Fatalf("User-Agent = %q, want scoop-avatar-refresh/1.0", gotUserAgent)
	}
	if gotAuthorization != "Bot discord-token" {
		t.Fatalf("Authorization = %q, want bot token", gotAuthorization)
	}
}

func TestAvatarResolverDiscordWithoutAvatarReturnsNil(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"42","avatar":null}`))
	}))
	defer server.Close()
	t.Setenv("DISCORD_BOT_TOKEN", "discord-token")

	userID := "42"
	resolver := avatarResolver{
		httpClient:        server.Client(),
		discordAPIBaseURL: server.URL,
	}
	got, err := resolver.resolve(context.Background(), &db.PersonIdentityRecord{
		Provider:       "discord",
		ProviderUserID: &userID,
		IdentityRef:    "id://discord/id/42",
	})
	if err != nil {
		t.Fatalf("resolve() error = %v", err)
	}
	if got != nil {
		t.Fatalf("resolve() = %v, want nil", got)
	}
}

func TestAvatarResolverRequiresDiscordUserIDAndToken(t *testing.T) {
	t.Parallel()

	resolver := avatarResolver{}
	if _, err := resolver.resolve(context.Background(), &db.PersonIdentityRecord{
		Provider:    "discord",
		IdentityRef: "id://discord/id/42",
	}); err == nil {
		t.Fatal("resolve() error = nil, want missing provider_user_id error")
	}
	userID := "42"
	if _, err := resolver.resolve(context.Background(), &db.PersonIdentityRecord{
		Provider:       "discord",
		ProviderUserID: &userID,
		IdentityRef:    "id://discord/id/42",
	}); err == nil {
		t.Fatal("resolve() error = nil, want missing token error")
	}
}

func TestAvatarResolverRequiresGitHubHandle(t *testing.T) {
	t.Parallel()

	resolver := avatarResolver{}
	if _, err := resolver.resolve(context.Background(), &db.PersonIdentityRecord{
		Provider:    "github",
		IdentityRef: "id://github/handle/joshavant",
	}); err == nil {
		t.Fatal("resolve() error = nil, want missing handle error")
	}
}

func TestAvatarResolverRejectsInvalidGitHubHandle(t *testing.T) {
	t.Parallel()

	handle := "-bad"
	resolver := avatarResolver{}
	if _, err := resolver.resolve(context.Background(), &db.PersonIdentityRecord{
		Provider:    "github",
		Handle:      &handle,
		IdentityRef: "id://github/handle/-bad",
	}); err == nil {
		t.Fatal("resolve() error = nil, want invalid handle error")
	}
}

func TestAvatarResolverRejectsUnsupportedProvider(t *testing.T) {
	t.Parallel()

	handle := "alice"
	resolver := avatarResolver{}
	if _, err := resolver.resolve(context.Background(), nil); err == nil {
		t.Fatal("resolve(nil) error = nil, want required identity error")
	}
	if _, err := resolver.resolve(context.Background(), &db.PersonIdentityRecord{
		Provider:    "x",
		Handle:      &handle,
		IdentityRef: "id://x/handle/alice",
	}); err == nil {
		t.Fatal("resolve() error = nil, want unsupported provider error")
	}
}

func TestParseRefreshAvatarCommand(t *testing.T) {
	t.Parallel()

	cfg, exitCode, ok := parseRefreshAvatarCommand([]string{"id://github/handle/octocat", "--format", "json", "--timeout", "4s"})
	if !ok || exitCode != 0 {
		t.Fatalf("parseRefreshAvatarCommand() ok=%t exit=%d", ok, exitCode)
	}
	if cfg.identityRefOrUUID != "id://github/handle/octocat" || cfg.format != outputFormatJSON || cfg.timeout != 4*time.Second {
		t.Fatalf("config = %#v", cfg)
	}

	cases := [][]string{
		nil,
		{"id://github/handle/octocat", "extra"},
		{"id://github/handle/octocat", "--format", "yaml"},
	}
	for _, args := range cases {
		if _, exitCode, ok := parseRefreshAvatarCommand(args); ok || exitCode != 2 {
			t.Fatalf("parseRefreshAvatarCommand(%v) ok=%t exit=%d, want validation failure", args, ok, exitCode)
		}
	}
}

func TestPersonIdentityLookupErrorMessages(t *testing.T) {
	t.Parallel()

	if got := personIdentityLookupError(db.ErrNoRows).Error(); got != "person identity not found" {
		t.Fatalf("not found error = %q", got)
	}
	if got := personIdentityLookupError(io.ErrUnexpectedEOF).Error(); !strings.Contains(got, "failed to show person identity") {
		t.Fatalf("lookup error = %q", got)
	}
}

func TestParseRefreshAvatarsCommand(t *testing.T) {
	t.Parallel()

	cfg, exitCode, ok := parseRefreshAvatarsCommand([]string{"--provider", "GitHub", "--include-archived", "--limit", "5", "--format", "json"})
	if !ok || exitCode != 0 {
		t.Fatalf("parseRefreshAvatarsCommand() ok=%t exit=%d", ok, exitCode)
	}
	if cfg.provider != "github" || cfg.format != outputFormatJSON || !cfg.includeArchived || cfg.limit != 5 {
		t.Fatalf("config = %#v", cfg)
	}

	cases := [][]string{
		{"extra"},
		{"--provider", "mastodon"},
		{"--format", "yaml"},
	}
	for _, args := range cases {
		if _, exitCode, ok := parseRefreshAvatarsCommand(args); ok || exitCode != 2 {
			t.Fatalf("parseRefreshAvatarsCommand(%v) ok=%t exit=%d, want validation failure", args, ok, exitCode)
		}
	}
}

func TestParsePersonIdentitiesListCommand(t *testing.T) {
	t.Parallel()

	cfg, exitCode, ok := parsePersonIdentitiesListCommand([]string{"--q", "octo", "--include-archived", "--limit", "3", "--format", "json"})
	if !ok || exitCode != 0 {
		t.Fatalf("parsePersonIdentitiesListCommand() ok=%t exit=%d", ok, exitCode)
	}
	if cfg.query != "octo" || cfg.limit != 3 || !cfg.includeArchived || cfg.format != outputFormatJSON {
		t.Fatalf("config = %#v", cfg)
	}
	if _, exitCode, ok := parsePersonIdentitiesListCommand([]string{"extra"}); ok || exitCode != 2 {
		t.Fatalf("positional parse ok=%t exit=%d, want validation failure", ok, exitCode)
	}
	if _, exitCode, ok := parsePersonIdentitiesListCommand([]string{"--format", "yaml"}); ok || exitCode != 2 {
		t.Fatalf("format parse ok=%t exit=%d, want validation failure", ok, exitCode)
	}
}

func TestPersonIdentityRendering(t *testing.T) {
	handle := "octocat"
	userID := "42"
	avatarURL := "https://avatars.example/octocat.png"
	identity := &db.PersonIdentityRecord{
		Provider:       "github",
		Handle:         &handle,
		ProviderUserID: &userID,
		AvatarURL:      &avatarURL,
		IdentityRef:    "id://github/handle/octocat",
	}

	output := captureStdout(t, func() error {
		if code := printPersonIdentityResult(identity, outputFormatTable); code != 0 {
			t.Fatalf("printPersonIdentityResult(table) code = %d, want 0", code)
		}
		return nil
	})
	assertContainsAll(t, output, "provider", "github", "octocat", "id://github/handle/octocat")

	output = captureStdout(t, func() error {
		if code := printPersonIdentityResult(identity, outputFormatJSON); code != 0 {
			t.Fatalf("printPersonIdentityResult(json) code = %d, want 0", code)
		}
		return nil
	})
	assertContainsAll(t, output, `"provider": "github"`, `"handle": "octocat"`)

	if code := printPersonIdentityResult(identity, "yaml"); code != 2 {
		t.Fatalf("printPersonIdentityResult(invalid format) code = %d, want 2", code)
	}
}

func TestDiscordAndGitHubAvatarResponseParsing(t *testing.T) {
	t.Parallel()

	discordURL, err := discordAvatarURLFromResponse(jsonResponse(`{"id":"42","avatar":"abc"}`, http.StatusOK), "42")
	if err != nil {
		t.Fatalf("discordAvatarURLFromResponse() error = %v", err)
	}
	if discordURL == nil || !strings.Contains(*discordURL, "/avatars/42/abc.webp") {
		t.Fatalf("discord avatar URL = %v", discordURL)
	}
	got, err := discordAvatarURLFromResponse(jsonResponse(`{"id":"42","avatar":null}`, http.StatusOK), "42")
	if err != nil {
		t.Fatalf("discordAvatarURLFromResponse(null) error = %v", err)
	}
	if got != nil {
		t.Fatalf("missing discord avatar URL = %v, want nil", got)
	}

	githubURL := "https://avatars.githubusercontent.com/u/1?v=4"
	got, err = githubAvatarURLFromResponse(jsonResponse(`{"avatar_url":"`+githubURL+`"}`, http.StatusOK))
	if err != nil {
		t.Fatalf("githubAvatarURLFromResponse() error = %v", err)
	}
	if got == nil || *got != githubURL {
		t.Fatalf("github avatar URL = %v", got)
	}
	got, err = githubAvatarURLFromResponse(jsonResponse(`{"avatar_url":""}`, http.StatusOK))
	if err != nil {
		t.Fatalf("githubAvatarURLFromResponse(empty) error = %v", err)
	}
	if got != nil {
		t.Fatalf("missing github avatar URL = %v, want nil", got)
	}

	if _, err := discordAvatarURLFromResponse(jsonResponse(`{"error":"missing"}`, http.StatusNotFound), "42"); err == nil {
		t.Fatalf("discordAvatarURLFromResponse(status) error = nil, want status error")
	}
	if _, err := discordAvatarURLFromResponse(jsonResponse(`{`, http.StatusOK), "42"); err == nil {
		t.Fatalf("discordAvatarURLFromResponse(bad json) error = nil, want decode error")
	}
	if _, err := githubAvatarURLFromResponse(jsonResponse(`{"message":"rate"}`, http.StatusTooManyRequests)); err == nil {
		t.Fatalf("githubAvatarURLFromResponse(status) error = nil, want status error")
	}
	if _, err := githubAvatarURLFromResponse(jsonResponse(`{`, http.StatusOK)); err == nil {
		t.Fatalf("githubAvatarURLFromResponse(bad json) error = nil, want decode error")
	}
	if err := requireHTTPSuccess(http.StatusNoContent, "fetch"); err != nil {
		t.Fatalf("requireHTTPSuccess(204) error = %v", err)
	}
	if err := requireHTTPSuccess(http.StatusInternalServerError, "fetch"); err == nil {
		t.Fatalf("requireHTTPSuccess(500) error = nil, want status error")
	}
}

func TestAvatarResolverHelpers(t *testing.T) {
	t.Parallel()

	resolver := avatarResolver{}
	if resolver.client() != http.DefaultClient {
		t.Fatalf("nil resolver client should fall back to http.DefaultClient")
	}
	if got := resolver.githubUsersURL("octocat"); got != "https://api.github.com/users/octocat" {
		t.Fatalf("githubUsersURL fallback = %q", got)
	}
	if got := resolver.discordUsersURL("42"); got != "https://discord.com/api/v10/users/42" {
		t.Fatalf("discordUsersURL fallback = %q", got)
	}
	if got := providerUsersURL(" https://api.example/ ", "https://fallback.example", "alice"); got != "https://api.example/users/alice" {
		t.Fatalf("providerUsersURL() = %q", got)
	}
	if !isValidGitHubHandle("octo-cat") || isValidGitHubHandle("-octocat") {
		t.Fatalf("GitHub handle validation returned unexpected result")
	}
}

func jsonResponse(body string, statusCode int) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
