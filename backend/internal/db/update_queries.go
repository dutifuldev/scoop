package db

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	textnormalize "horse.fit/scoop/internal/normalize"
	"horse.fit/scoop/internal/textmetrics"
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

type storyUpdateFields struct {
	title      *string
	status     *string
	collection *string
	urlValue   *string
	urlHash    []byte
}

type articleStaticUpdateFields struct {
	source           *string
	collection       *string
	canonicalURL     *string
	canonicalURLHash []byte
	sourceDomain     *string
}

type articleTitleUpdateFields struct {
	normalizedTitle *string
	titleHash       []byte
	contentHash     []byte
	titleSimhash    *int64
	tokenCount      *int
}

type sqlUpdatePlan struct {
	set  []string
	args []any
}

type sqlAssignment struct {
	column string
	value  any
}

func (p *Pool) UpdateStory(ctx context.Context, storyUUID string, opts UpdateStoryOptions, now time.Time) error {
	trimmedUUID := strings.TrimSpace(storyUUID)
	if trimmedUUID == "" {
		return fmt.Errorf("story UUID is required")
	}
	plan, err := buildStoryUpdatePlan(trimmedUUID, opts, now)
	if err != nil {
		return err
	}

	q := fmt.Sprintf(`
	UPDATE news.stories
	SET
		%s
	WHERE story_uuid = $1::uuid
	  AND deleted_at IS NULL
	`, strings.Join(plan.set, ",\n\t"))

	return p.executeSingleRowUpdate(ctx, q, plan.args, "update story")
}

func (p *Pool) executeSingleRowUpdate(ctx context.Context, query string, args []any, label string) error {
	tx, err := p.BeginTx(ctx, TxOptions{})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	tag, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
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
	staticFields, err := normalizeArticleStaticUpdateFields(opts)
	if err != nil {
		return err
	}
	return p.updateArticle(ctx, trimmedUUID, opts, staticFields, now)
}

func (p *Pool) updateArticle(
	ctx context.Context,
	articleUUID string,
	opts UpdateArticleOptions,
	staticFields articleStaticUpdateFields,
	now time.Time,
) error {
	tx, err := p.BeginTx(ctx, TxOptions{})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	existingNormalizedText, err := lockArticleTextTx(ctx, tx, articleUUID)
	if err != nil {
		return err
	}
	plan, err := buildArticleUpdatePlan(articleUUID, opts, staticFields, existingNormalizedText, now)
	if err != nil {
		return err
	}

	q := fmt.Sprintf(`
	UPDATE news.articles
	SET
		%s
	WHERE article_uuid = $1::uuid
	  AND deleted_at IS NULL
	`, strings.Join(plan.set, ",\n\t"))

	tag, err := tx.Exec(ctx, q, plan.args...)
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

func lockArticleTextTx(ctx context.Context, tx Tx, articleUUID string) (string, error) {
	const q = `
	SELECT normalized_text
	FROM news.articles
	WHERE article_uuid = $1::uuid
	  AND deleted_at IS NULL
	FOR UPDATE
	`
	var normalizedText string
	if err := tx.QueryRow(ctx, q, articleUUID).Scan(&normalizedText); err != nil {
		if errors.Is(err, ErrNoRows) {
			return "", ErrNoRows
		}
		return "", fmt.Errorf("lock article: %w", err)
	}
	return normalizedText, nil
}

func buildStoryUpdatePlan(storyUUID string, opts UpdateStoryOptions, now time.Time) (sqlUpdatePlan, error) {
	fields, err := normalizeStoryUpdateFields(opts)
	if err != nil {
		return sqlUpdatePlan{}, err
	}
	assignments := []sqlAssignment{}
	assignments = appendPointerAssignment(assignments, "canonical_title", fields.title)
	assignments = appendPointerAssignment(assignments, "status", fields.status)
	assignments = appendPointerAssignment(assignments, "collection", fields.collection)
	assignments = appendPointerAssignment(assignments, "canonical_url", fields.urlValue)
	assignments = appendBytesAssignment(assignments, "canonical_url_hash", fields.urlValue, fields.urlHash)
	return newSQLUpdatePlan(storyUUID, now, assignments), nil
}

func normalizeStoryUpdateFields(opts UpdateStoryOptions) (storyUpdateFields, error) {
	if !storyUpdateHasFields(opts) {
		return storyUpdateFields{}, fmt.Errorf("at least one update field is required")
	}
	fields := storyUpdateFields{}
	if err := setTrimmedString(&fields.title, opts.Title, "title must not be empty"); err != nil {
		return storyUpdateFields{}, err
	}
	if err := setLowerTrimmedString(&fields.status, opts.Status, "status must not be empty"); err != nil {
		return storyUpdateFields{}, err
	}
	if err := setNormalizedCollection(&fields.collection, opts.Collection); err != nil {
		return storyUpdateFields{}, err
	}
	if err := setCanonicalURL(&fields.urlValue, &fields.urlHash, nil, opts.URL); err != nil {
		return storyUpdateFields{}, err
	}
	return fields, nil
}

func storyUpdateHasFields(opts UpdateStoryOptions) bool {
	return opts.Title != nil || opts.Status != nil || opts.Collection != nil || opts.URL != nil
}

func normalizeArticleStaticUpdateFields(opts UpdateArticleOptions) (articleStaticUpdateFields, error) {
	fields := articleStaticUpdateFields{}
	if err := setTrimmedString(&fields.source, opts.Source, "source must not be empty"); err != nil {
		return articleStaticUpdateFields{}, err
	}
	if err := setNormalizedCollection(&fields.collection, opts.Collection); err != nil {
		return articleStaticUpdateFields{}, err
	}
	if err := setCanonicalURL(&fields.canonicalURL, &fields.canonicalURLHash, &fields.sourceDomain, opts.URL); err != nil {
		return articleStaticUpdateFields{}, err
	}
	return fields, nil
}

func buildArticleUpdatePlan(
	articleUUID string,
	opts UpdateArticleOptions,
	staticFields articleStaticUpdateFields,
	existingNormalizedText string,
	now time.Time,
) (sqlUpdatePlan, error) {
	titleFields, err := normalizeArticleTitleUpdateFields(opts.Title, existingNormalizedText)
	if err != nil {
		return sqlUpdatePlan{}, err
	}
	assignments := articleTitleAssignments(titleFields)
	assignments = appendPointerAssignment(assignments, "source", staticFields.source)
	assignments = appendPointerAssignment(assignments, "collection", staticFields.collection)
	assignments = appendPointerAssignment(assignments, "canonical_url", staticFields.canonicalURL)
	assignments = appendBytesAssignment(assignments, "canonical_url_hash", staticFields.canonicalURL, staticFields.canonicalURLHash)
	assignments = appendPointerAssignment(assignments, "source_domain", staticFields.sourceDomain)
	return newSQLUpdatePlan(articleUUID, now, assignments), nil
}

func normalizeArticleTitleUpdateFields(rawTitle *string, existingNormalizedText string) (articleTitleUpdateFields, error) {
	if rawTitle == nil {
		return articleTitleUpdateFields{}, nil
	}
	normalized := textnormalize.Text(*rawTitle)
	if normalized == "" {
		return articleTitleUpdateFields{}, fmt.Errorf("title must not be empty")
	}
	body := strings.TrimSpace(existingNormalizedText)
	titleHash := sha256.Sum256([]byte(normalized))
	contentHash := sha256.Sum256([]byte(normalized + "\n" + body))
	count := textmetrics.CountTokens(normalized + " " + body)
	fields := articleTitleUpdateFields{
		normalizedTitle: &normalized,
		titleHash:       append([]byte(nil), titleHash[:]...),
		contentHash:     append([]byte(nil), contentHash[:]...),
		tokenCount:      &count,
	}
	if value, ok := textmetrics.Simhash64(normalized); ok {
		simhash := int64(value)
		fields.titleSimhash = &simhash
	}
	return fields, nil
}

func articleTitleAssignments(fields articleTitleUpdateFields) []sqlAssignment {
	if fields.normalizedTitle == nil {
		return nil
	}
	assignments := []sqlAssignment{}
	assignments = appendPointerAssignment(assignments, "normalized_title", fields.normalizedTitle)
	assignments = appendAssignment(assignments, "title_hash", fields.titleHash)
	assignments = appendAssignment(assignments, "content_hash", fields.contentHash)
	assignments = appendAssignment(assignments, "title_simhash", fields.titleSimhash)
	assignments = appendPointerAssignment(assignments, "token_count", fields.tokenCount)
	return assignments
}

func setTrimmedString(dest **string, source *string, emptyMessage string) error {
	if source == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*source)
	if trimmed == "" {
		return fmt.Errorf("%s", emptyMessage)
	}
	*dest = &trimmed
	return nil
}

