package db

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// TranslationStoryTarget is a translation-ready story row.
type TranslationStoryTarget struct {
	StoryID    int64
	StoryUUID  string
	Collection string
	Title      string
	SourceLang string
}

// TranslationArticleTarget is a translation-ready article row.
type TranslationArticleTarget struct {
	ArticleID    int64
	ArticleUUID  string
	Collection   string
	Title        string
	Text         string
	SourceLang   string
	CanonicalURL *string
}

// StoryTranslationRow is one cached translation row for story translation listings.
type StoryTranslationRow struct {
	TranslationUUID string
	SourceType      string
	SourceID        int64
	SourceUUID      *string
	SourceLang      string
	TargetLang      string
	OriginalText    string
	TranslatedText  string
	ProviderName    string
	ModelName       *string
	LatencyMS       *int
	CreatedAt       time.Time
}

// CachedTranslationRow is one cached translation row for a source+target pair.
type CachedTranslationRow struct {
	TranslationUUID string
	SourceType      string
	SourceID        int64
	SourceLang      string
	TargetLang      string
	OriginalText    string
	TranslatedText  string
	ProviderName    string
	ModelName       *string
	LatencyMS       *int
	CreatedAt       time.Time
}

// UpsertTranslationSourceParams controls translation source upserts.
type UpsertTranslationSourceParams struct {
	SourceType    string
	SourceID      int64
	SourceLang    string
	ContentHash   []byte
	OriginalText  string
	ContentOrigin string
}

// UpsertTranslationResultParams controls translation result upserts.
type UpsertTranslationResultParams struct {
	TranslationSourceID int64
	TargetLang          string
	TranslatedText      string
	ProviderName        string
	ModelName           *string
	LatencyMS           *int
}

func (p *Pool) GetTranslationStoryByUUID(ctx context.Context, storyUUID string) (TranslationStoryTarget, error) {
	const q = `
SELECT
	s.story_id,
	s.story_uuid::text,
	s.collection,
	s.canonical_title,
	COALESCE(rep.normalized_language, 'und') AS source_lang
FROM news.stories s
LEFT JOIN news.articles rep
	ON rep.article_id = s.representative_article_id
	AND rep.deleted_at IS NULL
WHERE s.story_uuid = $1::uuid
  AND s.deleted_at IS NULL
LIMIT 1
`

	var row TranslationStoryTarget
	err := p.QueryRow(ctx, q, strings.TrimSpace(storyUUID)).Scan(
		&row.StoryID,
		&row.StoryUUID,
		&row.Collection,
		&row.Title,
		&row.SourceLang,
	)
	if err != nil {
		if IsNoRows(err) {
			return TranslationStoryTarget{}, ErrNoRows
		}
		return TranslationStoryTarget{}, fmt.Errorf("query translation story: %w", err)
	}
	return row, nil
}

func (p *Pool) ListTranslationStoriesByCollection(ctx context.Context, collection string) ([]TranslationStoryTarget, error) {
	const q = `
SELECT
	s.story_id,
	s.story_uuid::text,
	s.collection,
	s.canonical_title,
	COALESCE(rep.normalized_language, 'und') AS source_lang
FROM news.stories s
LEFT JOIN news.articles rep
	ON rep.article_id = s.representative_article_id
	AND rep.deleted_at IS NULL
WHERE s.deleted_at IS NULL
  AND ($1 = '' OR s.collection = $1)
ORDER BY s.last_seen_at DESC, s.story_id DESC
`

	rows, err := p.Query(ctx, q, normalizeCollection(collection))
	if err != nil {
		return nil, fmt.Errorf("query translation stories by collection: %w", err)
	}
	defer rows.Close()

	items := make([]TranslationStoryTarget, 0, 64)
	for rows.Next() {
		var row TranslationStoryTarget
		if err := rows.Scan(&row.StoryID, &row.StoryUUID, &row.Collection, &row.Title, &row.SourceLang); err != nil {
			return nil, fmt.Errorf("scan translation story row: %w", err)
		}
		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate translation story rows: %w", err)
	}

	return items, nil
}

