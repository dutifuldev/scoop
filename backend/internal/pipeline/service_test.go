package pipeline

import (
	"testing"
	"time"

	"github.com/rs/zerolog"

	textnormalize "horse.fit/scoop/internal/normalize"
)

func TestNormalizeURL_StripsTrackingAndNormalizes(t *testing.T) {
	t.Parallel()

	canonical, host := textnormalize.URL("https://Example.COM:443/news/path/?utm_source=abc&fbclid=123&b=2&a=1")
	if canonical != "https://example.com/news/path?a=1&b=2" {
		t.Fatalf("unexpected canonical url: %q", canonical)
	}
	if host != "example.com" {
		t.Fatalf("unexpected host: %q", host)
	}
}

func TestNormalizeURL_Invalid(t *testing.T) {
	t.Parallel()

	canonical, host := textnormalize.URL("not a url")
	if canonical != "" || host != "" {
		t.Fatalf("expected empty result for invalid URL, got canonical=%q host=%q", canonical, host)
	}
}

func TestTitleTokenJaccard(t *testing.T) {
	t.Parallel()

	score := titleTokenJaccard("Acme launches orbital drone", "Acme launches drone platform")
	if score <= 0 || score >= 1 {
		t.Fatalf("expected partial overlap score in (0,1), got %f", score)
	}
}

func TestTitleTrigramJaccard(t *testing.T) {
	t.Parallel()

	score := titleTrigramJaccard("OpenAI releases model", "OpenAI released model")
	if score <= 0 || score >= 1 {
		t.Fatalf("expected partial trigram overlap score in (0,1), got %f", score)
	}
}

func TestTitleSimhashDistance(t *testing.T) {
	t.Parallel()

	left := int64(0b101010)
	right := int64(0b111000)
	distance, ok := titleSimhashDistance(&left, &right)
	if !ok {
		t.Fatalf("expected simhash distance to be available")
	}
	if distance != 2 {
		t.Fatalf("unexpected simhash distance: got %d want 2", distance)
	}
}

func TestIsWithinDateWindow(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 13, 12, 0, 0, 0, time.UTC)
	inside := now.Add(-7 * 24 * time.Hour)
	outside := now.Add(-30 * 24 * time.Hour)

	if !isWithinDateWindow(&inside, now, 14*24*time.Hour) {
		t.Fatalf("expected inside date to be within window")
	}
	if isWithinDateWindow(&outside, now, 14*24*time.Hour) {
		t.Fatalf("expected outside date to be out of window")
	}
}

func TestComputeDateConsistency(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 13, 12, 0, 0, 0, time.UTC)

	if score := computeDateConsistency(nil, now); score != 0.5 {
		t.Fatalf("expected 0.5 for missing publish date, got %f", score)
	}

	d1 := now.Add(-24 * time.Hour)
	if score := computeDateConsistency(&d1, now); score != 1 {
		t.Fatalf("expected 1.0 for <=48h delta, got %f", score)
	}

	d2 := now.Add(-5 * 24 * time.Hour)
	if score := computeDateConsistency(&d2, now); score != 0.6 {
		t.Fatalf("expected 0.6 for <=7d delta, got %f", score)
	}

	d3 := now.Add(-20 * 24 * time.Hour)
	if score := computeDateConsistency(&d3, now); score != 0 {
		t.Fatalf("expected 0.0 for >7d delta, got %f", score)
	}
}

func TestShouldMarkSemanticGrayZone(t *testing.T) {
	t.Parallel()

	if !shouldMarkSemanticGrayZone(0.90) {
		t.Fatalf("expected 0.90 cosine to be gray zone")
	}
	if shouldMarkSemanticGrayZone(0.94) {
		t.Fatalf("did not expect >= auto-merge threshold cosine to be gray zone")
	}
	if shouldMarkSemanticGrayZone(0.75) {
		t.Fatalf("did not expect low cosine to be gray zone")
	}
}

