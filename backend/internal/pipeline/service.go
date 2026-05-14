package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"math/bits"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"horse.fit/scoop/internal/db"
	"horse.fit/scoop/internal/globaltime"
	"horse.fit/scoop/internal/langdetect"
	textnormalize "horse.fit/scoop/internal/normalize"
	"horse.fit/scoop/internal/textmetrics"
	payloadschema "horse.fit/scoop/schema"
)

const (
	DefaultDedupLookbackDays         = 365
	defaultSemanticCandidateLimit    = 20
	defaultSemanticSearchEF          = 64
	defaultLexicalSimhashMaxDistance = 3
	defaultLexicalTrigramThreshold   = 0.88
	defaultLexicalTrigramDateWindow  = 14 * 24 * time.Hour
	defaultSemanticAutoMergeCosine   = 0.935
	defaultSemanticOverrideCosine    = 0.965
	defaultSemanticTitleOverlapFloor = 0.30
	defaultSemanticGrayZoneMinCosine = 0.89
	semanticCompositeCosineWeight    = 0.75
	semanticCompositeTitleWeight     = 0.15
	semanticCompositeDateWeight      = 0.10
	storyCandidateLimit              = 300
)

type Service struct {
	pool   *db.Pool
	logger zerolog.Logger
}

type NormalizeResult struct {
	Processed int
	Inserted  int
}

type DedupResult struct {
	Processed  int
	NewStories int
	AutoMerges int
	GrayZones  int
}

type DedupOptions struct {
	Limit        int
	ModelName    string
	ModelVersion string
	LookbackDays int
}

type dedupRunConfig struct {
	limit          int
	modelName      string
	modelVersion   string
	lookbackCutoff time.Time
}

type rawArrivalRow struct {
	RawArrivalID      int64
	Source            string
	SourceItemID      string
	Collection        string
	SourceItemURL     *string
	SourcePublishedAt *time.Time
	RawPayload        []byte
	FetchedAt         time.Time
}

type normalizedArticle struct {
	RawArrivalID     int64
	Source           string
	SourceItemID     string
	Collection       string
	CanonicalURL     *string
	CanonicalURLHash []byte
	NormalizedTitle  string
	NormalizedText   string
	NormalizedLang   string
	PublishedAt      *time.Time
	SourceDomain     *string
	TitleSimhash     *int64
	TextSimhash      *int64
	TitleHash        []byte
	ContentHash      []byte
	TokenCount       int
	ArticleCreatedAt time.Time
}

type pendingArticle struct {
	ArticleID        int64
	Source           string
	SourceItemID     string
	Collection       string
	CanonicalURL     *string
	CanonicalURLHash []byte
	NormalizedTitle  string
	NormalizedText   string
	PublishedAt      *time.Time
	SourceDomain     *string
	TitleSimhash     *int64
	ContentHash      []byte
	EmbeddingVector  *string
	ArticleCreatedAt time.Time
}

type storyCandidate struct {
	StoryID      int64
	Title        string
	LastSeenAt   time.Time
	SourceCount  int
	ArticleCount int
	CanonicalURL *string
	TitleSimhash *int64
}

type semanticCandidate struct {
	StoryID    int64
	Title      string
	LastSeenAt time.Time
	Cosine     float64
}

type lexicalAutoMergeMatch struct {
	Candidate       storyCandidate
	Signal          string
	MatchScore      float64
	TitleOverlap    float64
	DateConsistency float64
	CompositeScore  float64
	SimhashDistance *int
}

type semanticMatch struct {
	Candidate       semanticCandidate
	TitleOverlap    float64
	DateConsistency float64
	CompositeScore  float64
}

type dedupDecisionKind string

const (
	decisionNone      dedupDecisionKind = ""
	decisionNewStory  dedupDecisionKind = "new_story"
	decisionAutoMerge dedupDecisionKind = "auto_merge"
	decisionGrayZone  dedupDecisionKind = "gray_zone"
)

const exactURLStoryQuery = `
SELECT story_id
FROM news.stories
WHERE deleted_at IS NULL
  AND status = 'active'
  AND collection = $1
  AND canonical_url_hash = $2
ORDER BY last_seen_at DESC
LIMIT 1
`

const exactContentHashStoryQuery = `
SELECT sm.story_id
FROM news.story_articles sm
JOIN news.articles d ON d.article_id = sm.article_id AND d.deleted_at IS NULL
JOIN news.stories s ON s.story_id = sm.story_id AND s.deleted_at IS NULL
WHERE s.status = 'active'
  AND s.collection = $1
  AND d.collection = $1
  AND d.content_hash = $2
ORDER BY sm.matched_at DESC
LIMIT 1
`

func NewService(pool *db.Pool, logger zerolog.Logger) *Service {
	return &Service{
		pool:   pool,
		logger: logger,
	}
}

func (s *Service) NormalizePending(ctx context.Context, limit int) (NormalizeResult, error) {
	if s == nil || s.pool == nil {
		return NormalizeResult{}, fmt.Errorf("pipeline service is not initialized")
	}
	if limit <= 0 {
		return NormalizeResult{}, nil
	}

	var result NormalizeResult
	for result.Processed < limit {
		step, err := s.normalizeOnePending(ctx)
		if err != nil {
			return result, err
		}
		if !step.found {
			break
		}
		result = applyNormalizeStep(result, step)
	}

	return result, nil
}

func applyNormalizeStep(result NormalizeResult, step normalizeStepResult) NormalizeResult {
	result.Processed++
	if step.inserted {
		result.Inserted++
	}
	return result
}

type normalizeStepResult struct {
	found    bool
	inserted bool
}

func (s *Service) normalizeOnePending(ctx context.Context) (normalizeStepResult, error) {
	return runPipelineTx(ctx, s.pool, "normalize", func(tx db.Tx) (normalizeStepResult, error) {
		row, found, err := claimOnePendingRawArrivalTx(ctx, tx)
		if err != nil {
			return normalizeStepResult{}, err
		}
		if !found {
			return normalizeStepResult{}, nil
		}
		inserted, err := insertArticleTx(ctx, tx, buildNormalizedArticle(row, s.logger))
		return normalizeStepResult{found: true, inserted: inserted}, err
	})
}

