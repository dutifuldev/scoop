package translation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewLocalProviderNormalizesEndpointAndModel(t *testing.T) {
	t.Parallel()

	provider := NewLocalProvider("localhost:1234/custom", " model-a ")
	if provider.endpointURL != "http://localhost:1234/custom/v1/chat/completions" {
		t.Fatalf("unexpected endpoint: %q", provider.endpointURL)
	}
	if provider.ModelName() != "model-a" {
		t.Fatalf("unexpected model: %q", provider.ModelName())
	}
}

func TestLocalProviderTranslate(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var payload localChatRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.Model != "model-a" {
			t.Fatalf("unexpected model: %q", payload.Model)
		}
		if len(payload.Messages) != 1 || !strings.Contains(payload.Messages[0].Content, "hello") {
			t.Fatalf("unexpected prompt: %#v", payload.Messages)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":" bonjour "}}]}`))
	}))
	t.Cleanup(server.Close)

	provider := NewLocalProvider(server.URL, "model-a")
	got, err := provider.Translate(context.Background(), TranslateRequest{
		Text:       "hello",
		SourceLang: "en",
		TargetLang: "fr",
	})
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if got.Text != "bonjour" || got.SourceLang != "en" || got.TargetLang != "fr" || got.ProviderName != "local" {
		t.Fatalf("unexpected response: %#v", got)
	}
}

func TestLocalProviderTranslateValidation(t *testing.T) {
	t.Parallel()

	var nilProvider *LocalProvider
	if _, err := nilProvider.Translate(context.Background(), TranslateRequest{Text: "hello", TargetLang: "fr"}); err == nil {
		t.Fatalf("expected nil provider error")
	}

	provider := NewLocalProvider("http://127.0.0.1:1", "model-a")
	if _, err := provider.Translate(context.Background(), TranslateRequest{TargetLang: "fr"}); err == nil {
		t.Fatalf("expected text validation error")
	}
	if _, err := provider.Translate(context.Background(), TranslateRequest{Text: "hello"}); err == nil {
		t.Fatalf("expected target language validation error")
	}
}

func TestLocalProviderTranslateEndpointErrors(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":{"message":"rate limited"}}`, http.StatusTooManyRequests)
	}))
	t.Cleanup(server.Close)

	provider := NewLocalProvider(server.URL, "model-a")
	if _, err := provider.Translate(context.Background(), TranslateRequest{Text: "hello", TargetLang: "fr"}); err == nil {
		t.Fatalf("expected endpoint status error")
	}
}

func TestPromptAndEndpointHelpers(t *testing.T) {
	t.Parallel()

	if !strings.Contains(buildHYMTPrompt("hello", "en", "fr"), "French") {
		t.Fatalf("expected English prompt to mention French")
	}
	if !strings.Contains(buildHYMTPrompt("hello", "zh", "en"), "英语") {
		t.Fatalf("expected Chinese prompt to mention 英语")
	}
	if targetLanguageLabel("").english != "English" {
		t.Fatalf("expected empty target fallback")
	}
	if normalizeEndpoint(":// bad") != DefaultLocalEndpoint {
		t.Fatalf("expected invalid endpoint fallback")
	}
	if chatCompletionsURL("not a url") != DefaultLocalEndpoint+"/chat/completions" {
		t.Fatalf("expected invalid chat completions fallback")
	}
}
