package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog"

	"horse.fit/scoop/internal/db"
	"horse.fit/scoop/internal/translation"
)

func TestHTTPErrorHandlerWritesAPIAndPlainResponses(t *testing.T) {
	t.Parallel()

	server := &Server{logger: zerolog.Nop()}
	_, apiContext, apiRecorder := newJSONContext(http.MethodGet, "/api/v1/missing", "")
	server.httpErrorHandler(echo.NewHTTPError(http.StatusNotFound, "missing"), apiContext)
	if apiRecorder.Code != http.StatusNotFound {
		t.Fatalf("api status = %d body=%s", apiRecorder.Code, apiRecorder.Body.String())
	}

	_, plainContext, plainRecorder := newJSONContext(http.MethodGet, "/missing", "")
	server.httpErrorHandler(echo.NewHTTPError(http.StatusTeapot, "short"), plainContext)
	if plainRecorder.Code != http.StatusTeapot || plainRecorder.Body.String() != "short" {
		t.Fatalf("plain response status=%d body=%q", plainRecorder.Code, plainRecorder.Body.String())
	}
}

func TestServerSetupHelpers(t *testing.T) {
	t.Parallel()

	service := &fakeTranslationService{}
	server := NewServerWithTranslationService(nil, zerolog.Nop(), Options{
		Host:               "127.0.0.1",
		Port:               8091,
		ReadTimeout:        time.Second,
		WriteTimeout:       2 * time.Second,
		ShutdownTimeout:    3 * time.Second,
		SessionTTL:         time.Hour,
		SessionCookie:      "session",
		CORSAllowedOrigins: []string{"https://example.com"},
	}, service)

	if server.translationService != service {
		t.Fatalf("translation service was not installed")
	}
	if server.listenAddr() != "127.0.0.1:8091" {
		t.Fatalf("listenAddr() = %q", server.listenAddr())
	}
	httpServer := server.httpServer(server.listenAddr(), echo.New())
	if httpServer.Addr != "127.0.0.1:8091" || httpServer.ReadTimeout != time.Second || httpServer.WriteTimeout != 2*time.Second {
		t.Fatalf("httpServer = %#v", httpServer)
	}
	corsConfig := server.corsConfig()
	if len(corsConfig.AllowOrigins) != 1 || corsConfig.AllowOrigins[0] != "https://example.com" {
		t.Fatalf("cors AllowOrigins = %#v", corsConfig.AllowOrigins)
	}
	if server.requestLoggerConfig().LogURI != true {
		t.Fatalf("request logger config should log URI")
	}

	openServer := NewServer(nil, zerolog.Nop(), Options{})
	if ok, err := openServer.corsConfig().AllowOriginFunc("https://client.example"); err != nil || !ok {
		t.Fatalf("default CORS AllowOriginFunc() = %t, %v", ok, err)
	}
	if serverInitialized(nil) || serverInitialized(openServer) {
		t.Fatalf("serverInitialized should require a non-nil server with pool")
	}
	if err := openServer.Start(context.Background()); err == nil {
		t.Fatalf("Start() error = nil, want uninitialized error")
	}
}

func TestRegisteredRootRoute(t *testing.T) {
	t.Parallel()

	server := &Server{logger: zerolog.Nop(), opts: resolveServerOptions(Options{}), authStore: newFakeAuthStore()}
	e := server.newEcho()
	registerRoutes(e, server)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("root status = %d body=%s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "scoop-api") || !strings.Contains(body, "ok") {
		t.Fatalf("root body = %s", body)
	}
}

