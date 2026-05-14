package httpapi

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/labstack/echo/v4"

	"horse.fit/scoop/internal/auth"
	"horse.fit/scoop/internal/db"
	"horse.fit/scoop/internal/language"
	"horse.fit/scoop/internal/translation"
)

const defaultViewerLanguage = "en"
const passwordEnabledUIPrefKey = "password_enabled"

var errHTTPResponseWritten = errors.New("http response written")

type userSettingsResponse struct {
	PreferredLanguage string         `json:"preferred_language"`
	PasswordEnabled   bool           `json:"password_enabled"`
	UIPrefs           map[string]any `json:"ui_prefs"`
}

type settingsUpdateState struct {
	preferredLanguage      string
	uiPrefs                map[string]any
	passwordEnabled        bool
	currentPasswordEnabled bool
	passwordProvided       bool
}

func (s *Server) handleGetMySettings(c echo.Context) error {
	store, principal, err := s.settingsStoreAndPrincipal(c)
	if err != nil {
		return err
	}

	settings, err := store.EnsureUserSettings(c.Request().Context(), principal.UserID)
	if err != nil {
		s.logger.Error().Err(err).Int64("user_id", principal.UserID).Msg("query user settings failed")
		return internalError(c, "Failed to load user settings")
	}

	return success(c, map[string]any{
		"settings": buildSettingsResponse(settings),
	})
}

func (s *Server) handlePutMySettings(c echo.Context) error {
	store, principal, err := s.settingsStoreAndPrincipal(c)
	if err != nil {
		return err
	}

	payload, err := decodeSettingsPayload(c)
	if err != nil {
		return ignoreWrittenResponse(err)
	}

	current, err := store.EnsureUserSettings(c.Request().Context(), principal.UserID)
	if err != nil {
		s.logger.Error().Err(err).Int64("user_id", principal.UserID).Msg("load current settings failed")
		return internalError(c, "Failed to load user settings")
	}

	state, err := s.applySettingsPayload(c, store, principal.UserID, newSettingsUpdateState(current), payload)
	if err != nil {
		return ignoreWrittenResponse(err)
	}

	updated, err := persistSettingsUpdate(c, store, principal.UserID, state)
	if err != nil {
		s.logger.Error().Err(err).Int64("user_id", principal.UserID).Msg("update user settings failed")
		return internalError(c, "Failed to update user settings")
	}

	return success(c, map[string]any{
		"settings": buildSettingsResponse(updated),
	})
}

func (s *Server) settingsStoreAndPrincipal(c echo.Context) (authStore, authPrincipal, error) {
	store := s.authDataStore()
	if store == nil {
		return nil, authPrincipal{}, internalError(c, "Failed to load user settings")
	}
	principal, ok := principalFromContext(c)
	if !ok {
		return nil, authPrincipal{}, unauthorizedResponse(c)
	}
	return store, principal, nil
}

func decodeSettingsPayload(c echo.Context) (map[string]json.RawMessage, error) {
	var payload map[string]json.RawMessage
	if err := decodeJSONBody(c, &payload); err != nil {
		return nil, responseWritten(failValidation(c, map[string]string{"body": err.Error()}))
	}
	if len(payload) == 0 {
		return nil, responseWritten(failValidation(c, map[string]string{"body": "at least one settings field is required"}))
	}
	if validationErrors := validateSettingsPayloadKeys(payload); len(validationErrors) > 0 {
		return nil, responseWritten(failValidation(c, validationErrors))
	}
	return payload, nil
}

func responseWritten(err error) error {
	if err != nil {
		return err
	}
	return errHTTPResponseWritten
}

func ignoreWrittenResponse(err error) error {
	if errors.Is(err, errHTTPResponseWritten) {
		return nil
	}
	return err
}

func validateSettingsPayloadKeys(payload map[string]json.RawMessage) map[string]string {
	for key := range payload {
		if !isSupportedSettingsField(key) {
			return map[string]string{key: "is not a supported settings field"}
		}
	}
	return nil
}

func isSupportedSettingsField(key string) bool {
	switch key {
	case "preferred_language", "ui_prefs", "password_enabled", "password":
		return true
	default:
		return false
	}
}

