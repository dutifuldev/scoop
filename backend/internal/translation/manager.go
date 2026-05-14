package translation

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"horse.fit/scoop/internal/db"
	"horse.fit/scoop/internal/reader"
)

const (
	SourceTypeStoryTitle    = "story_title"
	SourceTypeArticleTitle  = "article_title"
	SourceTypeArticleText   = "article_text"
	ContentOriginNormalized = "normalized"
	ContentOriginReader     = "reader"

	// Keep reader-fetched body translations bounded by truncating at rune boundaries.
	articleReaderTranslationMaxChars = 6000
)

var (
	ErrStoryNotFound       = errors.New("story not found")
	ErrArticleNotFound     = errors.New("article not found")
	ErrTranslationDisabled = errors.New("translation disabled")
)

// RunOptions controls translation execution.
type RunOptions struct {
	TargetLang string
	Provider   string
	Force      bool
	DryRun     bool
}

// CollectionRunOptions controls collection-level translation execution.
type CollectionRunOptions struct {
	RunOptions
	Progress func(CollectionProgress)
}

// CollectionProgress reports story-level progress for collection translations.
type CollectionProgress struct {
	Current   int
	Total     int
	StoryID   int64
	StoryUUID string
}

// RunStats reports translation execution counters.
type RunStats struct {
	Total      int `json:"total"`
	Translated int `json:"translated"`
	Cached     int `json:"cached"`
	Skipped    int `json:"skipped"`
}

type translationTaskOutcome int

const (
	translationTaskSkipped translationTaskOutcome = iota
	translationTaskCached
	translationTaskTranslated
)

type preparedTranslationTask struct {
	task         translationTask
	originalText string
	contentHash  []byte
	sourceLang   string
}

