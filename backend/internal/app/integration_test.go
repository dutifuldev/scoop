package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"horse.fit/scoop/internal/config"
	"horse.fit/scoop/internal/db"
	"horse.fit/scoop/internal/globaltime"
)

func TestCLIReadAndMutationCommandsIntegration(t *testing.T) {
	pool, databaseURL := newAppIntegrationPool(t)
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	fixture := seedAppStoryFixture(t, pool, now)
	configureAppCommandEnv(t, databaseURL)
	validateDir := createAppValidateDir(t)
	embeddingServer := newAppEmbeddingServer(t)
	defer embeddingServer.Close()
	avatarServer := newAppAvatarServer(t)
	defer avatarServer.Close()
	originalResolver := defaultAvatarResolver
	defaultAvatarResolver = avatarResolver{
		httpClient:        avatarServer.Client(),
		discordAPIBaseURL: avatarServer.URL,
		githubAPIBaseURL:  avatarServer.URL,
	}
	t.Cleanup(func() {
		defaultAvatarResolver = originalResolver
	})
	globaltime.SetMockTime(now)
	t.Cleanup(globaltime.ResetTime)

	successCommands := [][]string{
		{"health", "--timeout", "5s"},
		{"stats", "--format", "json"},
		{"collections", "--format", "json"},
		{"stories", "--collection", "openclaw", "--from", "2026-05-14", "--to", "2026-05-14", "--format", "json"},
		{"story", "--format", "json", fixture.story.StoryUUID},
		{"search", "--query", "Story", "--collection", "openclaw", "--format", "json"},
		{"articles", "list", "--collection", "openclaw", "--from", "2026-05-14", "--to", "2026-05-14", "--format", "json"},
		{"digest", "--collection", "openclaw", "--date", "2026-05-14", "--format", "json"},
		{"tags", "list", "--format", "json"},
		{"tags", "create", "temp", "--description", "temporary", "--color", "#ff0000", "--highlight-color", "#ffff99", "--format", "json"},
		{"tags", "update", "temp", "--description", "updated", "--color", "#00ff00", "--format", "json"},
		{"tags", "archive", "temp", "--format", "json"},
		{"tags", "unarchive", "temp", "--format", "json"},
		{"tags", "rename", "temp", "temp2", "--format", "json"},
		{"tags", "delete", "temp2"},
		{"person-identities", "list", "--format", "json"},
		{"person-identities", "show", "id://github/handle/octocat", "--format", "json"},
		{"person-identities", "refresh-avatar", "id://github/handle/octocat", "--format", "json"},
		{"person-identities", "refresh-avatars", "--provider", "github", "--format", "json"},
		{"person-identities", "archive", "id://github/handle/octocat", "--format", "json"},
		{"person-identities", "unarchive", "id://github/handle/octocat", "--format", "json"},
		{"articles", "list-people", fixture.firstArticle.ArticleUUID, "--format", "json"},
		{"update", "story", "--title", "Updated story", "--dry-run", fixture.story.StoryUUID},
		{"update", "article", "--title", "Updated article", "--dry-run", fixture.firstArticle.ArticleUUID},
		{"delete", "story", "--dry-run", "--force", fixture.story.StoryUUID},
		{"delete", "article", "--dry-run", "--force", fixture.firstArticle.ArticleUUID},
		{"delete", "collection", "--dry-run", "--force", "openclaw"},
		{"delete", "before", "--dry-run", "--force", "2026-05-15"},
		{"restore", "story", "--dry-run", "--force", fixture.story.StoryUUID},
		{"restore", "article", "--dry-run", "--force", fixture.firstArticle.ArticleUUID},
		{"translate", "story", "--lang", "en", "--dry-run", fixture.story.StoryUUID},
		{"translate", "article", "--lang", "en", "--dry-run", fixture.firstArticle.ArticleUUID},
		{"translate", "collection", "--lang", "en", "--dry-run", "openclaw"},
		{"ingest", "--payload", `{"payload_version":"v1","source":"manual_cli","source_item_id":"manual-2","title":"manual ingest event","body_text":"manual body","canonical_url":"https://example.com/manual-2","published_at":"2026-05-14T12:30:00Z","language":"en","source_metadata":{"collection":"openclaw","job_name":"manual_cli","job_run_id":"manual-2","scraped_at":"2026-05-14T12:31:00Z"}}`, "--checkpoint", `{"cursor":"manual-2"}`},
		{"validate", "--dir", validateDir},
		{"normalize", "--limit", "1"},
		{"embed", "--limit", "1", "--endpoint", embeddingServer.URL},
		{"dedup", "--limit", "1"},
		{"process", "--normalize-limit", "1", "--embed-limit", "1", "--embed-endpoint", embeddingServer.URL, "--dedup-limit", "1", "--until-empty=false"},
	}
	for _, args := range successCommands {
		code, stdout, stderr := runAppCommand(t, args...)
		if code != 0 {
			t.Fatalf("Run(%q) code=%d stdout=%q stderr=%q", strings.Join(args, " "), code, stdout, stderr)
		}
	}

	code, _, stderr := runAppCommand(t, "tags", "add-article", fixture.secondArticle.ArticleUUID, "i0")
	if code != 0 {
		t.Fatalf("tags add-article failed: %s", stderr)
	}
	code, _, stderr = runAppCommand(t, "tags", "remove-article", fixture.secondArticle.ArticleUUID, "i0")
	if code != 0 {
		t.Fatalf("tags remove-article failed: %s", stderr)
	}
	code, _, stderr = runAppCommand(t, "articles", "add-person", fixture.secondArticle.ArticleUUID, "id://discord/id/42?handle=funcracker")
	if code != 0 {
		t.Fatalf("articles add-person failed: %s", stderr)
	}
	code, _, stderr = runAppCommand(t, "articles", "remove-person", fixture.secondArticle.ArticleUUID, "id://discord/id/42")
	if code != 0 {
		t.Fatalf("articles remove-person failed: %s", stderr)
	}

	code, _, stderr = runAppCommand(t, "update", "story", "--status", "merged", fixture.story.StoryUUID)
	if code != 0 {
		t.Fatalf("update story failed: %s", stderr)
	}
	code, _, stderr = runAppCommand(t, "update", "article", "--source", "discord_archive", fixture.firstArticle.ArticleUUID)
	if code != 0 {
		t.Fatalf("update article failed: %s", stderr)
	}
	code, _, stderr = runAppCommand(t, "delete", "story", "--force", fixture.story.StoryUUID)
	if code != 0 {
		t.Fatalf("delete story failed: %s", stderr)
	}
	code, _, stderr = runAppCommand(t, "restore", "story", "--force", fixture.story.StoryUUID)
	if code != 0 {
		t.Fatalf("restore story failed: %s", stderr)
	}
	code, _, stderr = runAppCommand(t, "delete", "article", "--force", fixture.secondArticle.ArticleUUID)
	if code != 0 {
		t.Fatalf("delete article failed: %s", stderr)
	}
	code, _, stderr = runAppCommand(t, "restore", "article", "--force", fixture.secondArticle.ArticleUUID)
	if code != 0 {
		t.Fatalf("restore article failed: %s", stderr)
	}
	code, _, stderr = runAppCommand(t, "delete", "collection", "--force", "openclaw")
	if code != 0 {
		t.Fatalf("delete collection failed: %s", stderr)
	}
	code, _, stderr = runAppCommand(t, "delete", "before", "--force", "2026-05-16")
	if code != 0 {
		t.Fatalf("delete before failed: %s", stderr)
	}
}

