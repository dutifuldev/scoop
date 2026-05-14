package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/rs/zerolog"

	"horse.fit/scoop/internal/db"
	"horse.fit/scoop/internal/globaltime"
	"horse.fit/scoop/internal/language"
	"horse.fit/scoop/internal/translation"
)

const (
	defaultPageSize = 25
	maxPageSize     = 200
)

var errStoryNotFound = errors.New("story not found")

type Options struct {
	Host               string
	Port               int
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	ShutdownTimeout    time.Duration
	SessionTTL         time.Duration
	SessionCookie      string
	SessionSecure      bool
	CORSAllowedOrigins []string
}

type Server struct {
	pool               *db.Pool
	authStore          authStore
	logger             zerolog.Logger
	opts               Options
	registry           *translation.Registry
	translationService translation.Service
}

type storyListFilter struct {
	Collection string
	Status     string
	Query      string
	Tag        string
	From       *time.Time
	To         *time.Time
	TimeZone   string
	Lang       string
	Page       int
	PageSize   int
}

type storyRepresentative struct {
	ArticleUUID  string     `json:"article_uuid"`
	Source       string     `json:"source"`
	SourceItemID string     `json:"source_item_id"`
	PublishedAt  *time.Time `json:"published_at,omitempty"`
}

type storyListItem struct {
	StoryID          int64                     `json:"story_id"`
	StoryUUID        string                    `json:"story_uuid"`
	Collection       string                    `json:"collection"`
	TranslationMode  string                    `json:"translation_mode"`
	Title            string                    `json:"title"`
	OriginalTitle    string                    `json:"original_title"`
	TranslatedTitle  *string                   `json:"translated_title"`
	DetectedLanguage string                    `json:"detected_language"`
	CanonicalURL     *string                   `json:"canonical_url,omitempty"`
	Status           string                    `json:"status"`
	PublishedAt      *time.Time                `json:"published_at,omitempty"`
	FirstSeenAt      time.Time                 `json:"first_seen_at"`
	LastSeenAt       time.Time                 `json:"last_seen_at"`
	SourceCount      int                       `json:"source_count"`
	ArticleCount     int                       `json:"article_count"`
	Representative   *storyRepresentative      `json:"representative,omitempty"`
	Tags             []db.TagRecord            `json:"tags,omitempty"`
	PersonIdentities []db.PersonIdentityRecord `json:"person_identities,omitempty"`
}

type StoryArticle struct {
	StoryArticleUUID     string                    `json:"story_article_uuid"`
	ArticleUUID          string                    `json:"article_uuid"`
	Source               string                    `json:"source"`
	SourceItemID         string                    `json:"source_item_id"`
	Collection           string                    `json:"collection"`
	TranslationMode      string                    `json:"translation_mode"`
	CanonicalURL         *string                   `json:"canonical_url,omitempty"`
	PublishedAt          *time.Time                `json:"published_at,omitempty"`
	NormalizedTitle      string                    `json:"normalized_title"`
	NormalizedText       string                    `json:"normalized_text,omitempty"`
	DetectedLanguage     string                    `json:"detected_language"`
	OriginalTitle        string                    `json:"original_title"`
	TranslatedTitle      *string                   `json:"translated_title"`
	OriginalText         string                    `json:"original_text"`
	TranslatedText       *string                   `json:"translated_text"`
	SourceDomain         *string                   `json:"source_domain,omitempty"`
	MatchedAt            time.Time                 `json:"matched_at"`
	MatchType            string                    `json:"match_type"`
	MatchScore           *float64                  `json:"match_score,omitempty"`
	MatchDetails         map[string]any            `json:"match_details,omitempty"`
	DedupDecision        *string                   `json:"dedup_decision,omitempty"`
	DedupExactSignal     *string                   `json:"dedup_exact_signal,omitempty"`
	DedupBestCosine      *float64                  `json:"dedup_best_cosine,omitempty"`
	DedupTitleOverlap    *float64                  `json:"dedup_title_overlap,omitempty"`
	DedupDateConsistency *float64                  `json:"dedup_date_consistency,omitempty"`
	DedupCompositeScore  *float64                  `json:"dedup_composite_score,omitempty"`
	Tags                 []db.TagRecord            `json:"tags,omitempty"`
	PersonIdentities     []db.PersonIdentityRecord `json:"person_identities,omitempty"`
}

type storyDetail struct {
	Story   storyListItem  `json:"story"`
	Members []StoryArticle `json:"members"`
}

type collectionSummary struct {
	Collection      string     `json:"collection"`
	TranslationMode string     `json:"translation_mode"`
	Articles        int64      `json:"articles"`
	Stories         int64      `json:"stories"`
	StoryItems      int64      `json:"story_items"`
	LastStorySeenAt *time.Time `json:"last_story_seen_at,omitempty"`
}

type updateCollectionSettingsRequest struct {
	TranslationMode string `json:"translation_mode"`
}

type statsResponse struct {
	RawArrivals       int64            `json:"raw_arrivals"`
	Articles          int64            `json:"articles"`
	Stories           int64            `json:"stories"`
	StoryArticles     int64            `json:"story_articles"`
	DedupEvents       int64            `json:"dedup_events"`
	RunningIngestRuns int64            `json:"running_ingest_runs"`
	LastFetchedAt     *time.Time       `json:"last_fetched_at,omitempty"`
	LastStoryUpdated  *time.Time       `json:"last_story_updated,omitempty"`
	DedupDecisions    map[string]int64 `json:"dedup_decisions"`
}

type storyDayBucket struct {
	Day        string `json:"day"`
	StoryCount int64  `json:"story_count"`
}

type updateStoryRequest struct {
	Title      *string `json:"title"`
	Status     *string `json:"status"`
	Collection *string `json:"collection"`
	URL        *string `json:"url"`
}

type updateArticleRequest struct {
	Title      *string `json:"title"`
	Source     *string `json:"source"`
	Collection *string `json:"collection"`
	URL        *string `json:"url"`
}

type translateRequest struct {
	StoryUUID   *string `json:"story_uuid"`
	ArticleUUID *string `json:"article_uuid"`
	TargetLang  string  `json:"target_lang"`
	Provider    string  `json:"provider"`
}

type translateRequestConfig struct {
	storyUUID   string
	articleUUID string
	targetLang  string
	provider    string
	runOpts     translation.RunOptions
}

type updatedStory struct {
	StoryUUID    string     `json:"story_uuid"`
	Title        string     `json:"title"`
	Status       string     `json:"status"`
	Collection   string     `json:"collection"`
	CanonicalURL *string    `json:"canonical_url,omitempty"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
}

type updatedArticle struct {
	ArticleUUID  string     `json:"article_uuid"`
	Title        string     `json:"title"`
	Source       string     `json:"source"`
	Collection   string     `json:"collection"`
	CanonicalURL *string    `json:"canonical_url,omitempty"`
	SourceDomain *string    `json:"source_domain,omitempty"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
}

func NewServer(pool *db.Pool, logger zerolog.Logger, opts Options) *Server {
	return newServer(pool, logger, opts, translation.NewRegistryFromEnv(), nil)
}

func NewServerWithTranslationService(pool *db.Pool, logger zerolog.Logger, opts Options, service translation.Service) *Server {
	return newServer(pool, logger, opts, translation.NewRegistryFromEnv(), service)
}

func newServer(
	pool *db.Pool,
	logger zerolog.Logger,
	opts Options,
	registry *translation.Registry,
	service translation.Service,
) *Server {
	resolvedOpts := resolveServerOptions(opts)
	if registry == nil {
		registry = translation.NewRegistryFromEnv()
	}
	if service == nil {
		service = translation.NewManager(pool, registry)
	}
	var authDataStore authStore
	if pool != nil {
		authDataStore = pool
	}

	return &Server{
		pool:               pool,
		authStore:          authDataStore,
		logger:             logger,
		registry:           registry,
		translationService: service,
		opts:               resolvedOpts,
	}
}