func newSettingsUpdateState(current *db.UserSettingsRecord) settingsUpdateState {
	uiPrefs := decodeUIPrefs(current.UIPrefs)
	passwordEnabled := isPasswordEnabledMap(uiPrefs)
	return settingsUpdateState{
		preferredLanguage:      normalizeViewerLanguage(current.PreferredLanguage),
		uiPrefs:                uiPrefs,
		passwordEnabled:        passwordEnabled,
		currentPasswordEnabled: passwordEnabled,
	}
}

func (s *Server) applySettingsPayload(
	c echo.Context,
	store authStore,
	userID int64,
	state settingsUpdateState,
	payload map[string]json.RawMessage,
) (settingsUpdateState, error) {
	var err error
	if state, err = s.applyPreferredLanguagePayload(c, state, payload); err != nil {
		return settingsUpdateState{}, err
	}
	if state, err = applyUIPrefsPayload(c, state, payload); err != nil {
		return settingsUpdateState{}, err
	}
	if state, err = applyPasswordEnabledPayload(c, state, payload); err != nil {
		return settingsUpdateState{}, err
	}
	if state, err = s.applyPasswordPayload(c, store, userID, state, payload); err != nil {
		return settingsUpdateState{}, err
	}
	if err := requirePasswordWhenEnabling(c, state); err != nil {
		return settingsUpdateState{}, err
	}
	return state, nil
}

func requirePasswordWhenEnabling(c echo.Context, state settingsUpdateState) error {
	if !state.passwordEnabled || state.currentPasswordEnabled || state.passwordProvided {
		return nil
	}
	return responseWritten(failValidation(c, map[string]string{"password": "is required when enabling password authentication"}))
}

func (s *Server) applyPreferredLanguagePayload(
	c echo.Context,
	state settingsUpdateState,
	payload map[string]json.RawMessage,
) (settingsUpdateState, error) {
	rawLang, exists := payload["preferred_language"]
	if !exists {
		return state, nil
	}
	var langInput string
	if err := json.Unmarshal(rawLang, &langInput); err != nil {
		return settingsUpdateState{}, responseWritten(failValidation(c, map[string]string{"preferred_language": "must be a string"}))
	}
	preferredLanguage := normalizeViewerLanguage(langInput)
	if !isSupportedViewerLanguage(preferredLanguage, s.viewerLanguageOptions()) {
		return settingsUpdateState{}, responseWritten(failValidation(c, map[string]string{"preferred_language": "is not supported"}))
	}
	state.preferredLanguage = preferredLanguage
	return state, nil
}

func applyUIPrefsPayload(
	c echo.Context,
	state settingsUpdateState,
	payload map[string]json.RawMessage,
) (settingsUpdateState, error) {
	rawPrefs, exists := payload["ui_prefs"]
	if !exists {
		return state, nil
	}
	uiPrefs, err := decodeUIPrefsPayload(rawPrefs)
	if err != nil {
		return settingsUpdateState{}, responseWritten(failValidation(c, map[string]string{"ui_prefs": err.Error()}))
	}
	state.uiPrefs = uiPrefs
	state.passwordEnabled = isPasswordEnabledMap(uiPrefs)
	return state, nil
}

func decodeUIPrefsPayload(rawPrefs json.RawMessage) (map[string]any, error) {
	trimmed := strings.TrimSpace(string(rawPrefs))
	if trimmed == "" || trimmed == "null" {
		return map[string]any{}, nil
	}
	var asMap map[string]any
	if err := json.Unmarshal(rawPrefs, &asMap); err != nil {
		return nil, errors.New("must be a JSON object")
	}
	if asMap == nil {
		return map[string]any{}, nil
	}
	return asMap, nil
}

func applyPasswordEnabledPayload(
	c echo.Context,
	state settingsUpdateState,
	payload map[string]json.RawMessage,
) (settingsUpdateState, error) {
	rawPasswordEnabled, exists := payload["password_enabled"]
	if !exists {
		return state, nil
	}
	var enabled bool
	if err := json.Unmarshal(rawPasswordEnabled, &enabled); err != nil {
		return settingsUpdateState{}, responseWritten(failValidation(c, map[string]string{"password_enabled": "must be a boolean"}))
	}
	state.passwordEnabled = enabled
	return state, nil
}

