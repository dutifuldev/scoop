package cli

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

func TestAddEnvFlagDefaults(t *testing.T) {
	t.Parallel()

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	loader := AddEnvFlag(fs, "", "")

	if loader == nil {
		t.Fatalf("expected loader")
	}
	if loader.defaultPath != ".env" {
		t.Fatalf("default path mismatch: %q", loader.defaultPath)
	}
	if got := derefString(loader.value); got != ".env" {
		t.Fatalf("flag default mismatch: %q", got)
	}
}

func TestEnvLoaderLoadUsesOverrideEnvFile(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "custom.env")
	if err := os.WriteFile(envPath, []byte("SCOOP_TEST_ENV=override\n"), 0o600); err != nil {
		t.Fatalf("write env: %v", err)
	}
	t.Setenv("NEWS_PIPELINE_ENV_FILE", envPath)
	t.Setenv("HORSE_ENV_FILE", "")
	t.Setenv("SCOOP_TEST_ENV", "")

	loader := AddEnvFlag(flag.NewFlagSet("test", flag.ContinueOnError), ".env", "")
	loaded, err := loader.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded != envPath {
		t.Fatalf("loaded path mismatch: want %q, got %q", envPath, loaded)
	}
	if got := os.Getenv("SCOOP_TEST_ENV"); got != "override" {
		t.Fatalf("env value mismatch: %q", got)
	}
}

func TestEnvLoaderLoadFallsBackToBasename(t *testing.T) {
	dir := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(previous)
	})
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fallback.env"), []byte("SCOOP_TEST_BASE=fallback\n"), 0o600); err != nil {
		t.Fatalf("write env: %v", err)
	}
	t.Setenv("NEWS_PIPELINE_ENV_FILE", "")
	t.Setenv("HORSE_ENV_FILE", "")
	t.Setenv("SCOOP_TEST_BASE", "")

	requested := filepath.Join(dir, "missing", "fallback.env")
	value := requested
	loader := &EnvLoader{value: &value, defaultPath: ".env"}
	loaded, err := loader.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded != "fallback.env" {
		t.Fatalf("expected basename fallback, got %q", loaded)
	}
	if got := os.Getenv("SCOOP_TEST_BASE"); got != "fallback" {
		t.Fatalf("env value mismatch: %q", got)
	}
}

func TestEnvLoaderLoadRejectsNilLoader(t *testing.T) {
	t.Parallel()

	var loader *EnvLoader
	if _, err := loader.Load(); err == nil {
		t.Fatalf("expected nil loader error")
	}
}