func resolveServerOptions(opts Options) Options {
	return Options{
		Host:               defaultString(strings.TrimSpace(opts.Host), "0.0.0.0"),
		Port:               defaultPositiveInt(opts.Port, 8090),
		ReadTimeout:        defaultPositiveDuration(opts.ReadTimeout, 10*time.Second),
		WriteTimeout:       defaultPositiveDuration(opts.WriteTimeout, 30*time.Second),
		ShutdownTimeout:    defaultPositiveDuration(opts.ShutdownTimeout, 10*time.Second),
		SessionTTL:         defaultPositiveDuration(opts.SessionTTL, 7*24*time.Hour),
		SessionCookie:      defaultString(strings.TrimSpace(opts.SessionCookie), "scoop_session"),
		SessionSecure:      opts.SessionSecure,
		CORSAllowedOrigins: append([]string(nil), opts.CORSAllowedOrigins...),
	}
}

func defaultString(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func defaultPositiveInt(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func defaultPositiveDuration(value time.Duration, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}

func (s *Server) Start(ctx context.Context) error {
	if !serverInitialized(s) {
		return fmt.Errorf("server is not initialized")
	}
	e := s.newEcho()
	registerRoutes(e, s)
	return s.startEchoServer(ctx, e)
}

func serverInitialized(s *Server) bool {
	return s != nil && s.pool != nil
}

func (s *Server) startEchoServer(ctx context.Context, e *echo.Echo) error {
	addr := s.listenAddr()
	go s.shutdownEchoWhenDone(ctx, e)
	s.logger.Info().Str("addr", addr).Msg("scoop web server started")
	return s.finishEchoServer(e.StartServer(s.httpServer(addr, e)))
}

func (s *Server) finishEchoServer(err error) error {
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("start server: %w", err)
	}
	s.logger.Info().Msg("scoop web server stopped")
	return nil
}

func (s *Server) listenAddr() string {
	return fmt.Sprintf("%s:%d", s.opts.Host, s.opts.Port)
}

func (s *Server) httpServer(addr string, e *echo.Echo) *http.Server {
	return &http.Server{
		Addr:         addr,
		Handler:      e,
		ReadTimeout:  s.opts.ReadTimeout,
		WriteTimeout: s.opts.WriteTimeout,
		IdleTimeout:  60 * time.Second,
	}
}

func (s *Server) newEcho() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.HTTPErrorHandler = s.httpErrorHandler

	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(middleware.CORSWithConfig(s.corsConfig()))
	e.Use(middleware.RequestLoggerWithConfig(s.requestLoggerConfig()))
	return e
}

func (s *Server) corsConfig() middleware.CORSConfig {
	config := middleware.CORSConfig{
		AllowMethods:     []string{http.MethodGet, http.MethodOptions, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Cookie"},
		AllowCredentials: true,
		MaxAge:           3600,
	}
	if len(s.opts.CORSAllowedOrigins) > 0 {
		config.AllowOrigins = append([]string(nil), s.opts.CORSAllowedOrigins...)
	} else {
		config.AllowOriginFunc = func(origin string) (bool, error) {
			return strings.TrimSpace(origin) != "", nil
		}
	}
	return config
}

func (s *Server) requestLoggerConfig() middleware.RequestLoggerConfig {
	return middleware.RequestLoggerConfig{
		LogStatus:    true,
		LogURI:       true,
		LogMethod:    true,
		LogLatency:   true,
		LogRemoteIP:  true,
		LogRequestID: true,
		LogError:     true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			if v.Error != nil {
				s.logger.Error().
					Err(v.Error).
					Str("method", v.Method).
					Str("uri", v.URI).
					Int("status", v.Status).
					Dur("latency", v.Latency).
					Str("remote_ip", v.RemoteIP).
					Str("request_id", v.RequestID).
					Msg("http request failed")
				return nil
			}

			s.logger.Info().
				Str("method", v.Method).
				Str("uri", v.URI).
				Int("status", v.Status).
				Dur("latency", v.Latency).
				Str("remote_ip", v.RemoteIP).
				Str("request_id", v.RequestID).
				Msg("http request")
			return nil
		},
	}
}

func registerRoutes(e *echo.Echo, s *Server) {
	e.GET("/", func(c echo.Context) error {
		return success(c, map[string]any{
			"service": "scoop-api",
			"status":  "ok",
			"time":    globaltime.UTC(),
		})
	})

	api := e.Group("/api/v1")
	api.GET("/health", s.handleHealth)
	api.GET("/languages", s.handleLanguages)
	api.POST("/auth/login", s.handleLogin)
	api.POST("/auth/logout", s.handleLogout)

	protected := api.Group("")
	protected.Use(s.requireAuth())
	protected.GET("/me", s.handleMe)
	protected.GET("/me/settings", s.handleGetMySettings)
	protected.PUT("/me/settings", s.handlePutMySettings)
	protected.GET("/stats", s.handleStats)
	protected.GET("/tags", s.handleTags)
	protected.GET("/person-identities", s.handlePersonIdentities)
	protected.GET("/collections", s.handleCollections)
	protected.PATCH("/collections/:collection/settings", s.handleUpdateCollectionSettings)
	protected.GET("/story-days", s.handleStoryDays)
	protected.GET("/stories", s.handleStories)
	protected.GET("/stories/:story_uuid", s.handleStoryDetail)
	protected.POST("/translate", s.handleTranslate)
	protected.GET("/translations/:story_uuid", s.handleStoryTranslations)
	protected.DELETE("/stories/:story_uuid", s.handleDeleteStory)
	protected.PATCH("/stories/:story_uuid", s.handleUpdateStory)
	protected.POST("/stories/:story_uuid/restore", s.handleRestoreStory)
	protected.DELETE("/articles/:article_uuid", s.handleDeleteArticle)
	protected.PATCH("/articles/:article_uuid", s.handleUpdateArticle)
	protected.POST("/articles/:article_uuid/restore", s.handleRestoreArticle)
	protected.POST("/articles/:article_uuid/tags", s.handleAddArticleTag)
	protected.DELETE("/articles/:article_uuid/tags/:tag", s.handleRemoveArticleTag)
	protected.GET("/articles/:article_uuid/person-identities", s.handleArticlePersonIdentities)
	protected.POST("/articles/:article_uuid/person-identities", s.handleAddArticlePersonIdentity)
	protected.DELETE("/articles/:article_uuid/person-identities/:person_identity", s.handleRemoveArticlePersonIdentity)
	protected.GET("/articles/:story_article_uuid/preview", s.handleStoryArticlePreview)
}

func (s *Server) shutdownEchoWhenDone(ctx context.Context, e *echo.Echo) {
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.opts.ShutdownTimeout)
	defer cancel()
	if shutdownErr := e.Shutdown(shutdownCtx); shutdownErr != nil {
		s.logger.Error().Err(shutdownErr).Msg("server shutdown failed")
	}
}

func (s *Server) httpErrorHandler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}

	status, message := httpErrorStatusAndMessage(err)
	if strings.HasPrefix(c.Request().URL.Path, "/api/") {
		writeAPIError(c, status, message)
		return
	}

	_ = c.String(status, message)
}

func httpErrorStatusAndMessage(err error) (int, string) {
	status := http.StatusInternalServerError
	message := "Internal server error"
	if he, ok := err.(*echo.HTTPError); ok {
		status = he.Code
		message = echoHTTPErrorMessage(he, status, message)
	} else if err != nil {
		message = err.Error()
	}
	return status, message
}

func echoHTTPErrorMessage(he *echo.HTTPError, status int, fallback string) string {
	if message, ok := he.Message.(string); ok && strings.TrimSpace(message) != "" {
		return message
	}
	if text := strings.TrimSpace(http.StatusText(status)); text != "" {
		return text
	}
	return fallback
}

func writeAPIError(c echo.Context, status int, message string) {
	if status >= 500 {
		_ = internalError(c, "Internal server error")
		return
	}
	_ = fail(c, status, message, nil)
}

func (s *Server) handleHealth(c echo.Context) error {
	return success(c, map[string]any{
		"service": "scoop",
		"time":    globaltime.UTC(),
	})
}

func (s *Server) handleStats(c echo.Context) error {
	stats, err := s.queryStats(c.Request().Context())
	if err != nil {
		s.logger.Error().Err(err).Msg("query stats failed")
		return internalError(c, "Failed to load stats")
	}
	return success(c, stats)
}

func (s *Server) handleCollections(c echo.Context) error {
	rows, err := s.queryCollections(c.Request().Context())
	if err != nil {
		s.logger.Error().Err(err).Msg("query collections failed")
		return internalError(c, "Failed to load collections")
	}
	return success(c, map[string]any{
		"items": rows,
	})
}

