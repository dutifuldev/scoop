package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"horse.fit/scoop/internal/db"
	"horse.fit/scoop/internal/globaltime"
)

func TestServerStoryQueriesIntegration(t *testing.T) {
	pool := newHTTPIntegrationPool(t)
	server := NewServer(pool, zerolog.Nop(), Options{})
	ctx := context.Background()
	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	fixture := seedHTTPStoryFixture(t, pool, now)

	assertHTTPStoryListQuery(t, ctx, server, fixture)
	assertHTTPStoryDetailQuery(t, ctx, server, fixture)
	assertHTTPStoryDaysQuery(t, ctx, server)
	assertHTTPCollectionsAndStatsQueries(t, ctx, server)
}

func assertHTTPStoryListQuery(t *testing.T, ctx context.Context, server *Server, fixture httpStoryFixture) {
	t.Helper()

	total, items, err := server.queryStoryList(ctx, storyListFilter{
		Collection: "openclaw",
		Tag:        "i0",
		From:       timePtr(time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)),
		To:         timePtr(time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)),
		TimeZone:   "UTC",
		Page:       1,
		PageSize:   10,
	})
	if err != nil {
		t.Fatalf("queryStoryList() error = %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("queryStoryList() total=%d len=%d, want one story", total, len(items))
	}
	if items[0].StoryUUID != fixture.story.StoryUUID {
		t.Fatalf("story UUID = %s, want %s", items[0].StoryUUID, fixture.story.StoryUUID)
	}
	if len(items[0].Tags) != 1 || items[0].Tags[0].Tag != "i0" {
		t.Fatalf("story tags = %#v", items[0].Tags)
	}
	if len(items[0].PersonIdentities) != 1 || items[0].PersonIdentities[0].Provider != "github" {
		t.Fatalf("story identities = %#v", items[0].PersonIdentities)
	}
}

func assertHTTPStoryDetailQuery(t *testing.T, ctx context.Context, server *Server, fixture httpStoryFixture) {
	t.Helper()
	detail, err := server.queryStoryDetail(ctx, fixture.story.StoryUUID, "")
	if err != nil {
		t.Fatalf("queryStoryDetail() error = %v", err)
	}
	if detail.Story.ArticleCount != 2 || len(detail.Members) != 2 {
		t.Fatalf("story detail article counts = %d/%d, want 2", detail.Story.ArticleCount, len(detail.Members))
	}
	if detail.Members[0].PublishedAt == nil || detail.Members[0].ArticleUUID != fixture.secondArticle.ArticleUUID {
		t.Fatalf("newest article should be first, got %#v", detail.Members[0])
	}
	if len(detail.Members[1].Tags) != 1 || detail.Members[1].Tags[0].Tag != "i0" {
		t.Fatalf("older member tags = %#v", detail.Members[1].Tags)
	}
	if len(detail.Members[1].PersonIdentities) != 1 || detail.Members[1].PersonIdentities[0].Handle == nil {
		t.Fatalf("older member identities = %#v", detail.Members[1].PersonIdentities)
	}
}

func assertHTTPStoryDaysQuery(t *testing.T, ctx context.Context, server *Server) {
	t.Helper()

	days, err := server.queryStoryDays(ctx, "openclaw", 5, "UTC")
	if err != nil {
		t.Fatalf("queryStoryDays() error = %v", err)
	}
	if len(days) != 1 || days[0].Day != "2026-05-14" || days[0].StoryCount != 1 {
		t.Fatalf("story days = %#v", days)
	}
}