// CachedTranslation is a cached translation row enriched for API output.
type CachedTranslation struct {
	TranslationUUID string    `json:"translation_uuid"`
	SourceType      string    `json:"source_type"`
	SourceID        int64     `json:"source_id"`
	SourceUUID      *string   `json:"source_uuid,omitempty"`
	SourceLang      string    `json:"source_lang"`
	TargetLang      string    `json:"target_lang"`
	OriginalText    string    `json:"original_text"`
	TranslatedText  string    `json:"translated_text"`
	ProviderName    string    `json:"provider_name"`
	ModelName       *string   `json:"model_name,omitempty"`
	LatencyMS       *int      `json:"latency_ms,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type translationStore interface {
	GetTranslationStoryByUUID(ctx context.Context, storyUUID string) (db.TranslationStoryTarget, error)
	ListTranslationStoriesByCollection(ctx context.Context, collection string) ([]db.TranslationStoryTarget, error)
	ListTranslationStoryArticles(ctx context.Context, storyID int64) ([]db.TranslationArticleTarget, error)
	GetTranslationArticleByUUID(ctx context.Context, articleUUID string) (db.TranslationArticleTarget, error)
	GetCollectionTranslationMode(ctx context.Context, collection string) (string, error)
	ListStoryTranslationRows(ctx context.Context, storyID int64) ([]db.StoryTranslationRow, error)
	LookupCachedTranslationRow(ctx context.Context, translationSourceID int64, targetLang string) (*db.CachedTranslationRow, error)
	UpsertTranslationSource(ctx context.Context, row db.UpsertTranslationSourceParams) (int64, error)
	UpsertTranslationResult(ctx context.Context, row db.UpsertTranslationResultParams) error
}

// Manager coordinates provider calls and persistent translation caching.
type Manager struct {
	store    translationStore
	registry *Registry
}

func NewManager(pool *db.Pool, registry *Registry) *Manager {
	return NewManagerWithStore(pool, registry)
}

func NewManagerWithStore(store translationStore, registry *Registry) *Manager {
	return &Manager{store: store, registry: registry}
}

func (m *Manager) DefaultProvider() string {
	if m == nil || m.registry == nil {
		return ""
	}
	return m.registry.DefaultProvider()
}

func (m *Manager) TranslateStoryByUUID(ctx context.Context, storyUUID string, opts RunOptions) (RunStats, error) {
	if m == nil || m.store == nil {
		return RunStats{}, fmt.Errorf("translation manager is not initialized")
	}

	story, err := m.fetchStoryByUUID(ctx, storyUUID)
	if err != nil {
		return RunStats{}, err
	}
	if err := m.requireCollectionTranslationEnabled(ctx, story.Collection); err != nil {
		return RunStats{}, err
	}
	return m.translateStory(ctx, story, opts)
}

func (m *Manager) TranslateArticleByUUID(ctx context.Context, articleUUID string, opts RunOptions) (RunStats, error) {
	if m == nil || m.store == nil {
		return RunStats{}, fmt.Errorf("translation manager is not initialized")
	}

	targetLang := normalizeLangCode(opts.TargetLang)
	if targetLang == "" {
		return RunStats{}, fmt.Errorf("target language is required")
	}
	opts.TargetLang = targetLang

	article, err := m.fetchArticleByUUID(ctx, articleUUID)
	if err != nil {
		return RunStats{}, err
	}
	if err := m.requireCollectionTranslationEnabled(ctx, article.Collection); err != nil {
		return RunStats{}, err
	}

	tasks := make([]translationTask, 0, 2)
	if strings.TrimSpace(article.Title) != "" {
		tasks = append(tasks, translationTask{
			SourceType:    SourceTypeArticleTitle,
			SourceID:      article.ArticleID,
			SourceLang:    article.SourceLang,
			OriginalText:  article.Title,
			ContentOrigin: ContentOriginNormalized,
		})
	}
	if strings.TrimSpace(article.Text) != "" {
		tasks = append(tasks, translationTask{
			SourceType:    SourceTypeArticleText,
			SourceID:      article.ArticleID,
			SourceLang:    article.SourceLang,
			OriginalText:  article.Text,
			ContentOrigin: article.TextOrigin,
		})
	}

	return m.runTasks(ctx, tasks, opts)
}

func (m *Manager) TranslateCollection(ctx context.Context, collection string, opts CollectionRunOptions) (RunStats, error) {
	if m == nil || m.store == nil {
		return RunStats{}, fmt.Errorf("translation manager is not initialized")
	}
	if err := m.requireCollectionTranslationEnabled(ctx, collection); err != nil {
		return RunStats{}, err
	}

	stories, err := m.listStoriesByCollection(ctx, collection)
	if err != nil {
		return RunStats{}, err
	}

	total := RunStats{}
	for idx, story := range stories {
		if opts.Progress != nil {
			opts.Progress(CollectionProgress{
				Current:   idx + 1,
				Total:     len(stories),
				StoryID:   story.StoryID,
				StoryUUID: story.StoryUUID,
			})
		}

		stats, err := m.translateStory(ctx, story, opts.RunOptions)
		if err != nil {
			return total, err
		}

		total.Total += stats.Total
		total.Translated += stats.Translated
		total.Cached += stats.Cached
		total.Skipped += stats.Skipped
	}

	return total, nil
}

func (m *Manager) ListStoryTranslationsByUUID(ctx context.Context, storyUUID string) ([]CachedTranslation, error) {
	if m == nil || m.store == nil {
		return nil, fmt.Errorf("translation manager is not initialized")
	}

	story, err := m.fetchStoryByUUID(ctx, storyUUID)
	if err != nil {
		return nil, err
	}
	if err := m.requireCollectionTranslationEnabled(ctx, story.Collection); err != nil {
		return nil, err
	}

	rows, err := m.store.ListStoryTranslationRows(ctx, story.StoryID)
	if err != nil {
		return nil, err
	}

	return m.cachedTranslationsForEnabledCollections(ctx, rows)
}

func (m *Manager) cachedTranslationsForEnabledCollections(
	ctx context.Context,
	rows []db.StoryTranslationRow,
) ([]CachedTranslation, error) {
	items := make([]CachedTranslation, 0, len(rows))
	collectionEnabled := map[string]bool{}
	for _, row := range rows {
		enabled, err := m.collectionTranslationEnabled(ctx, row.SourceCollection, collectionEnabled)
		if err != nil {
			return nil, err
		}
		if !enabled {
			continue
		}
		items = append(items, cachedTranslationFromRow(row))
	}
	return items, nil
}

func cachedTranslationFromRow(row db.StoryTranslationRow) CachedTranslation {
	return CachedTranslation{
		TranslationUUID: row.TranslationUUID,
		SourceType:      row.SourceType,
		SourceID:        row.SourceID,
		SourceUUID:      row.SourceUUID,
		SourceLang:      row.SourceLang,
		TargetLang:      row.TargetLang,
		OriginalText:    row.OriginalText,
		TranslatedText:  row.TranslatedText,
		ProviderName:    row.ProviderName,
		ModelName:       row.ModelName,
		LatencyMS:       row.LatencyMS,
		CreatedAt:       row.CreatedAt,
	}
}

func (m *Manager) translateStory(ctx context.Context, story storyTranslationTarget, opts RunOptions) (RunStats, error) {
	targetLang := normalizeLangCode(opts.TargetLang)
	if targetLang == "" {
		return RunStats{}, fmt.Errorf("target language is required")
	}
	opts.TargetLang = targetLang

	articles, err := m.fetchStoryArticles(ctx, story.StoryID)
	if err != nil {
		return RunStats{}, err
	}

	tasks := make([]translationTask, 0, 1+(2*len(articles)))
	if strings.TrimSpace(story.Title) != "" {
		tasks = append(tasks, translationTask{
			SourceType:    SourceTypeStoryTitle,
			SourceID:      story.StoryID,
			SourceLang:    story.SourceLang,
			OriginalText:  story.Title,
			ContentOrigin: ContentOriginNormalized,
		})
	}

	for _, article := range articles {
		if strings.TrimSpace(article.Title) != "" {
			tasks = append(tasks, translationTask{
				SourceType:    SourceTypeArticleTitle,
				SourceID:      article.ArticleID,
				SourceLang:    article.SourceLang,
				OriginalText:  article.Title,
				ContentOrigin: ContentOriginNormalized,
			})
		}
		if strings.TrimSpace(article.Text) != "" {
			tasks = append(tasks, translationTask{
				SourceType:    SourceTypeArticleText,
				SourceID:      article.ArticleID,
				SourceLang:    article.SourceLang,
				OriginalText:  article.Text,
				ContentOrigin: article.TextOrigin,
			})
		}
	}

	return m.runTasks(ctx, tasks, opts)
}

func (m *Manager) runTasks(ctx context.Context, tasks []translationTask, opts RunOptions) (RunStats, error) {
	targetLang := normalizeLangCode(opts.TargetLang)
	if targetLang == "" {
		return RunStats{}, fmt.Errorf("target language is required")
	}

	provider, err := m.resolveProvider(opts.Provider)
	if err != nil {
		return RunStats{}, err
	}
	providerName := provider.Name()
	modelName := modelNameFromProvider(provider)

	stats := RunStats{}
	for _, task := range tasks {
		stats.Total++
		outcome, err := m.runTask(ctx, task, opts, targetLang, provider, providerName, modelName)
		if err != nil {
			return stats, err
		}
		switch outcome {
		case translationTaskCached:
			stats.Cached++
		case translationTaskSkipped:
			stats.Skipped++
		case translationTaskTranslated:
			stats.Translated++
		}
	}

	return stats, nil
}

func (m *Manager) runTask(
	ctx context.Context,
	task translationTask,
	opts RunOptions,
	targetLang string,
	provider Provider,
	providerName string,
	modelName *string,
) (translationTaskOutcome, error) {
	prepared, ok := prepareTranslationTask(task, targetLang)
	if !ok {
		return translationTaskSkipped, nil
	}
	translationSourceID, err := m.upsertPreparedTranslationSource(ctx, prepared)
	if err != nil {
		return translationTaskSkipped, err
	}
	cached, err := m.lookupCachedTranslation(ctx, translationSourceID, targetLang)
	if err != nil {
		return translationTaskSkipped, err
	}
	if cached != nil && !opts.Force {
		return translationTaskCached, nil
	}
	if opts.DryRun {
		return translationTaskSkipped, nil
	}
	resp, err := translatePreparedTask(ctx, provider, prepared, targetLang)
	if err != nil {
		return translationTaskSkipped, err
	}
	return translationTaskTranslated, m.persistTranslatedTask(ctx, prepared, translationSourceID, resp, targetLang, providerName, modelName)
}

func prepareTranslationTask(task translationTask, targetLang string) (preparedTranslationTask, bool) {
	originalText := strings.TrimSpace(task.OriginalText)
	if originalText == "" {
		return preparedTranslationTask{}, false
	}
	sourceLang := normalizeTaskSourceLang(task.SourceLang)
	if shouldSkipTranslationTask(sourceLang, targetLang) {
		return preparedTranslationTask{}, false
	}
	return preparedTranslationTask{
		task:         task,
		originalText: originalText,
		contentHash:  hashTranslationSourceText(originalText),
		sourceLang:   sourceLang,
	}, true
}

func normalizeTaskSourceLang(raw string) string {
	sourceLang := normalizeLangCode(raw)
	if sourceLang == "" {
		return "und"
	}
	return sourceLang
}

func (m *Manager) upsertPreparedTranslationSource(ctx context.Context, prepared preparedTranslationTask) (int64, error) {
	return m.upsertTranslationSource(ctx, upsertTranslationSourceInput{
		SourceType:    prepared.task.SourceType,
		SourceID:      prepared.task.SourceID,
		SourceLang:    prepared.sourceLang,
		ContentHash:   prepared.contentHash,
		OriginalText:  prepared.originalText,
		ContentOrigin: normalizeContentOrigin(prepared.task.ContentOrigin),
	})
}

func translatePreparedTask(
	ctx context.Context,
	provider Provider,
	prepared preparedTranslationTask,
	targetLang string,
) (*TranslateResponse, error) {
	resp, err := provider.Translate(ctx, TranslateRequest{
		Text:       prepared.originalText,
		SourceLang: prepared.sourceLang,
		TargetLang: targetLang,
	})
	if err != nil {
		return nil, fmt.Errorf("translate %s source_id=%d: %w", prepared.task.SourceType, prepared.task.SourceID, err)
	}
	if strings.TrimSpace(resp.Text) == "" {
		return nil, fmt.Errorf("translate %s source_id=%d: empty translation", prepared.task.SourceType, prepared.task.SourceID)
	}
	return resp, nil
}

func (m *Manager) persistTranslatedTask(
	ctx context.Context,
	prepared preparedTranslationTask,
	translationSourceID int64,
	resp *TranslateResponse,
	targetLang string,
	providerName string,
	modelName *string,
) error {
	resolvedSourceLang := resolvedResponseSourceLang(resp.SourceLang, prepared.task.SourceLang)
	if resolvedSourceLang != prepared.sourceLang {
		var err error
		translationSourceID, err = m.upsertPreparedTranslationSource(ctx, prepared.withSourceLang(resolvedSourceLang))
		if err != nil {
			return err
		}
	}
	latencyMS := normalizedLatencyMS(resp.LatencyMs)
	return m.upsertTranslationResult(ctx, upsertTranslationResultInput{
		TranslationSourceID: translationSourceID,
		TargetLang:          resolvedResponseTargetLang(resp.TargetLang, targetLang),
		TranslatedText:      strings.TrimSpace(resp.Text),
		ProviderName:        resolvedResponseProvider(resp.ProviderName, providerName),
		ModelName:           modelName,
		LatencyMS:           &latencyMS,
	})
}

func (prepared preparedTranslationTask) withSourceLang(sourceLang string) preparedTranslationTask {
	prepared.sourceLang = sourceLang
	return prepared
}

func resolvedResponseSourceLang(responseLang, fallbackLang string) string {
	if sourceLang := normalizeLangCode(responseLang); sourceLang != "" {
		return sourceLang
	}
	return normalizeTaskSourceLang(fallbackLang)
}

func resolvedResponseTargetLang(responseLang, fallbackLang string) string {
	if targetLang := normalizeLangCode(responseLang); targetLang != "" {
		return targetLang
	}
	return fallbackLang
}

func resolvedResponseProvider(responseProvider, fallbackProvider string) string {
	if provider := strings.TrimSpace(responseProvider); provider != "" {
		return provider
	}
	return fallbackProvider
}

func normalizedLatencyMS(latency int64) int {
	if latency < 0 {
		return 0
	}
	return int(latency)
}

func (m *Manager) requireCollectionTranslationEnabled(ctx context.Context, collection string) error {
	enabled, err := m.collectionTranslationEnabled(ctx, collection, nil)
	if err != nil {
		return err
	}
	if !enabled {
		return fmt.Errorf("%w for collection %q", ErrTranslationDisabled, strings.TrimSpace(collection))
	}
	return nil
}

func (m *Manager) collectionTranslationEnabled(
	ctx context.Context,
	collection string,
	cache map[string]bool,
) (bool, error) {
	key := strings.ToLower(strings.TrimSpace(collection))
	if cache != nil {
		if enabled, ok := cache[key]; ok {
			return enabled, nil
		}
	}

	mode, err := m.store.GetCollectionTranslationMode(ctx, collection)
	if err != nil {
		return false, err
	}
	enabled := db.IsCollectionTranslationEnabled(mode)
	if cache != nil {
		cache[key] = enabled
	}
	return enabled, nil
}

func (m *Manager) resolveProvider(requested string) (Provider, error) {
	if m == nil || m.registry == nil {
		return nil, fmt.Errorf("translation provider registry is not initialized")
	}
	return m.registry.Provider(requested)
}

func (m *Manager) fetchStoryByUUID(ctx context.Context, storyUUID string) (storyTranslationTarget, error) {
	row, err := m.store.GetTranslationStoryByUUID(ctx, storyUUID)
	if err != nil {
		if errors.Is(err, db.ErrNoRows) {
			return storyTranslationTarget{}, ErrStoryNotFound
		}
		return storyTranslationTarget{}, err
	}
	return storyTranslationTarget{
		StoryID:    row.StoryID,
		StoryUUID:  row.StoryUUID,
		Collection: row.Collection,
		Title:      row.Title,
		SourceLang: row.SourceLang,
	}, nil
}

func (m *Manager) listStoriesByCollection(ctx context.Context, collection string) ([]storyTranslationTarget, error) {
	rows, err := m.store.ListTranslationStoriesByCollection(ctx, collection)
	if err != nil {
		return nil, err
	}

	items := make([]storyTranslationTarget, 0, len(rows))
	for _, row := range rows {
		items = append(items, storyTranslationTarget{
			StoryID:    row.StoryID,
			StoryUUID:  row.StoryUUID,
			Collection: row.Collection,
			Title:      row.Title,
			SourceLang: row.SourceLang,
		})
	}

	return items, nil
}

func (m *Manager) fetchStoryArticles(ctx context.Context, storyID int64) ([]articleTranslationTarget, error) {
	rows, err := m.store.ListTranslationStoryArticles(ctx, storyID)
	if err != nil {
		return nil, err
	}

	items := make([]articleTranslationTarget, 0, len(rows))
	collectionEnabled := map[string]bool{}
	for _, row := range rows {
		enabled, err := m.collectionTranslationEnabled(ctx, row.Collection, collectionEnabled)
		if err != nil {
			return nil, err
		}
		if !enabled {
			continue
		}
		items = append(items, articleTranslationTarget{
			ArticleID:    row.ArticleID,
			ArticleUUID:  row.ArticleUUID,
			Collection:   row.Collection,
			Title:        row.Title,
			Text:         row.Text,
			SourceLang:   row.SourceLang,
			CanonicalURL: row.CanonicalURL,
		})
	}

	for i := range items {
		if err := m.hydrateArticleTextForTranslation(ctx, &items[i]); err != nil {
			return nil, err
		}
	}

	return items, nil
}

func (m *Manager) fetchArticleByUUID(ctx context.Context, articleUUID string) (articleTranslationTarget, error) {
	row, err := m.store.GetTranslationArticleByUUID(ctx, articleUUID)
	if err != nil {
		if errors.Is(err, db.ErrNoRows) {
			return articleTranslationTarget{}, ErrArticleNotFound
		}
		return articleTranslationTarget{}, err
	}

	item := articleTranslationTarget{
		ArticleID:    row.ArticleID,
		ArticleUUID:  row.ArticleUUID,
		Collection:   row.Collection,
		Title:        row.Title,
		Text:         row.Text,
		SourceLang:   row.SourceLang,
		CanonicalURL: row.CanonicalURL,
	}

	if err := m.hydrateArticleTextForTranslation(ctx, &item); err != nil {
		return articleTranslationTarget{}, err
	}

	return item, nil
}

func (m *Manager) hydrateArticleTextForTranslation(
	ctx context.Context,
	article *articleTranslationTarget,
) error {
	if article == nil {
		return nil
	}

	normalizeArticleTranslationText(article)
	if articleHasSubstantiveText(article) {
		return nil
	}

	canonicalURL := articleCanonicalURL(article)
	if canonicalURL == "" {
		return nil
	}

	readerText, err := reader.FetchText(ctx, canonicalURL, article.Title)
	if err != nil {
		// Reader fetch is best-effort; keep the prior behavior of skipping empty bodies.
		return nil
	}

	clipped, _ := reader.TruncateText(readerText, articleReaderTranslationMaxChars)
	article.Text = strings.TrimSpace(clipped)
	if article.Text != "" {
		article.TextOrigin = ContentOriginReader
	}
	return nil
}

func normalizeArticleTranslationText(article *articleTranslationTarget) {
	article.Title = strings.TrimSpace(article.Title)
	article.Text = strings.TrimSpace(article.Text)
	article.TextOrigin = ContentOriginNormalized
}

func articleHasSubstantiveText(article *articleTranslationTarget) bool {
	// During ingestion, the body can be just the title. Treat only clearly longer
	// text as enough for translation without reader hydration.
	return article.Text != "" && len(article.Text) > len(article.Title)+50
}

func articleCanonicalURL(article *articleTranslationTarget) string {
	if article.CanonicalURL == nil {
		return ""
	}
	return strings.TrimSpace(*article.CanonicalURL)
}

func (m *Manager) lookupCachedTranslation(
	ctx context.Context,
	translationSourceID int64,
	targetLang string,
) (*CachedTranslation, error) {
	row, err := m.store.LookupCachedTranslationRow(ctx, translationSourceID, targetLang)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	return &CachedTranslation{
		TranslationUUID: row.TranslationUUID,
		SourceType:      row.SourceType,
		SourceID:        row.SourceID,
		SourceLang:      row.SourceLang,
		TargetLang:      row.TargetLang,
		OriginalText:    row.OriginalText,
		TranslatedText:  row.TranslatedText,
		ProviderName:    row.ProviderName,
		ModelName:       row.ModelName,
		LatencyMS:       row.LatencyMS,
		CreatedAt:       row.CreatedAt,
	}, nil
}

type upsertTranslationSourceInput struct {
	SourceType    string
	SourceID      int64
	SourceLang    string
	ContentHash   []byte
	OriginalText  string
	ContentOrigin string
}

func (m *Manager) upsertTranslationSource(ctx context.Context, row upsertTranslationSourceInput) (int64, error) {
	return m.store.UpsertTranslationSource(ctx, db.UpsertTranslationSourceParams{
		SourceType:    row.SourceType,
		SourceID:      row.SourceID,
		SourceLang:    row.SourceLang,
		ContentHash:   row.ContentHash,
		OriginalText:  row.OriginalText,
		ContentOrigin: row.ContentOrigin,
	})
}

type upsertTranslationResultInput struct {
	TranslationSourceID int64
	TargetLang          string
	TranslatedText      string
	ProviderName        string
	ModelName           *string
	LatencyMS           *int
}

func (m *Manager) upsertTranslationResult(ctx context.Context, row upsertTranslationResultInput) error {
	return m.store.UpsertTranslationResult(ctx, db.UpsertTranslationResultParams{
		TranslationSourceID: row.TranslationSourceID,
		TargetLang:          row.TargetLang,
		TranslatedText:      row.TranslatedText,
		ProviderName:        row.ProviderName,
		ModelName:           row.ModelName,
		LatencyMS:           row.LatencyMS,
	})
}

type translationTask struct {
	SourceType    string
	SourceID      int64
	SourceLang    string
	OriginalText  string
	ContentOrigin string
}

type storyTranslationTarget struct {
	StoryID    int64
	StoryUUID  string
	Collection string
	Title      string
	SourceLang string
}

type articleTranslationTarget struct {
	ArticleID    int64
	ArticleUUID  string
	Collection   string
	Title        string
	Text         string
	TextOrigin   string
	SourceLang   string
	CanonicalURL *string
}

type modelNameProvider interface {
	ModelName() string
}

func modelNameFromProvider(provider Provider) *string {
	namedProvider, ok := provider.(modelNameProvider)
	if !ok {
		return nil
	}
	model := strings.TrimSpace(namedProvider.ModelName())
	if model == "" {
		return nil
	}
	return &model
}

func hashTranslationSourceText(text string) []byte {
	sum := sha256.Sum256([]byte(text))
	return sum[:]
}

func normalizeContentOrigin(origin string) string {
	normalized := strings.ToLower(strings.TrimSpace(origin))
	if normalized == ContentOriginReader {
		return ContentOriginReader
	}
	return ContentOriginNormalized
}

func shouldSkipTranslationTask(sourceLang, targetLang string) bool {
	return strings.TrimSpace(sourceLang) != "" && sourceLang == targetLang
}