func (p *Pool) ListTranslationStoryArticles(ctx context.Context, storyID int64) ([]TranslationArticleTarget, error) {
	const q = `
SELECT
	a.article_id,
	a.article_uuid::text,
	a.collection,
	a.normalized_title,
	a.normalized_text,
	a.normalized_language,
	a.canonical_url
FROM news.story_articles sa
JOIN news.articles a
	ON a.article_id = sa.article_id
	AND a.deleted_at IS NULL
WHERE sa.story_id = $1
ORDER BY sa.matched_at DESC, a.article_id DESC
`

	rows, err := p.Query(ctx, q, storyID)
	if err != nil {
		return nil, fmt.Errorf("query translation story articles: %w", err)
	}
	defer rows.Close()

	items := make([]TranslationArticleTarget, 0, 8)
	for rows.Next() {
		var row TranslationArticleTarget
		if err := rows.Scan(
			&row.ArticleID,
			&row.ArticleUUID,
			&row.Collection,
			&row.Title,
			&row.Text,
			&row.SourceLang,
			&row.CanonicalURL,
		); err != nil {
			return nil, fmt.Errorf("scan translation story article row: %w", err)
		}
		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate translation story article rows: %w", err)
	}

	return items, nil
}

func (p *Pool) GetTranslationArticleByUUID(ctx context.Context, articleUUID string) (TranslationArticleTarget, error) {
	const q = `
SELECT
	a.article_id,
	a.article_uuid::text,
	a.collection,
	a.normalized_title,
	a.normalized_text,
	a.normalized_language,
	a.canonical_url
FROM news.articles a
WHERE a.article_uuid = $1::uuid
  AND a.deleted_at IS NULL
LIMIT 1
`

	var row TranslationArticleTarget
	err := p.QueryRow(ctx, q, strings.TrimSpace(articleUUID)).Scan(
		&row.ArticleID,
		&row.ArticleUUID,
		&row.Collection,
		&row.Title,
		&row.Text,
		&row.SourceLang,
		&row.CanonicalURL,
	)
	if err != nil {
		if IsNoRows(err) {
			return TranslationArticleTarget{}, ErrNoRows
		}
		return TranslationArticleTarget{}, fmt.Errorf("query translation article: %w", err)
	}
	return row, nil
}

func (p *Pool) ListStoryTranslationRows(ctx context.Context, storyID int64) ([]StoryTranslationRow, error) {
	const q = `
SELECT
	r.translation_result_uuid::text,
	ts.source_type,
	ts.source_id,
	CASE
		WHEN ts.source_type = 'story_title' THEN s.story_uuid::text
		WHEN ts.source_type IN ('article_title', 'article_text') THEN a.article_uuid::text
		ELSE NULL
	END AS source_uuid,
	ts.source_lang,
	r.target_lang,
	ts.original_text,
	r.translated_text,
	r.provider_name,
	r.model_name,
	r.latency_ms,
	r.created_at
FROM news.translation_sources ts
JOIN news.translation_results r
	ON r.translation_source_id = ts.translation_source_id
LEFT JOIN news.stories s
	ON ts.source_type = 'story_title'
	AND s.story_id = ts.source_id
LEFT JOIN news.articles a
	ON ts.source_type IN ('article_title', 'article_text')
	AND a.article_id = ts.source_id
WHERE (ts.source_type = 'story_title' AND ts.source_id = $1)
   OR (
		ts.source_type IN ('article_title', 'article_text')
		AND ts.source_id IN (
			SELECT sa.article_id
			FROM news.story_articles sa
			WHERE sa.story_id = $1
		)
	)
ORDER BY r.target_lang, ts.source_type, ts.source_id, ts.captured_at DESC, r.created_at DESC
`

	rows, err := p.Query(ctx, q, storyID)
	if err != nil {
		return nil, fmt.Errorf("query story translation rows: %w", err)
	}
	defer rows.Close()

	items := make([]StoryTranslationRow, 0, 32)
	for rows.Next() {
		var row StoryTranslationRow
		if err := rows.Scan(
			&row.TranslationUUID,
			&row.SourceType,
			&row.SourceID,
			&row.SourceUUID,
			&row.SourceLang,
			&row.TargetLang,
			&row.OriginalText,
			&row.TranslatedText,
			&row.ProviderName,
			&row.ModelName,
			&row.LatencyMS,
			&row.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan story translation row: %w", err)
		}
		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate story translation rows: %w", err)
	}

	return items, nil
}

