package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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
	if _, err := resolver.resolve(context.Background(), &db.PersonIdentityRecord{
		Provider:    "x",
		Handle:      &handle,
		IdentityRef: "id://x/handle/alice",
	}); err == nil {
		t.Fatal("resolve() error = nil, want unsupported provider error")
	}
}