func (s *Server) handleUpdateCollectionSettings(c echo.Context) error {
	collection := normalizeCollection(c.Param("collection"))
	if collection == "" {
		return failValidation(c, map[string]string{"collection": "is required"})
	}

	var req updateCollectionSettingsRequest
	if err := decodeJSONBody(c, &req); err != nil {
		return failValidation(c, map[string]string{"body": err.Error()})
	}

	mode := db.NormalizeCollectionTranslationMode(req.TranslationMode)
	if strings.TrimSpace(req.TranslationMode) == "" {
		return failValidation(c, map[string]string{"translation_mode": "is required"})
	}
	if mode != strings.ToLower(strings.TrimSpace(req.TranslationMode)) {
		return failValidation(c, map[string]string{"translation_mode": "must be enabled or disabled"})
	}

	row, err := s.pool.UpsertCollectionTranslationMode(c.Request().Context(), collection, mode)
	if err != nil {
		s.logger.Error().Err(err).Str("collection", collection).Msg("update collection settings failed")
		return internalError(c, "Failed to update collection settings")
	}

	return success(c, map[string]any{
		"settings": row,
	})
}

func (s *Server) handleStoryDays(c echo.Context) error {
	limit, err := parsePositiveInt(c.QueryParam("limit"), 30, 1, 180)
	if err != nil {
		return failValidation(c, map[string]string{"limit": err.Error()})
	}

	collection := normalizeCollection(c.QueryParam("collection"))
	_, timeZone, err := parseClientTimeZone(c.QueryParam("tz"))
	if err != nil {
		return failValidation(c, map[string]string{"tz": "must be a valid IANA timezone"})
	}

	items, err := s.queryStoryDays(c.Request().Context(), collection, limit, timeZone)
	if err != nil {
		s.logger.Error().Err(err).Str("collection", collection).Msg("query story day buckets failed")
		return internalError(c, "Failed to load story day buckets")
	}

	return success(c, map[string]any{
		"items":      items,
		"collection": collection,
		"limit":      limit,
		"tz":         timeZone,
	})
}

func (s *Server) handleStories(c echo.Context) error {
	filter, err := parseStoryListFilter(c)
	if err != nil {
		return err
	}

	total, rows, err := s.queryStoryList(c.Request().Context(), filter)
	if err != nil {
		s.logger.Error().Err(err).Msg("query stories failed")
		return internalError(c, "Failed to load stories")
	}

	return success(c, storyListResponse(filter, total, rows))
}

func parseStoryListFilter(c echo.Context) (storyListFilter, error) {
	page, err := parsePositiveInt(c.QueryParam("page"), 1, 1, 1_000_000)
	if err != nil {
		return storyListFilter{}, failValidation(c, map[string]string{"page": err.Error()})
	}

	pageSize, err := parsePositiveInt(c.QueryParam("page_size"), defaultPageSize, 1, maxPageSize)
	if err != nil {
		return storyListFilter{}, failValidation(c, map[string]string{"page_size": err.Error()})
	}

	timeFilter, err := parseStoryListTimeFilter(c)
	if err != nil {
		return storyListFilter{}, err
	}

	return storyListFilter{
		Collection: normalizeCollection(c.QueryParam("collection")),
		Status:     strings.TrimSpace(strings.ToLower(c.QueryParam("status"))),
		Query:      strings.TrimSpace(c.QueryParam("q")),
		Tag:        db.NormalizeTagSlug(c.QueryParam("tag")),
		From:       timeFilter.from,
		To:         timeFilter.to,
		TimeZone:   timeFilter.timeZone,
		Lang:       normalizeLanguage(c.QueryParam("lang")),
		Page:       page,
		PageSize:   pageSize,
	}, nil
}

type storyListTimeFilter struct {
	from     *time.Time
	to       *time.Time
	timeZone string
}

func parseStoryListTimeFilter(c echo.Context) (storyListTimeFilter, error) {
	location, timeZone, err := parseClientTimeZone(c.QueryParam("tz"))
	if err != nil {
		return storyListTimeFilter{}, failValidation(c, map[string]string{"tz": "must be a valid IANA timezone"})
	}
	from, err := parseTimeFilter(c.QueryParam("from"), false, location)
	if err != nil {
		return storyListTimeFilter{}, failValidation(c, map[string]string{"from": "must be RFC3339 or YYYY-MM-DD"})
	}
	to, err := parseTimeFilter(c.QueryParam("to"), true, location)
	if err != nil {
		return storyListTimeFilter{}, failValidation(c, map[string]string{"to": "must be RFC3339 or YYYY-MM-DD"})
	}
	if from != nil && to != nil && from.After(*to) {
		return storyListTimeFilter{}, failValidation(c, map[string]string{"time_range": "from must be <= to"})
	}
	return storyListTimeFilter{from: from, to: to, timeZone: timeZone}, nil
}

func storyListResponse(filter storyListFilter, total int64, rows []storyListItem) map[string]any {
	totalPages := 0
	if total > 0 && filter.PageSize > 0 {
		totalPages = int((total + int64(filter.PageSize) - 1) / int64(filter.PageSize))
	}
	return map[string]any{
		"items": rows,
		"pagination": map[string]any{
			"page":        filter.Page,
			"page_size":   filter.PageSize,
			"total_items": total,
			"total_pages": totalPages,
		},
		"filters": map[string]any{
			"collection": filter.Collection,
			"status":     filter.Status,
			"q":          filter.Query,
			"tag":        filter.Tag,
			"from":       filter.From,
			"to":         filter.To,
			"tz":         filter.TimeZone,
			"lang":       filter.Lang,
		},
	}
}

func (s *Server) handleStoryDetail(c echo.Context) error {
	storyUUID := strings.TrimSpace(c.Param("story_uuid"))
	if storyUUID == "" {
		return failValidation(c, map[string]string{"story_uuid": "is required"})
	}

	lang := normalizeLanguage(c.QueryParam("lang"))

	detail, err := s.queryStoryDetail(c.Request().Context(), storyUUID, lang)
	if err != nil {
		if errors.Is(err, errStoryNotFound) {
			return failNotFound(c, "Story not found")
		}
		s.logger.Error().Err(err).Str("story_uuid", storyUUID).Msg("query story detail failed")
		return internalError(c, "Failed to load story detail")
	}

	return success(c, detail)
}

func (s *Server) handleTranslate(c echo.Context) error {
	if s.translationService == nil {
		return internalError(c, "Translation service is not initialized")
	}

	cfg, responseErr := decodeTranslateRequest(c)
	if responseErr != nil {
		return responseErr
	}

	stats, err := s.runTranslateRequest(c.Request().Context(), cfg)
	if err != nil {
		return s.handleTranslateError(c, err)
	}

	resolvedProvider := cfg.provider
	if resolvedProvider == "" {
		resolvedProvider = s.translationService.DefaultProvider()
	}

	return success(c, map[string]any{
		"story_uuid":   nullableString(cfg.storyUUID),
		"article_uuid": nullableString(cfg.articleUUID),
		"target_lang":  cfg.targetLang,
		"provider":     resolvedProvider,
		"stats":        stats,
	})
}

func decodeTranslateRequest(c echo.Context) (translateRequestConfig, error) {
	var req translateRequest
	if err := decodeJSONBody(c, &req); err != nil {
		return translateRequestConfig{}, failValidation(c, map[string]string{"body": err.Error()})
	}

	targetLang := normalizeLanguage(req.TargetLang)
	if targetLang == "" {
		return translateRequestConfig{}, failValidation(c, map[string]string{"target_lang": "is required"})
	}

	storyUUID := strings.TrimSpace(derefString(req.StoryUUID))
	articleUUID := strings.TrimSpace(derefString(req.ArticleUUID))
	if storyUUID == "" && articleUUID == "" {
		return translateRequestConfig{}, failValidation(c, map[string]string{"body": "either story_uuid or article_uuid is required"})
	}
	if storyUUID != "" && articleUUID != "" {
		return translateRequestConfig{}, failValidation(c, map[string]string{"body": "only one of story_uuid or article_uuid is allowed"})
	}

	provider := strings.TrimSpace(req.Provider)
	return translateRequestConfig{
		storyUUID:   storyUUID,
		articleUUID: articleUUID,
		targetLang:  targetLang,
		provider:    provider,
		runOpts: translation.RunOptions{
			TargetLang: targetLang,
			Provider:   provider,
		},
	}, nil
}

