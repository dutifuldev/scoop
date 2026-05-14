package translation

import (
	"context"
	"strings"
	"testing"
)

func TestGoogleProviderReportsUnimplemented(t *testing.T) {
	provider := NewGoogleProvider()
	if provider.Name() != "google" {
		t.Fatalf("Name() = %q, want google", provider.Name())
	}
	if len(provider.SupportedLanguages()) != 0 {
		t.Fatalf("SupportedLanguages() = %#v, want empty placeholder", provider.SupportedLanguages())
	}
	if _, err := provider.Translate(context.Background(), TranslateRequest{Text: "hello", TargetLang: "zh"}); err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("Translate() error = %v, want not implemented", err)
	}
}

func TestLocalProviderSupportedLanguages(t *testing.T) {
	t.Parallel()

	provider := NewLocalProvider("", "")
	if provider.Name() != "local" {
		t.Fatalf("Name() = %q, want local", provider.Name())
	}
	if provider.ModelName() != DefaultLocalModel {
		t.Fatalf("ModelName() = %q, want default model", provider.ModelName())
	}
	if len(provider.SupportedLanguages()) == 0 {
		t.Fatalf("SupportedLanguages() returned no languages")
	}
	if got := (*LocalProvider)(nil).ModelName(); got != "" {
		t.Fatalf("nil ModelName() = %q, want empty", got)
	}
}

func TestLanguageOptionsIncludeRegistryOnlyCodes(t *testing.T) {
	registry := NewRegistry("stub")
	if err := registry.Register(&stubProvider{name: "stub"}); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := registry.Register(&customLanguageProvider{name: "custom"}); err != nil {
		t.Fatalf("register custom provider: %v", err)
	}

	options := TranslationLanguageOptions(registry)
	if !hasProviderLanguageOption(options, "xx", "XX") {
		t.Fatalf("custom language missing from options: %#v", options)
	}
	viewerOptions := ViewerLanguageOptions(registry)
	if len(viewerOptions) == 0 || viewerOptions[0].Code != "original" {
		t.Fatalf("viewer options = %#v, want original first", viewerOptions)
	}
	codes := SupportedTranslationLanguageCodes()
	if len(codes) == 0 || codes[0] != "ar" {
		t.Fatalf("supported codes = %#v", codes)
	}
}

type customLanguageProvider struct {
	name string
}

func (p *customLanguageProvider) Name() string {
	return p.name
}

func (p *customLanguageProvider) SupportedLanguages() []string {
	return []string{"xx", " ", "EN"}
}

func (p *customLanguageProvider) Translate(context.Context, TranslateRequest) (*TranslateResponse, error) {
	return &TranslateResponse{Text: "translated"}, nil
}

func hasProviderLanguageOption(options []LanguageOption, code string, label string) bool {
	for _, option := range options {
		if option.Code == code && option.Label == label {
			return true
		}
	}
	return false
}