func assertHTTPCollectionsAndStatsQueries(t *testing.T, ctx context.Context, server *Server) {
	t.Helper()

	collections, err := server.queryCollections(ctx)
	if err != nil {
		t.Fatalf("queryCollections() error = %v", err)
	}
	if len(collections) != 1 || collections[0].Collection != "openclaw" || collections[0].Articles != 2 {
		t.Fatalf("collections = %#v", collections)
	}

	stats, err := server.queryStats(ctx)
	if err != nil {
		t.Fatalf("queryStats() error = %v", err)
	}
	if stats.Articles != 2 || stats.Stories != 1 || stats.StoryArticles != 2 {
		t.Fatalf("stats = %#v", stats)
	}
	if stats.DedupDecisions["new_story"] != 1 || stats.DedupDecisions["auto_merge"] != 1 {
		t.Fatalf("dedup decisions = %#v", stats.DedupDecisions)
	}
}

func TestServerMutationHandlersIntegration(t *testing.T) {
	pool := newHTTPIntegrationPool(t)
	server := NewServer(pool, zerolog.Nop(), Options{})
	now := time.Date(2026, 5, 14, 11, 0, 0, 0, time.UTC)
	fixture := seedHTTPStoryFixture(t, pool, now)
	createHTTPUser(t, pool, 7, "admin", now)
	globaltime.SetMockTime(now.Add(time.Hour))
	t.Cleanup(globaltime.ResetTime)

	_, addTagContext, addTagRecorder := newJSONContext(http.MethodPost, "/api/v1/articles/"+fixture.secondArticle.ArticleUUID+"/tags", `{"tag":"i0"}`)
	addTagContext.SetParamNames("article_uuid")
	addTagContext.SetParamValues(fixture.secondArticle.ArticleUUID)
	addTagContext.Set("auth.principal", authPrincipal{UserID: 7, Username: "admin"})
	if err := server.handleAddArticleTag(addTagContext); err != nil {
		t.Fatalf("handleAddArticleTag() error = %v", err)
	}
	assertHTTPStatus(t, addTagRecorder.Code, http.StatusOK)

	_, addPersonContext, addPersonRecorder := newJSONContext(http.MethodPost, "/api/v1/articles/"+fixture.secondArticle.ArticleUUID+"/person-identities", `{"identity_ref":"id://discord/id/42?handle=funcracker"}`)
	addPersonContext.SetParamNames("article_uuid")
	addPersonContext.SetParamValues(fixture.secondArticle.ArticleUUID)
	addPersonContext.Set("auth.principal", authPrincipal{UserID: 7, Username: "admin"})
	if err := server.handleAddArticlePersonIdentity(addPersonContext); err != nil {
		t.Fatalf("handleAddArticlePersonIdentity() error = %v", err)
	}
	assertHTTPStatus(t, addPersonRecorder.Code, http.StatusOK)

	_, updateStoryContext, updateStoryRecorder := newJSONContext(http.MethodPatch, "/api/v1/stories/"+fixture.story.StoryUUID, `{"title":"Updated story","status":"merged"}`)
	updateStoryContext.SetParamNames("story_uuid")
	updateStoryContext.SetParamValues(fixture.story.StoryUUID)
	if err := server.handleUpdateStory(updateStoryContext); err != nil {
		t.Fatalf("handleUpdateStory() error = %v", err)
	}
	assertHTTPStatus(t, updateStoryRecorder.Code, http.StatusOK)

	_, updateArticleContext, updateArticleRecorder := newJSONContext(http.MethodPatch, "/api/v1/articles/"+fixture.secondArticle.ArticleUUID, `{"title":"Updated article","url":"https://github.com/owner/repo/pull/123"}`)
	updateArticleContext.SetParamNames("article_uuid")
	updateArticleContext.SetParamValues(fixture.secondArticle.ArticleUUID)
	if err := server.handleUpdateArticle(updateArticleContext); err != nil {
		t.Fatalf("handleUpdateArticle() error = %v", err)
	}
	assertHTTPStatus(t, updateArticleRecorder.Code, http.StatusOK)

	_, deleteStoryContext, deleteStoryRecorder := newJSONContext(http.MethodDelete, "/api/v1/stories/"+fixture.story.StoryUUID, "")
	deleteStoryContext.SetParamNames("story_uuid")
	deleteStoryContext.SetParamValues(fixture.story.StoryUUID)
	if err := server.handleDeleteStory(deleteStoryContext); err != nil {
		t.Fatalf("handleDeleteStory() error = %v", err)
	}
	assertHTTPStatus(t, deleteStoryRecorder.Code, http.StatusOK)

	_, restoreStoryContext, restoreStoryRecorder := newJSONContext(http.MethodPost, "/api/v1/stories/"+fixture.story.StoryUUID+"/restore", "")
	restoreStoryContext.SetParamNames("story_uuid")
	restoreStoryContext.SetParamValues(fixture.story.StoryUUID)
	if err := server.handleRestoreStory(restoreStoryContext); err != nil {
		t.Fatalf("handleRestoreStory() error = %v", err)
	}
	assertHTTPStatus(t, restoreStoryRecorder.Code, http.StatusOK)

	_, deleteArticleContext, deleteArticleRecorder := newJSONContext(http.MethodDelete, "/api/v1/articles/"+fixture.secondArticle.ArticleUUID, "")
	deleteArticleContext.SetParamNames("article_uuid")
	deleteArticleContext.SetParamValues(fixture.secondArticle.ArticleUUID)
	if err := server.handleDeleteArticle(deleteArticleContext); err != nil {
		t.Fatalf("handleDeleteArticle() error = %v", err)
	}
	assertHTTPStatus(t, deleteArticleRecorder.Code, http.StatusOK)

	_, restoreArticleContext, restoreArticleRecorder := newJSONContext(http.MethodPost, "/api/v1/articles/"+fixture.secondArticle.ArticleUUID+"/restore", "")
	restoreArticleContext.SetParamNames("article_uuid")
	restoreArticleContext.SetParamValues(fixture.secondArticle.ArticleUUID)
	if err := server.handleRestoreArticle(restoreArticleContext); err != nil {
		t.Fatalf("handleRestoreArticle() error = %v", err)
	}
	assertHTTPStatus(t, restoreArticleRecorder.Code, http.StatusOK)

	_, removeTagContext, removeTagRecorder := newJSONContext(http.MethodDelete, "/api/v1/articles/"+fixture.secondArticle.ArticleUUID+"/tags/i0", "")
	removeTagContext.SetParamNames("article_uuid", "tag")
	removeTagContext.SetParamValues(fixture.secondArticle.ArticleUUID, "i0")
	removeTagContext.Set("auth.principal", authPrincipal{UserID: 7, Username: "admin"})
	if err := server.handleRemoveArticleTag(removeTagContext); err != nil {
		t.Fatalf("handleRemoveArticleTag() error = %v", err)
	}
	assertHTTPStatus(t, removeTagRecorder.Code, http.StatusOK)

	_, removePersonContext, removePersonRecorder := newJSONContext(http.MethodDelete, "/api/v1/articles/"+fixture.secondArticle.ArticleUUID+"/person-identities/id://discord/id/42", "")
	removePersonContext.SetParamNames("article_uuid", "person_identity")
	removePersonContext.SetParamValues(fixture.secondArticle.ArticleUUID, "id://discord/id/42")
	removePersonContext.Set("auth.principal", authPrincipal{UserID: 7, Username: "admin"})
	if err := server.handleRemoveArticlePersonIdentity(removePersonContext); err != nil {
		t.Fatalf("handleRemoveArticlePersonIdentity() error = %v", err)
	}
	assertHTTPStatus(t, removePersonRecorder.Code, http.StatusOK)
}

