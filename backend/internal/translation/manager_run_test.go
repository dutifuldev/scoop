package translation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"horse.fit/scoop/internal/db"
)

type stubTranslationStore struct {
	upsertSourceID      int64
	upsertSourceCalls   []db.UpsertTranslationSourceParams
	upsertResultCalls   []db.UpsertTranslationResultParams
	storyRow            db.TranslationStoryTarget
	articleRow          db.TranslationArticleTarget
	articleRows         []db.TranslationArticleTarget
	collectionMode      string
	collectionModes     map[string]string
	lookupCachedResult  *db.CachedTranslationRow
	lookupCachedErr     error
	lookupCachedTargets []string
}

func (s *stubTranslationStore) GetTranslationStoryByUUID(_ context.Context, _ string) (db.TranslationStoryTarget, error) {
	if s.storyRow.StoryID != 0 {
		return s.storyRow, nil
	}
	return db.TranslationStoryTarget{}, db.ErrNoRows
}

func (s *stubTranslationStore) ListTranslationStoriesByCollection(_ context.Context, _ string) ([]db.TranslationStoryTarget, error) {
	return nil, nil
}

func (s *stubTranslationStore) ListTranslationStoryArticles(_ context.Context, _ int64) ([]db.TranslationArticleTarget, error) {
	return s.articleRows, nil
}

func (s *stubTranslationStore) GetTranslationArticleByUUID(_ context.Context, _ string) (db.TranslationArticleTarget, error) {
	if s.articleRow.ArticleID != 0 {
		return s.articleRow, nil
	}
	return db.TranslationArticleTarget{}, db.ErrNoRows
}

func (s *stubTranslationStore) GetCollectionTranslationMode(_ context.Context, collection string) (string, error) {
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
	return nil, nil
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
	return nil
}

type stubProvider struct {
	name  string
	calls int
	resp  TranslateResponse
}

func (p *stubProvider) Translate(_ context.Context, _ TranslateRequest) (*TranslateResponse, error) {
	p.calls++
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
