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
	TagID          int64      `json:"tag_id"`
	TagUUID        string     `json:"tag_uuid"`
	Tag            string     `json:"tag"`
	Slug           string     `json:"-"`
	Name           string     `json:"-"`
	Description    *string    `json:"description,omitempty"`
	Color          *string    `json:"color,omitempty"`
	HighlightColor *string    `json:"highlight_color,omitempty"`
	ArchivedAt     *time.Time `json:"archived_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type UpsertTagOptions struct {
	Slug           string
	Description    *string
	Color          *string
	HighlightColor *string
}

type UpdateTagOptions struct {
	NewSlug        *string
	Description    *string
	Color          *string
	HighlightColor *string
}

type tagUpdateMutation struct {
	currentSlug string
	updates     map[string]any
	details     map[string]any
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
	highlightColor, err := normalizeOptionalColor(opts.HighlightColor)
	if err != nil {
		return nil, err
	}

	var tag Tag
	err = p.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tag = Tag{
			Slug:           slug,
			Name:           slug,
			Description:    description,
			Color:          color,
			HighlightColor: highlightColor,
			CreatedAt:      now.UTC(),
			UpdatedAt:      now.UTC(),
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
	mutation, err := buildTagUpdateMutation(slug, opts, now)
	if err != nil {
		return nil, err
	}

	var tag Tag
	err = p.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		tag, err = updateTagGORM(ctx, tx, mutation)
		return err
	})
	if err != nil {
		return nil, err
	}
	record := tagModelToRecord(tag)
	return &record, nil
}

func updateTagGORM(ctx context.Context, tx *gorm.DB, mutation tagUpdateMutation) (Tag, error) {
	tag, err := lookupTagForUpdateGORM(tx, mutation.currentSlug)
	if err != nil {
		return Tag{}, err
	}
	if err := tx.Model(&tag).Updates(mutation.updates).Error; err != nil {
		return Tag{}, fmt.Errorf("update tag: %w", err)
	}
	if err := tx.Where("tag_id = ?", tag.TagID).First(&tag).Error; err != nil {
		return Tag{}, fmt.Errorf("query updated tag: %w", err)
	}
	if err := insertAuditEventGORM(ctx, tx, nil, "tag.update", "tag", tag.Slug, mutation.details); err != nil {
		return Tag{}, err
	}
	return tag, nil
}

func lookupTagForUpdateGORM(tx *gorm.DB, slug string) (Tag, error) {
	var tag Tag
	if err := tx.Where("slug = ?", slug).First(&tag).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Tag{}, ErrNoRows
		}
		return Tag{}, fmt.Errorf("query tag: %w", err)
	}
	return tag, nil
}

func buildTagUpdateMutation(slug string, opts UpdateTagOptions, now time.Time) (tagUpdateMutation, error) {
	normalizedSlug := NormalizeTagSlug(slug)
	if err := ValidateTagSlug(normalizedSlug); err != nil {
		return tagUpdateMutation{}, err
	}
	mutation := tagUpdateMutation{
		currentSlug: normalizedSlug,
		updates:     map[string]any{},
		details:     map[string]any{"old_tag": normalizedSlug},
	}
	if err := applyTagSlugUpdate(&mutation, opts.NewSlug); err != nil {
		return tagUpdateMutation{}, err
	}
	if err := applyTagDescriptionUpdate(&mutation, opts.Description); err != nil {
		return tagUpdateMutation{}, err
	}
	if err := applyTagColorUpdate(&mutation, "color", opts.Color); err != nil {
		return tagUpdateMutation{}, err
	}
	if err := applyTagColorUpdate(&mutation, "highlight_color", opts.HighlightColor); err != nil {
		return tagUpdateMutation{}, err
	}
	if len(mutation.updates) == 0 {
		return tagUpdateMutation{}, fmt.Errorf("at least one update field is required")
	}
	mutation.updates["updated_at"] = now.UTC()
	return mutation, nil
}

func applyTagSlugUpdate(mutation *tagUpdateMutation, raw *string) error {
	if raw == nil {
		return nil
	}
	newSlug := NormalizeTagSlug(*raw)
	if err := ValidateTagSlug(newSlug); err != nil {
		return err
	}
	mutation.updates["slug"] = newSlug
	mutation.updates["name"] = newSlug
	mutation.details["new_tag"] = newSlug
	return nil
}

func applyTagDescriptionUpdate(mutation *tagUpdateMutation, raw *string) error {
	if raw == nil {
		return nil
	}
	description := normalizeOptionalString(raw)
	mutation.updates["description"] = description
	mutation.details["description"] = description
	return nil
}

func applyTagColorUpdate(mutation *tagUpdateMutation, field string, raw *string) error {
	if raw == nil {
		return nil
	}
	color, err := normalizeOptionalColor(raw)
	if err != nil {
		return err
	}
	mutation.updates[field] = color
	mutation.details[field] = color
	return nil
}

func (p *Pool) SetTagArchived(ctx context.Context, slug string, archived bool, now time.Time) (*TagRecord, error) {
	normalizedSlug := NormalizeTagSlug(slug)
	if err := ValidateTagSlug(normalizedSlug); err != nil {
		return nil, err
	}

	var tag Tag
	err := p.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		tag, err = setTagArchivedTx(ctx, tx, normalizedSlug, archived, now)
		return err
	})
	if err != nil {
		return nil, err
	}
	record := tagModelToRecord(tag)
	return &record, nil
}

func setTagArchivedTx(ctx context.Context, tx *gorm.DB, normalizedSlug string, archived bool, now time.Time) (Tag, error) {
	tag, err := lookupTagBySlugGORM(ctx, tx, normalizedSlug, false)
	if err != nil {
		return Tag{}, err
	}
	if err := tx.Model(&tag).Updates(tagArchiveUpdates(archived, now)).Error; err != nil {
		return Tag{}, fmt.Errorf("archive tag: %w", err)
	}
	if err := tx.Where("tag_id = ?", tag.TagID).First(&tag).Error; err != nil {
		return Tag{}, fmt.Errorf("query archived tag: %w", err)
	}
	if err := insertAuditEventGORM(ctx, tx, nil, tagArchiveAction(archived), "tag", tag.Slug, map[string]any{
		"tag":      tag.Slug,
		"archived": archived,
	}); err != nil {
		return Tag{}, err
	}
	return tag, nil
}

func tagArchiveUpdates(archived bool, now time.Time) map[string]any {
	var archivedAt *time.Time
	if archived {
		value := now.UTC()
		archivedAt = &value
	}
	return map[string]any{
		"archived_at": archivedAt,
		"updated_at":  now.UTC(),
	}
}

func tagArchiveAction(archived bool) string {
	if archived {
		return "tag.archive"
	}
	return "tag.unarchive"
}

func (p *Pool) DeleteTag(ctx context.Context, slug string) error {
	normalizedSlug := NormalizeTagSlug(slug)
	if err := ValidateTagSlug(normalizedSlug); err != nil {
		return err
	}

	return p.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tag, err := lookupDeletableTagGORM(tx, normalizedSlug)
		if err != nil {
			return err
		}
		return deleteTagGORM(ctx, tx, tag)
	})
}

func lookupDeletableTagGORM(tx *gorm.DB, slug string) (Tag, error) {
	var tag Tag
	if err := tx.Where("slug = ?", slug).First(&tag).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Tag{}, ErrNoRows
		}
		return Tag{}, fmt.Errorf("query tag: %w", err)
	}
	var usageCount int64
	if err := tx.Model(&ArticleTag{}).Where("tag_id = ?", tag.TagID).Count(&usageCount).Error; err != nil {
		return Tag{}, fmt.Errorf("count tag usage: %w", err)
	}
	if usageCount > 0 {
		return Tag{}, fmt.Errorf("tag %q is attached to %d articles; archive it instead", slug, usageCount)
	}
	return tag, nil
}

func deleteTagGORM(ctx context.Context, tx *gorm.DB, tag Tag) error {
	if err := tx.Delete(&tag).Error; err != nil {
		return fmt.Errorf("delete tag: %w", err)
	}
	return insertAuditEventGORM(ctx, tx, nil, "tag.delete", "tag", tag.Slug, map[string]any{
		"tag": tag.Slug,
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
	selectClause := "a.article_uuid::text AS article_uuid, t.tag_id, t.tag_uuid::text AS tag_uuid, t.slug, t.name, t.description, t.color, t.highlight_color, t.archived_at, t.created_at, t.updated_at"
	joins := []string{
		"JOIN news.article_tags AS at ON at.article_id = a.article_id",
		"JOIN news.tags AS t ON t.tag_id = at.tag_id",
	}
	return listArticleRelatedRecords(
		ctx,
		p.gdb,
		articleUUIDs,
		selectClause,
		joins,
		"t.slug",
		"query article tags",
		func(row articleTagRow) string { return row.ArticleUUID },
		func(row articleTagRow) TagRecord { return row.tagRecord() },
	)
}

func (p *Pool) ListTagsForStoryIDs(ctx context.Context, storyIDs []int64) (map[int64][]TagRecord, error) {
	selectClause := "sm.story_id, t.tag_id, t.tag_uuid::text AS tag_uuid, t.slug, t.name, t.description, t.color, t.highlight_color, t.archived_at, t.created_at, t.updated_at"
	joins := []string{
		"JOIN news.articles AS a ON a.article_id = sm.article_id AND a.deleted_at IS NULL",
		"JOIN news.article_tags AS at ON at.article_id = sm.article_id",
		"JOIN news.tags AS t ON t.tag_id = at.tag_id",
	}
	return listStoryRelatedRecords(
		ctx,
		p.gdb,
		storyIDs,
		selectClause,
		joins,
		"sm.story_id, t.slug",
		"query story tags",
		func(row storyTagRow) int64 { return row.StoryID },
		func(row storyTagRow) TagRecord { return row.tagRecord() },
	)
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
	ArticleUUID  string
	TagRowFields `gorm:"embedded"`
}

func (r articleTagRow) tagRecord() TagRecord {
	return r.TagRowFields.tagRecord()
}

type storyTagRow struct {
	StoryID      int64
	TagRowFields `gorm:"embedded"`
}

func (r storyTagRow) tagRecord() TagRecord {
	return r.TagRowFields.tagRecord()
}

type TagRowFields struct {
	TagID          int64
	TagUUID        string
	Slug           string
	Name           string
	Description    *string
	Color          *string
	HighlightColor *string
	ArchivedAt     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (r TagRowFields) tagRecord() TagRecord {
	return tagModelToRecord(Tag(r))
}

func tagModelToRecord(tag Tag) TagRecord {
	return TagRecord{
		TagID:          tag.TagID,
		TagUUID:        tag.TagUUID,
		Tag:            tag.Slug,
		Slug:           tag.Slug,
		Name:           tag.Name,
		Description:    tag.Description,
		Color:          tag.Color,
		HighlightColor: tag.HighlightColor,
		ArchivedAt:     tag.ArchivedAt,
		CreatedAt:      tag.CreatedAt,
		UpdatedAt:      tag.UpdatedAt,
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