func TestServerReadHandlersIntegration(t *testing.T) {
	pool := newHTTPIntegrationPool(t)
	server := NewServer(pool, zerolog.Nop(), Options{})
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	fixture := seedHTTPStoryFixture(t, pool, now)

	_, storiesContext, storiesRecorder := newJSONContext(http.MethodGet, "/api/v1/stories?collection=openclaw&tag=i0&page=1&page_size=10&from=2026-05-14&to=2026-05-14&tz=UTC", "")
	if err := server.handleStories(storiesContext); err != nil {
		t.Fatalf("handleStories() error = %v", err)
	}
	assertHTTPStatus(t, storiesRecorder.Code, http.StatusOK)

	_, detailContext, detailRecorder := newJSONContext(http.MethodGet, "/api/v1/stories/"+fixture.story.StoryUUID, "")
	detailContext.SetParamNames("story_uuid")
	detailContext.SetParamValues(fixture.story.StoryUUID)
	if err := server.handleStoryDetail(detailContext); err != nil {
		t.Fatalf("handleStoryDetail() error = %v", err)
	}
	assertHTTPStatus(t, detailRecorder.Code, http.StatusOK)

	_, collectionsContext, collectionsRecorder := newJSONContext(http.MethodGet, "/api/v1/collections", "")
	if err := server.handleCollections(collectionsContext); err != nil {
		t.Fatalf("handleCollections() error = %v", err)
	}
	assertHTTPStatus(t, collectionsRecorder.Code, http.StatusOK)

	_, daysContext, daysRecorder := newJSONContext(http.MethodGet, "/api/v1/story-days?collection=openclaw&limit=5&tz=UTC", "")
	if err := server.handleStoryDays(daysContext); err != nil {
		t.Fatalf("handleStoryDays() error = %v", err)
	}
	assertHTTPStatus(t, daysRecorder.Code, http.StatusOK)

	_, statsContext, statsRecorder := newJSONContext(http.MethodGet, "/api/v1/stats", "")
	if err := server.handleStats(statsContext); err != nil {
		t.Fatalf("handleStats() error = %v", err)
	}
	assertHTTPStatus(t, statsRecorder.Code, http.StatusOK)

	_, tagsContext, tagsRecorder := newJSONContext(http.MethodGet, "/api/v1/tags", "")
	if err := server.handleTags(tagsContext); err != nil {
		t.Fatalf("handleTags() error = %v", err)
	}
	assertHTTPStatus(t, tagsRecorder.Code, http.StatusOK)

	_, peopleContext, peopleRecorder := newJSONContext(http.MethodGet, "/api/v1/person-identities?q=octo", "")
	if err := server.handlePersonIdentities(peopleContext); err != nil {
		t.Fatalf("handlePersonIdentities() error = %v", err)
	}
	assertHTTPStatus(t, peopleRecorder.Code, http.StatusOK)

	_, articlePeopleContext, articlePeopleRecorder := newJSONContext(http.MethodGet, "/api/v1/articles/"+fixture.firstArticle.ArticleUUID+"/person-identities", "")
	articlePeopleContext.SetParamNames("article_uuid")
	articlePeopleContext.SetParamValues(fixture.firstArticle.ArticleUUID)
	if err := server.handleArticlePersonIdentities(articlePeopleContext); err != nil {
		t.Fatalf("handleArticlePersonIdentities() error = %v", err)
	}
	assertHTTPStatus(t, articlePeopleRecorder.Code, http.StatusOK)

	if err := pool.GORM().Model(&db.Article{}).
		Where("article_id = ?", fixture.firstArticle.ArticleID).
		Update("source", "discord_archive").Error; err != nil {
		t.Fatalf("update preview article source: %v", err)
	}
	var storyArticle db.StoryArticle
	if err := pool.GORM().
		Where("story_id = ? AND article_id = ?", fixture.story.StoryID, fixture.firstArticle.ArticleID).
		First(&storyArticle).Error; err != nil {
		t.Fatalf("query story article for preview: %v", err)
	}
	preview, err := server.queryStoryArticlePreview(context.Background(), storyArticle.StoryArticleUUID, 200)
	if err != nil {
		t.Fatalf("queryStoryArticlePreview() error = %v", err)
	}
	if preview.PreviewText != "first body" || preview.Source != "normalized_text" {
		t.Fatalf("preview = %#v", preview)
	}

	_, previewContext, previewRecorder := newJSONContext(http.MethodGet, "/api/v1/articles/"+storyArticle.StoryArticleUUID+"/preview?max_chars=200", "")
	previewContext.SetParamNames("story_article_uuid")
	previewContext.SetParamValues(storyArticle.StoryArticleUUID)
	if err := server.handleStoryArticlePreview(previewContext); err != nil {
		t.Fatalf("handleStoryArticlePreview() error = %v", err)
	}
	assertHTTPStatus(t, previewRecorder.Code, http.StatusOK)
}

