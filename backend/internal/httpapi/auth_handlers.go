package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"horse.fit/scoop/internal/auth"
	"horse.fit/scoop/internal/db"
	"horse.fit/scoop/internal/globaltime"
)

const (
	defaultSessionTouchInterval = time.Minute
)

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type authPrincipal struct {
	SessionID string
	UserID    int64
	Username  string
	ExpiresAt time.Time
}

type authCheckResult struct {
	principal authPrincipal
	err       error
}

type authUserResponse struct {
	UserID      int64      `json:"user_id"`
	Username    string     `json:"username"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginContext struct {
	username  string
	password  string
	user      *db.AuthUser
	settings  *db.UserSettingsRecord
	now       time.Time
	expiresAt time.Time
	sessionID string
}

type authStore interface {
	GetSession(ctx context.Context, sessionID string) (*db.AuthSession, error)
	DeleteSession(ctx context.Context, sessionID string) error
	TouchSession(ctx context.Context, sessionID string, seenAt time.Time) error
	GetUserByUsername(ctx context.Context, username string) (*db.AuthUser, error)
	GetUserByID(ctx context.Context, userID int64) (*db.AuthUser, error)
	CreateSession(ctx context.Context, userID int64, expiresAt, now time.Time) (string, error)
	SetUserLastLogin(ctx context.Context, userID int64, loginAt time.Time) error
	DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error)
	EnsureUserSettings(ctx context.Context, userID int64) (*db.UserSettingsRecord, error)
	UpsertUserSettings(ctx context.Context, userID int64, preferredLanguage string, uiPrefs json.RawMessage) (*db.UserSettingsRecord, error)
	SetUserPasswordHash(ctx context.Context, userID int64, passwordHash string, mustChangePassword bool) error
}

func (s *Server) authDataStore() authStore {
	if s == nil {
		return nil
	}
	if s.authStore != nil {
		return s.authStore
	}
	if s.pool == nil {
		return nil
	}
	return s.pool
}

func (s *Server) requireAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			result := s.checkAuth(c)
			if result.err != nil {
				return result.err
			}
			c.Set("auth.principal", result.principal)
			return next(c)
		}
	}
}

func (s *Server) checkAuth(c echo.Context) authCheckResult {
	if c == nil {
		return authCheckResult{err: unauthorizedResponse(c)}
	}
	store := s.authDataStore()
	if store == nil {
		return authCheckResult{err: internalError(c, "Failed to authorize request")}
	}

	sessionID, found := s.sessionIDFromCookie(c)
	if !found {
		return authCheckResult{err: unauthorizedResponse(c)}
	}

	session, err := s.loadAuthSession(c, store, sessionID)
	if err != nil {
		return authCheckResult{err: err}
	}
	return s.validateAuthSession(c, store, session)
}

func (s *Server) loadAuthSession(c echo.Context, store authStore, sessionID string) (*db.AuthSession, error) {
	session, err := store.GetSession(c.Request().Context(), sessionID)
	if err == nil {
		return session, nil
	}
	if errors.Is(err, db.ErrNoRows) {
		s.clearSessionCookie(c)
		return nil, unauthorizedResponse(c)
	}
	s.logger.Error().Err(err).Msg("session lookup failed")
	return nil, internalError(c, "Failed to authorize request")
}

func (s *Server) validateAuthSession(c echo.Context, store authStore, session *db.AuthSession) authCheckResult {
	now := globaltime.UTC()
	if !session.ExpiresAt.After(now) {
		_ = store.DeleteSession(c.Request().Context(), session.SessionID)
		s.clearSessionCookie(c)
		return authCheckResult{err: unauthorizedResponse(c)}
	}
	if now.Sub(session.LastSeenAt) >= defaultSessionTouchInterval {
		_ = store.TouchSession(c.Request().Context(), session.SessionID, now)
	}
	return authCheckResult{principal: authPrincipal{
		SessionID: session.SessionID,
		UserID:    session.UserID,
		Username:  session.Username,
		ExpiresAt: session.ExpiresAt.UTC(),
	}}
}

func (s *Server) handleLogin(c echo.Context) error {
	store := s.authDataStore()
	if store == nil {
		return internalError(c, "Failed to process login")
	}

	login, ok := s.prepareLogin(c, store)
	if !ok {
		return nil
	}
	s.setSessionCookie(c, login.sessionID, login.expiresAt)
	return s.loginSuccess(c, login)
}

func (s *Server) prepareLogin(c echo.Context, store authStore) (loginContext, bool) {
	login, ok := decodeLoginContext(c)
	if !ok {
		return loginContext{}, false
	}
	if ok := s.loadLoginUserAndSettings(c, store, &login); !ok {
		return loginContext{}, false
	}
	if isPasswordEnabled(login.settings) && !auth.VerifyPassword(login.password, login.user.PasswordHash) {
		_ = fail(c, http.StatusUnauthorized, "Invalid username or password", nil)
		return loginContext{}, false
	}
	if ok := s.createLoginSession(c, store, &login); !ok {
		return loginContext{}, false
	}
	s.updateLoginLastSeen(c, store, &login)
	return login, true
}

func decodeLoginContext(c echo.Context) (loginContext, bool) {
	var req loginRequest
	if err := decodeJSONBody(c, &req); err != nil {
		_ = failValidation(c, map[string]string{"body": err.Error()})
		return loginContext{}, false
	}
	login := loginContext{
		username: auth.NormalizeUsername(req.Username),
		password: strings.TrimSpace(req.Password),
	}
	if login.username == "" {
		_ = failValidation(c, map[string]string{"username": "is required"})
		return loginContext{}, false
	}
	return login, true
}

func (s *Server) loadLoginUserAndSettings(c echo.Context, store authStore, login *loginContext) bool {
	user, err := store.GetUserByUsername(c.Request().Context(), login.username)
	if err != nil {
		if errors.Is(err, db.ErrNoRows) {
			_ = fail(c, http.StatusUnauthorized, "Invalid username or password", nil)
			return false
		}
		s.logger.Error().Err(err).Str("username", login.username).Msg("login lookup failed")
		_ = internalError(c, "Failed to process login")
		return false
	}
	settings, err := store.EnsureUserSettings(c.Request().Context(), user.UserID)
	if err != nil {
		s.logger.Error().Err(err).Int64("user_id", user.UserID).Msg("ensure settings failed")
		_ = internalError(c, "Failed to load settings")
		return false
	}
	login.user = user
	login.settings = settings
	return true
}

func (s *Server) createLoginSession(c echo.Context, store authStore, login *loginContext) bool {
	login.now = globaltime.UTC()
	if _, cleanupErr := store.DeleteExpiredSessions(c.Request().Context(), login.now); cleanupErr != nil {
		s.logger.Warn().Err(cleanupErr).Msg("delete expired sessions failed")
	}

	login.expiresAt = s.sessionExpiry(login.now)
	sessionID, err := store.CreateSession(c.Request().Context(), login.user.UserID, login.expiresAt, login.now)
	if err != nil {
		s.logger.Error().Err(err).Int64("user_id", login.user.UserID).Msg("create session failed")
		_ = internalError(c, "Failed to process login")
		return false
	}
	login.sessionID = sessionID
	return true
}

func (s *Server) updateLoginLastSeen(c echo.Context, store authStore, login *loginContext) {
	if err := store.SetUserLastLogin(c.Request().Context(), login.user.UserID, login.now); err != nil {
		s.logger.Error().Err(err).Int64("user_id", login.user.UserID).Msg("update last login failed")
	}
	nowCopy := login.now
	login.user.LastLoginAt = &nowCopy
}

func (s *Server) loginSuccess(c echo.Context, login loginContext) error {
	return success(c, map[string]any{
		"user":      buildAuthUserResponse(login.user),
		"settings":  buildSettingsResponse(login.settings),
		"languages": s.viewerLanguageOptions(),
		"session": map[string]any{
			"session_id": login.sessionID,
			"expires_at": login.expiresAt.UTC(),
		},
	})
}

func (s *Server) handleLogout(c echo.Context) error {
	store := s.authDataStore()
	if sessionID, found := s.sessionIDFromCookie(c); found {
		if store != nil {
			_ = store.DeleteSession(c.Request().Context(), sessionID)
		}
	}
	s.clearSessionCookie(c)
	return success(c, map[string]any{"logged_out": true})
}

func (s *Server) handleMe(c echo.Context) error {
	store := s.authDataStore()
	if store == nil {
		return internalError(c, "Failed to load user")
	}

	principal, ok := principalFromContext(c)
	if !ok {
		return unauthorizedResponse(c)
	}

	user, err := store.GetUserByID(c.Request().Context(), principal.UserID)
	if err != nil {
		if errors.Is(err, db.ErrNoRows) {
			return unauthorizedResponse(c)
		}
		s.logger.Error().Err(err).Int64("user_id", principal.UserID).Msg("load me user failed")
		return internalError(c, "Failed to load user")
	}

	settings, err := store.EnsureUserSettings(c.Request().Context(), principal.UserID)
	if err != nil {
		s.logger.Error().Err(err).Int64("user_id", principal.UserID).Msg("load me settings failed")
		return internalError(c, "Failed to load settings")
	}

	return success(c, map[string]any{
		"user":      buildAuthUserResponse(user),
		"settings":  buildSettingsResponse(settings),
		"languages": s.viewerLanguageOptions(),
	})
}

func unauthorizedResponse(c echo.Context) error {
	if c == nil {
		return fmt.Errorf("authentication required")
	}
	return fail(c, http.StatusUnauthorized, "Authentication required", nil)
}

func buildAuthUserResponse(row *db.AuthUser) authUserResponse {
	if row == nil {
		return authUserResponse{}
	}
	return authUserResponse{
		UserID:      row.UserID,
		Username:    row.Username,
		CreatedAt:   row.CreatedAt.UTC(),
		LastLoginAt: row.LastLoginAt,
	}
}

func principalFromContext(c echo.Context) (authPrincipal, bool) {
	if c == nil {
		return authPrincipal{}, false
	}
	value := c.Get("auth.principal")
	principal, ok := value.(authPrincipal)
	if !ok {
		return authPrincipal{}, false
	}
	return principal, true
}

func (s *Server) sessionIDFromCookie(c echo.Context) (string, bool) {
	if c == nil {
		return "", false
	}

	cookie, err := c.Cookie(s.opts.SessionCookie)
	if err != nil || cookie == nil {
		return "", false
	}

	sessionID := strings.TrimSpace(cookie.Value)
	if sessionID == "" {
		return "", false
	}
	if !isUUID(sessionID) {
		s.clearSessionCookie(c)
		return "", false
	}
	return sessionID, true
}

func (s *Server) setSessionCookie(c echo.Context, sessionID string, expiresAt time.Time) {
	if c == nil {
		return
	}

	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 1 {
		maxAge = 1
	}

	c.SetCookie(&http.Cookie{
		Name:     s.opts.SessionCookie,
		Value:    strings.TrimSpace(sessionID),
		Path:     "/",
		HttpOnly: true,
		Secure:   s.opts.SessionSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt.UTC(),
		MaxAge:   maxAge,
	})
}

func (s *Server) clearSessionCookie(c echo.Context) {
	if c == nil {
		return
	}

	c.SetCookie(&http.Cookie{
		Name:     s.opts.SessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.opts.SessionSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  globaltime.UTC().Add(-1 * time.Hour),
	})
}

func (s *Server) sessionExpiry(now time.Time) time.Time {
	if s == nil {
		return now.UTC()
	}
	ttl := s.opts.SessionTTL
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	return now.UTC().Add(ttl)
}

func isUUID(value string) bool {
	return uuidPattern.MatchString(value)
}
