package translation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"horse.fit/scoop/internal/db"
)

type stubTranslationStore struct {
	upsertSourceID       int64
	upsertSourceErr      error
	upsertSourceCalls    []db.UpsertTranslationSourceParams
	upsertResultErr      error
	upsertResultCalls    []db.UpsertTranslationResultParams
	storyRow             db.TranslationStoryTarget
	storyErr             error
	storyRows            []db.TranslationStoryTarget
	storyRowsErr         error
	articleRow           db.TranslationArticleTarget
	articleErr           error
	articleRows          []db.TranslationArticleTarget
	articleRowsErr       error
	storyTranslationRows []db.StoryTranslationRow
	storyTranslationsErr error
	collectionMode       string
	collectionModes      map[string]string
	collectionModeErr    error
	lookupCachedResult   *db.CachedTranslationRow
	lookupCachedErr      error
	lookupCachedTargets  []string
}

func TestHydrateArticleTextForTranslationKeepsSubstantiveText(t *testing.T) {
	manager := &Manager{}
	article := articleTranslationTarget{
		Title: "Short title",
		Text:  strings.Repeat("body ", 30),
	}

	if err := manager.hydrateArticleTextForTranslation(context.Background(), &article); err != nil {
		t.Fatalf("hydrateArticleTextForTranslation() error = %v", err)
	}
	if article.TextOrigin != ContentOriginNormalized {
		t.Fatalf("unexpected origin: %q", article.TextOrigin)
	}
	if !strings.Contains(article.Text, "body") {
		t.Fatalf("unexpected text: %q", article.Text)
	}
}