func TestRegisteredRoutesIncludeExpectedAPIShape(t *testing.T) {
	t.Parallel()

	server := &Server{logger: zerolog.Nop(), opts: resolveServerOptions(Options{}), authStore: newFakeAuthStore()}
	e := server.newEcho()
	registerRoutes(e, server)

	want := map[string]bool{
		http.MethodGet + " /":                                            false,
		http.MethodGet + " /api/v1/languages":                            false,
		http.MethodPost + " /api/v1/auth/login":                          false,
		http.MethodGet + " /api/v1/me":                                   false,
		http.MethodGet + " /api/v1/stories/:story_uuid":                  false,
		http.MethodPost + " /api/v1/articles/:article_uuid/tags":         false,
		http.MethodGet + " /api/v1/articles/:story_article_uuid/preview": false,
	}
	for _, route := range e.Routes() {
		key := route.Method + " " + route.Path
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for route, found := range want {
		if !found {
			t.Fatalf("registered routes missing %s", route)
		}
	}
}

func TestHandleLanguages(t *testing.T) {
	t.Parallel()

	server := NewServer(nil, zerolog.Nop(), Options{})
	_, c, rec := newJSONContext(http.MethodGet, "/api/v1/languages", "")
	if err := server.handleLanguages(c); err != nil {
		t.Fatalf("handleLanguages() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, `"items"`) || !strings.Contains(body, `"Original"`) {
		t.Fatalf("languages body = %s", body)
	}
}

func TestMutationValidationMessage(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"invalid input syntax for type uuid: bad": "must be a valid UUID",
		"uuid is required":                        "is required",
		"at least one update field is required":   "at least one update field is required",
		"title must not be empty":                 "title must not be empty",
		"url must be a fully-qualified url value": "url must be a fully-qualified URL",
		"unrelated database failure":              "",
	}
	for input, want := range cases {
		if got := mutationValidationMessage(assertErr(input)); got != want {
			t.Fatalf("mutationValidationMessage(%q) = %q, want %q", input, got, want)
		}
	}
	if got := mutationValidationMessage(nil); got != "" {
		t.Fatalf("mutationValidationMessage(nil) = %q, want empty", got)
	}
}

func TestTimestampedEntityMutationErrorResponses(t *testing.T) {
	t.Parallel()

	server := &Server{logger: zerolog.Nop()}
	_, validationContext, validationRecorder := newJSONContext(http.MethodDelete, "/api/v1/stories/bad", "")
	validationContext.SetParamNames("story_uuid")
	validationContext.SetParamValues("bad")
	validationCfg := storyEntityMutationConfig("delete failed", "Delete failed", func(context.Context, string, time.Time) (int64, error) {
		return 0, assertErr("invalid input syntax for type uuid: bad")
	})
	if err := server.handleTimestampedEntityMutation(validationContext, validationCfg); err != nil {
		t.Fatalf("validation mutation error = %v", err)
	}
	if validationRecorder.Code != http.StatusBadRequest {
		t.Fatalf("validation status = %d body=%s", validationRecorder.Code, validationRecorder.Body.String())
	}

	_, internalContext, internalRecorder := newJSONContext(http.MethodDelete, "/api/v1/stories/story-uuid", "")
	internalContext.SetParamNames("story_uuid")
	internalContext.SetParamValues("story-uuid")
	internalCfg := storyEntityMutationConfig("delete failed", "Delete failed", func(context.Context, string, time.Time) (int64, error) {
		return 0, errors.New("database unavailable")
	})
	if err := server.handleTimestampedEntityMutation(internalContext, internalCfg); err != nil {
		t.Fatalf("internal mutation error = %v", err)
	}
	if internalRecorder.Code != http.StatusInternalServerError {
		t.Fatalf("internal status = %d body=%s", internalRecorder.Code, internalRecorder.Body.String())
	}
}

func TestStoryReadHandlerValidationFailures(t *testing.T) {
	t.Parallel()

	server := NewServer(nil, zerolog.Nop(), Options{})
	cases := []struct {
		name   string
		path   string
		handle func(echo.Context) error
		params map[string]string
	}{
		{name: "story days bad limit", path: "/api/v1/story-days?limit=0", handle: server.handleStoryDays},
		{name: "story days bad timezone", path: "/api/v1/story-days?tz=Missing/Zone", handle: server.handleStoryDays},
		{name: "stories bad page", path: "/api/v1/stories?page=0", handle: server.handleStories},
		{name: "stories bad page size", path: "/api/v1/stories?page_size=100000", handle: server.handleStories},
		{name: "stories bad from", path: "/api/v1/stories?from=bad-date", handle: server.handleStories},
		{name: "stories inverted range", path: "/api/v1/stories?from=2026-05-15&to=2026-05-14", handle: server.handleStories},
		{name: "story detail missing uuid", path: "/api/v1/stories/", handle: server.handleStoryDetail, params: map[string]string{"story_uuid": " "}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, c, rec := newJSONContext(http.MethodGet, tt.path, "")
			for key, value := range tt.params {
				c.SetParamNames(key)
				c.SetParamValues(value)
			}
			if err := tt.handle(c); err != nil {
				t.Fatalf("handler error = %v", err)
			}
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestCollectionSettingsValidationFailures(t *testing.T) {
	t.Parallel()

	server := NewServer(nil, zerolog.Nop(), Options{})
	cases := []struct {
		name       string
		collection string
		body       string
	}{
		{name: "missing collection", collection: " ", body: `{"translation_mode":"enabled"}`},
		{name: "missing mode", collection: "openclaw", body: `{}`},
		{name: "invalid mode", collection: "openclaw", body: `{"translation_mode":"paused"}`},
		{name: "bad json", collection: "openclaw", body: `{"translation_mode":`},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, c, rec := newJSONContext(http.MethodPatch, "/api/v1/collections/openclaw/settings", tt.body)
			c.SetParamNames("collection")
			c.SetParamValues(tt.collection)
			if err := server.handleUpdateCollectionSettings(c); err != nil {
				t.Fatalf("handleUpdateCollectionSettings() error = %v", err)
			}
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestArticleTagHandlerValidationFailures(t *testing.T) {
	t.Parallel()

	server := NewServer(nil, zerolog.Nop(), Options{})
	cases := []struct {
		name       string
		method     string
		path       string
		body       string
		paramNames []string
		paramVals  []string
		handle     func(echo.Context) error
		wantCode   int
	}{
		{
			name:       "add bad json",
			method:     http.MethodPost,
			path:       "/api/v1/articles/article-1/tags",
			body:       `{"tag":`,
			paramNames: []string{"article_uuid"},
			paramVals:  []string{"article-1"},
			handle:     server.handleAddArticleTag,
			wantCode:   http.StatusBadRequest,
		},
		{
			name:       "add invalid tag",
			method:     http.MethodPost,
			path:       "/api/v1/articles/article-1/tags",
			body:       `{"tag":"bad tag"}`,
			paramNames: []string{"article_uuid"},
			paramVals:  []string{"article-1"},
			handle:     server.handleAddArticleTag,
			wantCode:   http.StatusBadRequest,
		},
		{
			name:       "remove invalid tag",
			method:     http.MethodDelete,
			path:       "/api/v1/articles/article-1/tags/bad%20tag",
			paramNames: []string{"article_uuid", "tag"},
			paramVals:  []string{"article-1", "bad tag"},
			handle:     server.handleRemoveArticleTag,
			wantCode:   http.StatusBadRequest,
		},
		{
			name:       "add missing actor",
			method:     http.MethodPost,
			path:       "/api/v1/articles/article-1/tags",
			body:       `{"tag":"i0"}`,
			paramNames: []string{"article_uuid"},
			paramVals:  []string{"article-1"},
			handle:     server.handleAddArticleTag,
			wantCode:   http.StatusUnauthorized,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, c, rec := newJSONContext(tt.method, tt.path, tt.body)
			c.SetParamNames(tt.paramNames...)
			c.SetParamValues(tt.paramVals...)
			if err := tt.handle(c); err != nil {
				t.Fatalf("handler error = %v", err)
			}
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d body=%s, want %d", rec.Code, rec.Body.String(), tt.wantCode)
			}
		})
	}
}

func TestArticlePersonIdentityHandlerValidationFailures(t *testing.T) {
	t.Parallel()

	server := NewServer(nil, zerolog.Nop(), Options{})
	cases := []struct {
		name       string
		method     string
		path       string
		body       string
		paramNames []string
		paramVals  []string
		handle     func(echo.Context) error
		wantCode   int
	}{
		{
			name:       "add bad json",
			method:     http.MethodPost,
			path:       "/api/v1/articles/article-1/person-identities",
			body:       `{"identity_ref":`,
			paramNames: []string{"article_uuid"},
			paramVals:  []string{"article-1"},
			handle:     server.handleAddArticlePersonIdentity,
			wantCode:   http.StatusBadRequest,
		},
		{
			name:       "add invalid identity",
			method:     http.MethodPost,
			path:       "/api/v1/articles/article-1/person-identities",
			body:       `{"identity_ref":"not an identity"}`,
			paramNames: []string{"article_uuid"},
			paramVals:  []string{"article-1"},
			handle:     server.handleAddArticlePersonIdentity,
			wantCode:   http.StatusBadRequest,
		},
		{
			name:       "add missing actor",
			method:     http.MethodPost,
			path:       "/api/v1/articles/article-1/person-identities",
			body:       `{"identity_ref":"id://github/handle/octocat"}`,
			paramNames: []string{"article_uuid"},
			paramVals:  []string{"article-1"},
			handle:     server.handleAddArticlePersonIdentity,
			wantCode:   http.StatusUnauthorized,
		},
		{
			name:       "remove missing identity",
			method:     http.MethodDelete,
			path:       "/api/v1/articles/article-1/person-identities/%20",
			paramNames: []string{"article_uuid", "person_identity"},
			paramVals:  []string{"article-1", ""},
			handle:     server.handleRemoveArticlePersonIdentity,
			wantCode:   http.StatusBadRequest,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, c, rec := newJSONContext(tt.method, tt.path, tt.body)
			c.SetParamNames(tt.paramNames...)
			c.SetParamValues(tt.paramVals...)
			if err := tt.handle(c); err != nil {
				t.Fatalf("handler error = %v", err)
			}
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d body=%s, want %d", rec.Code, rec.Body.String(), tt.wantCode)
			}
		})
	}
}

func TestDecodeUIPrefsPayload(t *testing.T) {
	t.Parallel()

	empty, err := decodeUIPrefsPayload(nil)
	if err != nil {
		t.Fatalf("decode empty ui_prefs: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected empty ui prefs, got %#v", empty)
	}

	prefs, err := decodeUIPrefsPayload([]byte(`{"density":"compact"}`))
	if err != nil {
		t.Fatalf("decode ui_prefs: %v", err)
	}
	if prefs["density"] != "compact" {
		t.Fatalf("unexpected ui_prefs: %#v", prefs)
	}

	if _, err := decodeUIPrefsPayload([]byte(`[]`)); err == nil {
		t.Fatalf("expected non-object ui_prefs to fail")
	}
}

func TestHandleMeReturnsPrincipalUserAndSettings(t *testing.T) {
	t.Parallel()

	store := newFakeAuthStore()
	store.usersByID[9] = &db.AuthUser{
		UserID:    9,
		Username:  "admin",
		CreatedAt: time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC),
	}
	store.settingsByUserID[9] = &db.UserSettingsRecord{
		UserID:            9,
		PreferredLanguage: "en",
		UIPrefs:           []byte(`{"density":"compact"}`),
	}
	server := &Server{logger: zerolog.Nop(), authStore: store}
	_, c, rec := newJSONContext(http.MethodGet, "/api/v1/me", "")
	c.Set("auth.principal", authPrincipal{UserID: 9, Username: "admin"})

	if err := server.handleMe(c); err != nil {
		t.Fatalf("handleMe() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

type assertErr string

func (e assertErr) Error() string {
	return string(e)
}

func TestHandleStoryTranslations(t *testing.T) {
	t.Parallel()

	service := &fakeTranslationService{}
	serviceTranslations := []translation.CachedTranslation{{
		TargetLang:     "en",
		TranslatedText: "translated",
	}}
	serviceWithTranslations := &storyTranslationsService{
		fakeTranslationService: service,
		items:                  serviceTranslations,
	}
	server := &Server{logger: zerolog.Nop(), translationService: serviceWithTranslations}
	_, c, rec := newJSONContext(http.MethodGet, "/api/v1/translations/story-1", "")
	c.SetParamNames("story_uuid")
	c.SetParamValues("story-1")

	if err := server.handleStoryTranslations(c); err != nil {
		t.Fatalf("handleStoryTranslations() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleStoryTranslationsErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		service   translation.Service
		storyUUID string
		wantCode  int
	}{
		{name: "missing service", service: nil, storyUUID: "story-1", wantCode: http.StatusInternalServerError},
		{name: "missing uuid", service: &storyTranslationsService{fakeTranslationService: &fakeTranslationService{}}, storyUUID: " ", wantCode: http.StatusBadRequest},
		{name: "not found", service: &storyTranslationsService{fakeTranslationService: &fakeTranslationService{nextErr: translation.ErrStoryNotFound}}, storyUUID: "story-1", wantCode: http.StatusNotFound},
		{name: "disabled", service: &storyTranslationsService{fakeTranslationService: &fakeTranslationService{nextErr: translation.ErrTranslationDisabled}}, storyUUID: "story-1", wantCode: http.StatusBadRequest},
		{name: "unexpected", service: &storyTranslationsService{fakeTranslationService: &fakeTranslationService{nextErr: assertErr("database down")}}, storyUUID: "story-1", wantCode: http.StatusInternalServerError},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			server := &Server{logger: zerolog.Nop(), translationService: tt.service}
			_, c, rec := newJSONContext(http.MethodGet, "/api/v1/translations/story", "")
			c.SetParamNames("story_uuid")
			c.SetParamValues(tt.storyUUID)

			if err := server.handleStoryTranslations(c); err != nil {
				t.Fatalf("handleStoryTranslations() error = %v", err)
			}
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d body=%s, want %d", rec.Code, rec.Body.String(), tt.wantCode)
			}
		})
	}
}

func TestHandleMutationErrorMapsDatabaseAndValidationErrors(t *testing.T) {
	t.Parallel()

	server := &Server{logger: zerolog.Nop()}
	cfg := mutationErrorConfig{
		notFound:      "Story not found",
		logField:      "story_uuid",
		logMessage:    "update failed",
		clientMessage: "Failed to update",
	}
	cases := []struct {
		name string
		err  error
		code int
	}{
		{name: "not found", err: db.ErrNoRows, code: http.StatusNotFound},
		{name: "validation", err: assertErr("uuid is required"), code: http.StatusBadRequest},
		{name: "unexpected", err: assertErr("database down"), code: http.StatusInternalServerError},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, c, rec := newJSONContext(http.MethodPatch, "/api/v1/stories/story-1", "")
			if err := server.handleMutationError(c, tt.err, "story-1", cfg); err != nil {
				t.Fatalf("handleMutationError() error = %v", err)
			}
			if rec.Code != tt.code {
				t.Fatalf("status = %d body=%s, want %d", rec.Code, rec.Body.String(), tt.code)
			}
		})
	}
}