func setLowerTrimmedString(dest **string, source *string, emptyMessage string) error {
	if source == nil {
		return nil
	}
	trimmed := strings.TrimSpace(strings.ToLower(*source))
	if trimmed == "" {
		return fmt.Errorf("%s", emptyMessage)
	}
	*dest = &trimmed
	return nil
}

func setNormalizedCollection(dest **string, source *string) error {
	if source == nil {
		return nil
	}
	normalized := normalizeCollection(*source)
	if normalized == "" {
		return fmt.Errorf("collection must not be empty")
	}
	*dest = &normalized
	return nil
}

func setCanonicalURL(dest **string, hashDest *[]byte, hostDest **string, source *string) error {
	if source == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*source)
	if trimmed == "" {
		return fmt.Errorf("url must not be empty")
	}
	normalized, host := textnormalize.URL(trimmed)
	if normalized == "" {
		return fmt.Errorf("url must be a fully-qualified URL")
	}
	*dest = &normalized
	hash := sha256.Sum256([]byte(normalized))
	*hashDest = append([]byte(nil), hash[:]...)
	setURLHost(hostDest, host)
	return nil
}

func setURLHost(dest **string, host string) {
	if dest == nil {
		return
	}
	trimmed := strings.TrimSpace(strings.ToLower(host))
	if trimmed == "" {
		return
	}
	*dest = &trimmed
}

func appendAssignment(assignments []sqlAssignment, column string, value any) []sqlAssignment {
	return append(assignments, sqlAssignment{column: column, value: value})
}

func appendPointerAssignment[T any](assignments []sqlAssignment, column string, value *T) []sqlAssignment {
	if value == nil {
		return assignments
	}
	return appendAssignment(assignments, column, *value)
}

func appendBytesAssignment(assignments []sqlAssignment, column string, gate *string, value []byte) []sqlAssignment {
	if gate == nil {
		return assignments
	}
	return appendAssignment(assignments, column, value)
}

func newSQLUpdatePlan(rowID string, now time.Time, assignments []sqlAssignment) sqlUpdatePlan {
	set := make([]string, 0, len(assignments)+1)
	args := make([]any, 0, len(assignments)+2)
	args = append(args, rowID)
	for _, assignment := range assignments {
		set = append(set, fmt.Sprintf("%s = $%d", assignment.column, len(args)+1))
		args = append(args, assignment.value)
	}
	set = append(set, fmt.Sprintf("updated_at = $%d", len(args)+1))
	args = append(args, now.UTC())
	return sqlUpdatePlan{set: set, args: args}
}
