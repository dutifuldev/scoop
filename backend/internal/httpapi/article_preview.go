package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/labstack/echo/v4"

	"horse.fit/scoop/internal/db"
	"horse.fit/scoop/internal/reader"
)

const (
	defaultArticlePreviewMaxChars = 1000
	minArticlePreviewMaxChars     = 200
	maxArticlePreviewMaxChars     = 4000
)

var errStoryArticleNotFound = errors.New("story article not found")

type storyArticlePreview struct {
	StoryArticleUUID string  `json:"story_article_uuid"`
	PreviewText      string  `json:"preview_text"`
	Source           string  `json:"source"`
	CharCount        int     `json:"char_count"`
	Truncated        bool    `json:"truncated"`
	PreviewError     *string `json:"preview_error,omitempty"`
}

func (s *Server) handleStoryArticlePreview(c echo.Context) error {
	storyArticleUUID := strings.TrimSpace(c.Param("story_article_uuid"))
	if storyArticleUUID == "" {
		return failValidation(c, map[string]string{"story_article_uuid": "is required"})
	}

	maxChars, err := parsePositiveInt(
		c.QueryParam("max_chars"),
		defaultArticlePreviewMaxChars,
		minArticlePreviewMaxChars,
		maxArticlePreviewMaxChars,
	)
	if err != nil {
		return failValidation(c, map[string]string{"max_chars": err.Error()})
	}

	preview, err := s.queryStoryArticlePreview(c.Request().Context(), storyArticleUUID, maxChars)
	if err != nil {
		if errors.Is(err, errStoryArticleNotFound) {
			return failNotFound(c, "Story article not found")
		}
		s.logger.Error().Err(err).Str("story_article_uuid", storyArticleUUID).Msg("query story article preview failed")
		return internalError(c, "Failed to load story article preview")
	}

	return success(c, preview)
}

func (s *Server) queryStoryArticlePreview(ctx context.Context, storyArticleUUID string, maxChars int) (*storyArticlePreview, error) {
	const q = `
SELECT
	sm.story_article_uuid::text,
	d.normalized_text,
	d.canonical_url,
	d.normalized_title,
	d.source
FROM news.story_articles sm
JOIN news.articles d
	ON d.article_id = sm.article_id
	AND d.deleted_at IS NULL
WHERE sm.story_article_uuid = $1::uuid
LIMIT 1
`

	var (
		rowStoryArticleUUID string
		rowNormalizedText   *string
		rowCanonicalURL     *string
		rowTitle            string
		rowSource           string
	)

	if err := s.pool.QueryRow(ctx, q, storyArticleUUID).Scan(
		&rowStoryArticleUUID,
		&rowNormalizedText,
		&rowCanonicalURL,
		&rowTitle,
		&rowSource,
	); err != nil {
		if errors.Is(err, db.ErrNoRows) {
			return nil, errStoryArticleNotFound
		}
		return nil, fmt.Errorf("query story article preview row: %w", err)
	}

	normalizedText := ""
	if rowNormalizedText != nil {
		normalizedText = strings.TrimSpace(*rowNormalizedText)
	}

	previewRaw, source, previewErr := buildArticlePreviewText(ctx, rowCanonicalURL, rowTitle, normalizedText, rowSource)
	previewText, truncated := truncatePreviewText(previewRaw, maxChars)

	resp := &storyArticlePreview{
		StoryArticleUUID: rowStoryArticleUUID,
		PreviewText:      previewText,
		Source:           source,
		CharCount:        utf8.RuneCountInString(previewText),
		Truncated:        truncated,
	}
	if previewErr != nil {
		msg := previewErr.Error()
		resp.PreviewError = &msg
		s.logger.Warn().
			Err(previewErr).
			Str("story_article_uuid", storyArticleUUID).
			Str("source", source).
			Msg("reader preview fallback used")
	}

	return resp, nil
}

func buildArticlePreviewText(
	ctx context.Context,
	canonicalURL *string,
	normalizedTitle string,
	normalizedText string,
	sourceName string,
) (string, string, error) {
	if strings.EqualFold(strings.TrimSpace(sourceName), "discord_archive") && normalizedText != "" {
		return normalizedText, "normalized_text", nil
	}

	url := strings.TrimSpace(derefString(canonicalURL))
	if url != "" {
		readerText, err := fetchReaderPreviewText(ctx, url, normalizedTitle)
		if err == nil && strings.TrimSpace(readerText) != "" {
			return readerText, "reader", nil
		}
		if normalizedText != "" {
			return normalizedText, "normalized_text", err
		}
		return "", "none", err
	}

	if normalizedText != "" {
		return normalizedText, "normalized_text", nil
	}

	return "", "none", nil
}

func fetchReaderPreviewText(ctx context.Context, canonicalURL string, title string) (string, error) {
	return reader.FetchText(ctx, canonicalURL, title)
}

func truncatePreviewText(raw string, maxChars int) (string, bool) {
	limit := maxChars
	if limit <= 0 {
		limit = defaultArticlePreviewMaxChars
	}
	return reader.TruncateText(raw, limit)
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
