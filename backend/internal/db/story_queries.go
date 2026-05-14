package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// StorySummary is a read model used by list/search/digest commands.
type StorySummary struct {
	StoryID                 int64      `json:"story_id"`
	StoryUUID               string     `json:"story_uuid"`
	CanonicalTitle          string     `json:"canonical_title"`
	CanonicalURL            *string    `json:"canonical_url,omitempty"`
	Collection              string     `json:"collection"`
	RepresentativeArticleID *int64     `json:"representative_article_id,omitempty"`
	FirstSeenAt             time.Time  `json:"first_seen_at"`
	LastSeenAt              time.Time  `json:"last_seen_at"`
	SourceCount             int        `json:"source_count"`
	ArticleCount            int        `json:"article_count"`
	Status                  string     `json:"status"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
	SourceDomain            *string    `json:"source_domain,omitempty"`
	EventCreatedAt          *time.Time `json:"event_created_at,omitempty"`
}

// StoryEventListOptions controls list queries scoped by dedup event timestamps.
type StoryEventListOptions struct {
	Collection string
	From       time.Time
	To         time.Time
	Limit      int
}

// StoryDetail contains one story and all merged member articles.
type StoryDetail struct {
	Story    StoryDetailHeader    `json:"story"`
	Articles []StoryDetailArticle `json:"articles"`
}

// StoryDetailHeader is the story section for story detail output.
type StoryDetailHeader struct {
	StoryID        int64     `json:"story_id"`
	StoryUUID      string    `json:"story_uuid"`
	CanonicalTitle string    `json:"canonical_title"`
	CanonicalURL   *string   `json:"canonical_url,omitempty"`
	Collection     string    `json:"collection"`
	SourceCount    int       `json:"source_count"`
	ArticleCount   int       `json:"article_count"`
	CreatedAt      time.Time `json:"created_at"`
	FirstSeenAt    time.Time `json:"first_seen_at"`
	LastSeenAt     time.Time `json:"last_seen_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// StoryDetailArticle is an article row within a story.
type StoryDetailArticle struct {
	ArticleID    int64      `json:"article_id"`
	ArticleUUID  string     `json:"article_uuid"`
	Title        string     `json:"title"`
	URL          *string    `json:"url,omitempty"`
	Source       string     `json:"source"`
	SourceDomain *string    `json:"source_domain,omitempty"`
	PublishedAt  *time.Time `json:"published_at,omitempty"`
	MatchedAt    time.Time  `json:"matched_at"`
}

// ListStoriesByDedupEventWindow lists stories that had dedup events in the provided UTC time window.
func (p *Pool) ListStoriesByDedupEventWindow(ctx context.Context, opts StoryEventListOptions) ([]StorySummary, error) {
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
	s.story_id,
	s.story_uuid::text,
	s.canonical_title,
	s.canonical_url,
	s.collection,
	s.representative_article_id,
	s.first_seen_at,
	s.last_seen_at,
	(SELECT COUNT(DISTINCT a.source)
	 FROM news.story_articles sa
	 JOIN news.articles a
		ON a.article_id = sa.article_id
		AND a.deleted_at IS NULL
	 WHERE sa.story_id = s.story_id) AS source_count,
	(SELECT COUNT(*)
	 FROM news.story_articles sa
	 JOIN news.articles a
		ON a.article_id = sa.article_id
		AND a.deleted_at IS NULL
	 WHERE sa.story_id = s.story_id) AS article_count,
	s.status,
	s.created_at,
	s.updated_at,
	rep.source_domain,
	MAX(de.created_at) AS event_created_at
FROM news.stories s
JOIN news.dedup_events de
	ON de.chosen_story_id = s.story_id
JOIN news.articles a
	ON a.article_id = de.article_id
	AND a.deleted_at IS NULL
LEFT JOIN news.articles rep
	ON rep.article_id = s.representative_article_id
	AND rep.deleted_at IS NULL
WHERE s.deleted_at IS NULL
  AND de.created_at >= $1
  AND de.created_at < $2
  AND ($3 = '' OR a.collection = $3)
GROUP BY
	s.story_id,
	s.story_uuid,
	s.canonical_title,
	s.canonical_url,
	s.collection,
	s.representative_article_id,
	s.first_seen_at,
	s.last_seen_at,
	s.status,
	s.created_at,
	s.updated_at,
	rep.source_domain
ORDER BY source_count DESC, article_count DESC, MAX(de.created_at) DESC, s.story_id DESC
LIMIT $4
`

	rows, err := p.Query(ctx, q, from, to, normalizeCollection(opts.Collection), opts.Limit)
	if err != nil {
		return nil, fmt.Errorf("query stories by dedup window: %w", err)
	}
	defer rows.Close()

	items, err := scanStorySummaries(rows, opts.Limit)
	if err != nil {
		return nil, err
	}
	return items, nil
}

// SearchStoriesByTitle performs an ILIKE title search over canonical story titles.
func (p *Pool) SearchStoriesByTitle(ctx context.Context, query, collection string, limit int) ([]StorySummary, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be > 0")
	}

	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		return nil, fmt.Errorf("query is required")
	}
	search := "%" + trimmedQuery + "%"

	const q = `
SELECT
	s.story_id,
	s.story_uuid::text,
	s.canonical_title,
	s.canonical_url,
	s.collection,
	s.representative_article_id,
	s.first_seen_at,
	s.last_seen_at,
	(SELECT COUNT(DISTINCT a.source)
	 FROM news.story_articles sa
	 JOIN news.articles a
		ON a.article_id = sa.article_id
		AND a.deleted_at IS NULL
	 WHERE sa.story_id = s.story_id) AS source_count,
	(SELECT COUNT(*)
	 FROM news.story_articles sa
	 JOIN news.articles a
		ON a.article_id = sa.article_id
		AND a.deleted_at IS NULL
	 WHERE sa.story_id = s.story_id) AS article_count,
	s.status,
	s.created_at,
	s.updated_at,
	rep.source_domain,
	NULL::timestamptz AS event_created_at
FROM news.stories s
LEFT JOIN news.articles rep
	ON rep.article_id = s.representative_article_id
	AND rep.deleted_at IS NULL
WHERE s.deleted_at IS NULL
  AND ($1 = '' OR s.collection = $1)
  AND s.canonical_title ILIKE $2
ORDER BY source_count DESC, article_count DESC, s.created_at DESC, s.story_id DESC
LIMIT $3
`

	rows, err := p.Query(ctx, q, normalizeCollection(collection), search, limit)
	if err != nil {
		return nil, fmt.Errorf("search stories: %w", err)
	}
	defer rows.Close()

	items, err := scanStorySummaries(rows, limit)
	if err != nil {
		return nil, err
	}
	return items, nil
}

// GetStoryDetail returns one story by UUID and all merged member articles.
func (p *Pool) GetStoryDetail(ctx context.Context, storyUUID string) (*StoryDetail, error) {
	trimmedUUID := strings.TrimSpace(storyUUID)
	if trimmedUUID == "" {
		return nil, fmt.Errorf("story UUID is required")
	}
	header, err := p.getStoryDetailHeader(ctx, trimmedUUID)
	if err != nil {
		return nil, err
	}
	members, err := p.listStoryDetailArticles(ctx, header)
	if err != nil {
		return nil, err
	}
	return &StoryDetail{
		Story:    header,
		Articles: members,
	}, nil
}

func (p *Pool) getStoryDetailHeader(ctx context.Context, storyUUID string) (StoryDetailHeader, error) {
	const storyQuery = `
SELECT
	s.story_id,
	s.story_uuid::text,
	s.canonical_title,
	s.canonical_url,
	s.collection,
	(SELECT COUNT(DISTINCT a.source)
	 FROM news.story_articles sa
	 JOIN news.articles a
		ON a.article_id = sa.article_id
		AND a.deleted_at IS NULL
	 WHERE sa.story_id = s.story_id) AS source_count,
	(SELECT COUNT(*)
	 FROM news.story_articles sa
	 JOIN news.articles a
		ON a.article_id = sa.article_id
		AND a.deleted_at IS NULL
	 WHERE sa.story_id = s.story_id) AS article_count,
	s.created_at,
	s.first_seen_at,
	s.last_seen_at,
	s.updated_at
FROM news.stories s
WHERE s.story_uuid = $1::uuid
  AND s.deleted_at IS NULL
`

	var header StoryDetailHeader
	if err := p.QueryRow(ctx, storyQuery, storyUUID).Scan(
		&header.StoryID,
		&header.StoryUUID,
		&header.CanonicalTitle,
		&header.CanonicalURL,
		&header.Collection,
		&header.SourceCount,
		&header.ArticleCount,
		&header.CreatedAt,
		&header.FirstSeenAt,
		&header.LastSeenAt,
		&header.UpdatedAt,
	); err != nil {
		if errors.Is(err, ErrNoRows) {
			return StoryDetailHeader{}, ErrNoRows
		}
		return StoryDetailHeader{}, fmt.Errorf("query story detail header: %w", err)
	}
	return header, nil
}

func (p *Pool) listStoryDetailArticles(ctx context.Context, header StoryDetailHeader) ([]StoryDetailArticle, error) {
	const membersQuery = `
SELECT
	a.article_id,
	a.article_uuid::text,
	a.normalized_title,
	a.canonical_url,
	a.source,
	a.source_domain,
	a.published_at,
	sa.matched_at
FROM news.story_articles sa
JOIN news.articles a
	ON a.article_id = sa.article_id
	AND a.deleted_at IS NULL
WHERE sa.story_id = $1
ORDER BY sa.matched_at DESC, a.article_id DESC
`

	rows, err := p.Query(ctx, membersQuery, header.StoryID)
	if err != nil {
		return nil, fmt.Errorf("query story detail members: %w", err)
	}
	defer rows.Close()

	members := make([]StoryDetailArticle, 0, header.ArticleCount)
	for rows.Next() {
		var member StoryDetailArticle
		if err := rows.Scan(
			&member.ArticleID,
			&member.ArticleUUID,
			&member.Title,
			&member.URL,
			&member.Source,
			&member.SourceDomain,
			&member.PublishedAt,
			&member.MatchedAt,
		); err != nil {
			return nil, fmt.Errorf("scan story detail member: %w", err)
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate story detail members: %w", err)
	}
	return members, nil
}

// ListDigestStories lists stories that were marked as new_story in the provided UTC day window.
func (p *Pool) ListDigestStories(ctx context.Context, collection string, from, to time.Time) ([]StorySummary, error) {
	fromUTC := from.UTC()
	toUTC := to.UTC()
	if !fromUTC.Before(toUTC) {
		return nil, fmt.Errorf("from must be before to")
	}

	const q = `
SELECT
	s.story_id,
	s.story_uuid::text,
	s.canonical_title,
	s.canonical_url,
	s.collection,
	s.representative_article_id,
	s.first_seen_at,
	s.last_seen_at,
	(SELECT COUNT(DISTINCT a.source)
	 FROM news.story_articles sa
	 JOIN news.articles a
		ON a.article_id = sa.article_id
		AND a.deleted_at IS NULL
	 WHERE sa.story_id = s.story_id) AS source_count,
	(SELECT COUNT(*)
	 FROM news.story_articles sa
	 JOIN news.articles a
		ON a.article_id = sa.article_id
		AND a.deleted_at IS NULL
	 WHERE sa.story_id = s.story_id) AS article_count,
	s.status,
	s.created_at,
	s.updated_at,
	rep.source_domain,
	MAX(de.created_at) AS event_created_at
FROM news.dedup_events de
JOIN news.stories s
	ON s.story_id = de.chosen_story_id
JOIN news.articles a
	ON a.article_id = de.article_id
	AND a.deleted_at IS NULL
LEFT JOIN news.articles rep
	ON rep.article_id = s.representative_article_id
	AND rep.deleted_at IS NULL
WHERE s.deleted_at IS NULL
  AND de.decision = 'new_story'
  AND de.created_at >= $1
  AND de.created_at < $2
  AND ($3 = '' OR a.collection = $3)
GROUP BY
	s.story_id,
	s.story_uuid,
	s.canonical_title,
	s.canonical_url,
	s.collection,
	s.representative_article_id,
	s.first_seen_at,
	s.last_seen_at,
	s.status,
	s.created_at,
	s.updated_at,
	rep.source_domain
ORDER BY source_count DESC, article_count DESC, MAX(de.created_at) DESC, s.story_id DESC
`

	rows, err := p.Query(ctx, q, fromUTC, toUTC, normalizeCollection(collection))
	if err != nil {
		return nil, fmt.Errorf("query digest stories: %w", err)
	}
	defer rows.Close()

	items, err := scanStorySummaries(rows, 64)
	if err != nil {
		return nil, err
	}
	return items, nil
}

func scanStorySummaries(rows *Rows, capacity int) ([]StorySummary, error) {
	if capacity < 0 {
		capacity = 0
	}

	items := make([]StorySummary, 0, capacity)
	for rows.Next() {
		var row StorySummary
		if err := rows.Scan(
			&row.StoryID,
			&row.StoryUUID,
			&row.CanonicalTitle,
			&row.CanonicalURL,
			&row.Collection,
			&row.RepresentativeArticleID,
			&row.FirstSeenAt,
			&row.LastSeenAt,
			&row.SourceCount,
			&row.ArticleCount,
			&row.Status,
			&row.CreatedAt,
			&row.UpdatedAt,
			&row.SourceDomain,
			&row.EventCreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan story summary row: %w", err)
		}
		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate story summary rows: %w", err)
	}
	return items, nil
}

func normalizeCollection(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}