func TestBuildNormalizedArticle_UsesMetadataCollection(t *testing.T) {
	t.Parallel()

	row := rawArrivalRow{
		RawArrivalID: 1,
		Source:       "source-a",
		SourceItemID: "item-1",
		RawPayload: []byte(`{
			"payload_version":"v1",
			"source":"source-a",
			"source_item_id":"item-1",
			"title":"Hello",
			"source_metadata":{
				"collection":"Ai_News",
				"job_name":"job",
				"job_run_id":"run-1",
				"scraped_at":"2026-02-14T00:00:00Z"
			}
		}`),
		FetchedAt: time.Date(2026, 2, 14, 0, 0, 0, 0, time.UTC),
	}

	article := buildNormalizedArticle(row, zerolog.Nop())
	if article.Collection != "ai_news" {
		t.Fatalf("unexpected collection: got %q want %q", article.Collection, "ai_news")
	}
}

func TestBuildNormalizedArticle_FallsBackToRowCollection(t *testing.T) {
	t.Parallel()

	row := rawArrivalRow{
		RawArrivalID: 2,
		Source:       "source-b",
		SourceItemID: "item-2",
		Collection:   "World_News",
		RawPayload:   []byte(`{"bad":"payload"}`),
		FetchedAt:    time.Date(2026, 2, 14, 0, 0, 0, 0, time.UTC),
	}

	article := buildNormalizedArticle(row, zerolog.Nop())
	if article.Collection != "world_news" {
		t.Fatalf("unexpected collection fallback: got %q want %q", article.Collection, "world_news")
	}
}

func TestBuildNormalizedArticle_DetectsLanguageWhenMissing(t *testing.T) {
	t.Parallel()

	row := rawArrivalRow{
		RawArrivalID: 3,
		Source:       "source-c",
		SourceItemID: "item-3",
		RawPayload: []byte(`{
			"payload_version":"v1",
			"source":"source-c",
			"source_item_id":"item-3",
			"title":"Space startup closes major funding round",
			"body_text":"The company announced a new launch partnership and said the investment will expand satellite production capacity.",
			"source_metadata":{
				"collection":"space_news",
				"job_name":"job",
				"job_run_id":"run-1",
				"scraped_at":"2026-02-14T00:00:00Z"
			}
		}`),
		FetchedAt: time.Date(2026, 2, 14, 0, 0, 0, 0, time.UTC),
	}

	article := buildNormalizedArticle(row, zerolog.Nop())
	if article.NormalizedLang != "en" {
		t.Fatalf("expected detected language en, got %q", article.NormalizedLang)
	}
}

func TestBuildNormalizedArticle_FallsBackToUndWhenLinguaCannotDetect(t *testing.T) {
	t.Parallel()

	row := rawArrivalRow{
		RawArrivalID: 4,
		Source:       "source-d",
		SourceItemID: "item-4",
		RawPayload: []byte(`{
			"payload_version":"v1",
			"source":"source-d",
			"source_item_id":"item-4",
			"title":"Hi",
			"body_text":"ok",
			"source_metadata":{
				"collection":"space_news",
				"job_name":"job",
				"job_run_id":"run-1",
				"scraped_at":"2026-02-14T00:00:00Z"
			}
		}`),
		FetchedAt: time.Date(2026, 2, 14, 0, 0, 0, 0, time.UTC),
	}

	article := buildNormalizedArticle(row, zerolog.Nop())
	if article.NormalizedLang != "und" {
		t.Fatalf("expected und fallback language, got %q", article.NormalizedLang)
	}
}

func TestNormalizeISO6391Language(t *testing.T) {
	t.Parallel()

	if got := normalizeISO6391Language("EN-us"); got != "en" {
		t.Fatalf("expected en, got %q", got)
	}
	if got := normalizeISO6391Language("zh_Hans"); got != "zh" {
		t.Fatalf("expected zh, got %q", got)
	}
	if got := normalizeISO6391Language("english"); got != "" {
		t.Fatalf("expected empty normalization for non-iso input, got %q", got)
	}
}