func (s *Service) DedupPending(ctx context.Context, opts DedupOptions) (DedupResult, error) {
	config, shouldRun, err := s.dedupRunConfig(opts)
	if err != nil {
		return DedupResult{}, err
	}
	if !shouldRun {
		return DedupResult{}, nil
	}

	var result DedupResult
	for result.Processed < config.limit {
		decision, found, err := s.dedupOnePending(ctx, config.modelName, config.modelVersion, config.lookbackCutoff)
		if err != nil {
			return result, err
		}
		if !found {
			break
		}
		result = applyDedupDecision(result, decision)
	}

	return result, nil
}

func (s *Service) dedupRunConfig(opts DedupOptions) (dedupRunConfig, bool, error) {
	if s == nil || s.pool == nil {
		return dedupRunConfig{}, false, fmt.Errorf("pipeline service is not initialized")
	}
	if opts.Limit <= 0 {
		return dedupRunConfig{}, false, nil
	}
	modelName := strings.TrimSpace(opts.ModelName)
	if modelName == "" {
		modelName = DefaultEmbeddingModelName
	}
	modelVersion := strings.TrimSpace(opts.ModelVersion)
	if modelVersion == "" {
		modelVersion = DefaultEmbeddingModelVersion
	}
	lookbackDays := opts.LookbackDays
	if lookbackDays <= 0 {
		lookbackDays = DefaultDedupLookbackDays
	}
	return dedupRunConfig{
		limit:          opts.Limit,
		modelName:      modelName,
		modelVersion:   modelVersion,
		lookbackCutoff: globaltime.UTC().AddDate(0, 0, -lookbackDays),
	}, true, nil
}

func applyDedupDecision(result DedupResult, decision dedupDecisionKind) DedupResult {
	if decision == decisionNone {
		return result
	}
	result.Processed++
	switch decision {
	case decisionNewStory:
		result.NewStories++
	case decisionAutoMerge:
		result.AutoMerges++
	case decisionGrayZone:
		result.GrayZones++
	}
	return result
}

func (s *Service) dedupOnePending(
	ctx context.Context,
	modelName string,
	modelVersion string,
	lookbackCutoff time.Time,
) (dedupDecisionKind, bool, error) {
	result, err := runPipelineTx(ctx, s.pool, "dedup", func(tx db.Tx) (dedupStepResult, error) {
		article, found, err := claimOnePendingArticleTx(ctx, tx, modelName, modelVersion)
		if err != nil {
			return dedupStepResult{}, err
		}
		if !found {
			return dedupStepResult{}, nil
		}
		decision, err := dedupArticleTx(ctx, tx, article, modelName, modelVersion, lookbackCutoff)
		return dedupStepResult{decision: decision, found: true}, err
	})
	return result.decision, result.found, err
}

type dedupStepResult struct {
	decision dedupDecisionKind
	found    bool
}

func runPipelineTx[T any](ctx context.Context, pool *db.Pool, label string, fn func(db.Tx) (T, error)) (T, error) {
	var zero T
	tx, err := pool.BeginTx(ctx, db.TxOptions{})
	if err != nil {
		return zero, fmt.Errorf("begin %s tx: %w", label, err)
	}

	result, err := fn(tx)
	if err != nil {
		_ = tx.Rollback(ctx)
		return zero, err
	}
	if err := tx.Commit(ctx); err != nil {
		_ = tx.Rollback(ctx)
		return zero, fmt.Errorf("commit %s tx: %w", label, err)
	}
	return result, nil
}

func claimOnePendingRawArrivalTx(ctx context.Context, tx db.Tx) (rawArrivalRow, bool, error) {
	const q = `
SELECT
	ra.raw_arrival_id,
	ra.source,
	ra.source_item_id,
	ra.collection,
	ra.source_item_url,
	ra.source_published_at,
	ra.raw_payload,
	ra.fetched_at
FROM news.raw_arrivals ra
WHERE ra.deleted_at IS NULL
  AND NOT EXISTS (
	SELECT 1
	FROM news.articles d
	WHERE d.raw_arrival_id = ra.raw_arrival_id
)
ORDER BY ra.raw_arrival_id
LIMIT 1
FOR UPDATE SKIP LOCKED
`

	var row rawArrivalRow
	var sourceItemURL *string
	var sourcePublishedAt *time.Time
	err := tx.QueryRow(ctx, q).Scan(
		&row.RawArrivalID,
		&row.Source,
		&row.SourceItemID,
		&row.Collection,
		&sourceItemURL,
		&sourcePublishedAt,
		&row.RawPayload,
		&row.FetchedAt,
	)
	if err != nil {
		if err == db.ErrNoRows {
			return rawArrivalRow{}, false, nil
		}
		return rawArrivalRow{}, false, fmt.Errorf("claim raw_arrival: %w", err)
	}

	row.SourceItemURL = sourceItemURL
	row.SourcePublishedAt = sourcePublishedAt
	return row, true, nil
}

func insertArticleTx(ctx context.Context, tx db.Tx, article normalizedArticle) (bool, error) {
	const q = `
INSERT INTO news.articles (
	raw_arrival_id,
	source,
	source_item_id,
	collection,
	canonical_url,
	canonical_url_hash,
	normalized_title,
	normalized_text,
	normalized_language,
	published_at,
	source_domain,
	title_simhash,
	text_simhash,
	title_hash,
	content_hash,
	token_count,
	created_at,
	updated_at
)
VALUES (
	$1,
	$2,
	$3,
	$4,
	$5,
	$6,
	$7,
	$8,
	$9,
	$10,
	$11,
	$12,
	$13,
	$14,
	$15,
	$16,
	$17,
	$17
)
ON CONFLICT (raw_arrival_id) DO NOTHING
`

	commandTag, err := tx.Exec(
		ctx,
		q,
		article.RawArrivalID,
		article.Source,
		article.SourceItemID,
		article.Collection,
		article.CanonicalURL,
		nullableBytes(article.CanonicalURLHash),
		article.NormalizedTitle,
		article.NormalizedText,
		article.NormalizedLang,
		article.PublishedAt,
		article.SourceDomain,
		article.TitleSimhash,
		article.TextSimhash,
		article.TitleHash,
		article.ContentHash,
		article.TokenCount,
		article.ArticleCreatedAt,
	)
	if err != nil {
		return false, fmt.Errorf("insert article raw_arrival_id=%d: %w", article.RawArrivalID, err)
	}
	return commandTag.RowsAffected() == 1, nil
}

