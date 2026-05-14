package pipeline

import (
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"horse.fit/scoop/internal/db"
	textnormalize "horse.fit/scoop/internal/normalize"
	"horse.fit/scoop/internal/textmetrics"
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

	score := textmetrics.TitleTokenJaccard("Acme launches orbital drone", "Acme launches drone platform")
	if score <= 0 || score >= 1 {
		t.Fatalf("expected partial overlap score in (0,1), got %f", score)
	}
}

func TestTitleTrigramJaccard(t *testing.T) {
	t.Parallel()

	score := textmetrics.TitleTrigramJaccard("OpenAI releases model", "OpenAI released model")
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

func TestBestLexicalAutoMergePrefersClosestSimhash(t *testing.T) {
	t.Parallel()

	publishedAt := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	articleHash := int64(0b101010)
	nearHash := int64(0b101011)
	farHash := int64(0b111000)
	article := pendingArticle{
		NormalizedTitle: "openclaw gateway setup guide",
		PublishedAt:     &publishedAt,
		TitleSimhash:    &articleHash,
	}
	candidates := []storyCandidate{
		{
			StoryID:      1,
			Title:        "openclaw gateway install notes",
			LastSeenAt:   publishedAt.Add(2 * time.Hour),
			TitleSimhash: &farHash,
		},
		{
			StoryID:      2,
			Title:        "openclaw gateway setup guide",
			LastSeenAt:   publishedAt.Add(time.Hour),
			TitleSimhash: &nearHash,
		},
	}

	match := bestLexicalAutoMerge(article, candidates)
	if match.Candidate.StoryID != 2 {
		t.Fatalf("expected closest simhash story 2, got %d", match.Candidate.StoryID)
	}
	if match.Signal != "lexical_simhash" {
		t.Fatalf("expected lexical_simhash signal, got %q", match.Signal)
	}
	if match.SimhashDistance == nil || *match.SimhashDistance != 1 {
		t.Fatalf("expected distance 1, got %v", match.SimhashDistance)
	}
}

func TestBestLexicalAutoMergeUsesOverlapWhenSimhashUnavailable(t *testing.T) {
	t.Parallel()

	publishedAt := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	article := pendingArticle{
		NormalizedTitle: "openclaw cron plugin setup",
		PublishedAt:     &publishedAt,
	}
	candidates := []storyCandidate{
		{
			StoryID:    7,
			Title:      "openclaw cron plugin setup",
			LastSeenAt: publishedAt.Add(30 * time.Minute),
		},
	}

	match := bestLexicalAutoMerge(article, candidates)
	if match.Candidate.StoryID != 7 {
		t.Fatalf("expected overlap story 7, got %d", match.Candidate.StoryID)
	}
	if match.Signal != "lexical_overlap" {
		t.Fatalf("expected lexical_overlap signal, got %q", match.Signal)
	}
	if match.TitleOverlap < defaultLexicalTrigramThreshold {
		t.Fatalf("expected overlap above threshold, got %f", match.TitleOverlap)
	}
}

func TestLexicalOverlapRejectsStaleCandidate(t *testing.T) {
	t.Parallel()

	publishedAt := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	article := pendingArticle{
		NormalizedTitle: "openclaw cron plugin setup",
		PublishedAt:     &publishedAt,
	}
	candidate := storyCandidate{
		StoryID:    9,
		Title:      "openclaw cron plugin setup",
		LastSeenAt: publishedAt.Add(-30 * 24 * time.Hour),
	}

	if _, ok := lexicalOverlapMatch(article, candidate); ok {
		t.Fatalf("expected stale lexical candidate to be rejected")
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

func TestMatchTypeForSignal(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"exact_url":           "exact_url",
		"exact_source_id":     "exact_source_id",
		"exact_content_hash":  "exact_content_hash",
		"lexical_simhash":     "lexical_simhash",
		"lexical_overlap":     "lexical_overlap",
		"semantic":            "semantic",
		"unrecognized_signal": "manual",
	}
	for signal, want := range cases {
		if got := matchTypeForSignal(signal); got != want {
			t.Fatalf("matchTypeForSignal(%q) = %q, want %q", signal, got, want)
		}
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

func TestSemanticCandidateEvaluation(t *testing.T) {
	t.Parallel()

	published := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	article := pendingArticle{
		NormalizedTitle: "openclaw setup guide",
		PublishedAt:     &published,
	}
	candidates := []semanticCandidate{
		{StoryID: 1, Title: "unrelated hardware discussion", LastSeenAt: published, Cosine: 0.88},
		{StoryID: 2, Title: "openclaw setup guide", LastSeenAt: published.Add(time.Hour), Cosine: defaultSemanticAutoMergeCosine},
	}
	evaluation := evaluateSemanticCandidates(article, candidates)
	if evaluation.best == nil || evaluation.best.Candidate.StoryID != 2 {
		t.Fatalf("best semantic match = %#v, want story 2", evaluation.best)
	}
	if evaluation.autoMerge == nil || evaluation.autoMerge.Candidate.StoryID != 2 {
		t.Fatalf("auto merge = %#v, want story 2", evaluation.autoMerge)
	}
	if !shouldAutoMergeSemantic(defaultSemanticOverrideCosine, 0) {
		t.Fatalf("override cosine should auto-merge without title overlap")
	}
	if shouldAutoMergeSemantic(defaultSemanticAutoMergeCosine, 0) {
		t.Fatalf("auto-merge threshold should still require title overlap")
	}
}

func TestLexicalAndSemanticEventDetails(t *testing.T) {
	t.Parallel()

	distance := 3
	match := lexicalAutoMergeMatch{
		Candidate:       storyCandidate{StoryID: 42},
		Signal:          "lexical_simhash",
		TitleOverlap:    0.8,
		DateConsistency: 1,
		CompositeScore:  0.9,
		MatchScore:      0.95,
		SimhashDistance: &distance,
	}
	details := lexicalMatchDetails(match)
	if details["simhash_distance"] != 3 || details["signal"] != "lexical_simhash" {
		t.Fatalf("lexical details = %#v", details)
	}

	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	semantic := semanticMatch{
		Candidate:       semanticCandidate{StoryID: 99, Cosine: 0.91},
		TitleOverlap:    0.7,
		DateConsistency: 0.6,
		CompositeScore:  0.8,
	}
	event := grayZoneDedupEvent(7, 8, semantic, now)
	if event.BestCandidateStoryID == nil || *event.BestCandidateStoryID != 99 {
		t.Fatalf("gray zone event = %#v", event)
	}
	if event.BestCosine == nil || *event.BestCosine != 0.91 {
		t.Fatalf("gray zone cosine = %v", event.BestCosine)
	}
}

func TestDedupRunConfigDefaultsAndApplyDecision(t *testing.T) {
	t.Parallel()

	service := NewService(nil, zerolog.Nop())
	if _, shouldRun, err := service.dedupRunConfig(DedupOptions{Limit: 1}); err == nil || shouldRun {
		t.Fatalf("nil pool dedupRunConfig should fail and not run")
	}

	service.pool = fakePoolForConfig()
	cfg, shouldRun, err := service.dedupRunConfig(DedupOptions{Limit: 2})
	if err != nil || !shouldRun {
		t.Fatalf("dedupRunConfig() cfg=%#v shouldRun=%t err=%v", cfg, shouldRun, err)
	}
	if cfg.modelName != DefaultEmbeddingModelName || cfg.modelVersion != DefaultEmbeddingModelVersion {
		t.Fatalf("dedup defaults = %#v", cfg)
	}
	if _, shouldRun, err := service.dedupRunConfig(DedupOptions{}); err != nil || shouldRun {
		t.Fatalf("zero limit should not run: shouldRun=%t err=%v", shouldRun, err)
	}

	result := applyDedupDecision(DedupResult{}, decisionNewStory)
	result = applyDedupDecision(result, decisionAutoMerge)
	result = applyDedupDecision(result, decisionGrayZone)
	result = applyDedupDecision(result, decisionNone)
	if result.Processed != 3 || result.NewStories != 1 || result.AutoMerges != 1 || result.GrayZones != 1 {
		t.Fatalf("dedup result = %#v", result)
	}
}

func TestSmallPipelineValueHelpers(t *testing.T) {
	t.Parallel()

	sourceTime := time.Date(2026, 5, 14, 12, 0, 0, 0, time.FixedZone("offset", 3600))
	if got := completePublishedAt(nil, &sourceTime); got == nil || got.Location() != time.UTC {
		t.Fatalf("completePublishedAt() = %v, want UTC source time", got)
	}
	url := " https://example.com/item "
	if got := completeCanonicalURL("", &url); got != strings.TrimSpace(url) {
		t.Fatalf("completeCanonicalURL() = %q", got)
	}
	if got := completeCollection("", " "); got != "unknown" {
		t.Fatalf("completeCollection() = %q, want unknown", got)
	}
	if stringPtrIfNotEmpty("") != nil || stringPtrIfNotEmpty("x") == nil {
		t.Fatalf("stringPtrIfNotEmpty should only return a pointer for non-empty values")
	}
	if hashBytesIfNotEmpty("") != nil || len(hashBytesIfNotEmpty("x")) == 0 {
		t.Fatalf("hashBytesIfNotEmpty should hash only non-empty values")
	}
	if nullableBytes(nil) != nil || len(nullableBytes([]byte{1, 2})) != 2 {
		t.Fatalf("nullableBytes should copy non-empty byte slices")
	}
	if semanticCompositeScore(-1, 0, 0) != 0 || semanticCompositeScore(2, 2, 2) != 1 {
		t.Fatalf("semanticCompositeScore should clamp to [0,1]")
	}
}

func fakePoolForConfig() *db.Pool {
	return &db.Pool{}
}