func TestCLIValidationFailuresDoNotConnectToDatabase(t *testing.T) {
	tests := [][]string{
		{"unknown"},
		{"stories", "--limit", "0"},
		{"search", "--query", ""},
		{"digest"},
		{"articles", "missing"},
		{"tags", "create", "bad tag"},
		{"person-identities", "show"},
		{"ingest", "--payload", "{}"},
		{"update", "story", "--source", "discord", "story-uuid"},
		{"restore"},
		{"delete"},
		{"daemon", "missing"},
		{"daemon", "install", "--backend-port", "0"},
		{"daemon", "status", "extra"},
		{"serve", "--port", "0"},
		{"normalize", "--limit", "0"},
		{"embed", "--limit", "0"},
		{"dedup", "--lookback-days", "0"},
		{"process", "--max-cycles", "0"},
	}
	for _, args := range tests {
		code, _, _ := runAppCommand(t, args...)
		if code == 0 {
			t.Fatalf("Run(%q) code=0, want validation failure", strings.Join(args, " "))
		}
	}
}

func TestCLIDatabaseBackedFailurePathsIntegration(t *testing.T) {
	_, databaseURL := newAppIntegrationPool(t)
	configureAppCommandEnv(t, databaseURL)

	cases := [][]string{
		{"story", "00000000-0000-4000-8000-ffffffffffff"},
		{"update", "story", "--title", "missing", "00000000-0000-4000-8000-ffffffffffff"},
		{"update", "article", "--title", "missing", "00000000-0000-4000-8000-ffffffffffff"},
		{"tags", "archive", "missing-tag"},
		{"tags", "delete", "missing-tag"},
		{"tags", "add-article", "00000000-0000-4000-8000-ffffffffffff", "missing-tag"},
		{"person-identities", "show", "id://github/handle/missing"},
		{"person-identities", "archive", "id://github/handle/missing"},
	}
	for _, args := range cases {
		code, stdout, stderr := runAppCommand(t, args...)
		if code == 0 {
			t.Fatalf("Run(%q) code=0 stdout=%q stderr=%q, want failure", strings.Join(args, " "), stdout, stderr)
		}
	}
}