func (s *Server) runTranslateRequest(ctx context.Context, cfg translateRequestConfig) (translation.RunStats, error) {
	if cfg.storyUUID != "" {
		return s.translationService.TranslateStoryByUUID(ctx, cfg.storyUUID, cfg.runOpts)
	}
	return s.translationService.TranslateArticleByUUID(ctx, cfg.articleUUID, cfg.runOpts)
}

func (s *Server) handleTranslateError(c echo.Context, err error) error {
	if response := translateRequestError(c, err); response != nil {
		return response
	}
	s.logger.Error().Err(err).Msg("translate request failed")
	return internalError(c, "Failed to translate")
}

func translateRequestError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, translation.ErrStoryNotFound):
		return failNotFound(c, "Story not found")
	case errors.Is(err, translation.ErrArticleNotFound):
		return failNotFound(c, "Article not found")
	case errors.Is(err, translation.ErrTranslationDisabled):
		return failValidation(c, map[string]string{"collection": "translation is disabled for this collection"})
	case providerNotRegisteredError(err):
		return failValidation(c, map[string]string{"provider": err.Error()})
	default:
		return nil
	}
}

func providerNotRegisteredError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "provider") && strings.Contains(message, "not registered")
}

func (s *Server) handleStoryTranslations(c echo.Context) error {
	if s.translationService == nil {
		return internalError(c, "Translation service is not initialized")
	}

	storyUUID := strings.TrimSpace(c.Param("story_uuid"))
	if storyUUID == "" {
		return failValidation(c, map[string]string{"story_uuid": "is required"})
	}

	items, err := s.translationService.ListStoryTranslationsByUUID(c.Request().Context(), storyUUID)
	if err != nil {
		if errors.Is(err, translation.ErrStoryNotFound) {
			return failNotFound(c, "Story not found")
		}
		if errors.Is(err, translation.ErrTranslationDisabled) {
			return failValidation(c, map[string]string{"collection": "translation is disabled for this collection"})
		}
		s.logger.Error().Err(err).Str("story_uuid", storyUUID).Msg("query story translations failed")
		return internalError(c, "Failed to load story translations")
	}

	return success(c, map[string]any{
		"story_uuid": storyUUID,
		"items":      items,
	})
}

func (s *Server) handleDeleteStory(c echo.Context) error {
	return s.handleTimestampedEntityMutation(c, storyEntityMutationConfig("soft delete story failed", "Failed to soft delete story", s.pool.SoftDeleteStory))
}

func (s *Server) handleRestoreStory(c echo.Context) error {
	return s.handleTimestampedEntityMutation(c, storyEntityMutationConfig("restore story failed", "Failed to restore story", s.pool.RestoreStory))
}

func (s *Server) handleUpdateStory(c echo.Context) error {
	cfg := entityUpdateConfig[db.UpdateStoryOptions, updatedStory]{responseKey: "story"}
	cfg.decode = decodeUpdateStoryRequest
	cfg.apply = s.pool.UpdateStory
	cfg.load = s.loadUpdatedStoryResponse
	cfg.error = storyMutationErrorConfig()
	return handleEntityUpdate(s, c, cfg)
}

func decodeUpdateStoryRequest(c echo.Context) (string, db.UpdateStoryOptions, error) {
	return decodeEntityUpdateRequest(c, "story_uuid", storyUpdateOptionsFromRequest, storyUpdateHasFields)
}

func storyUpdateOptionsFromRequest(req updateStoryRequest) db.UpdateStoryOptions {
	return db.UpdateStoryOptions{Title: req.Title, Status: req.Status, Collection: req.Collection, URL: req.URL}
}

func storyUpdateHasFields(opts db.UpdateStoryOptions) bool {
	return opts.Title != nil || opts.Status != nil || opts.Collection != nil || opts.URL != nil
}

func (s *Server) handleDeleteArticle(c echo.Context) error {
	return s.handleTimestampedEntityMutation(c, articleEntityMutationConfig("soft delete article failed", "Failed to soft delete article", s.pool.SoftDeleteArticle))
}

func (s *Server) handleRestoreArticle(c echo.Context) error {
	return s.handleTimestampedEntityMutation(c, articleEntityMutationConfig("restore article failed", "Failed to restore article", s.pool.RestoreArticle))
}

type entityMutationConfig struct {
	paramName     string
	responseKey   string
	notFound      string
	logMessage    string
	clientMessage string
	mutate        func(context.Context, string, time.Time) (int64, error)
}

func storyEntityMutationConfig(logMessage string, clientMessage string, mutate func(context.Context, string, time.Time) (int64, error)) entityMutationConfig {
	cfg := entityMutationConfig{paramName: "story_uuid", responseKey: "story_uuid", notFound: "Story not found"}
	cfg.logMessage = logMessage
	cfg.clientMessage = clientMessage
	cfg.mutate = mutate
	return cfg
}

func articleEntityMutationConfig(logMessage string, clientMessage string, mutate func(context.Context, string, time.Time) (int64, error)) entityMutationConfig {
	return entityMutationConfig{
		paramName:     "article_uuid",
		responseKey:   "article_uuid",
		notFound:      "Article not found",
		logMessage:    logMessage,
		clientMessage: clientMessage,
		mutate:        mutate,
	}
}

func (s *Server) handleTimestampedEntityMutation(c echo.Context, cfg entityMutationConfig) error {
	value := strings.TrimSpace(c.Param(cfg.paramName))
	if value == "" {
		return failValidation(c, map[string]string{cfg.paramName: "is required"})
	}

	affected, err := cfg.mutate(c.Request().Context(), value, globaltime.UTC())
	if err != nil {
		return s.handleTimestampedEntityMutationError(c, cfg, value, err)
	}
	if affected == 0 {
		return failNotFound(c, cfg.notFound)
	}

	return success(c, map[string]any{
		cfg.responseKey: value,
		"affected":      affected,
	})
}

func (s *Server) handleTimestampedEntityMutationError(c echo.Context, cfg entityMutationConfig, value string, err error) error {
	if msg := mutationValidationMessage(err); msg != "" {
		return failValidation(c, map[string]string{cfg.paramName: msg})
	}
	s.logger.Error().Err(err).Str(cfg.paramName, value).Msg(cfg.logMessage)
	return internalError(c, cfg.clientMessage)
}

func (s *Server) handleUpdateArticle(c echo.Context) error {
	return handleEntityUpdate(s, c, entityUpdateConfig[db.UpdateArticleOptions, updatedArticle]{
		responseKey: "article",
		decode:      decodeUpdateArticleRequest,
		apply:       s.pool.UpdateArticle,
		load:        s.loadUpdatedArticleResponse,
		error:       articleMutationErrorConfig(),
	})
}

func decodeUpdateArticleRequest(c echo.Context) (string, db.UpdateArticleOptions, error) {
	return decodeEntityUpdateRequest(c, "article_uuid", articleUpdateOptionsFromRequest, articleUpdateHasFields)
}

func articleUpdateOptionsFromRequest(req updateArticleRequest) db.UpdateArticleOptions {
	return db.UpdateArticleOptions{Title: req.Title, Source: req.Source, Collection: req.Collection, URL: req.URL}
}

func articleUpdateHasFields(opts db.UpdateArticleOptions) bool {
	return opts.Title != nil || opts.Source != nil || opts.Collection != nil || opts.URL != nil
}

func decodeEntityUpdateRequest[Request any, Options any](
	c echo.Context,
	paramName string,
	buildOptions func(Request) Options,
	hasFields func(Options) bool,
) (string, Options, error) {
	uuid := strings.TrimSpace(c.Param(paramName))
	if uuid == "" {
		var zero Options
		return "", zero, failValidation(c, map[string]string{paramName: "is required"})
	}
	var req Request
	if err := decodeJSONBody(c, &req); err != nil {
		var zero Options
		return "", zero, failValidation(c, map[string]string{"body": err.Error()})
	}
	opts := buildOptions(req)
	if !hasFields(opts) {
		var zero Options
		return "", zero, failValidation(c, map[string]string{"body": "at least one update field is required"})
	}
	return uuid, opts, nil
}

type entityUpdateConfig[Options any, Row any] struct {
	responseKey string
	decode      func(echo.Context) (string, Options, error)
	apply       func(context.Context, string, Options, time.Time) error
	load        func(echo.Context, string) (*Row, error)
	error       mutationErrorConfig
}

