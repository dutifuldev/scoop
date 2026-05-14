package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"horse.fit/scoop/internal/db"
	"horse.fit/scoop/internal/globaltime"
)

const (
	DefaultEmbeddingEndpoint       = "http://127.0.0.1:8844/embed"
	DefaultEmbeddingModelName      = "Qwen3-Embedding-8B"
	DefaultEmbeddingModelVersion   = "v1"
	DefaultEmbeddingBatchSize      = 32
	DefaultEmbeddingMaxLength      = 512
	DefaultEmbeddingRequestTimeout = 45 * time.Second
	embeddingVectorDimensions      = 4096
)

type EmbedOptions struct {
	Limit          int
	BatchSize      int
	Endpoint       string
	ModelName      string
	ModelVersion   string
	MaxLength      int
	RequestTimeout time.Duration
}

type EmbedResult struct {
	Processed int
	Embedded  int
	Skipped   int
	Failed    int
}

type embeddingPendingArticle struct {
	ArticleID       int64
	NormalizedTitle string
	NormalizedText  string
}

type embedRequest struct {
	Texts     []string `json:"texts,omitempty"`
	Input     []string `json:"input,omitempty"`
	MaxLength int      `json:"max_length,omitempty"`
}

type embedResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
	ElapsedMS  *float64    `json:"elapsed_ms"`
	Data       []struct {
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

func (s *Service) EmbedPending(ctx context.Context, options EmbedOptions) (EmbedResult, error) {
	if s == nil || s.pool == nil {
		return EmbedResult{}, fmt.Errorf("pipeline service is not initialized")
	}

	opts := normalizeEmbedOptions(options)
	if opts.Limit <= 0 {
		return EmbedResult{}, nil
	}

	var result EmbedResult
	for result.Processed < opts.Limit {
		progress, err := s.embedNextBatch(ctx, opts, result.Processed)
		if err != nil {
			return result, err
		}
		if progress.Processed == 0 {
			break
		}
		result.add(progress)
	}

	return result, nil
}

func (s *Service) embedNextBatch(ctx context.Context, opts EmbedOptions, processed int) (EmbedResult, error) {
	batchSize := min(opts.BatchSize, opts.Limit-processed)
	articles, err := selectPendingEmbeddingArticles(ctx, s.pool, opts.ModelName, opts.ModelVersion, batchSize)
	if err != nil || len(articles) == 0 {
		return EmbedResult{}, err
	}
	vectors, _, err := requestEmbeddings(ctx, opts, embeddingInputs(articles))
	if err != nil {
		return EmbedResult{}, err
	}
	if len(vectors) != len(articles) {
		return EmbedResult{}, fmt.Errorf("embedding response count mismatch: requested=%d returned=%d", len(articles), len(vectors))
	}
	return s.insertEmbeddingBatch(ctx, opts, articles, vectors)
}

func embeddingInputs(articles []embeddingPendingArticle) []string {
	texts := make([]string, len(articles))
	for index, article := range articles {
		texts[index] = embeddingInput(article)
	}
	return texts
}

func (s *Service) insertEmbeddingBatch(
	ctx context.Context,
	opts EmbedOptions,
	articles []embeddingPendingArticle,
	vectors [][]float64,
) (EmbedResult, error) {
	var result EmbedResult
	for i, article := range articles {
		progress, err := s.insertEmbeddingVector(ctx, opts, article, vectors[i])
		result.add(progress)
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func (s *Service) insertEmbeddingVector(
	ctx context.Context,
	opts EmbedOptions,
	article embeddingPendingArticle,
	vector []float64,
) (EmbedResult, error) {
	vectorLiteral, err := toVectorLiteral(vector)
	if err != nil {
		return EmbedResult{Processed: 1, Failed: 1}, fmt.Errorf("article_id=%d invalid embedding vector: %w", article.ArticleID, err)
	}

	inserted, err := insertArticleEmbedding(
		ctx,
		s.pool,
		article.ArticleID,
		opts.ModelName,
		opts.ModelVersion,
		vectorLiteral,
		opts.Endpoint,
		globaltime.UTC(),
	)
	if err != nil {
		return EmbedResult{Processed: 1, Failed: 1}, err
	}
	if inserted {
		return EmbedResult{Processed: 1, Embedded: 1}, nil
	}
	return EmbedResult{Processed: 1, Skipped: 1}, nil
}

func (r *EmbedResult) add(delta EmbedResult) {
	r.Processed += delta.Processed
	r.Embedded += delta.Embedded
	r.Skipped += delta.Skipped
	r.Failed += delta.Failed
}

func normalizeEmbedOptions(opts EmbedOptions) EmbedOptions {
	normalized := opts
	normalized.Limit = nonNegativeInt(normalized.Limit)
	normalized.BatchSize = normalizedBatchSize(normalized.BatchSize, normalized.Limit)
	normalized.Endpoint = normalizeEmbeddingEndpoint(defaultString(normalized.Endpoint, DefaultEmbeddingEndpoint))
	normalized.ModelName = defaultString(normalized.ModelName, DefaultEmbeddingModelName)
	normalized.ModelVersion = defaultString(normalized.ModelVersion, DefaultEmbeddingModelVersion)
	normalized.MaxLength = defaultPositiveInt(normalized.MaxLength, DefaultEmbeddingMaxLength)
	normalized.RequestTimeout = defaultPositiveDuration(normalized.RequestTimeout, DefaultEmbeddingRequestTimeout)
	return normalized
}

func nonNegativeInt(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func normalizedBatchSize(batchSize int, limit int) int {
	if batchSize <= 0 {
		batchSize = DefaultEmbeddingBatchSize
	}
	if batchSize > limit && limit > 0 {
		return limit
	}
	return batchSize
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func defaultPositiveInt(value int, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func defaultPositiveDuration(value time.Duration, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	return value
}

func selectPendingEmbeddingArticles(
	ctx context.Context,
	pool *db.Pool,
	modelName string,
	modelVersion string,
	limit int,
) ([]embeddingPendingArticle, error) {
	const q = `
SELECT
	d.article_id,
	d.normalized_title,
	d.normalized_text
FROM news.articles d
WHERE NOT EXISTS (
	SELECT 1
	FROM news.article_embeddings de
	WHERE de.article_id = d.article_id
	  AND de.model_name = $1
	  AND de.model_version = $2
)
  AND d.deleted_at IS NULL
ORDER BY d.article_id
LIMIT $3
`

	rows, err := pool.Query(ctx, q, modelName, modelVersion, limit)
	if err != nil {
		return nil, fmt.Errorf("select pending articles for embedding: %w", err)
	}
	defer rows.Close()

	articles := make([]embeddingPendingArticle, 0, limit)
	for rows.Next() {
		var article embeddingPendingArticle
		if err := rows.Scan(&article.ArticleID, &article.NormalizedTitle, &article.NormalizedText); err != nil {
			return nil, fmt.Errorf("scan pending embedding article: %w", err)
		}
		articles = append(articles, article)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending embedding articles: %w", err)
	}
	return articles, nil
}

func insertArticleEmbedding(
	ctx context.Context,
	pool *db.Pool,
	articleID int64,
	modelName string,
	modelVersion string,
	vectorLiteral string,
	endpoint string,
	now time.Time,
) (bool, error) {
	const q = `
INSERT INTO news.article_embeddings (
	article_id,
	model_name,
	model_version,
	embedding,
	embedded_at,
	service_endpoint
)
VALUES ($1, $2, $3, $4::vector, $5, $6)
ON CONFLICT (article_id, model_name, model_version) DO NOTHING
`

	tag, err := pool.Exec(ctx, q, articleID, modelName, modelVersion, vectorLiteral, now, endpoint)
	if err != nil {
		return false, fmt.Errorf("insert article embedding article_id=%d: %w", articleID, err)
	}
	return tag.RowsAffected() == 1, nil
}

func embeddingInput(article embeddingPendingArticle) string {
	return strings.Join(nonEmptyEmbeddingParts(article), "\n\n")
}

func nonEmptyEmbeddingParts(article embeddingPendingArticle) []string {
	parts := make([]string, 0, 2)
	for _, part := range []string{article.NormalizedTitle, article.NormalizedText} {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

func requestEmbeddings(ctx context.Context, opts EmbedOptions, texts []string) ([][]float64, *float64, error) {
	body, err := json.Marshal(newEmbedRequest(opts, texts))
	if err != nil {
		return nil, nil, fmt.Errorf("marshal embedding request: %w", err)
	}

	requestCtx, cancel := context.WithTimeout(ctx, opts.RequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, opts.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("build embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("embedding request failed: %w", err)
	}
	defer resp.Body.Close()
	return parseEmbeddingHTTPResponse(resp)
}

func newEmbedRequest(opts EmbedOptions, texts []string) embedRequest {
	if isOpenAIEmbeddingEndpoint(opts.Endpoint) {
		return embedRequest{Input: texts}
	}
	return embedRequest{Texts: texts, MaxLength: opts.MaxLength}
}

func isOpenAIEmbeddingEndpoint(endpoint string) bool {
	parsedEndpoint, err := url.Parse(endpoint)
	return err == nil && strings.HasSuffix(parsedEndpoint.Path, "/v1/embeddings")
}

func parseEmbeddingHTTPResponse(resp *http.Response) ([][]float64, *float64, error) {
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read embedding response: %w", err)
	}
	if !isHTTPSuccess(resp.StatusCode) {
		return nil, nil, fmt.Errorf("embedding service status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed embedResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, nil, fmt.Errorf("decode embedding response: %w", err)
	}
	return vectorsFromEmbeddingResponse(parsed)
}

func isHTTPSuccess(statusCode int) bool {
	return statusCode >= 200 && statusCode < 300
}

func vectorsFromEmbeddingResponse(parsed embedResponse) ([][]float64, *float64, error) {
	vectors := parsed.Embeddings
	if len(vectors) == 0 && len(parsed.Data) > 0 {
		vectors = vectorsFromOpenAIEmbeddingData(parsed)
	}
	if len(vectors) == 0 {
		return nil, parsed.ElapsedMS, fmt.Errorf("embedding response missing vectors")
	}

	return vectors, parsed.ElapsedMS, nil
}

func vectorsFromOpenAIEmbeddingData(parsed embedResponse) [][]float64 {
	sort.Slice(parsed.Data, func(i, j int) bool {
		return parsed.Data[i].Index < parsed.Data[j].Index
	})
	vectors := make([][]float64, 0, len(parsed.Data))
	for _, row := range parsed.Data {
		vectors = append(vectors, row.Embedding)
	}
	return vectors
}

func normalizeEmbeddingEndpoint(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return DefaultEmbeddingEndpoint
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return trimmed
	}
	if parsed.Path == "" || parsed.Path == "/" {
		parsed.Path = "/embed"
	}
	return parsed.String()
}

func toVectorLiteral(values []float64) (string, error) {
	if len(values) != embeddingVectorDimensions {
		return "", fmt.Errorf("expected %d dimensions, got %d", embeddingVectorDimensions, len(values))
	}

	var builder strings.Builder
	builder.Grow(len(values) * 8)
	builder.WriteByte('[')
	for i, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return "", fmt.Errorf("vector has non-finite value at index %d", i)
		}
		if i > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(strconv.FormatFloat(value, 'f', -1, 64))
	}
	builder.WriteByte(']')
	return builder.String(), nil
}