type normalizedArticleFields struct {
	title        string
	bodyText     string
	language     string
	canonicalURL string
	sourceDomain string
	collection   string
	publishedAt  *time.Time
}

func buildNormalizedArticle(row rawArrivalRow, logger zerolog.Logger) normalizedArticle {
	now := globaltime.UTC()
	if row.FetchedAt.IsZero() {
		row.FetchedAt = now
	}

	source := strings.TrimSpace(row.Source)
	sourceItemID := strings.TrimSpace(row.SourceItemID)
	fields := completeNormalizedArticleFields(row, sourceItemID, fieldsFromPayload(row, logger))
	normalizedCanonicalURL, host := textnormalize.URL(fields.canonicalURL)
	if fields.sourceDomain == "" {
		fields.sourceDomain = host
	}

	normalizedTitle := textnormalize.Text(fields.title)
	if normalizedTitle == "" {
		normalizedTitle = sourceItemID
	}
	normalizedBody := textnormalize.Text(fields.bodyText)
	titleHash := sha256.Sum256([]byte(normalizedTitle))
	contentHash := sha256.Sum256([]byte(normalizedTitle + "\n" + normalizedBody))

	return normalizedArticle{
		RawArrivalID:     row.RawArrivalID,
		Source:           source,
		SourceItemID:     sourceItemID,
		Collection:       fields.collection,
		CanonicalURL:     stringPtrIfNotEmpty(normalizedCanonicalURL),
		CanonicalURLHash: hashBytesIfNotEmpty(normalizedCanonicalURL),
		NormalizedTitle:  normalizedTitle,
		NormalizedText:   normalizedBody,
		NormalizedLang:   fields.language,
		PublishedAt:      fields.publishedAt,
		SourceDomain:     stringPtrIfNotEmpty(fields.sourceDomain),
		TitleSimhash:     simhashPtr(normalizedTitle),
		TextSimhash:      simhashPtr(normalizedBody),
		TitleHash:        append([]byte(nil), titleHash[:]...),
		ContentHash:      append([]byte(nil), contentHash[:]...),
		TokenCount:       textmetrics.CountTokens(normalizedTitle + " " + normalizedBody),
		ArticleCreatedAt: row.FetchedAt.UTC(),
	}
}

func fieldsFromPayload(row rawArrivalRow, logger zerolog.Logger) normalizedArticleFields {
	articlePayload, err := payloadschema.ValidateNewsItemPayload(row.RawPayload)
	if err != nil {
		logger.Warn().
			Err(err).
			Int64("raw_arrival_id", row.RawArrivalID).
			Msg("payload schema validation failed during normalize; falling back to lenient extraction")
		return normalizedArticleFields{}
	}

	fields := normalizedArticleFields{
		title: strings.TrimSpace(articlePayload.Title),
		collection: extractCollectionFromMetadata(
			articlePayload.SourceMetadata,
		),
	}
	if articlePayload.BodyText != nil {
		fields.bodyText = strings.TrimSpace(*articlePayload.BodyText)
	}
	if articlePayload.Language != nil {
		fields.language = strings.TrimSpace(strings.ToLower(*articlePayload.Language))
	}
	if articlePayload.CanonicalURL != nil {
		fields.canonicalURL = strings.TrimSpace(*articlePayload.CanonicalURL)
	}
	if articlePayload.SourceDomain != nil {
		fields.sourceDomain = strings.TrimSpace(strings.ToLower(*articlePayload.SourceDomain))
	}
	if articlePayload.PublishedAt != nil {
		fields.publishedAt = parsePayloadPublishedAt(*articlePayload.PublishedAt)
	}
	return fields
}

func completeNormalizedArticleFields(
	row rawArrivalRow,
	sourceItemID string,
	fields normalizedArticleFields,
) normalizedArticleFields {
	if fields.title == "" {
		fields.title = sourceItemID
	}
	fields.language = completeArticleLanguage(fields.language, fields.title, fields.bodyText)
	fields.publishedAt = completePublishedAt(fields.publishedAt, row.SourcePublishedAt)
	fields.canonicalURL = completeCanonicalURL(fields.canonicalURL, row.SourceItemURL)
	fields.collection = completeCollection(fields.collection, row.Collection)
	return fields
}

func parsePayloadPublishedAt(value string) *time.Time {
	ts, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return nil
	}
	utc := ts.UTC()
	return &utc
}

func completeArticleLanguage(language, title, bodyText string) string {
	normalized := normalizeISO6391Language(language)
	if normalized != "" && normalized != "und" {
		return normalized
	}
	detected := langdetect.DetectISO6391(strings.TrimSpace(title + "\n" + bodyText))
	if detected != "" {
		return detected
	}
	return "und"
}

func completePublishedAt(payloadPublishedAt, sourcePublishedAt *time.Time) *time.Time {
	if payloadPublishedAt != nil {
		return payloadPublishedAt
	}
	if sourcePublishedAt == nil {
		return nil
	}
	utc := sourcePublishedAt.UTC()
	return &utc
}

func completeCanonicalURL(payloadCanonicalURL string, sourceItemURL *string) string {
	if payloadCanonicalURL != "" || sourceItemURL == nil {
		return payloadCanonicalURL
	}
	return strings.TrimSpace(*sourceItemURL)
}

func completeCollection(payloadCollection, rowCollection string) string {
	collection := payloadCollection
	if collection == "" {
		collection = normalizeCollectionLabel(rowCollection)
	}
	if collection == "" {
		return "unknown"
	}
	return collection
}

func stringPtrIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}

func hashBytesIfNotEmpty(value string) []byte {
	if value == "" {
		return nil
	}
	hash := sha256.Sum256([]byte(value))
	return append([]byte(nil), hash[:]...)
}

func simhashPtr(value string) *int64 {
	hash, ok := textmetrics.Simhash64(value)
	if !ok {
		return nil
	}
	value64 := int64(hash)
	return &value64
}