type mutationErrorConfig struct {
	notFound      string
	logField      string
	logMessage    string
	clientMessage string
}

func handleEntityUpdate[Options any, Row any](s *Server, c echo.Context, cfg entityUpdateConfig[Options, Row]) error {
	uuid, opts, err := cfg.decode(c)
	if err != nil {
		return err
	}
	if err := cfg.apply(c.Request().Context(), uuid, opts, globaltime.UTC()); err != nil {
		return s.handleMutationError(c, err, uuid, cfg.error)
	}
	row, err := cfg.load(c, uuid)
	if err != nil {
		return err
	}
	return success(c, map[string]any{cfg.responseKey: row})
}

func storyMutationErrorConfig() mutationErrorConfig {
	return newMutationErrorConfig("Story", "story_uuid", "update story failed", "Failed to update story")
}

func articleMutationErrorConfig() mutationErrorConfig {
	return newMutationErrorConfig("Article", "article_uuid", "update article failed", "Failed to update article")
}

func newMutationErrorConfig(noun string, logField string, logMessage string, clientMessage string) mutationErrorConfig {
	return mutationErrorConfig{
		notFound:      noun + " not found",
		logField:      logField,
		logMessage:    logMessage,
		clientMessage: clientMessage,
	}
}

func (s *Server) handleMutationError(c echo.Context, err error, uuid string, cfg mutationErrorConfig) error {
	if errors.Is(err, db.ErrNoRows) {
		return failNotFound(c, cfg.notFound)
	}
	if msg := mutationValidationMessage(err); msg != "" {
		return failValidation(c, map[string]string{"body": msg})
	}
	s.logger.Error().Err(err).Str(cfg.logField, uuid).Msg(cfg.logMessage)
	return internalError(c, cfg.clientMessage)
}

func (s *Server) loadUpdatedStoryResponse(c echo.Context, storyUUID string) (*updatedStory, error) {
	return loadUpdatedEntityResponse(s, c, storyUUID, s.queryUpdatedStory, updatedStoryLoadConfig())
}

func (s *Server) loadUpdatedArticleResponse(c echo.Context, articleUUID string) (*updatedArticle, error) {
	return loadUpdatedEntityResponse(s, c, articleUUID, s.queryUpdatedArticle, updatedArticleLoadConfig())
}

type updatedEntityLoadConfig struct {
	notFound      string
	logField      string
	logMessage    string
	clientMessage string
}

func updatedStoryLoadConfig() updatedEntityLoadConfig {
	cfg := updatedEntityLoadConfig{notFound: "Story not found"}
	cfg.logField = "story_uuid"
	cfg.logMessage = "query updated story failed"
	cfg.clientMessage = "Failed to load updated story"
	return cfg
}

func updatedArticleLoadConfig() updatedEntityLoadConfig {
	cfg := updatedEntityLoadConfig{notFound: "Article not found", logField: "article_uuid"}
	cfg.logMessage = "query updated article failed"
	cfg.clientMessage = "Failed to load updated article"
	return cfg
}

func loadUpdatedEntityResponse[Row any](
	s *Server,
	c echo.Context,
	uuid string,
	query func(context.Context, string) (*Row, error),
	cfg updatedEntityLoadConfig,
) (*Row, error) {
	row, err := query(c.Request().Context(), uuid)
	if err == nil {
		return row, nil
	}
	if errors.Is(err, db.ErrNoRows) {
		return nil, failNotFound(c, cfg.notFound)
	}
	s.logger.Error().Err(err).Str(cfg.logField, uuid).Msg(cfg.logMessage)
	return nil, internalError(c, cfg.clientMessage)
}

func (s *Server) queryStoryList(ctx context.Context, filter storyListFilter) (int64, []storyListItem, error) {
	const countQuery = `
WITH story_publish_times AS (
	SELECT
		sa.story_id,
		MAX(a.published_at) AS story_published_at
	FROM news.story_articles sa
	JOIN news.articles a
		ON a.article_id = sa.article_id
		AND a.deleted_at IS NULL
		AND a.published_at IS NOT NULL
	GROUP BY sa.story_id
)
SELECT COUNT(*)
FROM news.stories s
LEFT JOIN story_publish_times spt
	ON spt.story_id = s.story_id
WHERE s.deleted_at IS NULL
  AND ($1 = '' OR s.collection = $1)
  AND ($2 = '' OR s.status = $2)
  AND ($3 = '' OR s.canonical_title ILIKE $3 OR COALESCE(s.canonical_url, '') ILIKE $3)
  AND ($4::timestamptz IS NULL OR spt.story_published_at >= $4)
  AND ($5::timestamptz IS NULL OR spt.story_published_at <= $5)
  AND (
	$6 = ''
	OR EXISTS (
		SELECT 1
		FROM news.story_articles tag_sm
		JOIN news.articles tag_a
			ON tag_a.article_id = tag_sm.article_id
			AND tag_a.deleted_at IS NULL
		JOIN news.article_tags tag_at ON tag_at.article_id = tag_sm.article_id
		JOIN news.tags tag_t ON tag_t.tag_id = tag_at.tag_id
		WHERE tag_sm.story_id = s.story_id
			AND tag_t.slug = $6
	)
  )
`

	var total int64
	if err := s.pool.QueryRow(ctx, countQuery, storyListQueryArgs(filter)...).Scan(&total); err != nil {
		return 0, nil, fmt.Errorf("count stories: %w", err)
	}

	offset := (filter.Page - 1) * filter.PageSize

	const rowsQuery = `
WITH story_publish_times AS (
	SELECT
		sa.story_id,
		MAX(a.published_at) AS story_published_at
	FROM news.story_articles sa
	JOIN news.articles a
		ON a.article_id = sa.article_id
		AND a.deleted_at IS NULL
		AND a.published_at IS NOT NULL
	GROUP BY sa.story_id
)
SELECT
	s.story_id,
	s.story_uuid::text,
	s.collection,
	COALESCE(
		cs.translation_mode,
		CASE
			WHEN s.collection IN ('china_news', 'metal_news') THEN 'enabled'
			ELSE 'disabled'
		END
	) AS translation_mode,
	s.canonical_title AS original_title,
	st.translated_text,
	s.canonical_url,
	s.status,
	spt.story_published_at,
	s.first_seen_at,
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
	COALESCE(rd.normalized_language, 'und') AS detected_language,
	rd.article_uuid::text,
	rd.source,
	rd.source_item_id,
	rd.published_at
FROM news.stories s
LEFT JOIN story_publish_times spt
	ON spt.story_id = s.story_id
LEFT JOIN news.articles rd
	ON rd.article_id = s.representative_article_id
	AND rd.deleted_at IS NULL
LEFT JOIN news.collection_settings cs
	ON cs.collection = s.collection
LEFT JOIN LATERAL (
	SELECT tr.translated_text
	FROM news.translation_sources ts
	JOIN news.translation_results tr
		ON tr.translation_source_id = ts.translation_source_id
	WHERE ts.source_type = 'story_title'
		AND ts.source_id = s.story_id
		AND tr.target_lang = $7
		AND COALESCE(
			cs.translation_mode,
			CASE
				WHEN s.collection IN ('china_news', 'metal_news') THEN 'enabled'
				ELSE 'disabled'
			END
		) = 'enabled'
	ORDER BY ts.captured_at DESC, ts.translation_source_id DESC
	LIMIT 1
) st ON TRUE
WHERE s.deleted_at IS NULL
  AND ($1 = '' OR s.collection = $1)
  AND ($2 = '' OR s.status = $2)
  AND ($3 = '' OR s.canonical_title ILIKE $3 OR COALESCE(s.canonical_url, '') ILIKE $3)
  AND ($4::timestamptz IS NULL OR spt.story_published_at >= $4)
  AND ($5::timestamptz IS NULL OR spt.story_published_at <= $5)
  AND (
	$6 = ''
	OR EXISTS (
		SELECT 1
		FROM news.story_articles tag_sm
		JOIN news.articles tag_a
			ON tag_a.article_id = tag_sm.article_id
			AND tag_a.deleted_at IS NULL
		JOIN news.article_tags tag_at ON tag_at.article_id = tag_sm.article_id
		JOIN news.tags tag_t ON tag_t.tag_id = tag_at.tag_id
		WHERE tag_sm.story_id = s.story_id
			AND tag_t.slug = $6
	)
  )
ORDER BY spt.story_published_at DESC NULLS LAST, s.story_id DESC
LIMIT $8
OFFSET $9
`

	rows, err := s.pool.Query(ctx, rowsQuery, storyListRowsQueryArgs(filter, offset)...)
	if err != nil {
		return 0, nil, fmt.Errorf("query stories: %w", err)
	}
	defer rows.Close()

	items, storyIDs, err := scanStoryListRows(rows, filter)
	if err != nil {
		return 0, nil, err
	}
	if err := s.hydrateStoryListItems(ctx, items, storyIDs); err != nil {
		return 0, nil, err
	}
	return total, items, nil
}

