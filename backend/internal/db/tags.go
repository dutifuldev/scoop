package db

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var tagSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

type TagRecord struct {
	TagID       int64      `json:"tag_id"`
	TagUUID     string     `json:"tag_uuid"`
	Slug        string     `json:"slug"`
	Name        string     `json:"name"`
	Description *string    `json:"description,omitempty"`
	Color       *string    `json:"color,omitempty"`
	ArchivedAt  *time.Time `json:"archived_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type UpsertTagOptions struct {
	Slug        string
	Name        string
	Description *string
	Color       *string
}

type UpdateTagOptions struct {
	NewSlug     *string
	Name        *string
	Description *string
	Color       *string
}

func NormalizeTagSlug(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func ValidateTagSlug(slug string) error {
	normalized := NormalizeTagSlug(slug)
	if normalized == "" {
		return fmt.Errorf("tag slug is required")
	}
	if !tagSlugPattern.MatchString(normalized) {
		return fmt.Errorf("tag slug must start with a lowercase letter or digit and contain only lowercase letters, digits, _ or -")
	}
	return nil
}

func normalizeOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func normalizeOptionalColor(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, nil
	}
	if !regexp.MustCompile(`^#[0-9a-fA-F]{6}$`).MatchString(trimmed) {
		return nil, fmt.Errorf("color must be a #RRGGBB hex value")
	}
	return &trimmed, nil
}