func claimOnePendingArticleTx(ctx context.Context, tx db.Tx, modelName, modelVersion string) (pendingArticle, bool, error) {
	const q = `
SELECT
	d.article_id,
	d.source,
	d.source_item_id,
	d.collection,
	d.canonical_url,
	d.canonical_url_hash,
	d.normalized_title,
	d.normalized_text,
	d.published_at,
	d.source_domain,
	d.title_simhash,
	d.content_hash,
	de.embedding::text,
	d.created_at
FROM news.articles d
JOIN news.article_embeddings de
	ON de.article_id = d.article_id
	AND de.model_name = $1
	AND de.model_version = $2
WHERE d.deleted_at IS NULL
  AND NOT EXISTS (
	SELECT 1
	FROM news.story_articles sm
	WHERE sm.article_id = d.article_id
)
ORDER BY d.article_id
LIMIT 1
FOR UPDATE OF d SKIP LOCKED
`

	var row pendingArticle
	var canonicalURL *string
	var publishedAt *time.Time
	var sourceDomain *string
	var titleSimhash *int64
	var embeddingVector string
	err := tx.QueryRow(ctx, q, modelName, modelVersion).Scan(
		&row.ArticleID,
		&row.Source,
		&row.SourceItemID,
		&row.Collection,
		&canonicalURL,
		&row.CanonicalURLHash,
		&row.NormalizedTitle,
		&row.NormalizedText,
		&publishedAt,
		&sourceDomain,
		&titleSimhash,
		&row.ContentHash,
		&embeddingVector,
		&row.ArticleCreatedAt,
	)
	if err != nil {
		if err == db.ErrNoRows {
			return pendingArticle{}, false, nil
		}
		return pendingArticle{}, false, fmt.Errorf("claim pending article: %w", err)
	}

	row.CanonicalURL = canonicalURL
	row.PublishedAt = publishedAt
	row.SourceDomain = sourceDomain
	row.TitleSimhash = titleSimhash
	if strings.TrimSpace(embeddingVector) != "" {
		embeddingCopy := embeddingVector
		row.EmbeddingVector = &embeddingCopy
	}
	return row, true, nil
}

func dedupArticleTx(
	ctx context.Context,
	tx db.Tx,
	article pendingArticle,
	modelName string,
	modelVersion string,
	lookbackCutoff time.Time,
) (dedupDecisionKind, error) {
	now := globaltime.UTC()
	articleSeenAt := seenAtForArticle(article)

	exactMatch, found, err := findExactAutoMergeTx(ctx, tx, article)
	if err != nil {
		return decisionNone, err
	}
	if found {
		return applyExactAutoMergeTx(ctx, tx, article, exactMatch, now, articleSeenAt)
	}

	lexicalMatch, hasLexicalAutoMerge, err := findLexicalAutoMergeTx(ctx, tx, article, lookbackCutoff)
	if err != nil {
		return decisionNone, err
	}
	if hasLexicalAutoMerge {
		return applyLexicalAutoMergeTx(ctx, tx, article, lexicalMatch, now, articleSeenAt)
	}

	semanticEval, err := evaluateSemanticDedupTx(ctx, tx, article, modelName, modelVersion, lookbackCutoff)
	if err != nil {
		return decisionNone, err
	}
	if semanticEval.autoMerge != nil {
		return applySemanticAutoMergeTx(ctx, tx, article, *semanticEval.autoMerge, now, articleSeenAt)
	}
	return createSeedStoryWithEventTx(ctx, tx, article, semanticEval.best, now, articleSeenAt)
}

type exactAutoMergeMatch struct {
	storyID int64
	signal  string
}

type semanticEvaluation struct {
	best      *semanticMatch
	autoMerge *semanticMatch
}

func seenAtForArticle(article pendingArticle) time.Time {
	if article.PublishedAt != nil && !article.PublishedAt.IsZero() {
		return article.PublishedAt.UTC()
	}
	return article.ArticleCreatedAt
}

func findExactAutoMergeTx(ctx context.Context, tx db.Tx, article pendingArticle) (exactAutoMergeMatch, bool, error) {
	if storyID, found, err := findExactURLStoryTx(ctx, tx, article.Collection, article.CanonicalURLHash); err != nil || found {
		return exactAutoMergeMatch{storyID: storyID, signal: "exact_url"}, found, err
	}
	if storyID, found, err := findExactSourceIDStoryTx(ctx, tx, article.Collection, article.Source, article.SourceItemID); err != nil || found {
		return exactAutoMergeMatch{storyID: storyID, signal: "exact_source_id"}, found, err
	}
	storyID, found, err := findExactContentHashStoryTx(ctx, tx, article.Collection, article.ContentHash)
	return exactAutoMergeMatch{storyID: storyID, signal: "exact_content_hash"}, found, err
}

func applyExactAutoMergeTx(
	ctx context.Context,
	tx db.Tx,
	article pendingArticle,
	match exactAutoMergeMatch,
	now time.Time,
	articleSeenAt time.Time,
) (dedupDecisionKind, error) {
	return applyAutoMergeTx(ctx, tx, article, match.storyID, match.signal, 1, map[string]any{
		"signal": match.signal,
	}, now, articleSeenAt)
}

func applyLexicalAutoMergeTx(
	ctx context.Context,
	tx db.Tx,
	article pendingArticle,
	match lexicalAutoMergeMatch,
	now time.Time,
	articleSeenAt time.Time,
) (dedupDecisionKind, error) {
	return applyAutoMergeTx(
		ctx,
		tx,
		article,
		match.Candidate.StoryID,
		match.Signal,
		match.MatchScore,
		lexicalMatchDetails(match),
		now,
		articleSeenAt,
	)
}

func lexicalMatchDetails(match lexicalAutoMergeMatch) map[string]any {
	details := map[string]any{
		"signal":           match.Signal,
		"title_overlap":    match.TitleOverlap,
		"date_consistency": match.DateConsistency,
		"composite_score":  match.CompositeScore,
		"match_score":      match.MatchScore,
	}
	if match.SimhashDistance != nil {
		details["simhash_distance"] = *match.SimhashDistance
	}
	return details
}

func evaluateSemanticDedupTx(
	ctx context.Context,
	tx db.Tx,
	article pendingArticle,
	modelName string,
	modelVersion string,
	lookbackCutoff time.Time,
) (semanticEvaluation, error) {
	if article.EmbeddingVector == nil || strings.TrimSpace(*article.EmbeddingVector) == "" {
		return semanticEvaluation{}, nil
	}
	candidates, err := findSemanticCandidatesTx(
		ctx,
		tx,
		strings.TrimSpace(*article.EmbeddingVector),
		article.Collection,
		modelName,
		modelVersion,
		lookbackCutoff,
		defaultSemanticCandidateLimit,
	)
	if err != nil {
		return semanticEvaluation{}, err
	}
	return evaluateSemanticCandidates(article, candidates), nil
}