func storyListQueryArgs(filter storyListFilter) []any {
	return []any{filter.Collection, filter.Status, storyListSearchPattern(filter.Query), filter.From, filter.To, filter.Tag}
}

func storyListRowsQueryArgs(filter storyListFilter, offset int) []any {
	args := storyListQueryArgs(filter)
	return append(args, filter.Lang, filter.PageSize, offset)
}

func storyListSearchPattern(query string) string {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return ""
	}
	return "%" + trimmed + "%"
}

func scanStoryListRows(rows *db.Rows, filter storyListFilter) ([]storyListItem, []int64, error) {
	items := make([]storyListItem, 0, filter.PageSize)
	storyIDs := make([]int64, 0, filter.PageSize)
	for rows.Next() {
		row, err := scanStoryListRow(rows, filter.Lang)
		if err != nil {
			return nil, nil, err
		}
		storyIDs = append(storyIDs, row.StoryID)
		items = append(items, row)
	}

	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate story rows: %w", err)
	}
	return items, storyIDs, nil
}

func scanStoryListRow(rows *db.Rows, lang string) (storyListItem, error) {
	var (
		row             storyListItem
		repArticleUUID  *string
		repSource       *string
		repSourceItemID *string
		repPublishedAt  *time.Time
	)
	if err := rows.Scan(
		&row.StoryID,
		&row.StoryUUID,
		&row.Collection,
		&row.TranslationMode,
		&row.OriginalTitle,
		&row.TranslatedTitle,
		&row.CanonicalURL,
		&row.Status,
		&row.PublishedAt,
		&row.FirstSeenAt,
		&row.LastSeenAt,
		&row.SourceCount,
		&row.ArticleCount,
		&row.DetectedLanguage,
		&repArticleUUID,
		&repSource,
		&repSourceItemID,
		&repPublishedAt,
	); err != nil {
		return storyListItem{}, fmt.Errorf("scan story row: %w", err)
	}
	row.TranslationMode = db.NormalizeCollectionTranslationMode(row.TranslationMode)
	row.Representative = storyRepresentativeFromRow(repArticleUUID, repSource, repSourceItemID, repPublishedAt)
	row.Title = displayStoryListTitle(row, lang)
	return row, nil
}

func storyRepresentativeFromRow(articleUUID, source, sourceItemID *string, publishedAt *time.Time) *storyRepresentative {
	if articleUUID == nil || source == nil || sourceItemID == nil {
		return nil
	}
	return &storyRepresentative{ArticleUUID: *articleUUID, Source: *source, SourceItemID: *sourceItemID, PublishedAt: publishedAt}
}

func displayStoryListTitle(row storyListItem, lang string) string {
	if lang != "" && row.TranslatedTitle != nil {
		return *row.TranslatedTitle
	}
	return row.OriginalTitle
}

func (s *Server) hydrateStoryListItems(ctx context.Context, items []storyListItem, storyIDs []int64) error {
	tagsByStoryID, err := s.pool.ListTagsForStoryIDs(ctx, storyIDs)
	if err != nil {
		return err
	}
	personIdentitiesByStoryID, err := s.pool.ListPersonIdentitiesForStoryIDs(ctx, storyIDs)
	if err != nil {
		return err
	}
	for idx := range items {
		items[idx].Tags = tagsByStoryID[items[idx].StoryID]
		items[idx].PersonIdentities = personIdentitiesByStoryID[items[idx].StoryID]
	}
	return nil
}

func (s *Server) queryUpdatedStory(ctx context.Context, storyUUID string) (*updatedStory, error) {
	const q = `
SELECT
	s.story_uuid::text,
	s.canonical_title,
	s.status,
	s.collection,
	s.canonical_url,
	s.updated_at,
	s.deleted_at
FROM news.stories s
WHERE s.story_uuid = $1::uuid
LIMIT 1
`
	var row updatedStory
	if err := s.pool.QueryRow(ctx, q, storyUUID).Scan(
		&row.StoryUUID,
		&row.Title,
		&row.Status,
		&row.Collection,
		&row.CanonicalURL,
		&row.UpdatedAt,
		&row.DeletedAt,
	); err != nil {
		if errors.Is(err, db.ErrNoRows) {
			return nil, db.ErrNoRows
		}
		return nil, fmt.Errorf("query updated story: %w", err)
	}
	return &row, nil
}

func (s *Server) queryUpdatedArticle(ctx context.Context, articleUUID string) (*updatedArticle, error) {
	const q = `
SELECT
	a.article_uuid::text,
	a.normalized_title,
	a.source,
	a.collection,
	a.canonical_url,
	a.source_domain,
	a.updated_at,
	a.deleted_at
FROM news.articles a
WHERE a.article_uuid = $1::uuid
LIMIT 1
`
	var row updatedArticle
	if err := s.pool.QueryRow(ctx, q, articleUUID).Scan(
		&row.ArticleUUID,
		&row.Title,
		&row.Source,
		&row.Collection,
		&row.CanonicalURL,
		&row.SourceDomain,
		&row.UpdatedAt,
		&row.DeletedAt,
	); err != nil {
		if errors.Is(err, db.ErrNoRows) {
			return nil, db.ErrNoRows
		}
		return nil, fmt.Errorf("query updated article: %w", err)
	}
	return &row, nil
}

func (s *Server) queryStoryDetail(ctx context.Context, storyUUID string, lang string) (*storyDetail, error) {
	story, err := s.queryStoryDetailStory(ctx, storyUUID, lang)
	if err != nil {
		return nil, err
	}

	const membersQuery = `
SELECT
	sm.story_article_uuid::text,
	d.article_uuid::text,
	d.source,
	d.source_item_id,
	d.collection,
	COALESCE(
		dcs.translation_mode,
		CASE
			WHEN d.collection IN ('china_news', 'metal_news') THEN 'enabled'
			ELSE 'disabled'
		END
	) AS translation_mode,
	d.canonical_url,
	d.published_at,
	d.normalized_title,
	COALESCE(d.normalized_language, 'und') AS detected_language,
	at.translated_text,
	d.normalized_text,
	ax.translated_text,
	d.source_domain,
	sm.matched_at,
	sm.match_type::text,
	sm.match_score,
	sm.match_details,
	de.decision::text,
	de.exact_signal,
	de.best_cosine,
	de.title_overlap,
	de.entity_date_consistency,
	de.composite_score
FROM news.story_articles sm
JOIN news.articles d
	ON d.article_id = sm.article_id
	AND d.deleted_at IS NULL
LEFT JOIN news.collection_settings dcs
	ON dcs.collection = d.collection
LEFT JOIN LATERAL (
	SELECT atr.translated_text
	FROM news.translation_sources ats
	JOIN news.translation_results atr
		ON atr.translation_source_id = ats.translation_source_id
	WHERE ats.source_type = 'article_title'
		AND ats.source_id = d.article_id
		AND atr.target_lang = $2
		AND COALESCE(
			dcs.translation_mode,
			CASE
				WHEN d.collection IN ('china_news', 'metal_news') THEN 'enabled'
				ELSE 'disabled'
			END
		) = 'enabled'
	ORDER BY ats.captured_at DESC, ats.translation_source_id DESC
	LIMIT 1
) at ON TRUE
LEFT JOIN LATERAL (
	SELECT axr.translated_text
	FROM news.translation_sources axs
	JOIN news.translation_results axr
		ON axr.translation_source_id = axs.translation_source_id
	WHERE axs.source_type = 'article_text'
		AND axs.source_id = d.article_id
		AND axr.target_lang = $2
		AND COALESCE(
			dcs.translation_mode,
			CASE
				WHEN d.collection IN ('china_news', 'metal_news') THEN 'enabled'
				ELSE 'disabled'
			END
		) = 'enabled'
	ORDER BY axs.captured_at DESC, axs.translation_source_id DESC
	LIMIT 1
) ax ON TRUE
LEFT JOIN news.dedup_events de
	ON de.article_id = d.article_id
WHERE sm.story_id = $1
ORDER BY d.published_at DESC NULLS LAST, sm.matched_at DESC
`

	rows, err := s.pool.Query(ctx, membersQuery, story.StoryID, lang)
	if err != nil {
		return nil, fmt.Errorf("query story articles: %w", err)
	}
	defer rows.Close()

	members, articleUUIDs, err := scanStoryArticleMembers(rows, lang, story.ArticleCount)
	if err != nil {
		return nil, err
	}

	if err := s.hydrateStoryDetailRelations(ctx, &story, members, articleUUIDs); err != nil {
		return nil, err
	}

	return &storyDetail{
		Story:   story,
		Members: members,
	}, nil
}

