package db

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

func listArticleRelatedRecords[Row any, Record any](
	ctx context.Context,
	gdb *gorm.DB,
	articleUUIDs []string,
	selectExpr string,
	joins []string,
	orderExpr string,
	errorLabel string,
	key func(Row) string,
	record func(Row) Record,
) (map[string][]Record, error) {
	articleUUIDs = uniqueTrimmedStrings(articleUUIDs)
	if len(articleUUIDs) == 0 {
		return map[string][]Record{}, nil
	}
	var rows []Row
	uuidCondition, uuidArgs := uuidInCondition("a.article_uuid", articleUUIDs)
	query := gdb.WithContext(ctx).
		Table("news.articles AS a").
		Select(selectExpr).
		Where(uuidCondition, uuidArgs...).
		Where("a.deleted_at IS NULL").
		Order(orderExpr)
	for _, join := range joins {
		query = query.Joins(join)
	}
	if err := query.Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("%s: %w", errorLabel, err)
	}
	return groupRelatedRows(rows, key, record), nil
}

func listStoryRelatedRecords[Row any, Record any](
	ctx context.Context,
	gdb *gorm.DB,
	storyIDs []int64,
	selectExpr string,
	joins []string,
	orderExpr string,
	errorLabel string,
	key func(Row) int64,
	record func(Row) Record,
) (map[int64][]Record, error) {
	storyIDs = uniquePositiveInt64s(storyIDs)
	if len(storyIDs) == 0 {
		return map[int64][]Record{}, nil
	}
	var rows []Row
	query := gdb.WithContext(ctx).
		Table("news.story_articles AS sm").
		Distinct(selectExpr).
		Where("sm.story_id IN ?", storyIDs).
		Order(orderExpr)
	for _, join := range joins {
		query = query.Joins(join)
	}
	if err := query.Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("%s: %w", errorLabel, err)
	}
	return groupRelatedRows(rows, key, record), nil
}

func groupRelatedRows[Key comparable, Row any, Record any](
	rows []Row,
	key func(Row) Key,
	record func(Row) Record,
) map[Key][]Record {
	result := make(map[Key][]Record)
	for _, row := range rows {
		result[key(row)] = append(result[key(row)], record(row))
	}
	return result
}