func evaluateSemanticCandidates(article pendingArticle, candidates []semanticCandidate) semanticEvaluation {
	var evaluation semanticEvaluation
	for _, candidate := range candidates {
		match := semanticCandidateMatch(article, candidate)
		evaluation = updateBestSemanticMatch(evaluation, match)
		if shouldAutoMergeSemantic(candidate.Cosine, match.TitleOverlap) {
			evaluation.autoMerge = &match
			return evaluation
		}
	}
	return evaluation
}

func semanticCandidateMatch(article pendingArticle, candidate semanticCandidate) semanticMatch {
	titleOverlap := textmetrics.TitleTokenJaccard(article.NormalizedTitle, candidate.Title)
	dateConsistency := computeDateConsistency(article.PublishedAt, candidate.LastSeenAt)
	return semanticMatch{
		Candidate:       candidate,
		TitleOverlap:    titleOverlap,
		DateConsistency: dateConsistency,
		CompositeScore:  semanticCompositeScore(candidate.Cosine, titleOverlap, dateConsistency),
	}
}

func updateBestSemanticMatch(evaluation semanticEvaluation, match semanticMatch) semanticEvaluation {
	if evaluation.best == nil || match.CompositeScore > evaluation.best.CompositeScore {
		best := match
		evaluation.best = &best
	}
	return evaluation
}

func createSeedStoryWithEventTx(
	ctx context.Context,
	tx db.Tx,
	article pendingArticle,
	bestSemantic *semanticMatch,
	now time.Time,
	articleSeenAt time.Time,
) (dedupDecisionKind, error) {
	newStoryID, err := createStoryTx(ctx, tx, article, articleSeenAt, now)
	if err != nil {
		return decisionNone, err
	}

	if inserted, err := upsertStoryArticleTx(ctx, tx, newStoryID, article.ArticleID, "seed", nil, map[string]any{
		"signal": "seed",
	}, now); err != nil {
		return decisionNone, err
	} else if !inserted {
		return decisionNone, nil
	}

	decision := decisionNewStory
	event := newStoryDedupEvent(article.ArticleID, newStoryID, decision, now)
	if bestSemantic != nil && shouldMarkSemanticGrayZone(bestSemantic.Candidate.Cosine) {
		decision = decisionGrayZone
		event = grayZoneDedupEvent(article.ArticleID, newStoryID, *bestSemantic, now)
	}
	event.Decision = string(decision)
	if err := insertDedupEventTx(ctx, tx, event); err != nil {
		return decisionNone, err
	}

	return decision, nil
}

func newStoryDedupEvent(articleID int64, storyID int64, decision dedupDecisionKind, now time.Time) dedupEventRecord {
	return dedupEventRecord{
		ArticleID:     articleID,
		Decision:      string(decision),
		ChosenStoryID: &storyID,
		CreatedAt:     now,
	}
}

func grayZoneDedupEvent(articleID int64, storyID int64, match semanticMatch, now time.Time) dedupEventRecord {
	event := newStoryDedupEvent(articleID, storyID, decisionGrayZone, now)
	candidateStoryID := match.Candidate.StoryID
	event.BestCandidateStoryID = &candidateStoryID
	event.BestCosine = floatPtr(match.Candidate.Cosine)
	event.TitleOverlap = floatPtr(match.TitleOverlap)
	event.EntityDateConsistency = floatPtr(match.DateConsistency)
	event.CompositeScore = floatPtr(match.CompositeScore)
	return event
}

func applyAutoMergeTx(
	ctx context.Context,
	tx db.Tx,
	article pendingArticle,
	storyID int64,
	exactSignal string,
	matchScore float64,
	matchDetails map[string]any,
	now time.Time,
	articleSeenAt time.Time,
) (dedupDecisionKind, error) {
	if inserted, err := upsertStoryArticleTx(ctx, tx, storyID, article.ArticleID, matchTypeForSignal(exactSignal), floatPtr(matchScore), matchDetails, now); err != nil {
		return decisionNone, err
	} else if !inserted {
		return decisionNone, nil
	}

	if err := updateStoryMetadataTx(ctx, tx, storyID, article.ArticleID, article.NormalizedTitle, article.CanonicalURL, article.CanonicalURLHash, articleSeenAt, now); err != nil {
		return decisionNone, err
	}

	exactSignalCopy := exactSignal
	if err := insertDedupEventTx(ctx, tx, dedupEventRecord{
		ArticleID:             article.ArticleID,
		Decision:              string(decisionAutoMerge),
		ChosenStoryID:         &storyID,
		BestCandidateStoryID:  &storyID,
		BestCosine:            nil,
		TitleOverlap:          nil,
		EntityDateConsistency: nil,
		CompositeScore:        floatPtr(matchScore),
		ExactSignal:           &exactSignalCopy,
		CreatedAt:             now,
	}); err != nil {
		return decisionNone, err
	}

	return decisionAutoMerge, nil
}

func applySemanticAutoMergeTx(
	ctx context.Context,
	tx db.Tx,
	article pendingArticle,
	match semanticMatch,
	now time.Time,
	articleSeenAt time.Time,
) (dedupDecisionKind, error) {
	matchDetails := map[string]any{
		"signal":           "semantic",
		"cosine":           match.Candidate.Cosine,
		"title_overlap":    match.TitleOverlap,
		"date_consistency": match.DateConsistency,
		"composite_score":  match.CompositeScore,
	}

	if inserted, err := upsertStoryArticleTx(
		ctx,
		tx,
		match.Candidate.StoryID,
		article.ArticleID,
		"semantic",
		floatPtr(match.CompositeScore),
		matchDetails,
		now,
	); err != nil {
		return decisionNone, err
	} else if !inserted {
		return decisionNone, nil
	}

	if err := updateStoryMetadataTx(
		ctx,
		tx,
		match.Candidate.StoryID,
		article.ArticleID,
		article.NormalizedTitle,
		article.CanonicalURL,
		article.CanonicalURLHash,
		articleSeenAt,
		now,
	); err != nil {
		return decisionNone, err
	}

	signal := "semantic"
	storyID := match.Candidate.StoryID
	if err := insertDedupEventTx(ctx, tx, dedupEventRecord{
		ArticleID:             article.ArticleID,
		Decision:              string(decisionAutoMerge),
		ChosenStoryID:         &storyID,
		BestCandidateStoryID:  &storyID,
		BestCosine:            floatPtr(match.Candidate.Cosine),
		TitleOverlap:          floatPtr(match.TitleOverlap),
		EntityDateConsistency: floatPtr(match.DateConsistency),
		CompositeScore:        floatPtr(match.CompositeScore),
		ExactSignal:           &signal,
		CreatedAt:             now,
	}); err != nil {
		return decisionNone, err
	}

	return decisionAutoMerge, nil
}

