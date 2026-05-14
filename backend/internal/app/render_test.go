package app

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"horse.fit/scoop/internal/db"
)

func TestTableRenderersPreserveCommandOutputShape(t *testing.T) {
	createdAt := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	publishedAt := createdAt.Add(-time.Hour)
	url := "https://example.com/story"
	stats := &db.PipelineStats{
		Collections: []db.StatsCollectionCount{{Collection: "openclaw", Articles: 2, Stories: 1, Embeddings: 2}},
		Totals:      db.StatsTotals{Articles: 2, Stories: 1, Embeddings: 2},
		Throughput:  db.PipelineThroughput{ArticlesIngestedToday: 3, StoriesCreatedToday: 1, PendingNotEmbedded: 4, PendingNotDeduped: 5},
	}
	detail := &db.StoryDetail{
		Story: db.StoryDetailHeader{
			StoryUUID:      "story-uuid",
			CanonicalTitle: "Story title",
			CanonicalURL:   &url,
			SourceCount:    1,
			ArticleCount:   1,
			CreatedAt:      createdAt,
		},
		Articles: []db.StoryDetailArticle{{
			Title:       "Article title",
			URL:         &url,
			Source:      "discord",
			PublishedAt: &publishedAt,
		}},
	}
	digest := digestOutput{
		Date:       "2026-05-14",
		Collection: "openclaw",
		Today:      []db.StorySummary{{StoryUUID: "today", CanonicalTitle: "Today story", SourceCount: 1, ArticleCount: 1, CreatedAt: createdAt}},
		Yesterday:  []db.StorySummary{{StoryUUID: "yesterday", CanonicalTitle: "Yesterday story", SourceCount: 1, ArticleCount: 1, CreatedAt: createdAt}},
	}
	collections := []db.CollectionCount{{
		Collection:        "openclaw",
		TranslationMode:   "off",
		ArticleCount:      2,
		StoryCount:        1,
		EarliestArticleAt: &publishedAt,
		LatestArticleAt:   &createdAt,
	}}
	color := "#ff0000"
	highlightColor := "#fff3b0"
	tags := []db.TagRecord{{
		Tag:            "i0",
		Color:          &color,
		HighlightColor: &highlightColor,
		CreatedAt:      createdAt,
	}}
	articles := []db.ArticleListItem{{
		ArticleID:    42,
		ArticleUUID:  "article-uuid",
		Title:        "Article title",
		Source:       "discord",
		SourceDomain: &url,
		PublishedAt:  &publishedAt,
		Collection:   "openclaw",
		CreatedAt:    createdAt,
	}}
	handle := "octocat"
	providerID := "42"
	avatarURL := "https://avatars.example/octocat.png"
	identities := []db.PersonIdentityRecord{{
		Provider:       "github",
		Handle:         &handle,
		ProviderUserID: &providerID,
		AvatarURL:      &avatarURL,
		IdentityRef:    "id://github/id/42?handle=octocat",
		CreatedAt:      createdAt,
	}}
	summaries := []db.StorySummary{{
		StoryUUID:      "summary-uuid",
		CanonicalTitle: "Summary title",
		CanonicalURL:   &url,
		SourceCount:    1,
		ArticleCount:   1,
		Collection:     "openclaw",
		CreatedAt:      createdAt,
	}}

	output := captureStdout(t, func() error {
		if code := renderStats(stats, outputFormatTable); code != 0 {
			t.Fatalf("renderStats(table) code = %d, want 0", code)
		}
		return nil
	})
	assertContainsAll(t, output, "collection", "openclaw", "TOTAL", "articles_ingested_today", "pending_not_deduped")

	output = captureStdout(t, func() error {
		if code := renderStoryDetail(detail, outputFormatTable); code != 0 {
			t.Fatalf("renderStoryDetail(table) code = %d, want 0", code)
		}
		return nil
	})
	assertContainsAll(t, output, "story", "story_uuid", "Story title", "articles", "Article title", "2026-05-14T11:00:00Z")

	output = captureStdout(t, func() error {
		if code := renderDigest(digest, outputFormatTable); code != 0 {
			t.Fatalf("renderDigest(table) code = %d, want 0", code)
		}
		return nil
	})
	assertContainsAll(t, output, "date: 2026-05-14", "collection: openclaw", "today", "Today story", "yesterday", "Yesterday story")

	output = captureStdout(t, func() error {
		if code := renderCollections(collections, outputFormatTable); code != 0 {
			t.Fatalf("renderCollections() code = %d, want 0", code)
		}
		return nil
	})
	assertContainsAll(t, output, "collection", "translation_mode", "openclaw", "off")

	output = captureStdout(t, func() error {
		if code := renderTagsList(tags, outputFormatTable); code != 0 {
			t.Fatalf("renderTagsList() code = %d, want 0", code)
		}
		return nil
	})
	assertContainsAll(t, output, "tag", "highlight_color", "i0", "#fff3b0")

	output = captureStdout(t, func() error {
		if code := renderArticlesList(articles, outputFormatTable); code != 0 {
			t.Fatalf("renderArticlesList(table) code = %d, want 0", code)
		}
		return nil
	})
	assertContainsAll(t, output, "article_id", "Article title", "discord", "openclaw", "2026-05-14T11:00:00Z")

	output = captureStdout(t, func() error {
		if code := renderStats(stats, outputFormatJSON); code != 0 {
			t.Fatalf("renderStats(json) code = %d, want 0", code)
		}
		return nil
	})
	assertContainsAll(t, output, `"articles": 2`, `"pending_not_deduped": 5`)

	output = captureStdout(t, func() error {
		if code := renderStoryDetail(detail, outputFormatJSON); code != 0 {
			t.Fatalf("renderStoryDetail(json) code = %d, want 0", code)
		}
		return nil
	})
	assertContainsAll(t, output, `"story_uuid": "story-uuid"`, `"title": "Article title"`)

	output = captureStdout(t, func() error {
		if code := renderDigest(digest, outputFormatJSON); code != 0 {
			t.Fatalf("renderDigest(json) code = %d, want 0", code)
		}
		return nil
	})
	assertContainsAll(t, output, `"collection": "openclaw"`, `"story_uuid": "today"`)

	output = captureStdout(t, func() error {
		if code := renderArticlesList(articles, outputFormatJSON); code != 0 {
			t.Fatalf("renderArticlesList(json) code = %d, want 0", code)
		}
		return nil
	})
	assertContainsAll(t, output, `"article_uuid": "article-uuid"`, `"source": "discord"`)

	output = captureStdout(t, func() error {
		if code := renderPersonIdentities(identities, outputFormatTable); code != 0 {
			t.Fatalf("renderPersonIdentities(table) code = %d, want 0", code)
		}
		return nil
	})
	assertContainsAll(t, output, "provider", "github", "octocat", "id://github/id/42?handle=octocat")

	output = captureStdout(t, func() error {
		if code := renderPersonIdentities(identities, outputFormatJSON); code != 0 {
			t.Fatalf("renderPersonIdentities(json) code = %d, want 0", code)
		}
		return nil
	})
	assertContainsAll(t, output, `"provider": "github"`, `"identity_ref": "id://github/id/42?handle=octocat"`)

	output = captureStdout(t, func() error {
		if code := renderStorySummaries(summaries, outputFormatTable); code != 0 {
			t.Fatalf("renderStorySummaries(table) code = %d, want 0", code)
		}
		return nil
	})
	assertContainsAll(t, output, "story_id", "Summary title", "2026-05-14")

	output = captureStdout(t, func() error {
		if code := renderStorySummaries(summaries, outputFormatJSON); code != 0 {
			t.Fatalf("renderStorySummaries(json) code = %d, want 0", code)
		}
		return nil
	})
	assertContainsAll(t, output, `"story_uuid": "summary-uuid"`, `"collection": "openclaw"`)
}

