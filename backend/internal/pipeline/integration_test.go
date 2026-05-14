package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"horse.fit/scoop/internal/db"
)

func TestServiceNormalizeEmbedAndDedupIntegration(t *testing.T) {
	pool := newPipelineIntegrationPool(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	createPipelineRawArrival(t, pool, "discord_archive", "item-1", "https://example.com/openclaw", "OpenClaw release notes", now)
	createPipelineRawArrival(t, pool, "discord_archive", "item-2", "https://example.com/openclaw", "OpenClaw release notes update", now.Add(time.Minute))

	service := NewService(pool, zerolog.Nop())
	normalized, err := service.NormalizePending(ctx, 10)
	if err != nil {
		t.Fatalf("NormalizePending() error = %v", err)
	}
	if normalized.Processed != 2 || normalized.Inserted != 2 {
		t.Fatalf("NormalizePending() = %#v, want 2 processed and inserted", normalized)
	}

	embeddingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embed" {
			t.Fatalf("embedding path = %q, want /embed", r.URL.Path)
		}
		var req embedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode embedding request: %v", err)
		}
		vectors := make([][]float64, len(req.Texts))
		for index := range req.Texts {
			vectors[index] = unitVector(index)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(embedResponse{Embeddings: vectors}); err != nil {
			t.Fatalf("encode embedding response: %v", err)
		}
	}))
	defer embeddingServer.Close()

	embedded, err := service.EmbedPending(ctx, EmbedOptions{
		Limit:          10,
		BatchSize:      2,
		Endpoint:       embeddingServer.URL,
		ModelName:      "test-model",
		ModelVersion:   "test-version",
		RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("EmbedPending() error = %v", err)
	}
	if embedded.Processed != 2 || embedded.Embedded != 2 {
		t.Fatalf("EmbedPending() = %#v, want 2 processed and embedded", embedded)
	}

	deduped, err := service.DedupPending(ctx, DedupOptions{
		Limit:        10,
		ModelName:    "test-model",
		ModelVersion: "test-version",
		LookbackDays: 30,
	})
	if err != nil {
		t.Fatalf("DedupPending() error = %v", err)
	}
	if deduped.Processed != 2 || deduped.NewStories != 1 || deduped.AutoMerges != 1 {
		t.Fatalf("DedupPending() = %#v, want one new story and one auto-merge", deduped)
	}

	var storyCount int64
	if err := pool.GORM().WithContext(ctx).Model(&db.Story{}).Count(&storyCount).Error; err != nil {
		t.Fatalf("count stories: %v", err)
	}
	if storyCount != 1 {
		t.Fatalf("story count = %d, want 1", storyCount)
	}
	var event db.DedupEvent
	if err := pool.GORM().WithContext(ctx).Where("decision = ?", "auto_merge").First(&event).Error; err != nil {
		t.Fatalf("query auto-merge event: %v", err)
	}
	if event.ExactSignal == nil || *event.ExactSignal != "exact_url" {
		t.Fatalf("auto-merge signal = %v, want exact_url", event.ExactSignal)
	}
}

func TestServicePipelineNoopsWithEmptyLimits(t *testing.T) {
	t.Parallel()

	service := NewService(&db.Pool{}, zerolog.Nop())
	if got, err := service.NormalizePending(context.Background(), 0); err != nil || got != (NormalizeResult{}) {
		t.Fatalf("NormalizePending(0) = %#v, %v", got, err)
	}
	if got, err := service.EmbedPending(context.Background(), EmbedOptions{Limit: 0}); err != nil || got != (EmbedResult{}) {
		t.Fatalf("EmbedPending(0) = %#v, %v", got, err)
	}
	if got, err := service.DedupPending(context.Background(), DedupOptions{Limit: 0}); err != nil || got != (DedupResult{}) {
		t.Fatalf("DedupPending(0) = %#v, %v", got, err)
	}
}

