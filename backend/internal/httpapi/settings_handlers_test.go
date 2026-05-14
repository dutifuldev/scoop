package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog"

	"horse.fit/scoop/internal/db"
)

func TestHandlePutMySettingsUpdatesLanguageUIPrefsAndPassword(t *testing.T) {
	store := newFakeAuthStore()
	store.usersByID[11] = &db.AuthUser{UserID: 11, Username: "admin"}
	store.settingsByUserID[11] = &db.UserSettingsRecord{
		UserID:            11,
		PreferredLanguage: "en",
		UIPrefs:           json.RawMessage(`{"density":"cozy"}`),
	}
	server := &Server{logger: zerolog.Nop(), authStore: store}
	_, c, rec := newJSONContext(http.MethodPut, "/api/v1/me/settings", `{
		"preferred_language":"zh-Hant",
		"ui_prefs":{"density":"compact"},
		"password_enabled":true,
		"password":"new-password"
	}`)
	c.Set("auth.principal", authPrincipal{UserID: 11, Username: "admin"})

	if err := server.handlePutMySettings(c); err != nil {
		t.Fatalf("handlePutMySettings() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if len(store.setPasswordCalls) != 1 {
		t.Fatalf("password calls = %#v, want one", store.setPasswordCalls)
	}
	if len(store.upsertCalls) != 1 || store.upsertCalls[0].preferredLanguage != "zh" {
		t.Fatalf("upsert calls = %#v", store.upsertCalls)
	}
	if !strings.Contains(string(store.upsertCalls[0].uiPrefs), `"password_enabled":true`) {
		t.Fatalf("ui prefs = %s, want password_enabled true", store.upsertCalls[0].uiPrefs)
	}
}

func TestHandlePutMySettingsRequiresPasswordWhenEnablingPassword(t *testing.T) {
	store := newFakeAuthStore()
	store.usersByID[12] = &db.AuthUser{UserID: 12, Username: "admin"}
	store.settingsByUserID[12] = &db.UserSettingsRecord{UserID: 12, PreferredLanguage: "en", UIPrefs: json.RawMessage(`{}`)}
	server := &Server{logger: zerolog.Nop(), authStore: store}
	_, c, rec := newJSONContext(http.MethodPut, "/api/v1/me/settings", `{"password_enabled":true}`)
	c.Set("auth.principal", authPrincipal{UserID: 12, Username: "admin"})

	if err := server.handlePutMySettings(c); err != nil {
		t.Fatalf("handlePutMySettings() error = %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if len(store.upsertCalls) != 0 || len(store.setPasswordCalls) != 0 {
		t.Fatalf("unexpected writes: upsert=%#v password=%#v", store.upsertCalls, store.setPasswordCalls)
	}
}

func TestSettingsPayloadValidationBranches(t *testing.T) {
	server := &Server{logger: zerolog.Nop()}
	state := settingsUpdateState{
		preferredLanguage:      "en",
		uiPrefs:                map[string]any{},
		passwordEnabled:        false,
		currentPasswordEnabled: false,
	}

	_, c, rec := newJSONContext(http.MethodPut, "/api/v1/me/settings", "")
	if _, err := server.applyPreferredLanguagePayload(c, state, map[string]json.RawMessage{"preferred_language": json.RawMessage(`12`)}); err == nil {
		t.Fatalf("numeric preferred_language should fail")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("preferred_language status = %d", rec.Code)
	}

	_, c, rec = newJSONContext(http.MethodPut, "/api/v1/me/settings", "")
	if _, err := applyPasswordEnabledPayload(c, state, map[string]json.RawMessage{"password_enabled": json.RawMessage(`"yes"`)}); err == nil {
		t.Fatalf("string password_enabled should fail")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("password_enabled status = %d", rec.Code)
	}

	if _, err := decodeSettingsPayload(newJSONContextOnly(t, `{"bad":true}`)); err == nil {
		t.Fatalf("unsupported settings key should fail")
	}
	if _, err := decodeSettingsPassword(json.RawMessage(`"  "`)); err == nil {
		t.Fatalf("blank password should fail")
	}
	if got := isPasswordEnabled(&db.UserSettingsRecord{UIPrefs: json.RawMessage(`{"password_enabled":"yes"}`)}); !got {
		t.Fatalf("string yes should enable password")
	}
	if got := isPasswordEnabled(&db.UserSettingsRecord{UIPrefs: json.RawMessage(`{"password_enabled":1}`)}); !got {
		t.Fatalf("numeric 1 should enable password")
	}
	if got := isPasswordEnabled((*db.UserSettingsRecord)(nil)); got {
		t.Fatalf("nil settings should not enable password")
	}
}

func TestBuildSettingsResponseDefaults(t *testing.T) {
	response := buildSettingsResponse(nil)
	if response.PreferredLanguage != "en" || response.PasswordEnabled || len(response.UIPrefs) != 0 {
		t.Fatalf("default response = %#v", response)
	}

	row := &db.UserSettingsRecord{
		UserID:            1,
		PreferredLanguage: "bad tag",
		UIPrefs:           json.RawMessage(`{"password_enabled":"true"}`),
	}
	response = buildSettingsResponse(row)
	if response.PreferredLanguage != "en" || !response.PasswordEnabled {
		t.Fatalf("normalized response = %#v", response)
	}
}

func newJSONContextOnly(t *testing.T, body string) echo.Context {
	t.Helper()
	_, c, _ := newJSONContext(http.MethodPut, "/api/v1/me/settings", body)
	return c
}