func matchTypeForSignal(signal string) string {
	switch signal {
	case "exact_url":
		return "exact_url"
	case "exact_source_id":
		return "exact_source_id"
	case "exact_content_hash":
		return "exact_content_hash"
	case "lexical_simhash":
		return "lexical_simhash"
	case "lexical_overlap":
		return "lexical_overlap"
	case "semantic":
		return "semantic"
	default:
		return "manual"
	}
}

func findExactURLStoryTx(ctx context.Context, tx db.Tx, collection string, canonicalURLHash []byte) (int64, bool, error) {
	return findHashMatchedStoryID(ctx, tx, "find exact_url story", exactURLStoryQuery, collection, canonicalURLHash)
}

func findExactSourceIDStoryTx(ctx context.Context, tx db.Tx, collection, source, sourceItemID string) (int64, bool, error) {
	const q = `
SELECT sm.story_id
FROM news.story_articles sm
JOIN news.articles d ON d.article_id = sm.article_id AND d.deleted_at IS NULL
JOIN news.stories s ON s.story_id = sm.story_id AND s.deleted_at IS NULL
WHERE s.status = 'active'
  AND s.collection = $1
  AND d.collection = $1
  AND d.source = $2
  AND d.source_item_id = $3
ORDER BY sm.matched_at DESC
LIMIT 1
	`
	return queryMatchedStoryID(ctx, tx, "find exact_source_id story", q, collection, source, sourceItemID)
}

func findExactContentHashStoryTx(ctx context.Context, tx db.Tx, collection string, contentHash []byte) (int64, bool, error) {
	return findHashMatchedStoryID(ctx, tx, "find exact_content_hash story", exactContentHashStoryQuery, collection, contentHash)
}

func findHashMatchedStoryID(ctx context.Context, tx db.Tx, label string, query string, collection string, hash []byte) (int64, bool, error) {
	if len(hash) == 0 {
		return 0, false, nil
	}
	return queryMatchedStoryID(ctx, tx, label, query, collection, hash)
}

