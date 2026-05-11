package db

import (
	"context"
	"fmt"
	"time"
)

// ArticleListOptions controls article listing queries.
type ArticleListOptions struct {
	Collection string
	From       time.Time
	To         time.Time
	Limit      int
}

// ArticleListItem is used by the articles CLI command.
type ArticleListItem struct {
	ArticleID    int64      `json:"article_id"`
	ArticleUUID  string     `json:"article_uuid"`
	Title        string     `json:"title"`
	URL          *string    `json:"url,omitempty"`
	Source       string     `json:"source"`
	SourceDomain *string    `json:"source_domain,omitempty"`
	PublishedAt  *time.Time `json:"published_at,omitempty"`
	Collection   string     `json:"collection"`
	CreatedAt    time.Time  `json:"created_at"`
}

// CollectionCount is used by the collections CLI command.
type CollectionCount struct {
	Collection        string     `json:"collection"`
	TranslationMode   string     `json:"translation_mode"`
	ArticleCount      int64      `json:"article_count"`
	StoryCount        int64      `json:"story_count"`
	EarliestArticleAt *time.Time `json:"earliest_article_at,omitempty"`
	LatestArticleAt   *time.Time `json:"latest_article_at,omitempty"`
}

// ListArticles lists normalized articles in a UTC created_at window.
func (p *Pool) ListArticles(ctx context.Context, opts ArticleListOptions) ([]ArticleListItem, error) {
	if opts.Limit <= 0 {
		return nil, fmt.Errorf("limit must be > 0")
	}

	from := opts.From.UTC()
	to := opts.To.UTC()
	if !from.Before(to) {
		return nil, fmt.Errorf("from must be before to")
	}

	const q = `
SELECT
	a.article_id,
	a.article_uuid::text,
	a.normalized_title,
	a.canonical_url,
	a.source,
	a.source_domain,
	a.published_at,
	a.collection,
	a.created_at
FROM news.articles a
WHERE a.created_at >= $1
  AND a.created_at < $2
  AND ($3 = '' OR a.collection = $3)
  AND a.deleted_at IS NULL
ORDER BY a.created_at DESC, a.article_id DESC
LIMIT $4
`

	rows, err := p.Query(ctx, q, from, to, normalizeCollection(opts.Collection), opts.Limit)
	if err != nil {
		return nil, fmt.Errorf("query articles: %w", err)
	}
	defer rows.Close()

	items := make([]ArticleListItem, 0, opts.Limit)
	for rows.Next() {
		var row ArticleListItem
		if err := rows.Scan(
			&row.ArticleID,
			&row.ArticleUUID,
			&row.Title,
			&row.URL,
			&row.Source,
			&row.SourceDomain,
			&row.PublishedAt,
			&row.Collection,
			&row.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan article row: %w", err)
		}
		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate article rows: %w", err)
	}

	return items, nil
}

// ListCollectionsWithCounts returns collection-level counts and article date ranges.
func (p *Pool) ListCollectionsWithCounts(ctx context.Context) ([]CollectionCount, error) {
	const q = `
WITH article_stats AS (
	SELECT
		a.collection,
		COUNT(*)::BIGINT AS article_count,
		MIN(COALESCE(a.published_at, a.created_at)) AS earliest_article_at,
		MAX(COALESCE(a.published_at, a.created_at)) AS latest_article_at
	FROM news.articles a
	WHERE a.deleted_at IS NULL
	GROUP BY a.collection
),
story_counts AS (
	SELECT
		s.collection,
		COUNT(*)::BIGINT AS story_count
	FROM news.stories s
	WHERE s.deleted_at IS NULL
	GROUP BY s.collection
)
SELECT
	COALESCE(a.collection, s.collection) AS collection,
	COALESCE(
		cs.translation_mode,
		CASE
			WHEN COALESCE(a.collection, s.collection) IN ('china_news', 'metal_news') THEN 'enabled'
			ELSE 'disabled'
		END
	) AS translation_mode,
	COALESCE(a.article_count, 0) AS article_count,
	COALESCE(s.story_count, 0) AS story_count,
	a.earliest_article_at,
	a.latest_article_at
FROM article_stats a
FULL OUTER JOIN story_counts s
	ON s.collection = a.collection
LEFT JOIN news.collection_settings cs
	ON cs.collection = COALESCE(a.collection, s.collection)
ORDER BY 1
`

	rows, err := p.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query collections with counts: %w", err)
	}
	defer rows.Close()

	items := make([]CollectionCount, 0, 16)
	for rows.Next() {
		var row CollectionCount
		if err := rows.Scan(
			&row.Collection,
			&row.TranslationMode,
			&row.ArticleCount,
			&row.StoryCount,
			&row.EarliestArticleAt,
			&row.LatestArticleAt,
		); err != nil {
			return nil, fmt.Errorf("scan collection row: %w", err)
		}
		row.TranslationMode = NormalizeCollectionTranslationMode(row.TranslationMode)
		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate collection rows: %w", err)
	}

	return items, nil
}