func (s *Server) hydrateStoryDetailRelations(
	ctx context.Context,
	story *storyListItem,
	members []StoryArticle,
	articleUUIDs []string,
) error {
	tagsByArticleUUID, err := s.pool.ListTagsForArticleUUIDs(ctx, articleUUIDs)
	if err != nil {
		return err
	}
	personIdentitiesByArticleUUID, err := s.pool.ListPersonIdentitiesForArticleUUIDs(ctx, articleUUIDs)
	if err != nil {
		return err
	}
	for idx := range members {
		members[idx].Tags = tagsByArticleUUID[members[idx].ArticleUUID]
		members[idx].PersonIdentities = personIdentitiesByArticleUUID[members[idx].ArticleUUID]
	}
	personIdentitiesByStoryID, err := s.pool.ListPersonIdentitiesForStoryIDs(ctx, []int64{story.StoryID})
	if err != nil {
		return err
	}
	story.PersonIdentities = personIdentitiesByStoryID[story.StoryID]
	return nil
}

func (s *Server) queryStoryDetailStory(ctx context.Context, storyUUID string, lang string) (storyListItem, error) {
	const storyQuery = `
SELECT
	s.story_id,
	s.story_uuid::text,
	s.collection,
	COALESCE(
		cs.translation_mode,
		CASE
			WHEN s.collection IN ('china_news', 'metal_news') THEN 'enabled'
			ELSE 'disabled'
		END
	) AS translation_mode,
	s.canonical_title AS original_title,
	st.translated_text,
	s.canonical_url,
	s.status,
	spt.story_published_at,
	s.first_seen_at,
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
	COALESCE(rd.normalized_language, 'und') AS detected_language,
	rd.article_uuid::text,
	rd.source,
	rd.source_item_id,
	rd.published_at
FROM news.stories s
LEFT JOIN LATERAL (
	SELECT MAX(a.published_at) AS story_published_at
	FROM news.story_articles sa
	JOIN news.articles a
		ON a.article_id = sa.article_id
		AND a.deleted_at IS NULL
		AND a.published_at IS NOT NULL
	WHERE sa.story_id = s.story_id
) spt ON TRUE
LEFT JOIN news.articles rd
	ON rd.article_id = s.representative_article_id
	AND rd.deleted_at IS NULL
LEFT JOIN news.collection_settings cs
	ON cs.collection = s.collection
LEFT JOIN LATERAL (
	SELECT tr.translated_text
	FROM news.translation_sources ts
	JOIN news.translation_results tr
		ON tr.translation_source_id = ts.translation_source_id
	WHERE ts.source_type = 'story_title'
		AND ts.source_id = s.story_id
		AND tr.target_lang = $2
		AND COALESCE(
			cs.translation_mode,
			CASE
				WHEN s.collection IN ('china_news', 'metal_news') THEN 'enabled'
				ELSE 'disabled'
			END
		) = 'enabled'
	ORDER BY ts.captured_at DESC, ts.translation_source_id DESC
	LIMIT 1
) st ON TRUE
WHERE s.story_uuid = $1::uuid
  AND s.deleted_at IS NULL
`
	story, rep, err := s.scanStoryDetailStory(ctx, storyQuery, storyUUID, lang)
	if err != nil {
		return storyListItem{}, err
	}
	return normalizeStoryDetailStory(story, rep, lang), nil
}

type storyRepresentativeScan struct {
	articleUUID  *string
	source       *string
	sourceItemID *string
	publishedAt  *time.Time
}

func (s *Server) scanStoryDetailStory(
	ctx context.Context,
	query string,
	storyUUID string,
	lang string,
) (storyListItem, storyRepresentativeScan, error) {
	var story storyListItem
	var rep storyRepresentativeScan
	if err := s.pool.QueryRow(ctx, query, storyUUID, lang).Scan(
		&story.StoryID,
		&story.StoryUUID,
		&story.Collection,
		&story.TranslationMode,
		&story.OriginalTitle,
		&story.TranslatedTitle,
		&story.CanonicalURL,
		&story.Status,
		&story.PublishedAt,
		&story.FirstSeenAt,
		&story.LastSeenAt,
		&story.SourceCount,
		&story.ArticleCount,
		&story.DetectedLanguage,
		&rep.articleUUID,
		&rep.source,
		&rep.sourceItemID,
		&rep.publishedAt,
	); err != nil {
		if errors.Is(err, db.ErrNoRows) {
			return storyListItem{}, storyRepresentativeScan{}, errStoryNotFound
		}
		return storyListItem{}, storyRepresentativeScan{}, fmt.Errorf("query story: %w", err)
	}
	return story, rep, nil
}

func normalizeStoryDetailStory(story storyListItem, rep storyRepresentativeScan, lang string) storyListItem {
	story.TranslationMode = db.NormalizeCollectionTranslationMode(story.TranslationMode)
	story.Title = story.OriginalTitle
	if lang != "" && story.TranslatedTitle != nil {
		story.Title = *story.TranslatedTitle
	}
	if rep.articleUUID != nil && rep.source != nil && rep.sourceItemID != nil {
		story.Representative = &storyRepresentative{
			ArticleUUID:  *rep.articleUUID,
			Source:       *rep.source,
			SourceItemID: *rep.sourceItemID,
			PublishedAt:  rep.publishedAt,
		}
	}
	return story
}

func scanStoryArticleMembers(rows *db.Rows, lang string, articleCount int) ([]StoryArticle, []string, error) {
	members := make([]StoryArticle, 0, articleCount)
	articleUUIDs := make([]string, 0, articleCount)
	for rows.Next() {
		member, err := scanStoryArticleMember(rows, lang)
		if err != nil {
			return nil, nil, err
		}
		articleUUIDs = append(articleUUIDs, member.ArticleUUID)
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate story articles: %w", err)
	}
	return members, articleUUIDs, nil
}

func scanStoryArticleMember(rows *db.Rows, lang string) (StoryArticle, error) {
	var (
		member          StoryArticle
		matchDetailsRaw []byte
	)
	if err := rows.Scan(
		&member.StoryArticleUUID,
		&member.ArticleUUID,
		&member.Source,
		&member.SourceItemID,
		&member.Collection,
		&member.TranslationMode,
		&member.CanonicalURL,
		&member.PublishedAt,
		&member.NormalizedTitle,
		&member.DetectedLanguage,
		&member.TranslatedTitle,
		&member.NormalizedText,
		&member.TranslatedText,
		&member.SourceDomain,
		&member.MatchedAt,
		&member.MatchType,
		&member.MatchScore,
		&matchDetailsRaw,
		&member.DedupDecision,
		&member.DedupExactSignal,
		&member.DedupBestCosine,
		&member.DedupTitleOverlap,
		&member.DedupDateConsistency,
		&member.DedupCompositeScore,
	); err != nil {
		return StoryArticle{}, fmt.Errorf("scan story article: %w", err)
	}
	return normalizeStoryArticleMember(member, matchDetailsRaw, lang), nil
}

func normalizeStoryArticleMember(member StoryArticle, matchDetailsRaw []byte, lang string) StoryArticle {
	member.TranslationMode = db.NormalizeCollectionTranslationMode(member.TranslationMode)
	member.OriginalTitle = member.NormalizedTitle
	member.OriginalText = member.NormalizedText
	if lang != "" {
		if member.TranslatedTitle != nil {
			member.NormalizedTitle = *member.TranslatedTitle
		}
		if member.TranslatedText != nil {
			member.NormalizedText = *member.TranslatedText
		}
	}
	member.MatchDetails = decodeMatchDetails(matchDetailsRaw)
	return member
}