func TestHandleArticleRelationMutationErrorMapsKnownFailures(t *testing.T) {
	t.Parallel()

	server := &Server{logger: zerolog.Nop()}
	cases := []struct {
		name string
		err  error
		code int
	}{
		{name: "not found", err: db.ErrNoRows, code: http.StatusNotFound},
		{name: "validation", err: assertErr("invalid input syntax for type uuid: bad"), code: http.StatusBadRequest},
		{name: "unexpected", err: assertErr("database down"), code: http.StatusInternalServerError},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, c, rec := newJSONContext(http.MethodPost, "/api/v1/articles/article-1/tags", "")
			c.SetParamNames("article_uuid")
			c.SetParamValues("article-1")
			err := server.handleArticleRelationMutationError(
				c,
				tt.err,
				"Article or tag not found",
				"tag",
				"tag",
				"i0",
				"tag mutation failed",
				"Failed to mutate tag",
			)
			if err != nil {
				t.Fatalf("handleArticleRelationMutationError() error = %v", err)
			}
			if rec.Code != tt.code {
				t.Fatalf("status = %d body=%s, want %d", rec.Code, rec.Body.String(), tt.code)
			}
		})
	}
}

func TestFinishEchoServerTreatsNormalShutdownAsSuccess(t *testing.T) {
	t.Parallel()

	server := &Server{logger: zerolog.Nop()}
	if err := server.finishEchoServer(nil); err != nil {
		t.Fatalf("finishEchoServer(nil) error = %v", err)
	}
	if err := server.finishEchoServer(http.ErrServerClosed); err != nil {
		t.Fatalf("finishEchoServer(server closed) error = %v", err)
	}
	if err := server.finishEchoServer(assertErr("bind failed")); err == nil {
		t.Fatalf("finishEchoServer(unexpected) error = nil")
	}
}

func TestHandleUpdateCollectionSettingsIntegration(t *testing.T) {
	pool := newHTTPIntegrationPool(t)
	server := NewServer(pool, zerolog.Nop(), Options{})
	_, c, rec := newJSONContext(http.MethodPatch, "/api/v1/collections/openclaw/settings", `{"translation_mode":"enabled"}`)
	c.SetParamNames("collection")
	c.SetParamValues("OpenClaw")

	if err := server.handleUpdateCollectionSettings(c); err != nil {
		t.Fatalf("handleUpdateCollectionSettings() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	mode, err := pool.GetCollectionTranslationMode(c.Request().Context(), "openclaw")
	if err != nil {
		t.Fatalf("GetCollectionTranslationMode() error = %v", err)
	}
	if mode != "enabled" {
		t.Fatalf("mode = %q, want enabled", mode)
	}
}

type storyTranslationsService struct {
	*fakeTranslationService
	items []translation.CachedTranslation
}

func (s *storyTranslationsService) ListStoryTranslationsByUUID(_ context.Context, _ string) ([]translation.CachedTranslation, error) {
	return s.items, s.nextErr
}