func TestServerDatabaseBackedErrorResponsesIntegration(t *testing.T) {
	pool := newHTTPIntegrationPool(t)
	server := NewServer(pool, zerolog.Nop(), Options{})

	_, healthContext, healthRecorder := newJSONContext(http.MethodGet, "/api/v1/health", "")
	if err := server.handleHealth(healthContext); err != nil {
		t.Fatalf("handleHealth() error = %v", err)
	}
	assertHTTPStatus(t, healthRecorder.Code, http.StatusOK)

	_, detailContext, detailRecorder := newJSONContext(http.MethodGet, "/api/v1/stories/00000000-0000-4000-8000-ffffffffffff", "")
	detailContext.SetParamNames("story_uuid")
	detailContext.SetParamValues("00000000-0000-4000-8000-ffffffffffff")
	if err := server.handleStoryDetail(detailContext); err != nil {
		t.Fatalf("handleStoryDetail(missing) error = %v", err)
	}
	assertHTTPStatus(t, detailRecorder.Code, http.StatusNotFound)

	_, updateContext, updateRecorder := newJSONContext(http.MethodPatch, "/api/v1/stories/00000000-0000-4000-8000-ffffffffffff", `{"title":"missing"}`)
	updateContext.SetParamNames("story_uuid")
	updateContext.SetParamValues("00000000-0000-4000-8000-ffffffffffff")
	if err := server.handleUpdateStory(updateContext); err != nil {
		t.Fatalf("handleUpdateStory(missing) error = %v", err)
	}
	assertHTTPStatus(t, updateRecorder.Code, http.StatusNotFound)

	_, articleUpdateContext, articleUpdateRecorder := newJSONContext(http.MethodPatch, "/api/v1/articles/00000000-0000-4000-8000-ffffffffffff", `{"title":"missing"}`)
	articleUpdateContext.SetParamNames("article_uuid")
	articleUpdateContext.SetParamValues("00000000-0000-4000-8000-ffffffffffff")
	if err := server.handleUpdateArticle(articleUpdateContext); err != nil {
		t.Fatalf("handleUpdateArticle(missing) error = %v", err)
	}
	assertHTTPStatus(t, articleUpdateRecorder.Code, http.StatusNotFound)

	_, settingsContext, settingsRecorder := newJSONContext(http.MethodPatch, "/api/v1/collections/openclaw/settings", `{"translation_mode":"enabled"}`)
	settingsContext.SetParamNames("collection")
	settingsContext.SetParamValues("openclaw")
	if err := server.handleUpdateCollectionSettings(settingsContext); err != nil {
		t.Fatalf("handleUpdateCollectionSettings() error = %v", err)
	}
	assertHTTPStatus(t, settingsRecorder.Code, http.StatusOK)
}