func queryMatchedStoryID(ctx context.Context, tx db.Tx, label string, query string, args ...any) (int64, bool, error) {
	var storyID int64
	err := tx.QueryRow(ctx, query, args...).Scan(&storyID)
	if err != nil {
		if err == db.ErrNoRows {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("%s: %w", label, err)
	}
	return storyID, true, nil
}

func findSemanticCandidatesTx(
	ctx context.Context,
	tx db.Tx,
	embeddingVector string,
	collection string,
	modelName string,
	modelVersion string,
	lookbackCutoff time.Time,
	limit int,
) ([]semanticCandidate, error) {
	if limit <= 0 {
		limit = defaultSemanticCandidateLimit
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL hnsw.ef_search = %d", defaultSemanticSearchEF)); err != nil {
		return nil, fmt.Errorf("set hnsw.ef_search: %w", err)
	}

	const q = `
SELECT
	s.story_id,
	COALESCE(rd.normalized_title, s.canonical_title) AS candidate_title,
	s.last_seen_at,
	(1 - (de.embedding <=> $1::vector))::DOUBLE PRECISION AS cosine
FROM news.stories s
JOIN news.articles rd ON rd.article_id = s.representative_article_id AND rd.deleted_at IS NULL
JOIN news.article_embeddings de ON de.article_id = s.representative_article_id
WHERE s.deleted_at IS NULL
  AND s.status = 'active'
  AND s.collection = $2
  AND de.model_name = $3
  AND de.model_version = $4
  AND s.last_seen_at >= $5
ORDER BY de.embedding <=> $1::vector ASC
LIMIT $6
`

	rows, err := tx.Query(ctx, q, embeddingVector, collection, modelName, modelVersion, lookbackCutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("query semantic candidates: %w", err)
	}
	defer rows.Close()

	candidates := make([]semanticCandidate, 0, limit)
	for rows.Next() {
		var (
			c         semanticCandidate
			titleText string
		)
		if err := rows.Scan(&c.StoryID, &titleText, &c.LastSeenAt, &c.Cosine); err != nil {
			return nil, fmt.Errorf("scan semantic candidate: %w", err)
		}
		c.Title = titleText
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate semantic candidates: %w", err)
	}
	return candidates, nil
}

func findLexicalAutoMergeTx(
	ctx context.Context,
	tx db.Tx,
	article pendingArticle,
	lookbackCutoff time.Time,
) (lexicalAutoMergeMatch, bool, error) {
	const q = `
	SELECT
		s.story_id,
		COALESCE(rd.normalized_title, s.canonical_title) AS candidate_title,
		s.last_seen_at,
		(SELECT COUNT(DISTINCT a.source)
		 FROM news.story_articles sa
		 JOIN news.articles a
			ON a.article_id = sa.article_id
			AND a.deleted_at IS NULL
		 WHERE sa.story_id = s.story_id) AS source_count,
		(SELECT COUNT(*)
		 FROM news.story_articles sa
		 JOIN news.articles a
			ON a.article_id = sa.article_id
			AND a.deleted_at IS NULL
		 WHERE sa.story_id = s.story_id) AS article_count,
		s.canonical_url,
		rd.title_simhash
	FROM news.stories s
LEFT JOIN news.articles rd ON rd.article_id = s.representative_article_id AND rd.deleted_at IS NULL
WHERE s.deleted_at IS NULL
  AND s.status = 'active'
  AND s.collection = $3
  AND s.last_seen_at >= $2
ORDER BY s.last_seen_at DESC
LIMIT $1
`

	rows, err := tx.Query(ctx, q, storyCandidateLimit, lookbackCutoff, article.Collection)
	if err != nil {
		return lexicalAutoMergeMatch{}, false, fmt.Errorf("query lexical candidates: %w", err)
	}
	defer rows.Close()

	candidates, err := scanLexicalCandidates(rows)
	if err != nil {
		return lexicalAutoMergeMatch{}, false, err
	}
	match := bestLexicalAutoMerge(article, candidates)
	return match, match != (lexicalAutoMergeMatch{}), nil
}

func scanLexicalCandidates(rows *db.Rows) ([]storyCandidate, error) {
	candidates := make([]storyCandidate, 0, storyCandidateLimit)
	for rows.Next() {
		candidate, err := scanLexicalCandidate(rows)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lexical candidates: %w", err)
	}
	return candidates, nil
}

func scanLexicalCandidate(rows *db.Rows) (storyCandidate, error) {
	var c storyCandidate
	var canonicalURL *string
	var titleSimhash *int64
	if err := rows.Scan(
		&c.StoryID,
		&c.Title,
		&c.LastSeenAt,
		&c.SourceCount,
		&c.ArticleCount,
		&canonicalURL,
		&titleSimhash,
	); err != nil {
		return storyCandidate{}, fmt.Errorf("scan lexical candidate: %w", err)
	}
	c.CanonicalURL = canonicalURL
	c.TitleSimhash = titleSimhash
	return c, nil
}

func bestLexicalAutoMerge(article pendingArticle, candidates []storyCandidate) lexicalAutoMergeMatch {
	if match, ok := bestLexicalSimhashMatch(article, candidates); ok {
		return match
	}
	if match, ok := bestLexicalOverlapMatch(article, candidates); ok {
		return match
	}
	return lexicalAutoMergeMatch{}
}

func bestLexicalSimhashMatch(article pendingArticle, candidates []storyCandidate) (lexicalAutoMergeMatch, bool) {
	bestDistance := 65
	var best lexicalAutoMergeMatch
	var found bool
	for _, candidate := range candidates {
		match, distance, ok := lexicalSimhashMatch(article, candidate)
		if !ok || (found && !isBetterSimhashMatch(match, distance, best, bestDistance)) {
			continue
		}
		best = match
		bestDistance = distance
		found = true
	}
	return best, found
}

func lexicalSimhashMatch(article pendingArticle, candidate storyCandidate) (lexicalAutoMergeMatch, int, bool) {
	distance, ok := titleSimhashDistance(article.TitleSimhash, candidate.TitleSimhash)
	if !ok || distance > defaultLexicalSimhashMaxDistance {
		return lexicalAutoMergeMatch{}, 0, false
	}
	score := 1 - (float64(distance) / 64.0)
	distanceCopy := distance
	return lexicalAutoMergeMatch{
		Candidate:       candidate,
		Signal:          "lexical_simhash",
		MatchScore:      score,
		TitleOverlap:    textmetrics.TitleTokenJaccard(article.NormalizedTitle, candidate.Title),
		DateConsistency: computeDateConsistency(article.PublishedAt, candidate.LastSeenAt),
		CompositeScore:  score,
		SimhashDistance: &distanceCopy,
	}, distance, true
}

func isBetterSimhashMatch(current lexicalAutoMergeMatch, currentDistance int, best lexicalAutoMergeMatch, bestDistance int) bool {
	return currentDistance < bestDistance ||
		(currentDistance == bestDistance && current.Candidate.LastSeenAt.After(best.Candidate.LastSeenAt))
}

func bestLexicalOverlapMatch(article pendingArticle, candidates []storyCandidate) (lexicalAutoMergeMatch, bool) {
	var best lexicalAutoMergeMatch
	var found bool
	for _, candidate := range candidates {
		match, ok := lexicalOverlapMatch(article, candidate)
		if !ok || (found && match.CompositeScore <= best.CompositeScore) {
			continue
		}
		best = match
		found = true
	}
	return best, found
}

func lexicalOverlapMatch(article pendingArticle, candidate storyCandidate) (lexicalAutoMergeMatch, bool) {
	overlap := textmetrics.TitleTrigramJaccard(article.NormalizedTitle, candidate.Title)
	if overlap < defaultLexicalTrigramThreshold {
		return lexicalAutoMergeMatch{}, false
	}
	if !isWithinDateWindow(article.PublishedAt, candidate.LastSeenAt, defaultLexicalTrigramDateWindow) {
		return lexicalAutoMergeMatch{}, false
	}
	dateConsistency := computeDateConsistency(article.PublishedAt, candidate.LastSeenAt)
	composite := (0.8 * overlap) + (0.2 * dateConsistency)
	return lexicalAutoMergeMatch{
		Candidate:       candidate,
		Signal:          "lexical_overlap",
		MatchScore:      composite,
		TitleOverlap:    overlap,
		DateConsistency: dateConsistency,
		CompositeScore:  composite,
	}, true
}

func createStoryTx(
	ctx context.Context,
	tx db.Tx,
	article pendingArticle,
	articleSeenAt time.Time,
	now time.Time,
) (int64, error) {
	const q = `
INSERT INTO news.stories (
	canonical_title,
	canonical_url,
	canonical_url_hash,
	collection,
	representative_article_id,
	first_seen_at,
	last_seen_at,
	status,
	created_at,
	updated_at
)
VALUES (
	$1,
	$2,
	$3,
	$4,
	$5,
	$6,
	$6,
	'active',
	$7,
	$7
)
RETURNING story_id
`
	var storyID int64
	err := tx.QueryRow(
		ctx,
		q,
		article.NormalizedTitle,
		article.CanonicalURL,
		nullableBytes(article.CanonicalURLHash),
		article.Collection,
		article.ArticleID,
		articleSeenAt,
		now,
	).Scan(&storyID)
	if err != nil {
		return 0, fmt.Errorf("insert story for article_id=%d: %w", article.ArticleID, err)
	}
	return storyID, nil
}

func upsertStoryArticleTx(
	ctx context.Context,
	tx db.Tx,
	storyID int64,
	articleID int64,
	matchType string,
	matchScore *float64,
	matchDetails map[string]any,
	now time.Time,
) (bool, error) {
	const q = `
INSERT INTO news.story_articles (
	story_id,
	article_id,
	match_type,
	match_score,
	match_details,
	matched_at
)
VALUES ($1, $2, $3, $4, $5::jsonb, $6)
ON CONFLICT (article_id) DO NOTHING
`

	detailsJSON, err := json.Marshal(matchDetails)
	if err != nil {
		return false, fmt.Errorf("marshal story article details: %w", err)
	}

	commandTag, err := tx.Exec(ctx, q, storyID, articleID, matchType, matchScore, string(detailsJSON), now)
	if err != nil {
		return false, fmt.Errorf("insert story_article story_id=%d article_id=%d: %w", storyID, articleID, err)
	}
	return commandTag.RowsAffected() == 1, nil
}

func updateStoryMetadataTx(
	ctx context.Context,
	tx db.Tx,
	storyID int64,
	articleID int64,
	articleTitle string,
	articleCanonicalURL *string,
	articleCanonicalURLHash []byte,
	articleSeenAt time.Time,
	now time.Time,
) error {
	const q = `
	UPDATE news.stories s
	SET
		first_seen_at = LEAST(s.first_seen_at, $2),
		last_seen_at = GREATEST(s.last_seen_at, $3),
		representative_article_id = COALESCE(s.representative_article_id, $4),
		canonical_title = CASE WHEN s.representative_article_id IS NULL THEN $5 ELSE s.canonical_title END,
		canonical_url = CASE WHEN s.representative_article_id IS NULL THEN $6 ELSE s.canonical_url END,
		canonical_url_hash = CASE WHEN s.representative_article_id IS NULL THEN $7 ELSE s.canonical_url_hash END,
		updated_at = $1
	WHERE s.story_id = $8
	`
	_, err := tx.Exec(
		ctx,
		q,
		now,
		articleSeenAt,
		articleSeenAt,
		articleID,
		articleTitle,
		articleCanonicalURL,
		nullableBytes(articleCanonicalURLHash),
		storyID,
	)
	if err != nil {
		return fmt.Errorf("update story metadata story_id=%d: %w", storyID, err)
	}
	return nil
}

type dedupEventRecord struct {
	ArticleID             int64
	Decision              string
	ChosenStoryID         *int64
	BestCandidateStoryID  *int64
	BestCosine            *float64
	TitleOverlap          *float64
	EntityDateConsistency *float64
	CompositeScore        *float64
	ExactSignal           *string
	CreatedAt             time.Time
}

func insertDedupEventTx(ctx context.Context, tx db.Tx, record dedupEventRecord) error {
	const q = `
INSERT INTO news.dedup_events (
	article_id,
	decision,
	chosen_story_id,
	best_candidate_story_id,
	best_cosine,
	title_overlap,
	entity_date_consistency,
	composite_score,
	exact_signal,
	created_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (article_id) DO NOTHING
`
	_, err := tx.Exec(
		ctx,
		q,
		record.ArticleID,
		record.Decision,
		record.ChosenStoryID,
		record.BestCandidateStoryID,
		record.BestCosine,
		record.TitleOverlap,
		record.EntityDateConsistency,
		record.CompositeScore,
		record.ExactSignal,
		record.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert dedup_event article_id=%d: %w", record.ArticleID, err)
	}
	return nil
}

func extractCollectionFromMetadata(metadata map[string]any) string {
	if len(metadata) == 0 {
		return ""
	}
	raw, ok := metadata["collection"]
	if !ok {
		return ""
	}
	label, ok := raw.(string)
	if !ok {
		return ""
	}
	return normalizeCollectionLabel(label)
}

func normalizeCollectionLabel(raw string) string {
	return strings.TrimSpace(strings.ToLower(raw))
}

func normalizeISO6391Language(raw string) string {
	lang := strings.TrimSpace(strings.ToLower(raw))
	if lang == "" {
		return ""
	}

	lang = strings.ReplaceAll(lang, "_", "-")
	if parts := strings.Split(lang, "-"); len(parts) > 0 {
		lang = strings.TrimSpace(parts[0])
	}
	if len(lang) != 2 {
		return ""
	}
	for _, r := range lang {
		if r < 'a' || r > 'z' {
			return ""
		}
	}
	return lang
}

func titleSimhashDistance(left, right *int64) (int, bool) {
	if left == nil || right == nil {
		return 0, false
	}
	return bits.OnesCount64(uint64(*left) ^ uint64(*right)), true
}

func isWithinDateWindow(articlePublishedAt *time.Time, storyLastSeen time.Time, window time.Duration) bool {
	if articlePublishedAt == nil || articlePublishedAt.IsZero() {
		return false
	}
	diff := math.Abs(articlePublishedAt.UTC().Sub(storyLastSeen.UTC()).Hours())
	return diff <= window.Hours()
}

func computeDateConsistency(articlePublishedAt *time.Time, storyLastSeen time.Time) float64 {
	if articlePublishedAt == nil || articlePublishedAt.IsZero() {
		return 0.5
	}
	diff := math.Abs(articlePublishedAt.UTC().Sub(storyLastSeen.UTC()).Hours())
	switch {
	case diff <= 48:
		return 1
	case diff <= 7*24:
		return 0.6
	default:
		return 0
	}
}

func semanticCompositeScore(cosine, titleOverlap, dateConsistency float64) float64 {
	score := (semanticCompositeCosineWeight * cosine) +
		(semanticCompositeTitleWeight * titleOverlap) +
		(semanticCompositeDateWeight * dateConsistency)
	switch {
	case score < 0:
		return 0
	case score > 1:
		return 1
	default:
		return score
	}
}

func shouldAutoMergeSemantic(cosine, titleOverlap float64) bool {
	if cosine >= defaultSemanticOverrideCosine {
		return true
	}
	return cosine >= defaultSemanticAutoMergeCosine && titleOverlap >= defaultSemanticTitleOverlapFloor
}

func shouldMarkSemanticGrayZone(cosine float64) bool {
	return cosine >= defaultSemanticGrayZoneMinCosine && cosine < defaultSemanticAutoMergeCosine
}

func nullableBytes(value []byte) []byte {
	if len(value) == 0 {
		return nil
	}
	copyValue := make([]byte, len(value))
	copy(copyValue, value)
	return copyValue
}

func floatPtr(v float64) *float64 {
	p := new(float64)
	*p = v
	return p
}