func TestHydrateArticleTextForTranslationUsesReaderForThinText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body><article><p>Reader supplied translation text with more context than the title.</p></article></body></html>`))
	}))
	t.Cleanup(server.Close)

	manager := &Manager{}
	url := server.URL
	article := articleTranslationTarget{
		Title:        "Reader supplied translation text",
		Text:         "Reader supplied translation text",
		CanonicalURL: &url,
	}

	if err := manager.hydrateArticleTextForTranslation(context.Background(), &article); err != nil {
		t.Fatalf("hydrateArticleTextForTranslation() error = %v", err)
	}
	if article.TextOrigin != ContentOriginReader {
		t.Fatalf("unexpected origin: %q", article.TextOrigin)
	}
	if article.Text == "" || article.Text == article.Title {
		t.Fatalf("expected hydrated reader text, got %q", article.Text)
	}
}

func (s *stubTranslationStore) GetTranslationStoryByUUID(_ context.Context, _ string) (db.TranslationStoryTarget, error) {
	if s.storyErr != nil {
		return db.TranslationStoryTarget{}, s.storyErr
	}
	if s.storyRow.StoryID != 0 {
		return s.storyRow, nil
	}
	return db.TranslationStoryTarget{}, db.ErrNoRows
}

func (s *stubTranslationStore) ListTranslationStoriesByCollection(_ context.Context, _ string) ([]db.TranslationStoryTarget, error) {
	if s.storyRowsErr != nil {
		return nil, s.storyRowsErr
	}
	return s.storyRows, nil
}

func (s *stubTranslationStore) ListTranslationStoryArticles(_ context.Context, _ int64) ([]db.TranslationArticleTarget, error) {
	if s.articleRowsErr != nil {
		return nil, s.articleRowsErr
	}
	return s.articleRows, nil
}

func (s *stubTranslationStore) GetTranslationArticleByUUID(_ context.Context, _ string) (db.TranslationArticleTarget, error) {
	if s.articleErr != nil {
		return db.TranslationArticleTarget{}, s.articleErr
	}
	if s.articleRow.ArticleID != 0 {
		return s.articleRow, nil
	}
	return db.TranslationArticleTarget{}, db.ErrNoRows
}

func (s *stubTranslationStore) GetCollectionTranslationMode(_ context.Context, collection string) (string, error) {
	if s.collectionModeErr != nil {
		return "", s.collectionModeErr
	}
	if s.collectionModes != nil {
		if mode, ok := s.collectionModes[collection]; ok {
			return mode, nil
		}
	}
	if s.collectionMode != "" {
		return s.collectionMode, nil
	}
	return db.DefaultCollectionTranslationMode(collection), nil
}

func (s *stubTranslationStore) ListStoryTranslationRows(_ context.Context, _ int64) ([]db.StoryTranslationRow, error) {
	if s.storyTranslationsErr != nil {
		return nil, s.storyTranslationsErr
	}
	return s.storyTranslationRows, nil
}

func (s *stubTranslationStore) LookupCachedTranslationRow(
	_ context.Context,
	_ int64,
	targetLang string,
) (*db.CachedTranslationRow, error) {
	s.lookupCachedTargets = append(s.lookupCachedTargets, targetLang)
	if s.lookupCachedErr != nil {
		return nil, s.lookupCachedErr
	}
	return s.lookupCachedResult, nil
}

func (s *stubTranslationStore) UpsertTranslationSource(
	_ context.Context,
	row db.UpsertTranslationSourceParams,
) (int64, error) {
	s.upsertSourceCalls = append(s.upsertSourceCalls, row)
	if s.upsertSourceErr != nil {
		return 0, s.upsertSourceErr
	}
	if s.upsertSourceID == 0 {
		s.upsertSourceID = 1
	}
	return s.upsertSourceID, nil
}

func (s *stubTranslationStore) UpsertTranslationResult(
	_ context.Context,
	row db.UpsertTranslationResultParams,
) error {
	s.upsertResultCalls = append(s.upsertResultCalls, row)
	if s.upsertResultErr != nil {
		return s.upsertResultErr
	}
	return nil
}

type stubProvider struct {
	name  string
	calls int
	resp  TranslateResponse
	err   error
}

func (p *stubProvider) Translate(_ context.Context, _ TranslateRequest) (*TranslateResponse, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	resp := p.resp
	if resp.ProviderName == "" {
		resp.ProviderName = p.name
	}
	return &resp, nil
}

func (p *stubProvider) Name() string {
	return p.name
}

func (p *stubProvider) SupportedLanguages() []string {
	return []string{"en", "zh"}
}

func TestRunTasks_HashesSourceAndUpserts(t *testing.T) {
	t.Parallel()

	store := &stubTranslationStore{upsertSourceID: 77}
	provider := &stubProvider{
		name: "stub",
		resp: TranslateResponse{
			Text:       "你好，世界",
			SourceLang: "en",
			TargetLang: "zh",
			LatencyMs:  15,
		},
	}
	registry := NewRegistry("stub")
	if err := registry.Register(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	manager := NewManagerWithStore(store, registry)

	tasks := []translationTask{{
		SourceType:    SourceTypeStoryTitle,
		SourceID:      42,
		SourceLang:    "en",
		OriginalText:  "Hello world",
		ContentOrigin: ContentOriginNormalized,
	}}
	stats, err := manager.runTasks(context.Background(), tasks, RunOptions{TargetLang: "zh", Provider: "stub"})
	if err != nil {
		t.Fatalf("run tasks: %v", err)
	}

	if stats.Total != 1 || stats.Translated != 1 || stats.Cached != 0 || stats.Skipped != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if provider.calls != 1 {
		t.Fatalf("unexpected provider call count: got %d want 1", provider.calls)
	}

	if len(store.upsertSourceCalls) != 1 {
		t.Fatalf("unexpected upsert source call count: got %d want 1", len(store.upsertSourceCalls))
	}
	upsertSource := store.upsertSourceCalls[0]
	wantHash := sha256.Sum256([]byte("Hello world"))
	if !bytes.Equal(upsertSource.ContentHash, wantHash[:]) {
		t.Fatalf("unexpected content hash")
	}
	if upsertSource.SourceType != SourceTypeStoryTitle || upsertSource.SourceID != 42 {
		t.Fatalf("unexpected upsert source identity: %+v", upsertSource)
	}

	if len(store.upsertResultCalls) != 1 {
		t.Fatalf("unexpected upsert result call count: got %d want 1", len(store.upsertResultCalls))
	}
	upsertResult := store.upsertResultCalls[0]
	if upsertResult.TranslationSourceID != 77 {
		t.Fatalf("unexpected translation_source_id: got %d want 77", upsertResult.TranslationSourceID)
	}
	if upsertResult.TargetLang != "zh" {
		t.Fatalf("unexpected target lang: got %q want zh", upsertResult.TargetLang)
	}
}

func TestRunTasks_CachedAndDryRunOutcomes(t *testing.T) {
	t.Parallel()

	task := translationTask{
		SourceType:    SourceTypeStoryTitle,
		SourceID:      42,
		SourceLang:    "en",
		OriginalText:  "Hello world",
		ContentOrigin: "reader",
	}

	cachedStore := &stubTranslationStore{
		upsertSourceID: 77,
		lookupCachedResult: &db.CachedTranslationRow{
			TranslationUUID: "translation-uuid",
			SourceType:      SourceTypeStoryTitle,
			SourceID:        42,
			SourceLang:      "en",
			TargetLang:      "zh",
			OriginalText:    "Hello world",
			TranslatedText:  "你好",
			ProviderName:    "stub",
		},
	}
	cachedProvider := &stubProvider{name: "stub", resp: TranslateResponse{Text: "ignored"}}
	cachedManager := NewManagerWithStore(cachedStore, registryWithProvider(t, cachedProvider))

	stats, err := cachedManager.runTasks(context.Background(), []translationTask{task}, RunOptions{TargetLang: "zh", Provider: "stub"})
	if err != nil {
		t.Fatalf("run cached task: %v", err)
	}
	if stats.Total != 1 || stats.Cached != 1 || stats.Translated != 0 || stats.Skipped != 0 {
		t.Fatalf("unexpected cached stats: %+v", stats)
	}
	if cachedProvider.calls != 0 {
		t.Fatalf("cached task called provider %d times", cachedProvider.calls)
	}
	if got := cachedStore.upsertSourceCalls[0].ContentOrigin; got != ContentOriginReader {
		t.Fatalf("content origin = %q, want reader", got)
	}

	dryStore := &stubTranslationStore{upsertSourceID: 88}
	dryProvider := &stubProvider{name: "stub", resp: TranslateResponse{Text: "ignored"}}
	dryManager := NewManagerWithStore(dryStore, registryWithProvider(t, dryProvider))
	stats, err = dryManager.runTasks(context.Background(), []translationTask{task}, RunOptions{TargetLang: "zh", Provider: "stub", DryRun: true})
	if err != nil {
		t.Fatalf("run dry task: %v", err)
	}
	if stats.Total != 1 || stats.Skipped != 1 || stats.Translated != 0 || stats.Cached != 0 {
		t.Fatalf("unexpected dry-run stats: %+v", stats)
	}
	if dryProvider.calls != 0 {
		t.Fatalf("dry-run task called provider %d times", dryProvider.calls)
	}
	if len(dryStore.upsertResultCalls) != 0 {
		t.Fatalf("dry-run task persisted results: %+v", dryStore.upsertResultCalls)
	}
}

func TestRunTasks_ForceIgnoresCacheAndPersistsResolvedMetadata(t *testing.T) {
	t.Parallel()

	store := &stubTranslationStore{
		upsertSourceID: 77,
		lookupCachedResult: &db.CachedTranslationRow{
			TranslationUUID: "translation-uuid",
			TranslatedText:  "cached",
		},
	}
	provider := &stubProvider{
		name: "stub",
		resp: TranslateResponse{
			Text:         " refreshed ",
			SourceLang:   "fr",
			TargetLang:   "zh-Hant",
			ProviderName: "provider-response",
			LatencyMs:    -1,
		},
	}
	manager := NewManagerWithStore(store, registryWithProvider(t, provider))

	stats, err := manager.runTasks(context.Background(), []translationTask{{
		SourceType:    SourceTypeArticleText,
		SourceID:      101,
		SourceLang:    "",
		OriginalText:  "Hello",
		ContentOrigin: "unknown",
	}}, RunOptions{TargetLang: "zh", Provider: "stub", Force: true})
	if err != nil {
		t.Fatalf("run forced task: %v", err)
	}
	if stats.Total != 1 || stats.Translated != 1 || stats.Cached != 0 || stats.Skipped != 0 {
		t.Fatalf("unexpected force stats: %+v", stats)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
	if len(store.upsertSourceCalls) != 2 {
		t.Fatalf("source calls = %d, want original plus resolved source lang", len(store.upsertSourceCalls))
	}
	if store.upsertSourceCalls[0].SourceLang != "und" || store.upsertSourceCalls[1].SourceLang != "fr" {
		t.Fatalf("unexpected source langs: %+v", store.upsertSourceCalls)
	}
	result := store.upsertResultCalls[0]
	if result.TranslatedText != "refreshed" || result.TargetLang != "zh" || result.ProviderName != "provider-response" {
		t.Fatalf("unexpected result metadata: %+v", result)
	}
	if result.LatencyMS == nil || *result.LatencyMS != 0 {
		t.Fatalf("latency = %v, want 0", result.LatencyMS)
	}
}

func TestRunTasks_ReturnsOperationalErrors(t *testing.T) {
	t.Parallel()

	task := translationTask{
		SourceType:    SourceTypeStoryTitle,
		SourceID:      42,
		SourceLang:    "en",
		OriginalText:  "Hello",
		ContentOrigin: ContentOriginNormalized,
	}
	provider := &stubProvider{name: "stub", resp: TranslateResponse{Text: "translated"}}

	cases := []struct {
		name  string
		store *stubTranslationStore
		opts  RunOptions
		want  string
	}{
		{
			name:  "missing target language",
			store: &stubTranslationStore{},
			opts:  RunOptions{Provider: "stub"},
			want:  "target language is required",
		},
		{
			name:  "source upsert fails",
			store: &stubTranslationStore{upsertSourceErr: errors.New("source failed")},
			opts:  RunOptions{TargetLang: "zh", Provider: "stub"},
			want:  "source failed",
		},
		{
			name:  "cache lookup fails",
			store: &stubTranslationStore{lookupCachedErr: errors.New("cache failed")},
			opts:  RunOptions{TargetLang: "zh", Provider: "stub"},
			want:  "cache failed",
		},
		{
			name:  "result upsert fails",
			store: &stubTranslationStore{upsertResultErr: errors.New("result failed")},
			opts:  RunOptions{TargetLang: "zh", Provider: "stub"},
			want:  "result failed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manager := NewManagerWithStore(tc.store, registryWithProvider(t, provider))
			_, err := manager.runTasks(context.Background(), []translationTask{task}, tc.opts)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("runTasks() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestRunTasks_ReturnsProviderErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		provider *stubProvider
		want     string
	}{
		{
			name:     "provider translate fails",
			provider: &stubProvider{name: "stub", err: errors.New("provider failed")},
			want:     "provider failed",
		},
		{
			name:     "provider returns empty text",
			provider: &stubProvider{name: "stub", resp: TranslateResponse{Text: "   "}},
			want:     "empty translation",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &stubTranslationStore{}
			manager := NewManagerWithStore(store, registryWithProvider(t, tc.provider))
			_, err := manager.runTasks(context.Background(), []translationTask{{
				SourceType:    SourceTypeStoryTitle,
				SourceID:      42,
				SourceLang:    "en",
				OriginalText:  "Hello",
				ContentOrigin: ContentOriginNormalized,
			}}, RunOptions{TargetLang: "zh", Provider: "stub"})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("runTasks() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestRunTasks_SkipsWhenSourceEqualsTarget(t *testing.T) {
	t.Parallel()

	store := &stubTranslationStore{}
	provider := &stubProvider{
		name: "stub",
		resp: TranslateResponse{Text: "ignored"},
	}
	registry := NewRegistry("stub")
	if err := registry.Register(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	manager := NewManagerWithStore(store, registry)

	tasks := []translationTask{{
		SourceType:    SourceTypeStoryTitle,
		SourceID:      42,
		SourceLang:    "en",
		OriginalText:  "Hello world",
		ContentOrigin: ContentOriginNormalized,
	}}
	stats, err := manager.runTasks(context.Background(), tasks, RunOptions{TargetLang: "en", Provider: "stub"})
	if err != nil {
		t.Fatalf("run tasks: %v", err)
	}

	if stats.Total != 1 || stats.Skipped != 1 || stats.Translated != 0 || stats.Cached != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if provider.calls != 0 {
		t.Fatalf("did not expect provider calls, got %d", provider.calls)
	}
	if len(store.upsertSourceCalls) != 0 {
		t.Fatalf("did not expect upsert source calls, got %d", len(store.upsertSourceCalls))
	}
	if len(store.upsertResultCalls) != 0 {
		t.Fatalf("did not expect upsert result calls, got %d", len(store.upsertResultCalls))
	}
}

func TestTranslateStoryByUUID_RejectsDisabledCollection(t *testing.T) {
	t.Parallel()

	store := &stubTranslationStore{
		storyRow: db.TranslationStoryTarget{
			StoryID:    42,
			StoryUUID:  "story-uuid",
			Collection: "openclaw",
			Title:      "OpenClaw story",
			SourceLang: "en",
		},
		collectionMode: db.TranslationModeDisabled,
	}
	registry := NewRegistry("stub")
	if err := registry.Register(&stubProvider{name: "stub", resp: TranslateResponse{Text: "ignored"}}); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	manager := NewManagerWithStore(store, registry)

	_, err := manager.TranslateStoryByUUID(context.Background(), "story-uuid", RunOptions{TargetLang: "zh", Provider: "stub"})
	if !errors.Is(err, ErrTranslationDisabled) {
		t.Fatalf("expected ErrTranslationDisabled, got %v", err)
	}
	if len(store.upsertSourceCalls) != 0 {
		t.Fatalf("did not expect upsert source calls, got %d", len(store.upsertSourceCalls))
	}
}

func TestTranslateStoryByUUID_NotFoundAndStoreErrors(t *testing.T) {
	t.Parallel()

	manager := NewManagerWithStore(&stubTranslationStore{}, registryWithProvider(t, &stubProvider{name: "stub"}))
	_, err := manager.TranslateStoryByUUID(context.Background(), "missing", RunOptions{TargetLang: "zh", Provider: "stub"})
	if !errors.Is(err, ErrStoryNotFound) {
		t.Fatalf("TranslateStoryByUUID() error = %v, want ErrStoryNotFound", err)
	}

	storeErr := errors.New("story lookup failed")
	manager = NewManagerWithStore(&stubTranslationStore{storyErr: storeErr}, registryWithProvider(t, &stubProvider{name: "stub"}))
	_, err = manager.TranslateStoryByUUID(context.Background(), "story-uuid", RunOptions{TargetLang: "zh", Provider: "stub"})
	if !errors.Is(err, storeErr) {
		t.Fatalf("TranslateStoryByUUID() error = %v, want %v", err, storeErr)
	}

	_, err = (*Manager)(nil).TranslateStoryByUUID(context.Background(), "story-uuid", RunOptions{TargetLang: "zh"})
	if err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("nil manager error = %v, want not initialized", err)
	}
}

func TestTranslateStoryByUUID_SkipsDisabledMemberArticleCollections(t *testing.T) {
	t.Parallel()

	store := &stubTranslationStore{
		upsertSourceID: 77,
		storyRow: db.TranslationStoryTarget{
			StoryID:    42,
			StoryUUID:  "story-uuid",
			Collection: "china_news",
			Title:      "China story",
			SourceLang: "zh",
		},
		articleRows: []db.TranslationArticleTarget{
			{
				ArticleID:  101,
				Collection: "openclaw",
				Title:      "Disabled member title",
				Text:       "Disabled member text",
				SourceLang: "en",
			},
			{
				ArticleID:  102,
				Collection: "metal_news",
				Title:      "Enabled member title",
				Text:       "Enabled member text",
				SourceLang: "en",
			},
		},
		collectionModes: map[string]string{
			"china_news": db.TranslationModeEnabled,
			"metal_news": db.TranslationModeEnabled,
			"openclaw":   db.TranslationModeDisabled,
		},
	}
	provider := &stubProvider{name: "stub", resp: TranslateResponse{Text: "translated"}}
	registry := NewRegistry("stub")
	if err := registry.Register(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	manager := NewManagerWithStore(store, registry)

	stats, err := manager.TranslateStoryByUUID(context.Background(), "story-uuid", RunOptions{TargetLang: "en", Provider: "stub"})
	if err != nil {
		t.Fatalf("translate story: %v", err)
	}
	if stats.Total != 3 {
		t.Fatalf("unexpected total tasks: got %d want 3", stats.Total)
	}

	for _, call := range store.upsertSourceCalls {
		if call.SourceID == 101 {
			t.Fatalf("disabled collection article was queued for translation: %+v", call)
		}
	}
}

func TestTranslateArticleByUUID_TranslatesTitleAndText(t *testing.T) {
	t.Parallel()

	store := &stubTranslationStore{
		upsertSourceID: 77,
		articleRow: db.TranslationArticleTarget{
			ArticleID:   101,
			ArticleUUID: "article-uuid",
			Collection:  "metal_news",
			Title:       "Metal article",
			Text:        "Article body",
			SourceLang:  "en",
		},
		collectionMode: db.TranslationModeEnabled,
	}
	provider := &stubProvider{name: "stub", resp: TranslateResponse{Text: "translated"}}
	registry := NewRegistry("stub")
	if err := registry.Register(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	manager := NewManagerWithStore(store, registry)

	stats, err := manager.TranslateArticleByUUID(context.Background(), "article-uuid", RunOptions{TargetLang: "zh", Provider: "stub"})
	if err != nil {
		t.Fatalf("translate article: %v", err)
	}
	if stats.Total != 2 || stats.Translated != 2 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if provider.calls != 2 {
		t.Fatalf("provider calls = %d, want 2", provider.calls)
	}
	if len(store.upsertSourceCalls) != 2 {
		t.Fatalf("upsert source calls = %d, want 2", len(store.upsertSourceCalls))
	}
}

func TestTranslateArticleByUUID_RejectsDisabledCollection(t *testing.T) {
	t.Parallel()

	store := &stubTranslationStore{
		articleRow: db.TranslationArticleTarget{
			ArticleID:  101,
			Collection: "openclaw",
			Title:      "OpenClaw article",
			SourceLang: "en",
		},
		collectionMode: db.TranslationModeDisabled,
	}
	manager := NewManagerWithStore(store, NewRegistry("stub"))

	_, err := manager.TranslateArticleByUUID(context.Background(), "article-uuid", RunOptions{TargetLang: "zh", Provider: "stub"})
	if !errors.Is(err, ErrTranslationDisabled) {
		t.Fatalf("expected ErrTranslationDisabled, got %v", err)
	}
}

func TestTranslateArticleByUUID_NotFoundValidationAndStoreErrors(t *testing.T) {
	t.Parallel()

	manager := NewManagerWithStore(&stubTranslationStore{}, registryWithProvider(t, &stubProvider{name: "stub"}))
	_, err := manager.TranslateArticleByUUID(context.Background(), "article-uuid", RunOptions{TargetLang: "zh", Provider: "stub"})
	if !errors.Is(err, ErrArticleNotFound) {
		t.Fatalf("TranslateArticleByUUID() error = %v, want ErrArticleNotFound", err)
	}

	storeErr := errors.New("article lookup failed")
	manager = NewManagerWithStore(&stubTranslationStore{articleErr: storeErr}, registryWithProvider(t, &stubProvider{name: "stub"}))
	_, err = manager.TranslateArticleByUUID(context.Background(), "article-uuid", RunOptions{TargetLang: "zh", Provider: "stub"})
	if !errors.Is(err, storeErr) {
		t.Fatalf("TranslateArticleByUUID() error = %v, want %v", err, storeErr)
	}

	manager = NewManagerWithStore(&stubTranslationStore{}, registryWithProvider(t, &stubProvider{name: "stub"}))
	_, err = manager.TranslateArticleByUUID(context.Background(), "article-uuid", RunOptions{Provider: "stub"})
	if err == nil || !strings.Contains(err.Error(), "target language is required") {
		t.Fatalf("TranslateArticleByUUID() error = %v, want target language validation", err)
	}

	_, err = (*Manager)(nil).TranslateArticleByUUID(context.Background(), "article-uuid", RunOptions{TargetLang: "zh"})
	if err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("nil manager error = %v, want not initialized", err)
	}
}

func TestTranslateCollection_ReportsProgressAndAccumulatesStats(t *testing.T) {
	t.Parallel()

	store := &stubTranslationStore{
		upsertSourceID: 77,
		storyRows: []db.TranslationStoryTarget{
			{
				StoryID:    42,
				StoryUUID:  "story-1",
				Collection: "metal_news",
				Title:      "Story one",
				SourceLang: "en",
			},
			{
				StoryID:    43,
				StoryUUID:  "story-2",
				Collection: "metal_news",
				Title:      "Story two",
				SourceLang: "en",
			},
		},
		collectionMode: db.TranslationModeEnabled,
	}
	provider := &stubProvider{name: "stub", resp: TranslateResponse{Text: "translated"}}
	registry := NewRegistry("stub")
	if err := registry.Register(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	manager := NewManagerWithStore(store, registry)

	var progress []CollectionProgress
	stats, err := manager.TranslateCollection(context.Background(), "metal_news", CollectionRunOptions{
		RunOptions: RunOptions{TargetLang: "zh", Provider: "stub"},
		Progress: func(item CollectionProgress) {
			progress = append(progress, item)
		},
	})
	if err != nil {
		t.Fatalf("translate collection: %v", err)
	}
	if stats.Total != 2 || stats.Translated != 2 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if len(progress) != 2 || progress[0].Current != 1 || progress[1].Current != 2 {
		t.Fatalf("unexpected progress: %+v", progress)
	}
}

func TestTranslateCollection_PropagatesCollectionAndStoryErrors(t *testing.T) {
	t.Parallel()

	provider := &stubProvider{name: "stub", resp: TranslateResponse{Text: "translated"}}
	registry := registryWithProvider(t, provider)
	collectionErr := errors.New("collection mode failed")
	manager := NewManagerWithStore(&stubTranslationStore{collectionModeErr: collectionErr}, registry)
	_, err := manager.TranslateCollection(context.Background(), "metal_news", CollectionRunOptions{RunOptions: RunOptions{TargetLang: "zh", Provider: "stub"}})
	if !errors.Is(err, collectionErr) {
		t.Fatalf("TranslateCollection() error = %v, want %v", err, collectionErr)
	}

	listErr := errors.New("list failed")
	manager = NewManagerWithStore(&stubTranslationStore{
		collectionMode: db.TranslationModeEnabled,
		storyRowsErr:   listErr,
	}, registry)
	_, err = manager.TranslateCollection(context.Background(), "metal_news", CollectionRunOptions{RunOptions: RunOptions{TargetLang: "zh", Provider: "stub"}})
	if !errors.Is(err, listErr) {
		t.Fatalf("TranslateCollection() error = %v, want %v", err, listErr)
	}

	_, err = (*Manager)(nil).TranslateCollection(context.Background(), "metal_news", CollectionRunOptions{})
	if err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("nil manager error = %v, want not initialized", err)
	}
}

func TestListStoryTranslationsByUUID_RejectsDisabledCollection(t *testing.T) {
	t.Parallel()

	store := &stubTranslationStore{
		storyRow: db.TranslationStoryTarget{
			StoryID:    42,
			StoryUUID:  "story-uuid",
			Collection: "openclaw",
			Title:      "OpenClaw story",
			SourceLang: "en",
		},
		collectionMode: db.TranslationModeDisabled,
	}
	manager := NewManagerWithStore(store, NewRegistry("stub"))

	_, err := manager.ListStoryTranslationsByUUID(context.Background(), "story-uuid")
	if !errors.Is(err, ErrTranslationDisabled) {
		t.Fatalf("expected ErrTranslationDisabled, got %v", err)
	}
}

func TestListStoryTranslationsByUUID_NotFoundAndStoreErrors(t *testing.T) {
	t.Parallel()

	manager := NewManagerWithStore(&stubTranslationStore{}, NewRegistry("stub"))
	_, err := manager.ListStoryTranslationsByUUID(context.Background(), "missing")
	if !errors.Is(err, ErrStoryNotFound) {
		t.Fatalf("ListStoryTranslationsByUUID() error = %v, want ErrStoryNotFound", err)
	}

	rowsErr := errors.New("rows failed")
	manager = NewManagerWithStore(&stubTranslationStore{
		storyRow: db.TranslationStoryTarget{
			StoryID:    42,
			Collection: "metal_news",
		},
		collectionMode:       db.TranslationModeEnabled,
		storyTranslationsErr: rowsErr,
	}, NewRegistry("stub"))
	_, err = manager.ListStoryTranslationsByUUID(context.Background(), "story-uuid")
	if !errors.Is(err, rowsErr) {
		t.Fatalf("ListStoryTranslationsByUUID() error = %v, want %v", err, rowsErr)
	}

	_, err = (*Manager)(nil).ListStoryTranslationsByUUID(context.Background(), "story-uuid")
	if err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("nil manager error = %v, want not initialized", err)
	}
}

func TestListStoryTranslationsByUUID_FiltersDisabledMemberArticleCollections(t *testing.T) {
	t.Parallel()

	store := &stubTranslationStore{
		storyRow: db.TranslationStoryTarget{
			StoryID:    42,
			StoryUUID:  "story-uuid",
			Collection: "china_news",
			Title:      "China story",
			SourceLang: "zh",
		},
		storyTranslationRows: []db.StoryTranslationRow{
			{
				TranslationUUID:  "story-translation",
				SourceType:       SourceTypeStoryTitle,
				SourceID:         42,
				SourceCollection: "china_news",
				TranslatedText:   "story",
			},
			{
				TranslationUUID:  "disabled-article-translation",
				SourceType:       SourceTypeArticleText,
				SourceID:         101,
				SourceCollection: "openclaw",
				TranslatedText:   "disabled",
			},
			{
				TranslationUUID:  "enabled-article-translation",
				SourceType:       SourceTypeArticleText,
				SourceID:         102,
				SourceCollection: "metal_news",
				TranslatedText:   "enabled",
			},
		},
		collectionModes: map[string]string{
			"china_news": db.TranslationModeEnabled,
			"metal_news": db.TranslationModeEnabled,
			"openclaw":   db.TranslationModeDisabled,
		},
	}
	manager := NewManagerWithStore(store, NewRegistry("stub"))

	items, err := manager.ListStoryTranslationsByUUID(context.Background(), "story-uuid")
	if err != nil {
		t.Fatalf("list story translations: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("unexpected translation count: got %d want 2", len(items))
	}
	for _, item := range items {
		if item.SourceID == 101 {
			t.Fatalf("disabled collection translation was returned: %+v", item)
		}
	}
}

func registryWithProvider(t *testing.T, provider Provider) *Registry {
	t.Helper()
	registry := NewRegistry(provider.Name())
	if err := registry.Register(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	return registry
}
