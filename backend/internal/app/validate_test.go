package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectJSONFilesRecursive(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "a.json"), `{"k":"v"}`)
	mustWriteFile(t, filepath.Join(root, "b.txt"), `x`)
	mustWriteFile(t, filepath.Join(root, ".hidden.json"), `{}`)
	mustWriteFile(t, filepath.Join(root, "nested", "c.json"), `{"k":"v2"}`)

	files, err := collectJSONFiles(root, true)
	if err != nil {
		t.Fatalf("collectJSONFiles failed: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 json files, got %d (%v)", len(files), files)
	}
}

func TestCollectJSONFilesNonRecursive(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "a.json"), `{"k":"v"}`)
	mustWriteFile(t, filepath.Join(root, "nested", "c.json"), `{"k":"v2"}`)

	files, err := collectJSONFiles(root, false)
	if err != nil {
		t.Fatalf("collectJSONFiles failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 json file, got %d (%v)", len(files), files)
	}
}

func TestExecuteValidateCommandOutcomes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	valid := `{"payload_version":"v1","source":"manual","source_item_id":"1","title":"Valid","source_metadata":{"collection":"openclaw","job_name":"manual","job_run_id":"1","scraped_at":"2026-05-14T00:00:00Z"}}`
	mustWriteFile(t, filepath.Join(root, "valid.json"), valid)
	if code := executeValidateCommand(validateCommandConfig{dir: root, recursive: true}); code != 0 {
		t.Fatalf("executeValidateCommand(valid) = %d, want 0", code)
	}

	invalidRoot := t.TempDir()
	mustWriteFile(t, filepath.Join(invalidRoot, "bad.json"), `{"payload_version":"v1"`)
	if code := executeValidateCommand(validateCommandConfig{dir: invalidRoot, recursive: false}); code != 1 {
		t.Fatalf("executeValidateCommand(invalid) = %d, want 1", code)
	}

	emptyRoot := t.TempDir()
	if code := executeValidateCommand(validateCommandConfig{dir: emptyRoot, recursive: true}); code != 1 {
		t.Fatalf("executeValidateCommand(empty) = %d, want 1", code)
	}
	if code := executeValidateCommand(validateCommandConfig{dir: filepath.Join(root, "missing"), recursive: true}); code != 1 {
		t.Fatalf("executeValidateCommand(missing) = %d, want 1", code)
	}
}

func TestParseValidateCommand(t *testing.T) {
	t.Parallel()

	cfg, exitCode, ok := parseValidateCommand([]string{"--dir", " items ", "--recursive=false"})
	if !ok || exitCode != 0 || cfg.dir != "items" || cfg.recursive {
		t.Fatalf("parseValidateCommand() cfg=%#v exit=%d ok=%t", cfg, exitCode, ok)
	}
	if _, exitCode, ok := parseValidateCommand([]string{"--help"}); ok || exitCode != 0 {
		t.Fatalf("parseValidateCommand(help) ok=%t exit=%d, want help stop", ok, exitCode)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
}