func TestServicePipelineNoopsWithEmptyQueuesIntegration(t *testing.T) {
	pool := newPipelineIntegrationPool(t)
	service := NewService(pool, zerolog.Nop())
	ctx := context.Background()

	normalized, err := service.NormalizePending(ctx, 1)
	if err != nil {
		t.Fatalf("NormalizePending(empty) error = %v", err)
	}
	if normalized != (NormalizeResult{}) {
		t.Fatalf("NormalizePending(empty) = %#v, want zero result", normalized)
	}

	deduped, err := service.DedupPending(ctx, DedupOptions{
		Limit:        1,
		ModelName:    "test-model",
		ModelVersion: "test-version",
		LookbackDays: 30,
	})
	if err != nil {
		t.Fatalf("DedupPending(empty) error = %v", err)
	}
	if deduped != (DedupResult{}) {
		t.Fatalf("DedupPending(empty) = %#v, want zero result", deduped)
	}
}

func TestServiceDedupPendingSemanticAutoMergeIntegration(t *testing.T) {
	pool := newPipelineIntegrationPool(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 14, 13, 0, 0, 0, time.UTC)
	createPipelineRawArrival(t, pool, "discord_archive", "semantic-1", "https://example.com/openclaw-setup-a", "OpenClaw setup guide", now)
	createPipelineRawArrival(t, pool, "discord_archive", "semantic-2", "https://example.com/openclaw-setup-b", "OpenClaw setup walkthrough", now.Add(time.Minute))

	service := NewService(pool, zerolog.Nop())
	if _, err := service.NormalizePending(ctx, 10); err != nil {
		t.Fatalf("NormalizePending() error = %v", err)
	}
	embeddingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		vectors := [][]float64{unitVector(0), unitVector(0)}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(embedResponse{Embeddings: vectors}); err != nil {
			t.Fatalf("encode embedding response: %v", err)
		}
	}))
	defer embeddingServer.Close()
	if _, err := service.EmbedPending(ctx, EmbedOptions{
		Limit:          10,
		BatchSize:      2,
		Endpoint:       embeddingServer.URL,
		ModelName:      "semantic-model",
		ModelVersion:   "semantic-version",
		RequestTimeout: time.Second,
	}); err != nil {
		t.Fatalf("EmbedPending() error = %v", err)
	}
	deduped, err := service.DedupPending(ctx, DedupOptions{
		Limit:        10,
		ModelName:    "semantic-model",
		ModelVersion: "semantic-version",
		LookbackDays: 30,
	})
	if err != nil {
		t.Fatalf("DedupPending() error = %v", err)
	}
	if deduped.NewStories != 1 || deduped.AutoMerges != 1 {
		t.Fatalf("DedupPending() = %#v, want semantic auto-merge", deduped)
	}
	var event db.DedupEvent
	if err := pool.GORM().WithContext(ctx).Where("exact_signal = ?", "semantic").First(&event).Error; err != nil {
		t.Fatalf("query semantic event: %v", err)
	}
	if event.BestCosine == nil || *event.BestCosine < defaultSemanticAutoMergeCosine {
		t.Fatalf("semantic event best cosine = %v", event.BestCosine)
	}
}

func TestServiceDedupPendingExactContentHashIntegration(t *testing.T) {
	pool := newPipelineIntegrationPool(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 14, 14, 0, 0, 0, time.UTC)
	createPipelineRawArrival(t, pool, "discord_archive", "content-1", "https://example.com/content-a", "OpenClaw identical content", now)
	createPipelineRawArrival(t, pool, "github", "content-2", "https://github.com/owner/repo/issues/42", "OpenClaw identical content", now.Add(time.Minute))

	service := NewService(pool, zerolog.Nop())
	if _, err := service.NormalizePending(ctx, 10); err != nil {
		t.Fatalf("NormalizePending() error = %v", err)
	}
	embeddingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		vectors := [][]float64{unitVector(0), unitVector(1)}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(embedResponse{Embeddings: vectors}); err != nil {
			t.Fatalf("encode embedding response: %v", err)
		}
	}))
	defer embeddingServer.Close()
	if _, err := service.EmbedPending(ctx, EmbedOptions{
		Limit:          10,
		BatchSize:      2,
		Endpoint:       embeddingServer.URL,
		ModelName:      "content-model",
		ModelVersion:   "content-version",
		RequestTimeout: time.Second,
	}); err != nil {
		t.Fatalf("EmbedPending() error = %v", err)
	}
	deduped, err := service.DedupPending(ctx, DedupOptions{
		Limit:        10,
		ModelName:    "content-model",
		ModelVersion: "content-version",
		LookbackDays: 30,
	})
	if err != nil {
		t.Fatalf("DedupPending() error = %v", err)
	}
	if deduped.NewStories != 1 || deduped.AutoMerges != 1 {
		t.Fatalf("DedupPending() = %#v, want exact content hash auto-merge", deduped)
	}
	var event db.DedupEvent
	if err := pool.GORM().WithContext(ctx).Where("exact_signal = ?", "exact_content_hash").First(&event).Error; err != nil {
		t.Fatalf("query exact_content_hash event: %v", err)
	}
	if event.ExactSignal == nil || *event.ExactSignal != "exact_content_hash" {
		t.Fatalf("exact signal = %v, want exact_content_hash", event.ExactSignal)
	}
}

