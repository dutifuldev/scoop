package db

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type SoftDeleteCollectionResult struct {
	RawArrivals int64
	Articles    int64
	Stories     int64
}

type SoftDeleteBeforeResult struct {
	RawArrivals int64
	Articles    int64
	Stories     int64
}

type softDeleteCount struct {
	rawArrivals int64
	articles    int64
	stories     int64
}

type softDeleteMutation struct {
	name  string
	query string
	args  []any
}

func (p *Pool) SoftDeleteStory(ctx context.Context, storyUUID string, now time.Time) (int64, error) {
	return p.setDeletedStateByUUID(ctx, storyDeletedStateMutation(storyUUID, true, "soft delete story", now))
}

func (p *Pool) SoftDeleteArticle(ctx context.Context, articleUUID string, now time.Time) (int64, error) {
	return p.setDeletedStateByUUID(ctx, articleDeletedStateMutation(articleUUID, true, "soft delete article", now))
}

func (p *Pool) SoftDeleteCollection(ctx context.Context, collection string, now time.Time) (SoftDeleteCollectionResult, error) {
	normalizedCollection := strings.TrimSpace(strings.ToLower(collection))
	if normalizedCollection == "" {
		return SoftDeleteCollectionResult{}, fmt.Errorf("collection is required")
	}
	counts, err := p.runSoftDeleteMutations(ctx, collectionDeleteMutations(normalizedCollection, now.UTC()))
	if err != nil {
		return SoftDeleteCollectionResult{}, err
	}
	return SoftDeleteCollectionResult{
		RawArrivals: counts.rawArrivals,
		Articles:    counts.articles,
		Stories:     counts.stories,
	}, nil
}