func (p *Pool) ListTags(ctx context.Context, includeArchived bool) ([]TagRecord, error) {
	const q = `
SELECT tag_id, tag_uuid::text, slug, name, description, color, archived_at, created_at, updated_at
FROM news.tags
WHERE $1 OR archived_at IS NULL
ORDER BY archived_at NULLS FIRST, slug
`
	rows, err := p.Query(ctx, q, includeArchived)
	if err != nil {
		return nil, fmt.Errorf("query tags: %w", err)
	}
	defer rows.Close()

	tags := make([]TagRecord, 0, 32)
	for rows.Next() {
		var tag TagRecord
		if err := rows.Scan(
			&tag.TagID,
			&tag.TagUUID,
			&tag.Slug,
			&tag.Name,
			&tag.Description,
			&tag.Color,
			&tag.ArchivedAt,
			&tag.CreatedAt,
			&tag.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tags: %w", err)
	}
	return tags, nil
}

func (p *Pool) CreateTag(ctx context.Context, opts UpsertTagOptions, now time.Time) (*TagRecord, error) {
	slug := NormalizeTagSlug(opts.Slug)
	if err := ValidateTagSlug(slug); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		return nil, fmt.Errorf("tag name is required")
	}
	description := normalizeOptionalString(opts.Description)
	color, err := normalizeOptionalColor(opts.Color)
	if err != nil {
		return nil, err
	}

	tx, err := p.BeginTx(ctx, TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
INSERT INTO news.tags (slug, name, description, color, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $5)
RETURNING tag_id, tag_uuid::text, slug, name, description, color, archived_at, created_at, updated_at
`
	tag, err := scanTag(tx.QueryRow(ctx, q, slug, name, description, color, now.UTC()))
	if err != nil {
		return nil, fmt.Errorf("create tag: %w", err)
	}
	if err := insertAuditEvent(ctx, tx, nil, "tag.create", "tag", tag.Slug, map[string]any{
		"slug": tag.Slug,
		"name": tag.Name,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create tag: %w", err)
	}
	return tag, nil
}

func (p *Pool) UpdateTag(ctx context.Context, slug string, opts UpdateTagOptions, now time.Time) (*TagRecord, error) {
	normalizedSlug := NormalizeTagSlug(slug)
	if err := ValidateTagSlug(normalizedSlug); err != nil {
		return nil, err
	}

	set := make([]string, 0, 5)
	args := []any{normalizedSlug}
	argPos := 2
	details := map[string]any{"old_slug": normalizedSlug}

	if opts.NewSlug != nil {
		newSlug := NormalizeTagSlug(*opts.NewSlug)
		if err := ValidateTagSlug(newSlug); err != nil {
			return nil, err
		}
		set = append(set, fmt.Sprintf("slug = $%d", argPos))
		args = append(args, newSlug)
		details["new_slug"] = newSlug
		argPos++
	}
	if opts.Name != nil {
		name := strings.TrimSpace(*opts.Name)
		if name == "" {
			return nil, fmt.Errorf("tag name must not be empty")
		}
		set = append(set, fmt.Sprintf("name = $%d", argPos))
		args = append(args, name)
		details["name"] = name
		argPos++
	}
	if opts.Description != nil {
		description := normalizeOptionalString(opts.Description)
		set = append(set, fmt.Sprintf("description = $%d", argPos))
		args = append(args, description)
		details["description"] = description
		argPos++
	}
	if opts.Color != nil {
		color, err := normalizeOptionalColor(opts.Color)
		if err != nil {
			return nil, err
		}
		set = append(set, fmt.Sprintf("color = $%d", argPos))
		args = append(args, color)
		details["color"] = color
		argPos++
	}
	if len(set) == 0 {
		return nil, fmt.Errorf("at least one update field is required")
	}

	set = append(set, fmt.Sprintf("updated_at = $%d", argPos))
	args = append(args, now.UTC())

	tx, err := p.BeginTx(ctx, TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := fmt.Sprintf(`
UPDATE news.tags
SET %s
WHERE slug = $1
RETURNING tag_id, tag_uuid::text, slug, name, description, color, archived_at, created_at, updated_at
`, strings.Join(set, ", "))
	tag, err := scanTag(tx.QueryRow(ctx, q, args...))
	if err != nil {
		if IsNoRows(err) {
			return nil, ErrNoRows
		}
		return nil, fmt.Errorf("update tag: %w", err)
	}
	if err := insertAuditEvent(ctx, tx, nil, "tag.update", "tag", tag.Slug, details); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit update tag: %w", err)
	}
	return tag, nil
}

func (p *Pool) SetTagArchived(ctx context.Context, slug string, archived bool, now time.Time) (*TagRecord, error) {
	normalizedSlug := NormalizeTagSlug(slug)
	if err := ValidateTagSlug(normalizedSlug); err != nil {
		return nil, err
	}

	tx, err := p.BeginTx(ctx, TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
UPDATE news.tags
SET archived_at = CASE WHEN $2 THEN $3 ELSE NULL END,
	updated_at = $3
WHERE slug = $1
RETURNING tag_id, tag_uuid::text, slug, name, description, color, archived_at, created_at, updated_at
`
	tag, err := scanTag(tx.QueryRow(ctx, q, normalizedSlug, archived, now.UTC()))
	if err != nil {
		if IsNoRows(err) {
			return nil, ErrNoRows
		}
		return nil, fmt.Errorf("archive tag: %w", err)
	}
	action := "tag.unarchive"
	if archived {
		action = "tag.archive"
	}
	if err := insertAuditEvent(ctx, tx, nil, action, "tag", tag.Slug, map[string]any{
		"slug":     tag.Slug,
		"archived": archived,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit archive tag: %w", err)
	}
	return tag, nil
}

func (p *Pool) DeleteTag(ctx context.Context, slug string) error {
	normalizedSlug := NormalizeTagSlug(slug)
	if err := ValidateTagSlug(normalizedSlug); err != nil {
		return err
	}

	tx, err := p.BeginTx(ctx, TxOptions{})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var tagID int64
	if err := tx.QueryRow(ctx, `SELECT tag_id FROM news.tags WHERE slug = $1`, normalizedSlug).Scan(&tagID); err != nil {
		if IsNoRows(err) {
			return ErrNoRows
		}
		return fmt.Errorf("query tag: %w", err)
	}
	var usageCount int64
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM news.article_tags WHERE tag_id = $1`, tagID).Scan(&usageCount); err != nil {
		return fmt.Errorf("count tag usage: %w", err)
	}
	if usageCount > 0 {
		return fmt.Errorf("tag %q is attached to %d articles; archive it instead", normalizedSlug, usageCount)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM news.tags WHERE tag_id = $1`, tagID); err != nil {
		return fmt.Errorf("delete tag: %w", err)
	}
	if err := insertAuditEvent(ctx, tx, nil, "tag.delete", "tag", normalizedSlug, map[string]any{
		"slug": normalizedSlug,
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit delete tag: %w", err)
	}
	return nil
}

func (p *Pool) AddArticleTag(ctx context.Context, articleUUID string, slug string, actorUserID *int64, now time.Time) error {
	normalizedSlug := NormalizeTagSlug(slug)
	if err := ValidateTagSlug(normalizedSlug); err != nil {
		return err
	}

	tx, err := p.BeginTx(ctx, TxOptions{})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	articleID, normalizedArticleUUID, err := lookupArticleIDByUUID(ctx, tx, articleUUID)
	if err != nil {
		return err
	}
	tagID, err := lookupTagIDBySlug(ctx, tx, normalizedSlug, true)
	if err != nil {
		return err
	}

	const q = `
INSERT INTO news.article_tags (article_id, tag_id, created_by_user_id, created_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (article_id, tag_id) DO NOTHING
`
	tag, err := tx.Exec(ctx, q, articleID, tagID, actorUserID, now.UTC())
	if err != nil {
		return fmt.Errorf("add article tag: %w", err)
	}
	if tag.RowsAffected() > 0 {
		if err := insertAuditEvent(ctx, tx, actorUserID, "article_tag.add", "article", normalizedArticleUUID, map[string]any{
			"article_uuid": normalizedArticleUUID,
			"tag_slug":     normalizedSlug,
		}); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit add article tag: %w", err)
	}
	return nil
}

func (p *Pool) RemoveArticleTag(ctx context.Context, articleUUID string, slug string, actorUserID *int64) error {
	normalizedSlug := NormalizeTagSlug(slug)
	if err := ValidateTagSlug(normalizedSlug); err != nil {
		return err
	}

	tx, err := p.BeginTx(ctx, TxOptions{})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	articleID, normalizedArticleUUID, err := lookupArticleIDByUUID(ctx, tx, articleUUID)
	if err != nil {
		return err
	}
	tagID, err := lookupTagIDBySlug(ctx, tx, normalizedSlug, false)
	if err != nil {
		return err
	}

	tag, err := tx.Exec(ctx, `DELETE FROM news.article_tags WHERE article_id = $1 AND tag_id = $2`, articleID, tagID)
	if err != nil {
		return fmt.Errorf("remove article tag: %w", err)
	}
	if tag.RowsAffected() > 0 {
		if err := insertAuditEvent(ctx, tx, actorUserID, "article_tag.remove", "article", normalizedArticleUUID, map[string]any{
			"article_uuid": normalizedArticleUUID,
			"tag_slug":     normalizedSlug,
		}); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit remove article tag: %w", err)
	}
	return nil
}

func (p *Pool) ListTagsForArticleUUIDs(ctx context.Context, articleUUIDs []string) (map[string][]TagRecord, error) {
	placeholders, args := uuidPlaceholders(articleUUIDs)
	if len(args) == 0 {
		return map[string][]TagRecord{}, nil
	}
	q := fmt.Sprintf(`
SELECT a.article_uuid::text, t.tag_id, t.tag_uuid::text, t.slug, t.name, t.description, t.color, t.archived_at, t.created_at, t.updated_at
FROM news.articles a
JOIN news.article_tags at ON at.article_id = a.article_id
JOIN news.tags t ON t.tag_id = at.tag_id
WHERE a.article_uuid IN (%s)
  AND a.deleted_at IS NULL
ORDER BY t.slug
`, placeholders)
	return scanTagMapByArticleUUID(ctx, p, q, args...)
}

func (p *Pool) ListTagsForStoryIDs(ctx context.Context, storyIDs []int64) (map[int64][]TagRecord, error) {
	placeholders, args := int64Placeholders(storyIDs)
	if len(args) == 0 {
		return map[int64][]TagRecord{}, nil
	}
	q := fmt.Sprintf(`
SELECT DISTINCT sm.story_id, t.tag_id, t.tag_uuid::text, t.slug, t.name, t.description, t.color, t.archived_at, t.created_at, t.updated_at
FROM news.story_articles sm
JOIN news.articles a
  ON a.article_id = sm.article_id
  AND a.deleted_at IS NULL
JOIN news.article_tags at ON at.article_id = sm.article_id
JOIN news.tags t ON t.tag_id = at.tag_id
WHERE sm.story_id IN (%s)
ORDER BY sm.story_id, t.slug
`, placeholders)
	rows, err := p.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query story tags: %w", err)
	}
	defer rows.Close()

	result := make(map[int64][]TagRecord, len(storyIDs))
	for rows.Next() {
		var storyID int64
		tag, err := scanTagFromRows(rows, &storyID)
		if err != nil {
			return nil, err
		}
		result[storyID] = append(result[storyID], tag)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate story tags: %w", err)
	}
	return result, nil
}

func scanTag(row *Row) (*TagRecord, error) {
	var tag TagRecord
	if err := row.Scan(
		&tag.TagID,
		&tag.TagUUID,
		&tag.Slug,
		&tag.Name,
		&tag.Description,
		&tag.Color,
		&tag.ArchivedAt,
		&tag.CreatedAt,
		&tag.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &tag, nil
}

func scanTagFromRows(rows *Rows, prefixDest ...any) (TagRecord, error) {
	dest := make([]any, 0, len(prefixDest)+9)
	dest = append(dest, prefixDest...)
	var tag TagRecord
	dest = append(dest,
		&tag.TagID,
		&tag.TagUUID,
		&tag.Slug,
		&tag.Name,
		&tag.Description,
		&tag.Color,
		&tag.ArchivedAt,
		&tag.CreatedAt,
		&tag.UpdatedAt,
	)
	if err := rows.Scan(dest...); err != nil {
		return TagRecord{}, fmt.Errorf("scan tag row: %w", err)
	}
	return tag, nil
}

func scanTagMapByArticleUUID(ctx context.Context, p *Pool, query string, args ...any) (map[string][]TagRecord, error) {
	rows, err := p.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query article tags: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]TagRecord)
	for rows.Next() {
		var articleUUID string
		tag, err := scanTagFromRows(rows, &articleUUID)
		if err != nil {
			return nil, err
		}
		result[articleUUID] = append(result[articleUUID], tag)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate article tags: %w", err)
	}
	return result, nil
}