type httpStoryFixture struct {
	story         db.Story
	firstArticle  db.Article
	secondArticle db.Article
}

func newHTTPIntegrationPool(t *testing.T) *db.Pool {
	t.Helper()

	databaseURL := strings.TrimSpace(os.Getenv("SCOOP_TEST_DATABASE_URL"))
	if databaseURL == "" {
		databaseURL = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if databaseURL == "" {
		t.Skip("set SCOOP_TEST_DATABASE_URL or DATABASE_URL to run httpapi integration tests")
	}
	databaseURL = ensureHTTPTestDatabase(t, databaseURL)

	pool, err := db.NewIntegrationTestPool(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("NewPool() error = %v", err)
	}
	lockHTTPIntegrationDB(t, pool)
	t.Cleanup(func() {
		cleanHTTPIntegrationDB(t, pool)
		unlockHTTPIntegrationDB(t, pool)
		if err := pool.Close(); err != nil {
			t.Fatalf("close pool: %v", err)
		}
	})
	cleanHTTPIntegrationDB(t, pool)
	return pool
}

func ensureHTTPTestDatabase(t *testing.T, databaseURL string) string {
	t.Helper()

	targetURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse database URL: %v", err)
	}
	dbName := strings.TrimPrefix(targetURL.Path, "/")
	if dbName == "" {
		t.Fatalf("database URL must include a database name")
	}
	targetName := dbName + "_httpapi_test"
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

func lockHTTPIntegrationDB(t *testing.T, pool *db.Pool) {
	t.Helper()
	if err := pool.GORM().Exec("SELECT pg_advisory_lock(54718802)").Error; err != nil {
		t.Fatalf("lock integration db: %v", err)
	}
}

func unlockHTTPIntegrationDB(t *testing.T, pool *db.Pool) {
	t.Helper()
	if err := pool.GORM().Exec("SELECT pg_advisory_unlock(54718802)").Error; err != nil {
		t.Fatalf("unlock integration db: %v", err)
	}
}

func cleanHTTPIntegrationDB(t *testing.T, pool *db.Pool) {
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

func seedHTTPStoryFixture(t *testing.T, pool *db.Pool, now time.Time) httpStoryFixture {
	t.Helper()

	firstArticle := createHTTPArticle(t, pool, "00000000-0000-4000-8000-000000000601", "first article", "first body", now.Add(-2*time.Hour))
	secondArticle := createHTTPArticle(t, pool, "00000000-0000-4000-8000-000000000602", "second article", "second body", now.Add(-time.Hour))
	story := db.Story{
		StoryUUID:               "00000000-0000-4000-8000-000000000603",
		CanonicalTitle:          "Story title",
		Collection:              "openclaw",
		RepresentativeArticleID: &secondArticle.ArticleID,
		FirstSeenAt:             now.Add(-2 * time.Hour),
		LastSeenAt:              now,
		Status:                  "active",
		CreatedAt:               now.Add(-2 * time.Hour),
		UpdatedAt:               now,
	}
	if err := pool.GORM().Create(&story).Error; err != nil {
		t.Fatalf("create story: %v", err)
	}
	createHTTPStoryArticle(t, pool, story.StoryID, firstArticle.ArticleID, "seed", now.Add(-2*time.Hour))
	createHTTPStoryArticle(t, pool, story.StoryID, secondArticle.ArticleID, "exact_url", now.Add(-time.Hour))
	createHTTPDedupEvent(t, pool, firstArticle.ArticleID, story.StoryID, "new_story", nil, now.Add(-2*time.Hour))
	signal := "exact_url"
	createHTTPDedupEvent(t, pool, secondArticle.ArticleID, story.StoryID, "auto_merge", &signal, now.Add(-time.Hour))

	tagColor := "#ff4d4f"
	tagHighlight := "#fff2b8"
	if _, err := pool.CreateTag(context.Background(), db.UpsertTagOptions{
		Slug:           "i0",
		Color:          &tagColor,
		HighlightColor: &tagHighlight,
	}, now); err != nil {
		t.Fatalf("create tag: %v", err)
	}
	if err := pool.AddArticleTag(context.Background(), firstArticle.ArticleUUID, "i0", nil, now); err != nil {
		t.Fatalf("add article tag: %v", err)
	}
	if _, err := pool.AddArticlePersonIdentity(context.Background(), firstArticle.ArticleUUID, "id://github/handle/octocat", nil, now); err != nil {
		t.Fatalf("add article person identity: %v", err)
	}
	return httpStoryFixture{
		story:         story,
		firstArticle:  firstArticle,
		secondArticle: secondArticle,
	}
}

func createHTTPArticle(t *testing.T, pool *db.Pool, articleUUID string, title string, body string, publishedAt time.Time) db.Article {
	t.Helper()

	run := db.IngestRun{Source: "integration", Status: "completed", CreatedAt: publishedAt, UpdatedAt: publishedAt}
	if err := pool.GORM().Create(&run).Error; err != nil {
		t.Fatalf("create ingest run: %v", err)
	}
	payloadHash := sha256.Sum256([]byte(articleUUID))
	sourceURL := "https://example.com/" + articleUUID
	raw := db.RawArrival{
		RunID:             run.RunID,
		Source:            "integration",
		SourceItemID:      articleUUID,
		Collection:        "openclaw",
		SourceItemURL:     &sourceURL,
		SourcePublishedAt: &publishedAt,
		RawPayload:        []byte(`{"title":"` + title + `"}`),
		PayloadHash:       payloadHash[:],
		FetchedAt:         publishedAt,
		CreatedAt:         publishedAt,
	}
	if err := pool.GORM().Create(&raw).Error; err != nil {
		t.Fatalf("create raw arrival: %v", err)
	}
	contentHash := sha256.Sum256([]byte(title + "\n" + body))
	sourceDomain := "example.com"
	article := db.Article{
		ArticleUUID:        articleUUID,
		RawArrivalID:       raw.RawArrivalID,
		Source:             "integration",
		SourceItemID:       articleUUID,
		Collection:         "openclaw",
		CanonicalURL:       &sourceURL,
		NormalizedTitle:    title,
		NormalizedText:     body,
		NormalizedLanguage: "en",
		PublishedAt:        &publishedAt,
		SourceDomain:       &sourceDomain,
		ContentHash:        contentHash[:],
		TokenCount:         4,
		CreatedAt:          publishedAt,
		UpdatedAt:          publishedAt,
	}
	if err := pool.GORM().Create(&article).Error; err != nil {
		t.Fatalf("create article: %v", err)
	}
	return article
}

func createHTTPStoryArticle(t *testing.T, pool *db.Pool, storyID int64, articleID int64, matchType string, matchedAt time.Time) {
	t.Helper()

	score := 1.0
	link := db.StoryArticle{
		StoryID:      storyID,
		ArticleID:    articleID,
		MatchType:    matchType,
		MatchScore:   &score,
		MatchDetails: json.RawMessage(`{"source":"integration"}`),
		MatchedAt:    matchedAt,
	}
	if err := pool.GORM().Create(&link).Error; err != nil {
		t.Fatalf("create story article: %v", err)
	}
}

func createHTTPDedupEvent(t *testing.T, pool *db.Pool, articleID int64, storyID int64, decision string, signal *string, createdAt time.Time) {
	t.Helper()

	event := db.DedupEvent{
		ArticleID:     articleID,
		Decision:      decision,
		ChosenStoryID: &storyID,
		ExactSignal:   signal,
		CreatedAt:     createdAt,
	}
	if err := pool.GORM().Create(&event).Error; err != nil {
		t.Fatalf("create dedup event: %v", err)
	}
}

func createHTTPUser(t *testing.T, pool *db.Pool, userID int64, username string, createdAt time.Time) {
	t.Helper()

	user := db.User{
		UserID:       userID,
		Username:     username,
		PasswordHash: "hash",
		CreatedAt:    createdAt,
	}
	if err := pool.GORM().Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
}

func timePtr(value time.Time) *time.Time {
	return &value
}

func assertHTTPStatus(t *testing.T, got int, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("HTTP status = %d, want %d", got, want)
	}
}