func decodeMatchDetails(raw []byte) map[string]any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var details map[string]any
	if err := json.Unmarshal(raw, &details); err != nil {
		return nil
	}
	return details
}

func (s *Server) queryCollections(ctx context.Context) ([]collectionSummary, error) {
	const q = `
WITH article_counts AS (
	SELECT collection, COUNT(*)::BIGINT AS articles
	FROM news.articles
	WHERE deleted_at IS NULL
	GROUP BY collection
),
story_counts AS (
	SELECT
		s.collection,
		COUNT(*)::BIGINT AS stories,
		COALESCE(SUM(
			(SELECT COUNT(*)
			 FROM news.story_articles sa
			 JOIN news.articles a
				ON a.article_id = sa.article_id
				AND a.deleted_at IS NULL
			 WHERE sa.story_id = s.story_id)
		), 0)::BIGINT AS story_items,
		MAX(s.last_seen_at) AS last_story_seen_at
	FROM news.stories s
	WHERE s.deleted_at IS NULL
	GROUP BY s.collection
)
SELECT
	COALESCE(d.collection, s.collection) AS collection,
	COALESCE(
		cs.translation_mode,
		CASE
			WHEN COALESCE(d.collection, s.collection) IN ('china_news', 'metal_news') THEN 'enabled'
			ELSE 'disabled'
		END
	) AS translation_mode,
	COALESCE(d.articles, 0) AS articles,
	COALESCE(s.stories, 0) AS stories,
	COALESCE(s.story_items, 0) AS story_items,
	s.last_story_seen_at
FROM article_counts d
FULL OUTER JOIN story_counts s
	ON s.collection = d.collection
LEFT JOIN news.collection_settings cs
	ON cs.collection = COALESCE(d.collection, s.collection)
ORDER BY 1
`

	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query collection summary: %w", err)
	}
	defer rows.Close()

	items := make([]collectionSummary, 0, 8)
	for rows.Next() {
		var row collectionSummary
		if err := rows.Scan(
			&row.Collection,
			&row.TranslationMode,
			&row.Articles,
			&row.Stories,
			&row.StoryItems,
			&row.LastStorySeenAt,
		); err != nil {
			return nil, fmt.Errorf("scan collection summary: %w", err)
		}
		row.TranslationMode = db.NormalizeCollectionTranslationMode(row.TranslationMode)
		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate collection summary: %w", err)
	}
	return items, nil
}

func (s *Server) queryStoryDays(ctx context.Context, collection string, limit int, timeZone string) ([]storyDayBucket, error) {
	const q = `
WITH story_publish_times AS (
	SELECT
		sa.story_id,
		MAX(a.published_at) AS story_published_at
	FROM news.story_articles sa
	JOIN news.articles a
		ON a.article_id = sa.article_id
		AND a.deleted_at IS NULL
		AND a.published_at IS NOT NULL
	GROUP BY sa.story_id
)
SELECT
	TO_CHAR(timezone($3, spt.story_published_at)::date, 'YYYY-MM-DD') AS day_bucket,
	COUNT(*)::BIGINT AS story_count
FROM news.stories s
JOIN story_publish_times spt
	ON spt.story_id = s.story_id
WHERE s.deleted_at IS NULL
  AND ($1 = '' OR s.collection = $1)
GROUP BY day_bucket
ORDER BY day_bucket DESC
LIMIT $2
`
	rows, err := s.pool.Query(ctx, q, collection, limit, timeZone)
	if err != nil {
		return nil, fmt.Errorf("query story day buckets: %w", err)
	}
	defer rows.Close()

	items := make([]storyDayBucket, 0, limit)
	for rows.Next() {
		var row storyDayBucket
		if err := rows.Scan(&row.Day, &row.StoryCount); err != nil {
			return nil, fmt.Errorf("scan story day bucket: %w", err)
		}
		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate story day buckets: %w", err)
	}
	return items, nil
}

func (s *Server) queryStats(ctx context.Context) (*statsResponse, error) {
	const q = `
SELECT
	(SELECT COUNT(*) FROM news.raw_arrivals WHERE deleted_at IS NULL) AS raw_arrivals,
	(SELECT COUNT(*) FROM news.articles WHERE deleted_at IS NULL) AS articles,
	(SELECT COUNT(*) FROM news.stories WHERE deleted_at IS NULL) AS stories,
	(SELECT COUNT(*) FROM news.story_articles) AS story_articles,
	(SELECT COUNT(*) FROM news.dedup_events) AS dedup_events,
	(SELECT COUNT(*) FROM news.ingest_runs WHERE status = 'running') AS running_ingest_runs,
	(SELECT MAX(fetched_at) FROM news.raw_arrivals WHERE deleted_at IS NULL) AS last_fetched_at,
	(SELECT MAX(updated_at) FROM news.stories WHERE deleted_at IS NULL) AS last_story_updated
`

	var stats statsResponse
	if err := s.pool.QueryRow(ctx, q).Scan(
		&stats.RawArrivals,
		&stats.Articles,
		&stats.Stories,
		&stats.StoryArticles,
		&stats.DedupEvents,
		&stats.RunningIngestRuns,
		&stats.LastFetchedAt,
		&stats.LastStoryUpdated,
	); err != nil {
		return nil, fmt.Errorf("query stats: %w", err)
	}

	const decisionQuery = `
SELECT decision::text, COUNT(*)::BIGINT
FROM news.dedup_events
GROUP BY decision
ORDER BY decision
`
	rows, err := s.pool.Query(ctx, decisionQuery)
	if err != nil {
		return nil, fmt.Errorf("query dedup decisions: %w", err)
	}
	defer rows.Close()

	stats.DedupDecisions = map[string]int64{}
	for rows.Next() {
		var decision string
		var count int64
		if err := rows.Scan(&decision, &count); err != nil {
			return nil, fmt.Errorf("scan dedup decision: %w", err)
		}
		stats.DedupDecisions[decision] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dedup decisions: %w", err)
	}

	return &stats, nil
}

func normalizeCollection(raw string) string {
	return strings.TrimSpace(strings.ToLower(raw))
}

func normalizeLanguage(raw string) string {
	return language.NormalizeCode(raw)
}

func nullableString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) > 0 {
		return &trimmed
	}
	return nil
}

func decodeJSONBody(c echo.Context, dst any) error {
	if c == nil || c.Request() == nil || c.Request().Body == nil {
		return fmt.Errorf("request body is required")
	}

	decoder := json.NewDecoder(c.Request().Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("request body is required")
		}
		return fmt.Errorf("must be valid JSON")
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return fmt.Errorf("must contain only one JSON object")
	}

	return nil
}

func mutationValidationMessage(err error) string {
	if err == nil {
		return ""
	}

	text := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(text, "invalid input syntax for type uuid"):
		return "must be a valid UUID"
	case strings.Contains(text, "uuid is required"):
		return "is required"
	case strings.Contains(text, "at least one update field is required"):
		return "at least one update field is required"
	case strings.Contains(text, "must not be empty"):
		return err.Error()
	case strings.Contains(text, "fully-qualified url"):
		return "url must be a fully-qualified URL"
	default:
		return ""
	}
}

func parsePositiveInt(raw string, defaultValue, minValue, maxValue int) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultValue, nil
	}

	value, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("must be an integer")
	}
	if value < minValue || value > maxValue {
		return 0, fmt.Errorf("must be between %d and %d", minValue, maxValue)
	}
	return value, nil
}

func parseClientTimeZone(raw string) (*time.Location, string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		trimmed = "UTC"
	}

	location, err := time.LoadLocation(trimmed)
	if err != nil {
		return nil, "", err
	}
	return location, trimmed, nil
}

func parseTimeFilter(raw string, endOfDay bool, location *time.Location) (*time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	if location == nil {
		location = time.UTC
	}

	if ts, err := time.Parse(time.RFC3339, trimmed); err == nil {
		utc := ts.UTC()
		return &utc, nil
	}

	if day, err := time.Parse("2006-01-02", trimmed); err == nil {
		local := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, location)
		utc := local.UTC()
		if endOfDay {
			utc = local.Add((24 * time.Hour) - time.Nanosecond).UTC()
		}
		return &utc, nil
	}

	return nil, fmt.Errorf("invalid time format")
}