func (s *Server) applyPasswordPayload(
	c echo.Context,
	store authStore,
	userID int64,
	state settingsUpdateState,
	payload map[string]json.RawMessage,
) (settingsUpdateState, error) {
	rawPassword, exists := payload["password"]
	if !exists {
		return state, nil
	}
	password, err := decodeSettingsPassword(rawPassword)
	if err != nil {
		return settingsUpdateState{}, responseWritten(failValidation(c, map[string]string{"password": err.Error()}))
	}
	if err := s.updateSettingsPassword(c, store, userID, password); err != nil {
		return settingsUpdateState{}, err
	}
	state.passwordProvided = true
	return state, nil
}

func decodeSettingsPassword(rawPassword json.RawMessage) (string, error) {
	var password string
	if err := json.Unmarshal(rawPassword, &password); err != nil {
		return "", errors.New("must be a string")
	}
	password = strings.TrimSpace(password)
	if password == "" {
		return "", errors.New("is required")
	}
	return password, nil
}

func (s *Server) updateSettingsPassword(c echo.Context, store authStore, userID int64, password string) error {
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		return responseWritten(internalError(c, "Failed to update password"))
	}
	if err := store.SetUserPasswordHash(c.Request().Context(), userID, passwordHash, false); err != nil {
		if errors.Is(err, db.ErrNoRows) {
			return responseWritten(unauthorizedResponse(c))
		}
		s.logger.Error().Err(err).Int64("user_id", userID).Msg("update user password failed")
		return responseWritten(internalError(c, "Failed to update password"))
	}
	return nil
}

func persistSettingsUpdate(
	c echo.Context,
	store authStore,
	userID int64,
	state settingsUpdateState,
) (*db.UserSettingsRecord, error) {
	state.uiPrefs[passwordEnabledUIPrefKey] = state.passwordEnabled
	uiPrefs, err := json.Marshal(state.uiPrefs)
	if err != nil {
		return nil, err
	}
	return store.UpsertUserSettings(c.Request().Context(), userID, state.preferredLanguage, uiPrefs)
}

func (s *Server) handleLanguages(c echo.Context) error {
	return success(c, map[string]any{
		"items": s.viewerLanguageOptions(),
	})
}

func (s *Server) viewerLanguageOptions() []translation.LanguageOption {
	if s == nil {
		return translation.ViewerLanguageOptions(nil)
	}
	return translation.ViewerLanguageOptions(s.registry)
}

func buildSettingsResponse(row *db.UserSettingsRecord) userSettingsResponse {
	if row == nil {
		return userSettingsResponse{
			PreferredLanguage: defaultViewerLanguage,
			PasswordEnabled:   false,
			UIPrefs:           map[string]any{},
		}
	}

	uiPrefs := decodeUIPrefs(row.UIPrefs)

	return userSettingsResponse{
		PreferredLanguage: normalizeViewerLanguage(row.PreferredLanguage),
		PasswordEnabled:   isPasswordEnabledMap(uiPrefs),
		UIPrefs:           uiPrefs,
	}
}

func normalizeViewerLanguage(raw string) string {
	lang := language.NormalizeTag(raw)
	if lang == "" {
		return defaultViewerLanguage
	}
	if lang == "original" {
		return "original"
	}
	lang = language.NormalizeCode(lang)
	if lang == "" {
		return defaultViewerLanguage
	}
	return lang
}

func isSupportedViewerLanguage(lang string, options []translation.LanguageOption) bool {
	normalized := normalizeViewerLanguage(lang)
	for _, option := range options {
		if normalizeViewerLanguage(option.Code) == normalized {
			return true
		}
	}
	return false
}

func decodeUIPrefs(raw json.RawMessage) map[string]any {
	uiPrefs := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &uiPrefs)
	}
	if uiPrefs == nil {
		uiPrefs = map[string]any{}
	}
	return uiPrefs
}

func isPasswordEnabled(settings *db.UserSettingsRecord) bool {
	if settings == nil {
		return false
	}
	return isPasswordEnabledMap(decodeUIPrefs(settings.UIPrefs))
}

func isPasswordEnabledMap(uiPrefs map[string]any) bool {
	raw, exists := uiPrefs[passwordEnabledUIPrefKey]
	if !exists {
		return false
	}
	return isPasswordEnabledValue(raw)
}

func isPasswordEnabledValue(raw any) bool {
	switch value := raw.(type) {
	case bool:
		return value
	case string:
		return isTruthyPasswordEnabledString(value)
	case float64:
		return value == 1
	default:
		return false
	}
}

func isTruthyPasswordEnabledString(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "1", "yes":
		return true
	default:
		return false
	}
}
