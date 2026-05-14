package db

import (
	"bytes"
	"crypto/sha256"
	"testing"
	"time"
)

func TestBuildStoryUpdatePlanNormalizesFields(t *testing.T) {
	t.Parallel()

	title := "  New Title  "
	status := " ACTIVE "
	collection := " OpenClaw "
	rawURL := "https://Example.com/path?q=1"
	now := time.Date(2026, 5, 14, 12, 30, 0, 0, time.FixedZone("test", 3600))

	plan, err := buildStoryUpdatePlan("story-uuid", UpdateStoryOptions{
		Title:      &title,
		Status:     &status,
		Collection: &collection,
		URL:        &rawURL,
	}, now)
	if err != nil {
		t.Fatalf("buildStoryUpdatePlan() error = %v", err)
	}

	wantSet := []string{
		"canonical_title = $2",
		"status = $3",
		"collection = $4",
		"canonical_url = $5",
		"canonical_url_hash = $6",
		"updated_at = $7",
	}
	assertStringSlices(t, plan.set, wantSet)
	assertArgs(t, plan.args, []any{
		"story-uuid",
		"New Title",
		"active",
		"openclaw",
		"https://example.com/path?q=1",
		sha256Bytes("https://example.com/path?q=1"),
		now.UTC(),
	})
}

func TestBuildStoryUpdatePlanRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	empty := "  "
	for _, tt := range []struct {
		name string
		opts UpdateStoryOptions
		want string
	}{
		{name: "no fields", want: "at least one update field is required"},
		{name: "empty title", opts: UpdateStoryOptions{Title: &empty}, want: "title must not be empty"},
		{name: "empty status", opts: UpdateStoryOptions{Status: &empty}, want: "status must not be empty"},
		{name: "empty collection", opts: UpdateStoryOptions{Collection: &empty}, want: "collection must not be empty"},
		{name: "empty url", opts: UpdateStoryOptions{URL: &empty}, want: "url must not be empty"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildStoryUpdatePlan("story-uuid", tt.opts, time.Time{})
			if err == nil || err.Error() != tt.want {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestBuildArticleUpdatePlanIncludesDerivedTitleFields(t *testing.T) {
	t.Parallel()

	title := " New Title "
	source := " discord_archive "
	collection := " OpenClaw "
	rawURL := "https://GitHub.com/owner/repo/issues/123"
	now := time.Date(2026, 5, 14, 12, 30, 0, 0, time.FixedZone("test", -7200))

	staticFields, err := normalizeArticleStaticUpdateFields(UpdateArticleOptions{
		Source:     &source,
		Collection: &collection,
		URL:        &rawURL,
	})
	if err != nil {
		t.Fatalf("normalizeArticleStaticUpdateFields() error = %v", err)
	}
	plan, err := buildArticleUpdatePlan("article-uuid", UpdateArticleOptions{Title: &title}, staticFields, "old body text", now)
	if err != nil {
		t.Fatalf("buildArticleUpdatePlan() error = %v", err)
	}

	wantSet := []string{
		"normalized_title = $2",
		"title_hash = $3",
		"content_hash = $4",
		"title_simhash = $5",
		"token_count = $6",
		"source = $7",
		"collection = $8",
		"canonical_url = $9",
		"canonical_url_hash = $10",
		"source_domain = $11",
		"updated_at = $12",
	}
	assertStringSlices(t, plan.set, wantSet)
	assertArgs(t, plan.args[:5], []any{
		"article-uuid",
		"new title",
		sha256Bytes("new title"),
		sha256Bytes("new title\nold body text"),
		plan.args[4],
	})
	if plan.args[4] == nil {
		t.Fatal("title_simhash arg is nil, want a computed simhash pointer")
	}
	assertArgs(t, plan.args[5:], []any{
		5,
		"discord_archive",
		"openclaw",
		"https://github.com/owner/repo/issues/123",
		sha256Bytes("https://github.com/owner/repo/issues/123"),
		"github.com",
		now.UTC(),
	})
}

func TestBuildArticleUpdatePlanAllowsStaticOnlyUpdates(t *testing.T) {
	t.Parallel()

	source := "github"
	staticFields, err := normalizeArticleStaticUpdateFields(UpdateArticleOptions{Source: &source})
	if err != nil {
		t.Fatalf("normalizeArticleStaticUpdateFields() error = %v", err)
	}
	plan, err := buildArticleUpdatePlan("article-uuid", UpdateArticleOptions{}, staticFields, "", time.Unix(10, 0))
	if err != nil {
		t.Fatalf("buildArticleUpdatePlan() error = %v", err)
	}
	assertStringSlices(t, plan.set, []string{"source = $2", "updated_at = $3"})
	assertArgs(t, plan.args, []any{"article-uuid", "github", time.Unix(10, 0).UTC()})
}

func TestNormalizeArticleUpdateFieldsRejectInvalidInput(t *testing.T) {
	t.Parallel()

	empty := " "
	badURL := "not-a-url"
	if _, err := normalizeArticleStaticUpdateFields(UpdateArticleOptions{Source: &empty}); err == nil || err.Error() != "source must not be empty" {
		t.Fatalf("source error = %v", err)
	}
	if _, err := normalizeArticleStaticUpdateFields(UpdateArticleOptions{Collection: &empty}); err == nil || err.Error() != "collection must not be empty" {
		t.Fatalf("collection error = %v", err)
	}
	if _, err := normalizeArticleStaticUpdateFields(UpdateArticleOptions{URL: &badURL}); err == nil || err.Error() != "url must be a fully-qualified URL" {
		t.Fatalf("url error = %v", err)
	}
	if _, err := normalizeArticleTitleUpdateFields(&empty, "body"); err == nil || err.Error() != "title must not be empty" {
		t.Fatalf("title error = %v", err)
	}
}

func assertStringSlices(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %#v", len(got), len(want), got)
	}
	for index := range got {
		if got[index] != want[index] {
			t.Fatalf("item %d = %q, want %q", index, got[index], want[index])
		}
	}
}

func assertArgs(t *testing.T, got, want []any) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %#v", len(got), len(want), got)
	}
	for index := range got {
		if !argEqual(got[index], want[index]) {
			t.Fatalf("arg %d = %#v, want %#v", index, got[index], want[index])
		}
	}
}

func argEqual(got, want any) bool {
	gotBytes, gotOK := got.([]byte)
	wantBytes, wantOK := want.([]byte)
	if gotOK || wantOK {
		return gotOK && wantOK && bytes.Equal(gotBytes, wantBytes)
	}
	return got == want
}

func sha256Bytes(value string) []byte {
	hash := sha256.Sum256([]byte(value))
	return append([]byte(nil), hash[:]...)
}