func TestTagResultRendering(t *testing.T) {
	createdAt := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	color := "#ff0000"
	tag := &db.TagRecord{Tag: "i0", Color: &color, CreatedAt: createdAt}

	output := captureStdout(t, func() error {
		if code := printTagResult(tag, outputFormatTable); code != 0 {
			t.Fatalf("printTagResult(table) code = %d, want 0", code)
		}
		return nil
	})
	assertContainsAll(t, output, "tag", "color", "i0", "#ff0000")

	output = captureStdout(t, func() error {
		if code := printTagResult(tag, outputFormatJSON); code != 0 {
			t.Fatalf("printTagResult(json) code = %d, want 0", code)
		}
		return nil
	})
	assertContainsAll(t, output, `"tag": "i0"`, `"color": "#ff0000"`)

	if code := printTagResult(tag, "yaml"); code != 2 {
		t.Fatalf("printTagResult(invalid format) code = %d, want 2", code)
	}
}

func TestWriteStoryDetailTableRejectsNilDetail(t *testing.T) {
	t.Parallel()

	if err := writeStoryDetailTable(nil); err == nil || !strings.Contains(err.Error(), "story detail is nil") {
		t.Fatalf("writeStoryDetailTable(nil) error = %v, want nil detail error", err)
	}
}

func TestTruncateForTable(t *testing.T) {
	t.Parallel()

	if got := truncateForTable("  abcdef  ", 4); got != "a..." {
		t.Fatalf("truncateForTable() = %q, want a...", got)
	}
	if got := truncateForTable("abcdef", 2); got != "ab" {
		t.Fatalf("truncateForTable short max = %q, want ab", got)
	}
	if got := truncateForTable("abcdef", 0); got != "abcdef" {
		t.Fatalf("truncateForTable no limit = %q, want abcdef", got)
	}
}

func captureStdout(t *testing.T, run func() error) string {
	t.Helper()

	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	os.Stdout = writer
	runErr := run()
	_ = writer.Close()
	os.Stdout = original
	if runErr != nil {
		t.Fatalf("run() error = %v", runErr)
	}

	var output bytes.Buffer
	if _, err := io.Copy(&output, reader); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	return output.String()
}

func assertContainsAll(t *testing.T, text string, needles ...string) {
	t.Helper()

	for _, needle := range needles {
		if !strings.Contains(text, needle) {
			t.Fatalf("output missing %q:\n%s", needle, text)
		}
	}
}
