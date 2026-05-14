package translation

import "testing"

func TestNewRegistryFromEnvFallsBackToRegisteredProvider(t *testing.T) {
	t.Setenv(ProviderEnvVar, "missing")
	t.Setenv("TRANSLATION_ENDPOINT", "http://127.0.0.1:8845/custom")
	t.Setenv("TRANSLATION_MODEL", "local-model")

	registry := NewRegistryFromEnv()
	if registry.DefaultProvider() != DefaultProviderName {
		t.Fatalf("DefaultProvider() = %q, want %q", registry.DefaultProvider(), DefaultProviderName)
	}
	provider, err := registry.Provider("")
	if err != nil {
		t.Fatalf("Provider(default) error = %v", err)
	}
	localProvider, ok := provider.(*LocalProvider)
	if !ok {
		t.Fatalf("default provider type = %T, want *LocalProvider", provider)
	}
	if localProvider.ModelName() != "local-model" {
		t.Fatalf("ModelName() = %q, want local-model", localProvider.ModelName())
	}
}

func TestRegistryProviderNamesAreSorted(t *testing.T) {
	t.Parallel()

	registry := NewRegistry("second")
	if err := registry.Register(&stubProvider{name: "second"}); err != nil {
		t.Fatalf("register second: %v", err)
	}
	if err := registry.Register(&stubProvider{name: "first"}); err != nil {
		t.Fatalf("register first: %v", err)
	}
	names := registry.ProviderNames()
	if len(names) != 2 || names[0] != "first" || names[1] != "second" {
		t.Fatalf("ProviderNames() = %#v, want sorted names", names)
	}
}

func TestManagerDefaultProvider(t *testing.T) {
	t.Parallel()

	registry := NewRegistry("stub")
	if got := NewManager(nil, registry).DefaultProvider(); got != "stub" {
		t.Fatalf("NewManager DefaultProvider() = %q, want stub", got)
	}
	if got := NewManagerWithStore(&stubTranslationStore{}, registry).DefaultProvider(); got != "stub" {
		t.Fatalf("DefaultProvider() = %q, want stub", got)
	}

	if got := (*Manager)(nil).DefaultProvider(); got != "" {
		t.Fatalf("nil manager DefaultProvider() = %q, want empty", got)
	}
	if got := NewManagerWithStore(&stubTranslationStore{}, nil).DefaultProvider(); got != "" {
		t.Fatalf("nil registry DefaultProvider() = %q, want empty", got)
	}
}
