package translation

import (
	"context"
	"testing"
)

type testProvider struct {
	name      string
	languages []string
}

func (p testProvider) Name() string { return p.name }

func (p testProvider) SupportedLanguages() []string {
	return append([]string(nil), p.languages...)
}

func (p testProvider) Translate(_ context.Context, _ TranslateRequest) (*TranslateResponse, error) {
	return nil, nil
}

func TestLanguageOptions(t *testing.T) {
	t.Parallel()

	registry := NewRegistry("custom")
	if err := registry.Register(testProvider{name: "custom", languages: []string{"EN-us", "nl"}}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	options := TranslationLanguageOptions(registry)
	if !hasLanguageOption(options, "en", "English") {
		t.Fatalf("expected English option in %#v", options)
	}
	if !hasLanguageOption(options, "nl", "NL") {
		t.Fatalf("expected provider language option in %#v", options)
	}

	viewerOptions := ViewerLanguageOptions(registry)
	if len(viewerOptions) == 0 || viewerOptions[0].Code != "original" {
		t.Fatalf("expected original viewer option first, got %#v", viewerOptions)
	}
}

func hasLanguageOption(options []LanguageOption, code string, label string) bool {
	for _, option := range options {
		if option.Code == code && option.Label == label {
			return true
		}
	}
	return false
}