func TestEnsureDefaultAdminIntegration(t *testing.T) {
	pool, _ := newAppIntegrationPool(t)
	cfg := &config.Config{
		DatabaseURL:                    "unused",
		DefaultAdminUser:               " AdminUser ",
		DefaultAdminPassword:           "secret",
		DefaultAdminMustChangePassword: true,
		SessionTTLHours:                1,
		SessionCookieName:              "scoop_session",
	}

	if err := ensureDefaultAdmin(context.Background(), pool, cfg, zerolog.Nop()); err != nil {
		t.Fatalf("ensureDefaultAdmin() error = %v", err)
	}
	user, err := pool.GetUserByUsername(context.Background(), "adminuser")
	if err != nil {
		t.Fatalf("GetUserByUsername() error = %v", err)
	}
	if !user.MustChangePassword {
		t.Fatalf("MustChangePassword = false, want true")
	}

	cfg.DefaultAdminUser = "other"
	if err := ensureDefaultAdmin(context.Background(), pool, cfg, zerolog.Nop()); err != nil {
		t.Fatalf("second ensureDefaultAdmin() error = %v", err)
	}
	count, err := pool.CountUsers(context.Background())
	if err != nil {
		t.Fatalf("CountUsers() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("user count = %d, want 1", count)
	}
}

func TestEnsureDefaultAdminRejectsMissingDependencies(t *testing.T) {
	t.Parallel()

	if err := ensureDefaultAdmin(context.Background(), nil, &config.Config{}, zerolog.Nop()); err == nil {
		t.Fatal("ensureDefaultAdmin(nil pool) error = nil")
	}
	if err := ensureDefaultAdmin(context.Background(), &db.Pool{}, nil, zerolog.Nop()); err == nil {
		t.Fatal("ensureDefaultAdmin(nil cfg) error = nil")
	}
}

type appStoryFixture struct {
	story         db.Story
	firstArticle  db.Article
	secondArticle db.Article
}

func newAppIntegrationPool(t *testing.T) (*db.Pool, string) {
	t.Helper()

	databaseURL := strings.TrimSpace(os.Getenv("SCOOP_TEST_DATABASE_URL"))
	if databaseURL == "" {
		databaseURL = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if databaseURL == "" {
		t.Skip("set SCOOP_TEST_DATABASE_URL or DATABASE_URL to run app integration tests")
	}
	databaseURL = ensureAppTestDatabase(t, databaseURL)
	pool, err := db.NewPool(context.Background(), &config.Config{
		DatabaseURL:       databaseURL,
		DBMinConns:        0,
		DBMaxConns:        1,
		Environment:       "test",
		LogLevel:          "silent",
		DefaultAdminUser:  "admin",
		SessionTTLHours:   1,
		SessionCookieName: "scoop_session",
	})
	if err != nil {
		t.Fatalf("NewPool() error = %v", err)
	}
	lockAppIntegrationDB(t, pool)
	t.Cleanup(func() {
		cleanAppIntegrationDB(t, pool)
		unlockAppIntegrationDB(t, pool)
		if err := pool.Close(); err != nil {
			t.Fatalf("close pool: %v", err)
		}
	})
	cleanAppIntegrationDB(t, pool)
	return pool, databaseURL
}

func ensureAppTestDatabase(t *testing.T, databaseURL string) string {
	t.Helper()

	targetURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse database URL: %v", err)
	}
	dbName := strings.TrimPrefix(targetURL.Path, "/")
	if dbName == "" {
		t.Fatalf("database URL must include a database name")
	}
	targetName := dbName + "_app_test"
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

func configureAppCommandEnv(t *testing.T, databaseURL string) {
	t.Helper()
	t.Setenv("DATABASE_URL", databaseURL)
	t.Setenv("ENVIRONMENT", "test")
	t.Setenv("LOG_LEVEL", "error")
	t.Setenv("NP_DB_MIN_CONNS", "0")
	t.Setenv("NP_DB_MAX_CONNS", "1")
	t.Setenv("DEFAULT_ADMIN_USER", "admin")
	t.Setenv("SESSION_TTL_HOURS", "1")
	t.Setenv("SESSION_COOKIE_NAME", "scoop_session")
}

func lockAppIntegrationDB(t *testing.T, pool *db.Pool) {
	t.Helper()
	if err := pool.GORM().Exec("SELECT pg_advisory_lock(54718803)").Error; err != nil {
		t.Fatalf("lock integration db: %v", err)
	}
}

func unlockAppIntegrationDB(t *testing.T, pool *db.Pool) {
	t.Helper()
	if err := pool.GORM().Exec("SELECT pg_advisory_unlock(54718803)").Error; err != nil {
		t.Fatalf("unlock integration db: %v", err)
	}
}

func cleanAppIntegrationDB(t *testing.T, pool *db.Pool) {
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

func seedAppStoryFixture(t *testing.T, pool *db.Pool, now time.Time) appStoryFixture {
	t.Helper()

	firstArticle := createAppArticle(t, pool, "00000000-0000-4000-8000-000000000701", "Story alpha", "alpha body", now.Add(-2*time.Hour))
	secondArticle := createAppArticle(t, pool, "00000000-0000-4000-8000-000000000702", "Story beta", "beta body", now.Add(-time.Hour))
	story := db.Story{
		StoryUUID:               "00000000-0000-4000-8000-000000000703",
		CanonicalTitle:          "Story alpha cluster",
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
	createAppStoryArticle(t, pool, story.StoryID, firstArticle.ArticleID, "seed", now.Add(-2*time.Hour))
	createAppStoryArticle(t, pool, story.StoryID, secondArticle.ArticleID, "exact_url", now.Add(-time.Hour))
	createAppDedupEvent(t, pool, firstArticle.ArticleID, story.StoryID, "new_story", nil, now.Add(-2*time.Hour))
	signal := "exact_url"
	createAppDedupEvent(t, pool, secondArticle.ArticleID, story.StoryID, "auto_merge", &signal, now.Add(-time.Hour))

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
	if _, err := pool.UpsertCollectionTranslationMode(context.Background(), "openclaw", "enabled"); err != nil {
		t.Fatalf("enable collection translation: %v", err)
	}
	return appStoryFixture{story: story, firstArticle: firstArticle, secondArticle: secondArticle}
}

func createAppArticle(t *testing.T, pool *db.Pool, articleUUID string, title string, body string, publishedAt time.Time) db.Article {
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
		RawPayload:        json.RawMessage(`{"title":"` + title + `"}`),
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

func createAppStoryArticle(t *testing.T, pool *db.Pool, storyID int64, articleID int64, matchType string, matchedAt time.Time) {
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

func createAppDedupEvent(t *testing.T, pool *db.Pool, articleID int64, storyID int64, decision string, signal *string, createdAt time.Time) {
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

func newAppEmbeddingServer(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("embedding method = %s, want POST", r.Method)
		}
		vector := make([]float64, 4096)
		vector[0] = 1
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"embeddings": [][]float64{vector},
		}); err != nil {
			t.Fatalf("encode embedding response: %v", err)
		}
	}))
}

func newAppAvatarServer(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/users/octocat"):
			_, _ = w.Write([]byte(`{"avatar_url":"https://avatars.githubusercontent.com/u/583231?v=4"}`))
		case strings.HasPrefix(r.URL.Path, "/users/"):
			_, _ = w.Write([]byte(`{"id":"42","avatar":"abc123"}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func createAppValidateDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	payload := `{"payload_version":"v1","source":"manual_cli","source_item_id":"manual-validate","title":"manual validate event","body_text":"manual body","canonical_url":"https://example.com/manual-validate","published_at":"2026-05-14T12:30:00Z","language":"en","source_metadata":{"collection":"openclaw","job_name":"manual_cli","job_run_id":"manual-validate","scraped_at":"2026-05-14T12:31:00Z"}}`
	if err := os.WriteFile(filepath.Join(dir, "item.json"), []byte(payload), 0o644); err != nil {
		t.Fatalf("write validation fixture: %v", err)
	}
	return dir
}

func runAppCommand(t *testing.T, args ...string) (int, string, string) {
	t.Helper()

	originalStdout := os.Stdout
	originalStderr := os.Stderr
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	os.Stdout = stdoutWriter
	os.Stderr = stderrWriter
	code := Run(args)
	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	os.Stdout = originalStdout
	os.Stderr = originalStderr

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if _, err := io.Copy(&stdout, stdoutReader); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	if _, err := io.Copy(&stderr, stderrReader); err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	return code, stdout.String(), stderr.String()
}