func lookupArticleIDByUUID(ctx context.Context, tx Tx, articleUUID string) (int64, string, error) {
	trimmedUUID := strings.TrimSpace(articleUUID)
	if trimmedUUID == "" {
		return 0, "", fmt.Errorf("article UUID is required")
	}
	var (
		articleID     int64
		storedArticle string
	)
	const q = `
SELECT article_id, article_uuid::text
FROM news.articles
WHERE article_uuid = $1::uuid
  AND deleted_at IS NULL
LIMIT 1
`
	if err := tx.QueryRow(ctx, q, trimmedUUID).Scan(&articleID, &storedArticle); err != nil {
		if IsNoRows(err) {
			return 0, "", ErrNoRows
		}
		return 0, "", fmt.Errorf("query article: %w", err)
	}
	return articleID, storedArticle, nil
}

func lookupTagIDBySlug(ctx context.Context, tx Tx, slug string, requireActive bool) (int64, error) {
	condition := ""
	if requireActive {
		condition = "AND archived_at IS NULL"
	}
	q := fmt.Sprintf(`
SELECT tag_id
FROM news.tags
WHERE slug = $1
%s
LIMIT 1
`, condition)
	var tagID int64
	if err := tx.QueryRow(ctx, q, slug).Scan(&tagID); err != nil {
		if IsNoRows(err) {
			return 0, ErrNoRows
		}
		return 0, fmt.Errorf("query tag: %w", err)
	}
	return tagID, nil
}

func insertAuditEvent(ctx context.Context, tx Tx, actorUserID *int64, action string, targetType string, targetID string, details map[string]any) error {
	rawDetails, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("marshal audit details: %w", err)
	}
	const q = `
INSERT INTO news.audit_events (actor_user_id, action, target_type, target_id, details, created_at)
VALUES ($1, $2, $3, $4, $5, now())
`
	if _, err := tx.Exec(ctx, q, actorUserID, action, targetType, targetID, rawDetails); err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}
	return nil
}

func uuidPlaceholders(values []string) (string, []any) {
	args := make([]any, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		args = append(args, trimmed)
	}
	return placeholders(len(args), "::uuid"), args
}

func int64Placeholders(values []int64) (string, []any) {
	args := make([]any, 0, len(values))
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		args = append(args, value)
	}
	return placeholders(len(args), "::bigint"), args
}

func placeholders(count int, cast string) string {
	parts := make([]string, 0, count)
	for idx := 1; idx <= count; idx++ {
		parts = append(parts, fmt.Sprintf("$%d%s", idx, cast))
	}
	return strings.Join(parts, ", ")
}
