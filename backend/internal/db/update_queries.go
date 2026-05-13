package db

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash/fnv"
	"strings"
	"time"
	"unicode"

	textnormalize "horse.fit/scoop/internal/normalize"
)

type UpdateStoryOptions struct {
	Title      *string
	Status     *string
	Collection *string
	URL        *string
}

type UpdateArticleOptions struct {
	Title      *string
	Source     *string
	Collection *string
	URL        *string
}

func (p *Pool) UpdateStory(ctx context.Context, storyUUID string, opts UpdateStoryOptions, now time.Time) error {
	trimmedUUID := strings.TrimSpace(storyUUID)
	if trimmedUUID == "" {
		return fmt.Errorf("story UUID is required")
	}
	if opts.Title == nil && opts.Status == nil && opts.Collection == nil && opts.URL == nil {
		return fmt.Errorf("at least one update field is required")
	}

	var (
		title      *string
		status     *string
		collection *string
		urlValue   *string
		urlHash    []byte
	)

	if opts.Title != nil {
		trimmed := strings.TrimSpace(*opts.Title)
		if trimmed == "" {
			return fmt.Errorf("title must not be empty")
		}
		title = &trimmed
	}

	if opts.Status != nil {
		trimmed := strings.TrimSpace(strings.ToLower(*opts.Status))
		if trimmed == "" {
			return fmt.Errorf("status must not be empty")
		}
		status = &trimmed
	}

	if opts.Collection != nil {
		normalized := normalizeCollection(*opts.Collection)
		if normalized == "" {
			return fmt.Errorf("collection must not be empty")
		}
		collection = &normalized
	}

	if opts.URL != nil {
		trimmed := strings.TrimSpace(*opts.URL)
		if trimmed == "" {
			return fmt.Errorf("url must not be empty")
		}
		canonical, _ := textnormalize.URL(trimmed)
		if canonical == "" {
			return fmt.Errorf("url must be a fully-qualified URL")
		}
		urlValue = &canonical
		hash := sha256.Sum256([]byte(canonical))
		urlHash = append([]byte(nil), hash[:]...)
	}

	set := make([]string, 0, 8)
	args := make([]any, 0, 8)
	args = append(args, trimmedUUID)
	argPos := 2

	if title != nil {
		set = append(set, fmt.Sprintf("canonical_title = $%d", argPos))
		args = append(args, *title)
		argPos++
	}
	if status != nil {
		set = append(set, fmt.Sprintf("status = $%d", argPos))
		args = append(args, *status)
		argPos++
	}
	if collection != nil {
		set = append(set, fmt.Sprintf("collection = $%d", argPos))
		args = append(args, *collection)
		argPos++
	}
	if urlValue != nil {
		set = append(set, fmt.Sprintf("canonical_url = $%d", argPos))
		args = append(args, *urlValue)
		argPos++

		set = append(set, fmt.Sprintf("canonical_url_hash = $%d", argPos))
		args = append(args, urlHash)
		argPos++
	}

	set = append(set, fmt.Sprintf("updated_at = $%d", argPos))
	args = append(args, now.UTC())

	tx, err := p.BeginTx(ctx, TxOptions{})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	q := fmt.Sprintf(`
UPDATE news.stories
SET
	%s
WHERE story_uuid = $1::uuid
  AND deleted_at IS NULL
`, strings.Join(set, ",\n\t"))

	tag, err := tx.Exec(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("update story: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoRows
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func (p *Pool) UpdateArticle(ctx context.Context, articleUUID string, opts UpdateArticleOptions, now time.Time) error {
	trimmedUUID := strings.TrimSpace(articleUUID)
	if trimmedUUID == "" {
		return fmt.Errorf("article UUID is required")
	}
	if opts.Title == nil && opts.Source == nil && opts.Collection == nil && opts.URL == nil {
		return fmt.Errorf("at least one update field is required")
	}

	var (
		normalizedTitle *string
		titleHash       []byte
		contentHash     []byte
		titleSimhash    *int64
		tokenCount      *int

		source     *string
		collection *string

		canonicalURL     *string
		canonicalURLHash []byte
		sourceDomain     *string
	)

	if opts.Source != nil {
		trimmed := strings.TrimSpace(*opts.Source)
		if trimmed == "" {
			return fmt.Errorf("source must not be empty")
		}
		source = &trimmed
	}

	if opts.Collection != nil {
		normalized := normalizeCollection(*opts.Collection)
		if normalized == "" {
			return fmt.Errorf("collection must not be empty")
		}
		collection = &normalized
	}

	if opts.URL != nil {
		trimmed := strings.TrimSpace(*opts.URL)
		if trimmed == "" {
			return fmt.Errorf("url must not be empty")
		}
		normalized, host := textnormalize.URL(trimmed)
		if normalized == "" {
			return fmt.Errorf("url must be a fully-qualified URL")
		}
		canonicalURL = &normalized
		hash := sha256.Sum256([]byte(normalized))
		canonicalURLHash = append([]byte(nil), hash[:]...)
		if strings.TrimSpace(host) != "" {
			hostCopy := strings.TrimSpace(strings.ToLower(host))
			sourceDomain = &hostCopy
		}
	}

	tx, err := p.BeginTx(ctx, TxOptions{})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var existingNormalizedText string
	const lockQuery = `
SELECT normalized_text
FROM news.articles
WHERE article_uuid = $1::uuid
  AND deleted_at IS NULL
FOR UPDATE
`
	if err := tx.QueryRow(ctx, lockQuery, trimmedUUID).Scan(&existingNormalizedText); err != nil {
		if errors.Is(err, ErrNoRows) {
			return ErrNoRows
		}
		return fmt.Errorf("lock article: %w", err)
	}

	if opts.Title != nil {
		raw := strings.TrimSpace(*opts.Title)
		if raw == "" {
			return fmt.Errorf("title must not be empty")
		}
		normalized := textnormalize.Text(raw)
		if normalized == "" {
			return fmt.Errorf("title must not be empty")
		}
		normalizedTitle = &normalized

		th := sha256.Sum256([]byte(normalized))
		titleHash = append([]byte(nil), th[:]...)

		body := strings.TrimSpace(existingNormalizedText)
		ch := sha256.Sum256([]byte(normalized + "\n" + body))
		contentHash = append([]byte(nil), ch[:]...)

		if value, ok := simhash64(normalized); ok {
			v := int64(value)
			titleSimhash = &v
		} else {
			titleSimhash = nil
		}

		count := countTokens(normalized + " " + body)
		tokenCount = &count
	}

	set := make([]string, 0, 16)
	args := make([]any, 0, 16)
	args = append(args, trimmedUUID)
	argPos := 2

	if normalizedTitle != nil {
		set = append(set, fmt.Sprintf("normalized_title = $%d", argPos))
		args = append(args, *normalizedTitle)
		argPos++

		set = append(set, fmt.Sprintf("title_hash = $%d", argPos))
		args = append(args, titleHash)
		argPos++

		set = append(set, fmt.Sprintf("content_hash = $%d", argPos))
		args = append(args, contentHash)
		argPos++

		set = append(set, fmt.Sprintf("title_simhash = $%d", argPos))
		args = append(args, titleSimhash)
		argPos++

		set = append(set, fmt.Sprintf("token_count = $%d", argPos))
		args = append(args, *tokenCount)
		argPos++
	}

	if source != nil {
		set = append(set, fmt.Sprintf("source = $%d", argPos))
		args = append(args, *source)
		argPos++
	}

	if collection != nil {
		set = append(set, fmt.Sprintf("collection = $%d", argPos))
		args = append(args, *collection)
		argPos++
	}

	if canonicalURL != nil {
		set = append(set, fmt.Sprintf("canonical_url = $%d", argPos))
		args = append(args, *canonicalURL)
		argPos++

		set = append(set, fmt.Sprintf("canonical_url_hash = $%d", argPos))
		args = append(args, canonicalURLHash)
		argPos++

		if sourceDomain != nil {
			set = append(set, fmt.Sprintf("source_domain = $%d", argPos))
			args = append(args, *sourceDomain)
			argPos++
		}
	}

	set = append(set, fmt.Sprintf("updated_at = $%d", argPos))
	args = append(args, now.UTC())

	q := fmt.Sprintf(`
UPDATE news.articles
SET
	%s
WHERE article_uuid = $1::uuid
  AND deleted_at IS NULL
`, strings.Join(set, ",\n\t"))

	tag, err := tx.Exec(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("update article: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoRows
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func countTokens(text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	return len(strings.Fields(text))
}

func simhash64(text string) (uint64, bool) {
	tokens := tokenize(text)
	if len(tokens) == 0 {
		return 0, false
	}

	var bitWeights [64]int
	for _, token := range tokens {
		h := hashToken64(token)
		for bit := range 64 {
			mask := uint64(1) << bit
			if h&mask != 0 {
				bitWeights[bit]++
			} else {
				bitWeights[bit]--
			}
		}
	}

	var result uint64
	for bit := range 64 {
		if bitWeights[bit] > 0 {
			result |= uint64(1) << bit
		}
	}
	return result, true
}

func tokenize(text string) []string {
	normalized := textnormalize.Text(text)
	if normalized == "" {
		return nil
	}

	parts := strings.FieldsFunc(normalized, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	tokens := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		tokens = append(tokens, p)
	}
	return tokens
}

func hashToken64(token string) uint64 {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(token))
	return hasher.Sum64()
}
