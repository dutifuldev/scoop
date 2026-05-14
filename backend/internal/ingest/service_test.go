package ingest

import (
	"context"
	"encoding/json"
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

func TestServiceIngestOneIntegration(t *testing.T) {
	pool := newIngestIntegrationPool(t)
	service := NewService(pool, zerolog.Nop())
	publishedAt := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	req := Request{
		Source:            "discord_archive",
		SourceItemID:      "item-1",
		Collection:        " OpenClaw ",
		SourceItemURL:     "https://example.com/item-1",
		SourcePublishedAt: &publishedAt,
		RawPayload:        json.RawMessage(`{"title":"OpenClaw","source":"discord_archive"}`),
		CursorCheckpoint:  json.RawMessage(`{"cursor":"item-1"}`),
		TriggeredByTopic:  "openclaw",
		ResponseHeaders:   json.RawMessage(`{"etag":"abc"}`),
	}

	first, err := service.IngestOne(context.Background(), req)
	if err != nil {
		t.Fatalf("IngestOne() error = %v", err)
	}
	if !first.Inserted || first.RawArrivalID == nil || first.RawArrivalUUID == nil || first.Status != "completed" {
		t.Fatalf("first result = %#v", first)
	}

	second, err := service.IngestOne(context.Background(), req)
	if err != nil {
		t.Fatalf("second IngestOne() error = %v", err)
	}
	if second.Inserted || second.RawArrivalID != nil || second.RawArrivalUUID != nil {
		t.Fatalf("duplicate result = %#v, want no inserted raw arrival", second)
	}

	var runCount int64
	if err := pool.GORM().Model(&db.IngestRun{}).Count(&runCount).Error; err != nil {
		t.Fatalf("count ingest runs: %v", err)
	}
	if runCount != 2 {
		t.Fatalf("run count = %d, want 2", runCount)
	}
	var checkpoint db.SourceCheckpoint
	if err := pool.GORM().Where("source = ?", "discord_archive").First(&checkpoint).Error; err != nil {
		t.Fatalf("query checkpoint: %v", err)
	}
	var checkpointValue map[string]string
	if err := json.Unmarshal(checkpoint.CursorCheckpoint, &checkpointValue); err != nil {
		t.Fatalf("decode checkpoint: %v", err)
	}
	if checkpointValue["cursor"] != "item-1" {
		t.Fatalf("checkpoint = %s", checkpoint.CursorCheckpoint)
	}
}

func TestServiceIngestOneValidation(t *testing.T) {
	t.Parallel()

	service := NewService(&db.Pool{}, zerolog.Nop())
	validPayload := json.RawMessage(`{"title":"OpenClaw"}`)
	tests := []struct {
		name string
		req  Request
		want string
	}{
		{name: "nil service", req: Request{}, want: "ingest service is not initialized"},
		{name: "missing source", req: Request{SourceItemID: "1", Collection: "openclaw", RawPayload: validPayload}, want: "source is required"},
		{name: "missing item", req: Request{Source: "discord", Collection: "openclaw", RawPayload: validPayload}, want: "source_item_id is required"},
		{name: "missing collection", req: Request{Source: "discord", SourceItemID: "1", RawPayload: validPayload}, want: "collection is required"},
		{name: "invalid payload", req: Request{Source: "discord", SourceItemID: "1", Collection: "openclaw", RawPayload: json.RawMessage(`{`)}, want: "canonicalize raw payload"},
	}

	if _, err := (*Service)(nil).IngestOne(context.Background(), Request{}); err == nil || err.Error() != tests[0].want {
		t.Fatalf("nil service error = %v", err)
	}
	for _, tt := range tests[1:] {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.IngestOne(context.Background(), tt.req)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestCanonicalizeJSONRejectsEmptyAndTrailingData(t *testing.T) {
	t.Parallel()

	if _, err := canonicalizeJSON(nil); err == nil {
		t.Fatal("canonicalizeJSON(nil) error = nil")
	}
	if _, err := canonicalizeJSON([]byte(`{} {}`)); err == nil {
		t.Fatal("canonicalizeJSON(trailing) error = nil")
	}
	got, err := canonicalizeJSON([]byte(`{"b":2,"a":1}`))
	if err != nil {
		t.Fatalf("canonicalizeJSON() error = %v", err)
	}
	if string(got) != `{"a":1,"b":2}` {
		t.Fatalf("canonical JSON = %s", got)
	}
}

func TestNormalizeRequestDefaultsCheckpointAndOptionalFields(t *testing.T) {
	t.Parallel()

	service := NewService(&db.Pool{}, zerolog.Nop())
	sourcePublishedAt := time.Date(2026, 5, 14, 12, 0, 0, 0, time.FixedZone("offset", 3600))
	req := Request{
		Source:            " discord ",
		SourceItemID:      " item-1 ",
		Collection:        " OpenClaw ",
		SourceItemURL:     " https://example.com/item ",
		SourcePublishedAt: &sourcePublishedAt,
		RawPayload:        json.RawMessage(`{"b":2,"a":1}`),
		TriggeredByTopic:  " openclaw ",
		ResponseHeaders:   json.RawMessage(`{"etag":"abc"}`),
	}
	got, err := service.normalizeRequest(req)
	if err != nil {
		t.Fatalf("normalizeRequest() error = %v", err)
	}
	if got.source != "discord" || got.sourceItemID != "item-1" || got.collection != "openclaw" {
		t.Fatalf("normalized identity = %#v", got)
	}
	if got.cursorCheckpoint != `{"last_source_item_id":"item-1"}` {
		t.Fatalf("checkpoint = %s", got.cursorCheckpoint)
	}
	if got.sourcePublishedAt == nil || got.sourcePublishedAt.Location() != time.UTC {
		t.Fatalf("sourcePublishedAt = %v, want UTC", got.sourcePublishedAt)
	}
	if got.sourceItemURL == nil || *got.sourceItemURL != "https://example.com/item" {
		t.Fatalf("source item url = %v", got.sourceItemURL)
	}
	if got.triggeredByTopic == nil || *got.triggeredByTopic != "openclaw" {
		t.Fatalf("triggered topic = %v", got.triggeredByTopic)
	}
	if got.responseHeaders == nil || *got.responseHeaders != `{"etag":"abc"}` {
		t.Fatalf("response headers = %v", got.responseHeaders)
	}
}

func TestNormalizeNullableHelpers(t *testing.T) {
	t.Parallel()

	if normalizeNullableString(" ") != nil || normalizeNullableString(" x ") == nil {
		t.Fatalf("normalizeNullableString should only keep non-empty values")
	}
	if normalizeNullableTime(nil) != nil {
		t.Fatalf("normalizeNullableTime(nil) should be nil")
	}
	if normalizeNullableJSON(nil) != nil || normalizeNullableJSON(json.RawMessage("  ")) != nil {
		t.Fatalf("normalizeNullableJSON should ignore empty JSON")
	}
	if got := normalizeNullableJSON(json.RawMessage(`{"ok":true}`)); got == nil || *got != `{"ok":true}` {
		t.Fatalf("normalizeNullableJSON = %v", got)
	}
}

func newIngestIntegrationPool(t *testing.T) *db.Pool {
	t.Helper()

	databaseURL := strings.TrimSpace(os.Getenv("SCOOP_TEST_DATABASE_URL"))
	if databaseURL == "" {
		databaseURL = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if databaseURL == "" {
		t.Skip("set SCOOP_TEST_DATABASE_URL or DATABASE_URL to run ingest integration tests")
	}
	databaseURL = ensureIngestTestDatabase(t, databaseURL)
	pool, err := db.NewIntegrationTestPool(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("NewPool() error = %v", err)
	}
	lockIngestIntegrationDB(t, pool)
	t.Cleanup(func() {
		cleanIngestIntegrationDB(t, pool)
		unlockIngestIntegrationDB(t, pool)
		if err := pool.Close(); err != nil {
			t.Fatalf("close pool: %v", err)
		}
	})
	cleanIngestIntegrationDB(t, pool)
	return pool
}

func ensureIngestTestDatabase(t *testing.T, databaseURL string) string {
	t.Helper()

	targetURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse database URL: %v", err)
	}
	dbName := strings.TrimPrefix(targetURL.Path, "/")
	if dbName == "" {
		t.Fatalf("database URL must include a database name")
	}
	targetName := dbName + "_ingest_test"
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

func lockIngestIntegrationDB(t *testing.T, pool *db.Pool) {
	t.Helper()
	if err := pool.GORM().Exec("SELECT pg_advisory_lock(54718804)").Error; err != nil {
		t.Fatalf("lock integration db: %v", err)
	}
}

func unlockIngestIntegrationDB(t *testing.T, pool *db.Pool) {
	t.Helper()
	if err := pool.GORM().Exec("SELECT pg_advisory_unlock(54718804)").Error; err != nil {
		t.Fatalf("unlock integration db: %v", err)
	}
}

func cleanIngestIntegrationDB(t *testing.T, pool *db.Pool) {
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