func TestServiceDedupPendingSemanticGrayZoneIntegration(t *testing.T) {
	pool := newPipelineIntegrationPool(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 14, 15, 0, 0, 0, time.UTC)
	createPipelineRawArrival(t, pool, "discord_archive", "gray-1", "https://example.com/gray-a", "OpenClaw deployment gateway setup", now)
	createPipelineRawArrival(t, pool, "discord_archive", "gray-2", "https://example.com/gray-b", "OpenClaw deployment gateway notes", now.Add(time.Minute))

	service := NewService(pool, zerolog.Nop())
	if _, err := service.NormalizePending(ctx, 10); err != nil {
		t.Fatalf("NormalizePending() error = %v", err)
	}
	embeddingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		second := make([]float64, embeddingVectorDimensions)
		second[0] = 0.91
		second[1] = 0.4146082489441243
		vectors := [][]float64{unitVector(0), second}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(embedResponse{Embeddings: vectors}); err != nil {
			t.Fatalf("encode embedding response: %v", err)
		}
	}))
	defer embeddingServer.Close()
	if _, err := service.EmbedPending(ctx, EmbedOptions{
		Limit:          10,
		BatchSize:      2,
		Endpoint:       embeddingServer.URL,
		ModelName:      "gray-model",
		ModelVersion:   "gray-version",
		RequestTimeout: time.Second,
	}); err != nil {
		t.Fatalf("EmbedPending() error = %v", err)
	}
	deduped, err := service.DedupPending(ctx, DedupOptions{
		Limit:        10,
		ModelName:    "gray-model",
		ModelVersion: "gray-version",
		LookbackDays: 30,
	})
	if err != nil {
		t.Fatalf("DedupPending() error = %v", err)
	}
	if deduped.NewStories != 1 || deduped.GrayZones != 1 {
		t.Fatalf("DedupPending() = %#v, want one new story and one gray zone", deduped)
	}
	var event db.DedupEvent
	if err := pool.GORM().WithContext(ctx).Where("decision = ?", "gray_zone").First(&event).Error; err != nil {
		t.Fatalf("query gray-zone event: %v", err)
	}
	if event.BestCosine == nil || *event.BestCosine < defaultSemanticGrayZoneMinCosine || *event.BestCosine >= defaultSemanticAutoMergeCosine {
		t.Fatalf("gray-zone cosine = %v", event.BestCosine)
	}
}

