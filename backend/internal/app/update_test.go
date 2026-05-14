package app

import (
	"errors"
	"testing"
	"time"

	"horse.fit/scoop/internal/db"
)

func TestValidateStoryUpdateOptions(t *testing.T) {
	t.Parallel()

	title := "Updated title"
	empty := " "
	badURL := "not-a-url"
	goodURL := "https://example.com/story"

	tests := []struct {
		name string
		opts db.UpdateStoryOptions
		want string
	}{
		{name: "no fields", want: "at least one update flag is required"},
		{name: "valid title", opts: db.UpdateStoryOptions{Title: &title}},
		{name: "valid url", opts: db.UpdateStoryOptions{URL: &goodURL}},
		{name: "empty title", opts: db.UpdateStoryOptions{Title: &empty}, want: "--title must not be empty"},
		{name: "empty status", opts: db.UpdateStoryOptions{Status: &empty}, want: "--status must not be empty"},
		{name: "empty collection", opts: db.UpdateStoryOptions{Collection: &empty}, want: "--collection must not be empty"},
		{name: "invalid url", opts: db.UpdateStoryOptions{URL: &badURL}, want: "--url must be a fully-qualified URL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateStoryUpdateOptions(tt.opts)
			assertValidationError(t, err, tt.want)
		})
	}
}

func TestValidateArticleUpdateOptions(t *testing.T) {
	t.Parallel()

	source := "discord"
	empty := " "
	badURL := "not-a-url"
	goodURL := "https://example.com/article"

	tests := []struct {
		name string
		opts db.UpdateArticleOptions
		want string
	}{
		{name: "no fields", want: "at least one update flag is required"},
		{name: "valid source", opts: db.UpdateArticleOptions{Source: &source}},
		{name: "valid url", opts: db.UpdateArticleOptions{URL: &goodURL}},
		{name: "empty title", opts: db.UpdateArticleOptions{Title: &empty}, want: "--title must not be empty"},
		{name: "empty source", opts: db.UpdateArticleOptions{Source: &empty}, want: "--source must not be empty"},
		{name: "empty collection", opts: db.UpdateArticleOptions{Collection: &empty}, want: "--collection must not be empty"},
		{name: "invalid url", opts: db.UpdateArticleOptions{URL: &badURL}, want: "--url must be a fully-qualified URL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateArticleUpdateOptions(tt.opts)
			assertValidationError(t, err, tt.want)
		})
	}
}

func TestParseUpdateCommandBuildsStoryOptions(t *testing.T) {
	t.Parallel()

	cfg, exitCode, ok := parseUpdateCommand([]string{
		"story",
		"--title", "  New Story  ",
		"--status", " MERGED ",
		"--collection", " OpenClaw ",
		"--url", "https://example.com/story",
		"--dry-run",
		"00000000-0000-4000-8000-000000000001",
	})
	if !ok || exitCode != 0 {
		t.Fatalf("parseUpdateCommand() ok=%t exit=%d", ok, exitCode)
	}
	if cfg.target != updateTargetStory || cfg.uuid != "00000000-0000-4000-8000-000000000001" || !cfg.dryRun {
		t.Fatalf("config = %#v", cfg)
	}
	if cfg.storyOpts.Title == nil || *cfg.storyOpts.Title != "New Story" {
		t.Fatalf("story title = %v", cfg.storyOpts.Title)
	}
	if cfg.storyOpts.Status == nil || *cfg.storyOpts.Status != "merged" {
		t.Fatalf("story status = %v", cfg.storyOpts.Status)
	}
	if cfg.storyOpts.Collection == nil || *cfg.storyOpts.Collection != "openclaw" {
		t.Fatalf("story collection = %v", cfg.storyOpts.Collection)
	}
}

func TestParseUpdateCommandRejectsTargetSpecificFlags(t *testing.T) {
	t.Parallel()

	if _, exitCode, ok := parseUpdateCommand([]string{"story", "--source", "discord", "story-uuid"}); ok || exitCode != 2 {
		t.Fatalf("story --source ok=%t exit=%d, want validation failure", ok, exitCode)
	}
	if _, exitCode, ok := parseUpdateCommand([]string{"article", "--status", "merged", "article-uuid"}); ok || exitCode != 2 {
		t.Fatalf("article --status ok=%t exit=%d, want validation failure", ok, exitCode)
	}
}

func TestBuildUpdatePreviews(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 14, 8, 0, 0, 0, time.UTC)
	storyTitle := " New Story "
	storyStatus := " MERGED "
	storyURL := " "
	storyPreview := buildStoryUpdatePreview(updateStorySnapshot{
		Title:      "old story",
		Status:     "active",
		Collection: "openclaw",
		URL:        stringPointer("https://old.example/story"),
		UpdatedAt:  now.Add(-time.Hour),
	}, db.UpdateStoryOptions{
		Title:  &storyTitle,
		Status: &storyStatus,
		URL:    &storyURL,
	}, now)
	if storyPreview.Title != "New Story" || storyPreview.Status != "merged" || storyPreview.URL != nil {
		t.Fatalf("story preview = %#v", storyPreview)
	}
	if !storyPreview.UpdatedAt.Equal(now) {
		t.Fatalf("story preview updated_at = %s", storyPreview.UpdatedAt)
	}

	articleTitle := " New Article  Title "
	articleURL := "https://GitHub.com/owner/repo/issues/123"
	articlePreview := buildArticleUpdatePreview(updateArticleSnapshot{
		Title:        "old article",
		Source:       "discord",
		Collection:   "openclaw",
		URL:          stringPointer("https://old.example/article"),
		SourceDomain: stringPointer("old.example"),
		UpdatedAt:    now.Add(-time.Hour),
	}, db.UpdateArticleOptions{
		Title: &articleTitle,
		URL:   &articleURL,
	}, now)
	if articlePreview.Title != "new article title" || articlePreview.URL == nil || *articlePreview.URL != articleURL {
		t.Fatalf("article preview = %#v", articlePreview)
	}
	if articlePreview.SourceDomain == nil || *articlePreview.SourceDomain != "github.com" {
		t.Fatalf("article source domain = %v", articlePreview.SourceDomain)
	}
}

func TestUpdateErrorHelpersReturnFailure(t *testing.T) {
	t.Parallel()

	if code := printUpdateLoadError("story-uuid", db.ErrNoRows, "missing %s\n", "failed %v\n"); code != 1 {
		t.Fatalf("printUpdateLoadError(no rows) code = %d, want 1", code)
	}
	if code := printUpdateApplyError("story-uuid", errors.New("boom"), "missing %s\n", "failed %v\n"); code != 1 {
		t.Fatalf("printUpdateApplyError(error) code = %d, want 1", code)
	}
	if code := writeUpdateResult("before", "after", func(string, string) error {
		return errors.New("render failed")
	}); code != 1 {
		t.Fatalf("writeUpdateResult(error) code = %d, want 1", code)
	}
}

func stringPointer(value string) *string {
	return &value
}

func assertValidationError(t *testing.T, err error, want string) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Fatalf("error = %v, want nil", err)
		}
		return
	}
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}
