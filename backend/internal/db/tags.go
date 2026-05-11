package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var tagPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

type TagRecord struct {
	TagID       int64      `json:"tag_id"`
	TagUUID     string     `json:"tag_uuid"`
	Tag         string     `json:"tag"`
	Slug        string     `json:"-"`
	Name        string     `json:"-"`
	Description *string    `json:"description,omitempty"`
	Color       *string    `json:"color,omitempty"`
	ArchivedAt  *time.Time `json:"archived_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type UpsertTagOptions struct {
	Slug        string
	Description *string
	Color       *string
}

type UpdateTagOptions struct {
	NewSlug     *string
	Description *string
	Color       *string
}

func NormalizeTagSlug(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func ValidateTagSlug(slug string) error {
	normalized := NormalizeTagSlug(slug)
	if normalized == "" {
		return fmt.Errorf("tag is required")
	}
	if len(normalized) > 64 {
		return fmt.Errorf("tag must be 64 characters or fewer")
	}
	if !tagPattern.MatchString(normalized) {
		return fmt.Errorf("tag must contain only lowercase letters, numbers, and single dashes")
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
	var tags []Tag
	query := p.gdb.WithContext(ctx).Model(&Tag{})
	if !includeArchived {
		query = query.Where("archived_at IS NULL")
	}
	if err := query.Order("archived_at NULLS FIRST, slug").Find(&tags).Error; err != nil {
		return nil, fmt.Errorf("query tags: %w", err)
	}

	records := make([]TagRecord, 0, len(tags))
	for _, tag := range tags {
		records = append(records, tagModelToRecord(tag))
	}
	return records, nil
}

func (p *Pool) CreateTag(ctx context.Context, opts UpsertTagOptions, now time.Time) (*TagRecord, error) {
	slug := NormalizeTagSlug(opts.Slug)
	if err := ValidateTagSlug(slug); err != nil {
		return nil, err
	}
	description := normalizeOptionalString(opts.Description)
	color, err := normalizeOptionalColor(opts.Color)
	if err != nil {
		return nil, err
	}

	var tag Tag
	err = p.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tag = Tag{
			Slug:        slug,
			Name:        slug,
			Description: description,
			Color:       color,
			CreatedAt:   now.UTC(),
			UpdatedAt:   now.UTC(),
		}
		if err := tx.Create(&tag).Error; err != nil {
			return fmt.Errorf("create tag: %w", err)
		}
		if err := insertAuditEventGORM(ctx, tx, nil, "tag.create", "tag", tag.Slug, map[string]any{
			"tag": tag.Slug,
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	record := tagModelToRecord(tag)
	return &record, nil
}

func (p *Pool) UpdateTag(ctx context.Context, slug string, opts UpdateTagOptions, now time.Time) (*TagRecord, error) {
	normalizedSlug := NormalizeTagSlug(slug)
	if err := ValidateTagSlug(normalizedSlug); err != nil {
		return nil, err
	}

	updates := map[string]any{}
	details := map[string]any{"old_tag": normalizedSlug}

	if opts.NewSlug != nil {
		newSlug := NormalizeTagSlug(*opts.NewSlug)
		if err := ValidateTagSlug(newSlug); err != nil {
			return nil, err
		}
		updates["slug"] = newSlug
		updates["name"] = newSlug
		details["new_tag"] = newSlug
	}
	if opts.Description != nil {
		description := normalizeOptionalString(opts.Description)
		updates["description"] = description
		details["description"] = description
	}
	if opts.Color != nil {
		color, err := normalizeOptionalColor(opts.Color)
		if err != nil {
			return nil, err
		}
		updates["color"] = color
		details["color"] = color
	}
	if len(updates) == 0 {
		return nil, fmt.Errorf("at least one update field is required")
	}
	updates["updated_at"] = now.UTC()

	var tag Tag
	err := p.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("slug = ?", normalizedSlug).First(&tag).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNoRows
			}
			return fmt.Errorf("query tag: %w", err)
		}
		if err := tx.Model(&tag).Updates(updates).Error; err != nil {
			return fmt.Errorf("update tag: %w", err)
		}
		if err := tx.Where("tag_id = ?", tag.TagID).First(&tag).Error; err != nil {
			return fmt.Errorf("query updated tag: %w", err)
		}
		if err := insertAuditEventGORM(ctx, tx, nil, "tag.update", "tag", tag.Slug, details); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	record := tagModelToRecord(tag)
	return &record, nil
}

func (p *Pool) SetTagArchived(ctx context.Context, slug string, archived bool, now time.Time) (*TagRecord, error) {
	normalizedSlug := NormalizeTagSlug(slug)
	if err := ValidateTagSlug(normalizedSlug); err != nil {
		return nil, err
	}

	action := "tag.unarchive"
	if archived {
		action = "tag.archive"
	}

	var tag Tag
	err := p.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("slug = ?", normalizedSlug).First(&tag).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNoRows
			}
			return fmt.Errorf("query tag: %w", err)
		}
		var archivedAt *time.Time
		if archived {
			value := now.UTC()
			archivedAt = &value
		}
		if err := tx.Model(&tag).Updates(map[string]any{
			"archived_at": archivedAt,
			"updated_at":  now.UTC(),
		}).Error; err != nil {
			return fmt.Errorf("archive tag: %w", err)
		}
		if err := tx.Where("tag_id = ?", tag.TagID).First(&tag).Error; err != nil {
			return fmt.Errorf("query archived tag: %w", err)
		}
		if err := insertAuditEventGORM(ctx, tx, nil, action, "tag", tag.Slug, map[string]any{
			"tag":      tag.Slug,
			"archived": archived,
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	record := tagModelToRecord(tag)
	return &record, nil
}

func (p *Pool) DeleteTag(ctx context.Context, slug string) error {
	normalizedSlug := NormalizeTagSlug(slug)
	if err := ValidateTagSlug(normalizedSlug); err != nil {
		return err
	}

	return p.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tag Tag
		if err := tx.Where("slug = ?", normalizedSlug).First(&tag).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNoRows
			}
			return fmt.Errorf("query tag: %w", err)
		}
		var usageCount int64
		if err := tx.Model(&ArticleTag{}).Where("tag_id = ?", tag.TagID).Count(&usageCount).Error; err != nil {
			return fmt.Errorf("count tag usage: %w", err)
		}
		if usageCount > 0 {
			return fmt.Errorf("tag %q is attached to %d articles; archive it instead", normalizedSlug, usageCount)
		}
		if err := tx.Delete(&tag).Error; err != nil {
			return fmt.Errorf("delete tag: %w", err)
		}
		if err := insertAuditEventGORM(ctx, tx, nil, "tag.delete", "tag", normalizedSlug, map[string]any{
			"tag": normalizedSlug,
		}); err != nil {
			return err
		}
		return nil
	})
}

func (p *Pool) AddArticleTag(ctx context.Context, articleUUID string, slug string, actorUserID *int64, now time.Time) error {
	normalizedSlug := NormalizeTagSlug(slug)
	if err := ValidateTagSlug(normalizedSlug); err != nil {
		return err
	}

	return p.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		article, err := lookupArticleByUUIDGORM(ctx, tx, articleUUID)
		if err != nil {
			return err
		}
		tag, err := lookupTagBySlugGORM(ctx, tx, normalizedSlug, true)
		if err != nil {
			return err
		}
		articleTag := ArticleTag{
			ArticleID:       article.ArticleID,
			TagID:           tag.TagID,
			CreatedByUserID: actorUserID,
			CreatedAt:       now.UTC(),
		}
		res := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "article_id"}, {Name: "tag_id"}},
			DoNothing: true,
		}).Create(&articleTag)
		if res.Error != nil {
			return fmt.Errorf("add article tag: %w", res.Error)
		}
		if res.RowsAffected > 0 {
			if err := insertAuditEventGORM(ctx, tx, actorUserID, "article_tag.add", "article", article.ArticleUUID, map[string]any{
				"article_uuid": article.ArticleUUID,
				"tag":          normalizedSlug,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (p *Pool) RemoveArticleTag(ctx context.Context, articleUUID string, slug string, actorUserID *int64) error {
	normalizedSlug := NormalizeTagSlug(slug)
	if err := ValidateTagSlug(normalizedSlug); err != nil {
		return err
	}

	return p.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		article, err := lookupArticleByUUIDGORM(ctx, tx, articleUUID)
		if err != nil {
			return err
		}
		tag, err := lookupTagBySlugGORM(ctx, tx, normalizedSlug, false)
		if err != nil {
			return err
		}
		res := tx.Where("article_id = ? AND tag_id = ?", article.ArticleID, tag.TagID).Delete(&ArticleTag{})
		if res.Error != nil {
			return fmt.Errorf("remove article tag: %w", res.Error)
		}
		if res.RowsAffected > 0 {
			if err := insertAuditEventGORM(ctx, tx, actorUserID, "article_tag.remove", "article", article.ArticleUUID, map[string]any{
				"article_uuid": article.ArticleUUID,
				"tag":          normalizedSlug,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (p *Pool) ListTagsForArticleUUIDs(ctx context.Context, articleUUIDs []string) (map[string][]TagRecord, error) {
	articleUUIDs = uniqueTrimmedStrings(articleUUIDs)
	if len(articleUUIDs) == 0 {
		return map[string][]TagRecord{}, nil
	}
	var rows []articleTagRow
	uuidCondition, uuidArgs := uuidInCondition("a.article_uuid", articleUUIDs)
	if err := p.gdb.WithContext(ctx).
		Table("news.articles AS a").
		Select("a.article_uuid::text AS article_uuid, t.tag_id, t.tag_uuid::text AS tag_uuid, t.slug, t.name, t.description, t.color, t.archived_at, t.created_at, t.updated_at").
		Joins("JOIN news.article_tags AS at ON at.article_id = a.article_id").
		Joins("JOIN news.tags AS t ON t.tag_id = at.tag_id").
		Where(uuidCondition, uuidArgs...).
		Where("a.deleted_at IS NULL").
		Order("t.slug").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("query article tags: %w", err)
	}
	result := make(map[string][]TagRecord)
	for _, row := range rows {
		result[row.ArticleUUID] = append(result[row.ArticleUUID], row.tagRecord())
	}
	return result, nil
}

func (p *Pool) ListTagsForStoryIDs(ctx context.Context, storyIDs []int64) (map[int64][]TagRecord, error) {
	storyIDs = uniquePositiveInt64s(storyIDs)
	if len(storyIDs) == 0 {
		return map[int64][]TagRecord{}, nil
	}
	var rows []storyTagRow
	if err := p.gdb.WithContext(ctx).
		Table("news.story_articles AS sm").
		Distinct("sm.story_id, t.tag_id, t.tag_uuid::text AS tag_uuid, t.slug, t.name, t.description, t.color, t.archived_at, t.created_at, t.updated_at").
		Joins("JOIN news.articles AS a ON a.article_id = sm.article_id AND a.deleted_at IS NULL").
		Joins("JOIN news.article_tags AS at ON at.article_id = sm.article_id").
		Joins("JOIN news.tags AS t ON t.tag_id = at.tag_id").
		Where("sm.story_id IN ?", storyIDs).
		Order("sm.story_id, t.slug").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("query story tags: %w", err)
	}
	result := make(map[int64][]TagRecord, len(storyIDs))
	for _, row := range rows {
		result[row.StoryID] = append(result[row.StoryID], row.tagRecord())
	}
	return result, nil
}

func lookupArticleByUUIDGORM(ctx context.Context, tx *gorm.DB, articleUUID string) (Article, error) {
	trimmedUUID := strings.TrimSpace(articleUUID)
	if trimmedUUID == "" {
		return Article{}, fmt.Errorf("article UUID is required")
	}
	var article Article
	if err := tx.WithContext(ctx).Where("article_uuid = ?::uuid AND deleted_at IS NULL", trimmedUUID).First(&article).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Article{}, ErrNoRows
		}
		return Article{}, fmt.Errorf("query article: %w", err)
	}
	return article, nil
}

func lookupTagBySlugGORM(ctx context.Context, tx *gorm.DB, slug string, requireActive bool) (Tag, error) {
	query := tx.WithContext(ctx).Where("slug = ?", slug)
	if requireActive {
		query = query.Where("archived_at IS NULL")
	}
	var tag Tag
	if err := query.First(&tag).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Tag{}, ErrNoRows
		}
		return Tag{}, fmt.Errorf("query tag: %w", err)
	}
	return tag, nil
}

func insertAuditEventGORM(ctx context.Context, tx *gorm.DB, actorUserID *int64, action string, targetType string, targetID string, details map[string]any) error {
	rawDetails, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("marshal audit details: %w", err)
	}
	event := AuditEvent{
		ActorUserID: actorUserID,
		Action:      action,
		TargetType:  targetType,
		TargetID:    targetID,
		Details:     rawDetails,
	}
	if err := tx.WithContext(ctx).Create(&event).Error; err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}
	return nil
}

type articleTagRow struct {
	ArticleUUID string
	TagID       int64
	TagUUID     string
	Slug        string
	Name        string
	Description *string
	Color       *string
	ArchivedAt  *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (r articleTagRow) tagRecord() TagRecord {
	return tagModelToRecord(Tag{
		TagID:       r.TagID,
		TagUUID:     r.TagUUID,
		Slug:        r.Slug,
		Name:        r.Name,
		Description: r.Description,
		Color:       r.Color,
		ArchivedAt:  r.ArchivedAt,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	})
}

type storyTagRow struct {
	StoryID     int64
	TagID       int64
	TagUUID     string
	Slug        string
	Name        string
	Description *string
	Color       *string
	ArchivedAt  *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (r storyTagRow) tagRecord() TagRecord {
	return tagModelToRecord(Tag{
		TagID:       r.TagID,
		TagUUID:     r.TagUUID,
		Slug:        r.Slug,
		Name:        r.Name,
		Description: r.Description,
		Color:       r.Color,
		ArchivedAt:  r.ArchivedAt,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	})
}

func tagModelToRecord(tag Tag) TagRecord {
	return TagRecord{
		TagID:       tag.TagID,
		TagUUID:     tag.TagUUID,
		Tag:         tag.Slug,
		Slug:        tag.Slug,
		Name:        tag.Name,
		Description: tag.Description,
		Color:       tag.Color,
		ArchivedAt:  tag.ArchivedAt,
		CreatedAt:   tag.CreatedAt,
		UpdatedAt:   tag.UpdatedAt,
	}
}

func uniqueTrimmedStrings(values []string) []string {
	result := make([]string, 0, len(values))
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
		result = append(result, trimmed)
	}
	return result
}

func uuidInCondition(column string, values []string) (string, []any) {
	placeholders := make([]string, 0, len(values))
	args := make([]any, 0, len(values))
	for _, value := range values {
		placeholders = append(placeholders, "?::uuid")
		args = append(args, value)
	}
	return fmt.Sprintf("%s IN (%s)", column, strings.Join(placeholders, ", ")), args
}

func uniquePositiveInt64s(values []int64) []int64 {
	result := make([]int64, 0, len(values))
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
