package db

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	TranslationModeEnabled  = "enabled"
	TranslationModeDisabled = "disabled"
)

// CollectionSettingsRow is the API/CLI-facing collection policy row.
type CollectionSettingsRow struct {
	Collection      string    `json:"collection"`
	TranslationMode string    `json:"translation_mode"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func DefaultCollectionTranslationMode(collection string) string {
	switch normalizeCollection(collection) {
	case "china_news", "metal_news":
		return TranslationModeEnabled
	default:
		return TranslationModeDisabled
	}
}

func NormalizeCollectionTranslationMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case TranslationModeEnabled:
		return TranslationModeEnabled
	default:
		return TranslationModeDisabled
	}
}

func IsCollectionTranslationEnabled(mode string) bool {
	return NormalizeCollectionTranslationMode(mode) == TranslationModeEnabled
}

func (p *Pool) GetCollectionTranslationMode(ctx context.Context, collection string) (string, error) {
	normalizedCollection := normalizeCollection(collection)
	if normalizedCollection == "" {
		return TranslationModeDisabled, nil
	}

	const q = `
SELECT translation_mode
FROM news.collection_settings
WHERE collection = $1
LIMIT 1
`

	var mode string
	if err := p.QueryRow(ctx, q, normalizedCollection).Scan(&mode); err != nil {
		if IsNoRows(err) {
			return DefaultCollectionTranslationMode(normalizedCollection), nil
		}
		return "", fmt.Errorf("query collection translation mode: %w", err)
	}
	return NormalizeCollectionTranslationMode(mode), nil
}

func (p *Pool) UpsertCollectionTranslationMode(
	ctx context.Context,
	collection string,
	translationMode string,
) (*CollectionSettingsRow, error) {
	normalizedCollection := normalizeCollection(collection)
	if normalizedCollection == "" {
		return nil, fmt.Errorf("collection is required")
	}
	mode := NormalizeCollectionTranslationMode(translationMode)

	const q = `
INSERT INTO news.collection_settings (
	collection,
	translation_mode,
	created_at,
	updated_at
)
VALUES ($1, $2, now(), now())
ON CONFLICT (collection)
DO UPDATE SET
	translation_mode = EXCLUDED.translation_mode,
	updated_at = now()
RETURNING collection, translation_mode, created_at, updated_at
`

	var row CollectionSettingsRow
	if err := p.QueryRow(ctx, q, normalizedCollection, mode).Scan(
		&row.Collection,
		&row.TranslationMode,
		&row.CreatedAt,
		&row.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("upsert collection translation mode: %w", err)
	}
	row.TranslationMode = NormalizeCollectionTranslationMode(row.TranslationMode)
	return &row, nil
}