func newPipelineIntegrationPool(t *testing.T) *db.Pool {
	t.Helper()

	databaseURL := strings.TrimSpace(os.Getenv("SCOOP_TEST_DATABASE_URL"))
	if databaseURL == "" {
		databaseURL = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if databaseURL == "" {
		t.Skip("set SCOOP_TEST_DATABASE_URL or DATABASE_URL to run pipeline integration tests")
	}
	databaseURL = ensurePipelineTestDatabase(t, databaseURL)

	pool, err := db.NewIntegrationTestPool(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("NewPool() error = %v", err)
	}
	lockPipelineIntegrationDB(t, pool)
	t.Cleanup(func() {
		cleanPipelineIntegrationDB(t, pool)
		unlockPipelineIntegrationDB(t, pool)
		if err := pool.Close(); err != nil {
			t.Fatalf("close pool: %v", err)
		}
	})
	cleanPipelineIntegrationDB(t, pool)
	return pool
}

func ensurePipelineTestDatabase(t *testing.T, databaseURL string) string {
	t.Helper()

	targetURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse database URL: %v", err)
	}
	dbName := strings.TrimPrefix(targetURL.Path, "/")
	if dbName == "" {
		t.Fatalf("database URL must include a database name")
	}
	targetName := dbName + "_pipeline_test"
	targetURL.Path = "/" + targetName

	adminURL := *targetURL
	adminURL.Path = "/postgres"
	adminDB, err := gorm.Open(postgres.Open(adminURL.String()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open admin database: %v", err)
	}
	var exists bool
	if err := adminDB.Raw("SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = ?)", targetName).Scan(&exists).Error; err != nil {
		t.Fatalf("check test database: %v", err)
	}
	if !exists {
		if err := adminDB.Exec(`CREATE DATABASE "` + strings.ReplaceAll(targetName, `"`, `""`) + `"`).Error; err != nil {
			t.Fatalf("create test database: %v", err)
		}
	}
	sqlDB, err := adminDB.DB()
	if err != nil {
		t.Fatalf("get admin sql db: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close admin db: %v", err)
	}
	return targetURL.String()
}

func lockPipelineIntegrationDB(t *testing.T, pool *db.Pool) {
	t.Helper()
	if err := pool.GORM().Exec("SELECT pg_advisory_lock(54718801)").Error; err != nil {
		t.Fatalf("lock integration db: %v", err)
	}
}

func unlockPipelineIntegrationDB(t *testing.T, pool *db.Pool) {
	t.Helper()
	if err := pool.GORM().Exec("SELECT pg_advisory_unlock(54718801)").Error; err != nil {
		t.Fatalf("unlock integration db: %v", err)
	}
}

func cleanPipelineIntegrationDB(t *testing.T, pool *db.Pool) {
	t.Helper()

	const q = `
	TRUNCATE TABLE
		news.audit_events,
		news.user_settings,
		news.sessions,
		news.users,
		news.collection_settings,
		news.translation_results,
		news.translation_sources,
		news.digest_entries,
		news.digest_runs,
		news.story_topic_state,
		news.topic_keyword_rules,
		news.topic_source_rules,
		news.topics,
		news.dedup_events,
		news.story_articles,
		news.stories,
		news.article_embeddings,
		news.article_person_identities,
		news.person_identities,
		news.article_tags,
		news.tags,
		news.articles,
		news.raw_arrivals,
		news.source_checkpoints,
		news.ingest_runs
	RESTART IDENTITY CASCADE
	`
	if err := pool.GORM().Exec(q).Error; err != nil {
		t.Fatalf("clean integration db: %v", err)
	}
}

func createPipelineRawArrival(
	t *testing.T,
	pool *db.Pool,
	source string,
	sourceItemID string,
	canonicalURL string,
	title string,
	fetchedAt time.Time,
) {
	t.Helper()

	run := db.IngestRun{Source: source, Status: "completed", CreatedAt: fetchedAt, UpdatedAt: fetchedAt}
	if err := pool.GORM().Create(&run).Error; err != nil {
		t.Fatalf("create ingest run: %v", err)
	}
	payload := []byte(fmt.Sprintf(`{
		"payload_version":"v1",
		"source":%q,
		"source_item_id":%q,
		"title":%q,
		"body_text":"The OpenClaw team shared setup notes and release details for maintainers.",
		"canonical_url":%q,
		"published_at":%q,
		"language":"en",
		"source_metadata":{
			"collection":"openclaw",
			"job_name":"integration",
			"job_run_id":%q,
			"scraped_at":%q
		}
	}`, source, sourceItemID, title, canonicalURL, fetchedAt.Format(time.RFC3339), sourceItemID, fetchedAt.Format(time.RFC3339)))
	hash := sha256.Sum256(payload)
	raw := db.RawArrival{
		RunID:             run.RunID,
		Source:            source,
		SourceItemID:      sourceItemID,
		Collection:        "openclaw",
		SourceItemURL:     &canonicalURL,
		SourcePublishedAt: &fetchedAt,
		FetchedAt:         fetchedAt,
		RawPayload:        payload,
		PayloadHash:       hash[:],
		CreatedAt:         fetchedAt,
	}
	if err := pool.GORM().Create(&raw).Error; err != nil {
		t.Fatalf("create raw arrival: %v", err)
	}
}

func unitVector(offset int) []float64 {
	vector := make([]float64, embeddingVectorDimensions)
	vector[offset%embeddingVectorDimensions] = 1
	return vector
}