func (p *Pool) runSoftDeleteMutations(ctx context.Context, mutations []softDeleteMutation) (softDeleteCount, error) {
	tx, err := p.BeginTx(ctx, TxOptions{})
	if err != nil {
		return softDeleteCount{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	counts, err := execSoftDeleteMutations(ctx, tx, mutations)
	if err != nil {
		return softDeleteCount{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return softDeleteCount{}, fmt.Errorf("commit transaction: %w", err)
	}
	return counts, nil
}

func execSoftDeleteMutations(ctx context.Context, tx Tx, mutations []softDeleteMutation) (softDeleteCount, error) {
	var counts softDeleteCount
	for _, mutation := range mutations {
		tag, err := tx.Exec(ctx, mutation.query, mutation.args...)
		if err != nil {
			return softDeleteCount{}, fmt.Errorf("%s: %w", mutation.name, err)
		}
		applySoftDeleteCount(&counts, mutation.name, tag.RowsAffected())
	}
	return counts, nil
}

func applySoftDeleteCount(counts *softDeleteCount, name string, rowsAffected int64) {
	switch name {
	case "raw_arrivals":
		counts.rawArrivals = rowsAffected
	case "articles":
		counts.articles = rowsAffected
	case "stories":
		counts.stories = rowsAffected
	}
}

func collectionDeleteMutations(collection string, now time.Time) []softDeleteMutation {
	const rawArrivalsQuery = `
UPDATE news.raw_arrivals
SET deleted_at = $2
WHERE collection = $1
  AND deleted_at IS NULL
`
	const articlesQuery = `
UPDATE news.articles
SET
	deleted_at = $2,
	updated_at = $2
WHERE collection = $1
  AND deleted_at IS NULL
`
	const storiesQuery = `
UPDATE news.stories
SET
	deleted_at = $2,
	updated_at = $2
WHERE collection = $1
  AND deleted_at IS NULL
`
	return []softDeleteMutation{
		{name: "raw_arrivals", query: rawArrivalsQuery, args: []any{collection, now}},
		{name: "articles", query: articlesQuery, args: []any{collection, now}},
		{name: "stories", query: storiesQuery, args: []any{collection, now}},
	}
}

func (p *Pool) SoftDeleteBefore(ctx context.Context, before time.Time, now time.Time) (SoftDeleteBeforeResult, error) {
	beforeUTC := before.UTC()
	if beforeUTC.IsZero() {
		return SoftDeleteBeforeResult{}, fmt.Errorf("before time is required")
	}
	counts, err := p.runSoftDeleteMutations(ctx, beforeDeleteMutations(beforeUTC, now.UTC()))
	if err != nil {
		return SoftDeleteBeforeResult{}, err
	}
	return SoftDeleteBeforeResult{
		RawArrivals: counts.rawArrivals,
		Articles:    counts.articles,
		Stories:     counts.stories,
	}, nil
}

func beforeDeleteMutations(beforeUTC time.Time, now time.Time) []softDeleteMutation {
	const rawArrivalsQuery = `
UPDATE news.raw_arrivals
SET deleted_at = $2
WHERE fetched_at < $1
  AND deleted_at IS NULL
`
	const articlesQuery = `
UPDATE news.articles
SET
	deleted_at = $2,
	updated_at = $2
WHERE created_at < $1
  AND deleted_at IS NULL
`
	const storiesQuery = `
UPDATE news.stories
SET
	deleted_at = $2,
	updated_at = $2
WHERE last_seen_at < $1
  AND deleted_at IS NULL
`
	return []softDeleteMutation{
		{name: "raw_arrivals", query: rawArrivalsQuery, args: []any{beforeUTC, now}},
		{name: "articles", query: articlesQuery, args: []any{beforeUTC, now}},
		{name: "stories", query: storiesQuery, args: []any{beforeUTC, now}},
	}
}

func (p *Pool) RestoreStory(ctx context.Context, storyUUID string, now time.Time) (int64, error) {
	return p.setDeletedStateByUUID(ctx, storyDeletedStateMutation(storyUUID, false, "restore story", now))
}

func (p *Pool) RestoreArticle(ctx context.Context, articleUUID string, now time.Time) (int64, error) {
	return p.setDeletedStateByUUID(ctx, articleDeletedStateMutation(articleUUID, false, "restore article", now))
}

func storyDeletedStateMutation(storyUUID string, deleted bool, errorLabel string, now time.Time) deletedStateMutation {
	return newDeletedStateMutation("news.stories", "story_uuid", storyUUID, "story UUID is required", deleted, errorLabel, now)
}

func articleDeletedStateMutation(articleUUID string, deleted bool, errorLabel string, now time.Time) deletedStateMutation {
	return newDeletedStateMutation("news.articles", "article_uuid", articleUUID, "article UUID is required", deleted, errorLabel, now)
}

func newDeletedStateMutation(
	tableName string,
	uuidColumn string,
	uuidValue string,
	uuidRequiredMsg string,
	deleted bool,
	errorLabel string,
	now time.Time,
) deletedStateMutation {
	return deletedStateMutation{
		tableName:       tableName,
		uuidColumn:      uuidColumn,
		uuidValue:       uuidValue,
		uuidRequiredMsg: uuidRequiredMsg,
		deleted:         deleted,
		errorLabel:      errorLabel,
		now:             now,
	}
}

type deletedStateMutation struct {
	tableName       string
	uuidColumn      string
	uuidValue       string
	uuidRequiredMsg string
	deleted         bool
	errorLabel      string
	now             time.Time
}

func (p *Pool) setDeletedStateByUUID(ctx context.Context, mutation deletedStateMutation) (int64, error) {
	trimmedUUID := strings.TrimSpace(mutation.uuidValue)
	if trimmedUUID == "" {
		return 0, fmt.Errorf("%s", mutation.uuidRequiredMsg)
	}

	tx, err := p.BeginTx(ctx, TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	deletedAssignment := "deleted_at = NULL"
	deletedPredicate := "deleted_at IS NOT NULL"
	if mutation.deleted {
		deletedAssignment = "deleted_at = $2"
		deletedPredicate = "deleted_at IS NULL"
	}
	q := fmt.Sprintf(`
	UPDATE %s
	SET
		%s,
		updated_at = $2
	WHERE %s = $1::uuid
	  AND %s
	`, mutation.tableName, deletedAssignment, mutation.uuidColumn, deletedPredicate)
	tag, err := tx.Exec(ctx, q, trimmedUUID, mutation.now.UTC())
	if err != nil {
		return 0, fmt.Errorf("%s: %w", mutation.errorLabel, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}

	return tag.RowsAffected(), nil
}
