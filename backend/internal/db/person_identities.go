package db

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var providerKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

type PersonIdentityRecord struct {
	PersonIdentityID   int64      `json:"person_identity_id"`
	PersonIdentityUUID string     `json:"person_identity_uuid"`
	Provider           string     `json:"provider"`
	ProviderUserID     *string    `json:"provider_user_id,omitempty"`
	Handle             *string    `json:"handle,omitempty"`
	AvatarURL          *string    `json:"avatar_url,omitempty"`
	IdentityRef        string     `json:"identity_ref"`
	ArchivedAt         *time.Time `json:"archived_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type ParsedIdentityRef struct {
	Provider       string
	ProviderUserID *string
	Handle         *string
	IdentityRef    string
}

func ParseIdentityRef(raw string) (ParsedIdentityRef, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ParsedIdentityRef{}, fmt.Errorf("identity ref is required")
	}
	parsedURL, err := url.Parse(trimmed)
	if err != nil {
		return ParsedIdentityRef{}, fmt.Errorf("invalid identity ref: %w", err)
	}
	if strings.ToLower(parsedURL.Scheme) != "id" {
		return ParsedIdentityRef{}, fmt.Errorf("identity ref must use id://")
	}
	provider := strings.ToLower(strings.TrimSpace(parsedURL.Host))
	if !providerKeyPattern.MatchString(provider) {
		return ParsedIdentityRef{}, fmt.Errorf("provider must match [a-z][a-z0-9-]*")
	}
	segments := strings.Split(strings.Trim(parsedURL.EscapedPath(), "/"), "/")
	if len(segments) != 2 {
		return ParsedIdentityRef{}, fmt.Errorf("identity ref path must be /id/<value> or /handle/<handle>")
	}
	kind := strings.ToLower(strings.TrimSpace(segments[0]))
	value, err := url.PathUnescape(segments[1])
	if err != nil {
		return ParsedIdentityRef{}, fmt.Errorf("invalid identity value: %w", err)
	}
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, " \t\r\n") {
		return ParsedIdentityRef{}, fmt.Errorf("identity value must be non-empty and contain no whitespace")
	}

	var providerUserID *string
	var handle *string
	switch kind {
	case "id":
		providerUserID = stringPtr(value)
		if rawHandle := parsedURL.Query().Get("handle"); rawHandle != "" {
			normalizedHandle, err := normalizeIdentityHandle(rawHandle)
			if err != nil {
				return ParsedIdentityRef{}, err
			}
			handle = &normalizedHandle
		}
	case "handle":
		normalizedHandle, err := normalizeIdentityHandle(value)
		if err != nil {
			return ParsedIdentityRef{}, err
		}
		handle = &normalizedHandle
	default:
		return ParsedIdentityRef{}, fmt.Errorf("identity ref kind must be id or handle")
	}

	identityRef := buildCanonicalIdentityRef(provider, providerUserID, handle)
	return ParsedIdentityRef{
		Provider:       provider,
		ProviderUserID: providerUserID,
		Handle:         handle,
		IdentityRef:    identityRef,
	}, nil
}

func normalizeIdentityHandle(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, "@")
	trimmed = strings.ToLower(trimmed)
	if trimmed == "" || strings.ContainsAny(trimmed, " \t\r\n") {
		return "", fmt.Errorf("handle must be non-empty and contain no whitespace")
	}
	return trimmed, nil
}

func buildCanonicalIdentityRef(provider string, providerUserID *string, handle *string) string {
	if providerUserID != nil && strings.TrimSpace(*providerUserID) != "" {
		base := fmt.Sprintf("id://%s/id/%s", provider, url.PathEscape(strings.TrimSpace(*providerUserID)))
		if handle != nil && strings.TrimSpace(*handle) != "" {
			query := url.Values{}
			query.Set("handle", strings.TrimSpace(*handle))
			return base + "?" + query.Encode()
		}
		return base
	}
	if handle != nil {
		return fmt.Sprintf("id://%s/handle/%s", provider, url.PathEscape(strings.TrimSpace(*handle)))
	}
	return ""
}

func stringPtr(value string) *string {
	return &value
}

func (p *Pool) UpsertPersonIdentity(ctx context.Context, rawIdentityRef string, now time.Time) (*PersonIdentityRecord, error) {
	parsed, err := ParseIdentityRef(rawIdentityRef)
	if err != nil {
		return nil, err
	}

	var identity PersonIdentity
	err = p.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.WithContext(ctx).Model(&PersonIdentity{}).Where("provider = ?", parsed.Provider)
		if parsed.ProviderUserID != nil {
			query = query.Where("provider_user_id = ?", *parsed.ProviderUserID)
		} else if parsed.Handle != nil {
			query = query.Where("provider_user_id IS NULL AND handle = ?", *parsed.Handle)
		} else {
			return fmt.Errorf("identity ref must include provider user ID or handle")
		}

		if err := query.First(&identity).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("query person identity: %w", err)
			}
			identity = PersonIdentity{
				Provider:       parsed.Provider,
				ProviderUserID: parsed.ProviderUserID,
				Handle:         parsed.Handle,
				IdentityRef:    parsed.IdentityRef,
				CreatedAt:      now.UTC(),
				UpdatedAt:      now.UTC(),
			}
			if err := tx.Create(&identity).Error; err != nil {
				return fmt.Errorf("create person identity: %w", err)
			}
			if err := insertAuditEventGORM(ctx, tx, nil, "person_identity.create", "person_identity", identity.IdentityRef, map[string]any{
				"identity_ref": identity.IdentityRef,
			}); err != nil {
				return err
			}
			return nil
		}

		updates := map[string]any{"updated_at": now.UTC()}
		if parsed.ProviderUserID != nil && identity.ProviderUserID == nil {
			updates["provider_user_id"] = *parsed.ProviderUserID
		}
		if parsed.Handle != nil && (identity.Handle == nil || *identity.Handle != *parsed.Handle) {
			updates["handle"] = *parsed.Handle
		}
		if parsed.IdentityRef != "" && identity.IdentityRef != parsed.IdentityRef {
			updates["identity_ref"] = parsed.IdentityRef
		}
		if len(updates) > 1 {
			if err := tx.Model(&identity).Updates(updates).Error; err != nil {
				return fmt.Errorf("update person identity: %w", err)
			}
			if err := tx.Where("person_identity_id = ?", identity.PersonIdentityID).First(&identity).Error; err != nil {
				return fmt.Errorf("query updated person identity: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	record := personIdentityModelToRecord(identity)
	return &record, nil
}

func (p *Pool) AddArticlePersonIdentity(ctx context.Context, articleUUID string, rawIdentityRef string, actorUserID *int64, now time.Time) (*PersonIdentityRecord, error) {
	var identity PersonIdentity
	err := p.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		article, err := lookupArticleByUUIDGORM(ctx, tx, articleUUID)
		if err != nil {
			return err
		}
		record, err := p.upsertPersonIdentityTx(ctx, tx, rawIdentityRef, now)
		if err != nil {
			return err
		}
		identity = record
		articleIdentity := ArticlePersonIdentity{
			ArticleID:        article.ArticleID,
			PersonIdentityID: identity.PersonIdentityID,
			CreatedByUserID:  actorUserID,
			CreatedAt:        now.UTC(),
		}
		res := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "article_id"}, {Name: "person_identity_id"}},
			DoNothing: true,
		}).Create(&articleIdentity)
		if res.Error != nil {
			return fmt.Errorf("add article person identity: %w", res.Error)
		}
		if res.RowsAffected > 0 {
			if err := insertAuditEventGORM(ctx, tx, actorUserID, "article_person_identity.add", "article", article.ArticleUUID, map[string]any{
				"article_uuid": article.ArticleUUID,
				"identity_ref": identity.IdentityRef,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	record := personIdentityModelToRecord(identity)
	return &record, nil
}

func (p *Pool) RemoveArticlePersonIdentity(ctx context.Context, articleUUID string, identityRefOrUUID string, actorUserID *int64) error {
	return p.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		article, err := lookupArticleByUUIDGORM(ctx, tx, articleUUID)
		if err != nil {
			return err
		}
		identity, err := lookupPersonIdentityGORM(ctx, tx, identityRefOrUUID)
		if err != nil {
			return err
		}
		res := tx.Where("article_id = ? AND person_identity_id = ?", article.ArticleID, identity.PersonIdentityID).Delete(&ArticlePersonIdentity{})
		if res.Error != nil {
			return fmt.Errorf("remove article person identity: %w", res.Error)
		}
		if res.RowsAffected > 0 {
			if err := insertAuditEventGORM(ctx, tx, actorUserID, "article_person_identity.remove", "article", article.ArticleUUID, map[string]any{
				"article_uuid": article.ArticleUUID,
				"identity_ref": identity.IdentityRef,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (p *Pool) ListPersonIdentities(ctx context.Context, query string, includeArchived bool, limit int) ([]PersonIdentityRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var identities []PersonIdentity
	dbq := p.gdb.WithContext(ctx).Model(&PersonIdentity{})
	if !includeArchived {
		dbq = dbq.Where("archived_at IS NULL")
	}
	trimmedQuery := strings.ToLower(strings.TrimSpace(query))
	if trimmedQuery != "" {
		like := "%" + trimmedQuery + "%"
		dbq = dbq.Where("LOWER(person_identity_uuid::text) LIKE ? OR LOWER(identity_ref) LIKE ? OR LOWER(COALESCE(handle, '')) LIKE ? OR LOWER(COALESCE(provider_user_id, '')) LIKE ?", like, like, like, like)
	}
	if err := dbq.Order("provider, COALESCE(handle, provider_user_id), person_identity_id").Limit(limit).Find(&identities).Error; err != nil {
		return nil, fmt.Errorf("query person identities: %w", err)
	}
	return personIdentityModelsToRecords(identities), nil
}

func (p *Pool) GetPersonIdentity(ctx context.Context, identityRefOrUUID string) (*PersonIdentityRecord, error) {
	identity, err := lookupPersonIdentityGORM(ctx, p.gdb, identityRefOrUUID)
	if err != nil {
		return nil, err
	}
	record := personIdentityModelToRecord(identity)
	return &record, nil
}

func (p *Pool) ListPersonIdentitiesForArticleUUID(ctx context.Context, articleUUID string) ([]PersonIdentityRecord, error) {
	var rows []articlePersonIdentityRow
	if err := p.gdb.WithContext(ctx).
		Table("news.articles AS a").
		Select("pi.person_identity_id, pi.person_identity_uuid::text AS person_identity_uuid, pi.provider, pi.provider_user_id, pi.handle, pi.avatar_url, pi.identity_ref, pi.archived_at, pi.created_at, pi.updated_at").
		Joins("JOIN news.article_person_identities AS api ON api.article_id = a.article_id").
		Joins("JOIN news.person_identities AS pi ON pi.person_identity_id = api.person_identity_id").
		Where("a.article_uuid = ?::uuid AND a.deleted_at IS NULL", strings.TrimSpace(articleUUID)).
		Order("pi.provider, COALESCE(pi.handle, pi.provider_user_id), pi.person_identity_id").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("query article person identities: %w", err)
	}
	records := make([]PersonIdentityRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, row.personIdentityRecord())
	}
	return records, nil
}

func (p *Pool) ListPersonIdentitiesForArticleUUIDs(ctx context.Context, articleUUIDs []string) (map[string][]PersonIdentityRecord, error) {
	articleUUIDs = uniqueTrimmedStrings(articleUUIDs)
	if len(articleUUIDs) == 0 {
		return map[string][]PersonIdentityRecord{}, nil
	}
	var rows []articlePersonIdentityRow
	uuidCondition, uuidArgs := uuidInCondition("a.article_uuid", articleUUIDs)
	if err := p.gdb.WithContext(ctx).
		Table("news.articles AS a").
		Select("a.article_uuid::text AS article_uuid, pi.person_identity_id, pi.person_identity_uuid::text AS person_identity_uuid, pi.provider, pi.provider_user_id, pi.handle, pi.avatar_url, pi.identity_ref, pi.archived_at, pi.created_at, pi.updated_at").
		Joins("JOIN news.article_person_identities AS api ON api.article_id = a.article_id").
		Joins("JOIN news.person_identities AS pi ON pi.person_identity_id = api.person_identity_id").
		Where(uuidCondition, uuidArgs...).
		Where("a.deleted_at IS NULL").
		Order("pi.provider, COALESCE(pi.handle, pi.provider_user_id), pi.person_identity_id").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("query article person identities: %w", err)
	}
	result := make(map[string][]PersonIdentityRecord)
	for _, row := range rows {
		result[row.ArticleUUID] = append(result[row.ArticleUUID], row.personIdentityRecord())
	}
	return result, nil
}

func (p *Pool) ListPersonIdentitiesForStoryIDs(ctx context.Context, storyIDs []int64) (map[int64][]PersonIdentityRecord, error) {
	storyIDs = uniquePositiveInt64s(storyIDs)
	if len(storyIDs) == 0 {
		return map[int64][]PersonIdentityRecord{}, nil
	}
	var rows []storyPersonIdentityRow
	if err := p.gdb.WithContext(ctx).
		Table("news.story_articles AS sm").
		Distinct("sm.story_id, pi.person_identity_id, pi.person_identity_uuid::text AS person_identity_uuid, pi.provider, pi.provider_user_id, pi.handle, pi.avatar_url, pi.identity_ref, pi.archived_at, pi.created_at, pi.updated_at").
		Joins("JOIN news.articles AS a ON a.article_id = sm.article_id AND a.deleted_at IS NULL").
		Joins("JOIN news.article_person_identities AS api ON api.article_id = sm.article_id").
		Joins("JOIN news.person_identities AS pi ON pi.person_identity_id = api.person_identity_id").
		Where("sm.story_id IN ?", storyIDs).
		Order("sm.story_id, pi.provider, pi.person_identity_id").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("query story person identities: %w", err)
	}
	result := make(map[int64][]PersonIdentityRecord, len(storyIDs))
	for _, row := range rows {
		result[row.StoryID] = append(result[row.StoryID], row.personIdentityRecord())
	}
	return result, nil
}

func (p *Pool) SetPersonIdentityArchived(ctx context.Context, identityRefOrUUID string, archived bool, now time.Time) (*PersonIdentityRecord, error) {
	var identity PersonIdentity
	err := p.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		found, err := lookupPersonIdentityGORM(ctx, tx, identityRefOrUUID)
		if err != nil {
			return err
		}
		var archivedAt *time.Time
		if archived {
			value := now.UTC()
			archivedAt = &value
		}
		if err := tx.Model(&found).Updates(map[string]any{
			"archived_at": archivedAt,
			"updated_at":  now.UTC(),
		}).Error; err != nil {
			return fmt.Errorf("archive person identity: %w", err)
		}
		if err := tx.Where("person_identity_id = ?", found.PersonIdentityID).First(&identity).Error; err != nil {
			return fmt.Errorf("query person identity: %w", err)
		}
		action := "person_identity.unarchive"
		if archived {
			action = "person_identity.archive"
		}
		if err := insertAuditEventGORM(ctx, tx, nil, action, "person_identity", identity.IdentityRef, map[string]any{
			"identity_ref": identity.IdentityRef,
			"archived":     archived,
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	record := personIdentityModelToRecord(identity)
	return &record, nil
}

func (p *Pool) SetPersonIdentityAvatarURL(ctx context.Context, identityRefOrUUID string, avatarURL *string, now time.Time) (*PersonIdentityRecord, error) {
	var identity PersonIdentity
	err := p.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		found, err := lookupPersonIdentityGORM(ctx, tx, identityRefOrUUID)
		if err != nil {
			return err
		}
		var normalizedAvatarURL *string
		if avatarURL != nil {
			trimmed := strings.TrimSpace(*avatarURL)
			if trimmed != "" {
				normalizedAvatarURL = &trimmed
			}
		}
		if err := tx.Model(&found).Updates(map[string]any{
			"avatar_url": normalizedAvatarURL,
			"updated_at": now.UTC(),
		}).Error; err != nil {
			return fmt.Errorf("update person identity avatar: %w", err)
		}
		if err := tx.Where("person_identity_id = ?", found.PersonIdentityID).First(&identity).Error; err != nil {
			return fmt.Errorf("query person identity: %w", err)
		}
		if err := insertAuditEventGORM(ctx, tx, nil, "person_identity.avatar_url.update", "person_identity", identity.IdentityRef, map[string]any{
			"identity_ref": identity.IdentityRef,
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	record := personIdentityModelToRecord(identity)
	return &record, nil
}

func (p *Pool) upsertPersonIdentityTx(ctx context.Context, tx *gorm.DB, rawIdentityRef string, now time.Time) (PersonIdentity, error) {
	parsed, err := ParseIdentityRef(rawIdentityRef)
	if err != nil {
		return PersonIdentity{}, err
	}
	var identity PersonIdentity
	query := tx.WithContext(ctx).Model(&PersonIdentity{}).Where("provider = ?", parsed.Provider)
	if parsed.ProviderUserID != nil {
		query = query.Where("provider_user_id = ?", *parsed.ProviderUserID)
	} else if parsed.Handle != nil {
		query = query.Where("provider_user_id IS NULL AND handle = ?", *parsed.Handle)
	} else {
		return PersonIdentity{}, fmt.Errorf("identity ref must include provider user ID or handle")
	}
	if err := query.First(&identity).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return PersonIdentity{}, fmt.Errorf("query person identity: %w", err)
		}
		identity = PersonIdentity{
			Provider:       parsed.Provider,
			ProviderUserID: parsed.ProviderUserID,
			Handle:         parsed.Handle,
			IdentityRef:    parsed.IdentityRef,
			CreatedAt:      now.UTC(),
			UpdatedAt:      now.UTC(),
		}
		if err := tx.Create(&identity).Error; err != nil {
			return PersonIdentity{}, fmt.Errorf("create person identity: %w", err)
		}
		if err := insertAuditEventGORM(ctx, tx, nil, "person_identity.create", "person_identity", identity.IdentityRef, map[string]any{
			"identity_ref": identity.IdentityRef,
		}); err != nil {
			return PersonIdentity{}, err
		}
		return identity, nil
	}
	updates := map[string]any{"updated_at": now.UTC()}
	if parsed.Handle != nil && (identity.Handle == nil || *identity.Handle != *parsed.Handle) {
		updates["handle"] = *parsed.Handle
	}
	if parsed.IdentityRef != "" && identity.IdentityRef != parsed.IdentityRef {
		updates["identity_ref"] = parsed.IdentityRef
	}
	if len(updates) > 1 {
		if err := tx.Model(&identity).Updates(updates).Error; err != nil {
			return PersonIdentity{}, fmt.Errorf("update person identity: %w", err)
		}
		if err := tx.Where("person_identity_id = ?", identity.PersonIdentityID).First(&identity).Error; err != nil {
			return PersonIdentity{}, fmt.Errorf("query updated person identity: %w", err)
		}
	}
	return identity, nil
}

func lookupPersonIdentityGORM(ctx context.Context, tx *gorm.DB, identityRefOrUUID string) (PersonIdentity, error) {
	trimmed := strings.TrimSpace(identityRefOrUUID)
	if trimmed == "" {
		return PersonIdentity{}, fmt.Errorf("person identity ref or UUID is required")
	}
	query := tx.WithContext(ctx).Model(&PersonIdentity{})
	if strings.HasPrefix(strings.ToLower(trimmed), "id://") {
		parsed, err := ParseIdentityRef(trimmed)
		if err != nil {
			return PersonIdentity{}, err
		}
		if parsed.ProviderUserID != nil {
			query = query.Where("provider = ? AND provider_user_id = ?", parsed.Provider, *parsed.ProviderUserID)
		} else {
			query = query.Where("provider = ? AND provider_user_id IS NULL AND handle = ?", parsed.Provider, *parsed.Handle)
		}
	} else {
		query = query.Where("person_identity_uuid = ?::uuid", trimmed)
	}
	var identity PersonIdentity
	if err := query.First(&identity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return PersonIdentity{}, ErrNoRows
		}
		return PersonIdentity{}, fmt.Errorf("query person identity: %w", err)
	}
	return identity, nil
}

func personIdentityModelsToRecords(identities []PersonIdentity) []PersonIdentityRecord {
	records := make([]PersonIdentityRecord, 0, len(identities))
	for _, identity := range identities {
		records = append(records, personIdentityModelToRecord(identity))
	}
	return records
}

func personIdentityModelToRecord(identity PersonIdentity) PersonIdentityRecord {
	return PersonIdentityRecord(identity)
}

type articlePersonIdentityRow struct {
	ArticleUUID        string
	PersonIdentityID   int64
	PersonIdentityUUID string
	Provider           string
	ProviderUserID     *string
	Handle             *string
	AvatarURL          *string
	IdentityRef        string
	ArchivedAt         *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (r articlePersonIdentityRow) personIdentityRecord() PersonIdentityRecord {
	return PersonIdentityRecord{
		PersonIdentityID:   r.PersonIdentityID,
		PersonIdentityUUID: r.PersonIdentityUUID,
		Provider:           r.Provider,
		ProviderUserID:     r.ProviderUserID,
		Handle:             r.Handle,
		AvatarURL:          r.AvatarURL,
		IdentityRef:        r.IdentityRef,
		ArchivedAt:         r.ArchivedAt,
		CreatedAt:          r.CreatedAt,
		UpdatedAt:          r.UpdatedAt,
	}
}

type storyPersonIdentityRow struct {
	StoryID            int64
	PersonIdentityID   int64
	PersonIdentityUUID string
	Provider           string
	ProviderUserID     *string
	Handle             *string
	AvatarURL          *string
	IdentityRef        string
	ArchivedAt         *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (r storyPersonIdentityRow) personIdentityRecord() PersonIdentityRecord {
	return PersonIdentityRecord{
		PersonIdentityID:   r.PersonIdentityID,
		PersonIdentityUUID: r.PersonIdentityUUID,
		Provider:           r.Provider,
		ProviderUserID:     r.ProviderUserID,
		Handle:             r.Handle,
		AvatarURL:          r.AvatarURL,
		IdentityRef:        r.IdentityRef,
		ArchivedAt:         r.ArchivedAt,
		CreatedAt:          r.CreatedAt,
		UpdatedAt:          r.UpdatedAt,
	}
}

func SortPersonIdentityRecords(records []PersonIdentityRecord) {
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].Provider != records[j].Provider {
			return records[i].Provider < records[j].Provider
		}
		return personIdentityDisplayValue(records[i]) < personIdentityDisplayValue(records[j])
	})
}

func personIdentityDisplayValue(record PersonIdentityRecord) string {
	if record.Handle != nil {
		return *record.Handle
	}
	if record.ProviderUserID != nil {
		return *record.ProviderUserID
	}
	return record.IdentityRef
}