func (p *Pool) LookupCachedTranslationRow(
	ctx context.Context,
	translationSourceID int64,
	targetLang string,
) (*CachedTranslationRow, error) {
	const q = `
SELECT
	r.translation_result_uuid::text,
	ts.source_type,
	ts.source_id,
	ts.source_lang,
	r.target_lang,
	ts.original_text,
	r.translated_text,
	r.provider_name,
	r.model_name,
	r.latency_ms,
	r.created_at
FROM news.translation_results r
JOIN news.translation_sources ts
	ON ts.translation_source_id = r.translation_source_id
WHERE r.translation_source_id = $1
	  AND r.target_lang = $2
LIMIT 1
	`

	var row CachedTranslationRow
	err := p.QueryRow(ctx, q, translationSourceID, targetLang).Scan(
		&row.TranslationUUID,
		&row.SourceType,
		&row.SourceID,
		&row.SourceLang,
		&row.TargetLang,
		&row.OriginalText,
		&row.TranslatedText,
		&row.ProviderName,
		&row.ModelName,
		&row.LatencyMS,
		&row.CreatedAt,
	)
	if err != nil {
		if IsNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("query cached translation row: %w", err)
	}
	return &row, nil
}

func (p *Pool) UpsertTranslationSource(ctx context.Context, row UpsertTranslationSourceParams) (int64, error) {
	const q = `
INSERT INTO news.translation_sources (
	source_type,
	source_id,
	source_lang,
	content_hash,
	original_text,
	content_origin,
	captured_at
)
VALUES ($1, $2, $3, $4, $5, $6, now())
ON CONFLICT (source_type, source_id, content_hash)
DO UPDATE SET
	source_lang = EXCLUDED.source_lang,
	original_text = EXCLUDED.original_text,
	content_origin = EXCLUDED.content_origin,
	captured_at = now()
RETURNING translation_source_id
	`

	var translationSourceID int64
	if err := p.QueryRow(
		ctx,
		q,
		row.SourceType,
		row.SourceID,
		row.SourceLang,
		row.ContentHash,
		row.OriginalText,
		row.ContentOrigin,
	).Scan(&translationSourceID); err != nil {
		return 0, fmt.Errorf("upsert translation source: %w", err)
	}
	return translationSourceID, nil
}

func (p *Pool) UpsertTranslationResult(ctx context.Context, row UpsertTranslationResultParams) error {
	const q = `
INSERT INTO news.translation_results (
	translation_source_id,
	target_lang,
	translated_text,
	provider_name,
	model_name,
	latency_ms
)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (translation_source_id, target_lang)
DO UPDATE SET
	translated_text = EXCLUDED.translated_text,
	provider_name = EXCLUDED.provider_name,
	model_name = EXCLUDED.model_name,
	latency_ms = EXCLUDED.latency_ms,
	created_at = now()
	`

	if _, err := p.Exec(
		ctx,
		q,
		row.TranslationSourceID,
		row.TargetLang,
		row.TranslatedText,
		row.ProviderName,
		row.ModelName,
		row.LatencyMS,
	); err != nil {
		return fmt.Errorf("upsert translation result: %w", err)
	}
	return nil
}
