package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadJSONInputUsesFileWhenProvided(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	payloadPath := filepath.Join(dir, "payload.json")
	if err := os.WriteFile(payloadPath, []byte(" \n {\"title\":\"file\"} \n"), 0o644); err != nil {
		t.Fatalf("write payload file: %v", err)
	}

	got, err := loadJSONInput(`{"title":"inline"}`, payloadPath, "payload")
	if err != nil {
		t.Fatalf("loadJSONInput() error = %v", err)
	}
	if string(got) != `{"title":"file"}` {
		t.Fatalf("loadJSONInput() = %s, want trimmed file payload", got)
	}
}

func TestLoadJSONInputRejectsEmptyInputs(t *testing.T) {
	t.Parallel()

	if _, err := loadJSONInput(" \n ", "", "payload"); err == nil || !strings.Contains(err.Error(), "payload JSON is empty") {
		t.Fatalf("blank inline error = %v", err)
	}

	emptyPath := filepath.Join(t.TempDir(), "empty.json")
	if err := os.WriteFile(emptyPath, []byte(" \n "), 0o644); err != nil {
		t.Fatalf("write empty payload file: %v", err)
	}
	if _, err := loadJSONInput(`{"title":"inline"}`, emptyPath, "payload"); err == nil || !strings.Contains(err.Error(), "is empty") {
		t.Fatalf("blank file error = %v", err)
	}
	if _, err := loadJSONInput(`{"title":"inline"}`, filepath.Join(t.TempDir(), "missing.json"), "payload"); err == nil || !strings.Contains(err.Error(), "read payload file") {
		t.Fatalf("missing file error = %v", err)
	}
}

func TestConfirmTargetAction(t *testing.T) {
	if code := confirmTargetAction(targetActionCommandConfig{force: true}, "delete"); code != 0 {
		t.Fatalf("forced confirmTargetAction() = %d, want 0", code)
	}

	oldStdin := os.Stdin
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdin = read
	t.Cleanup(func() {
		os.Stdin = oldStdin
		_ = read.Close()
	})
	if _, err := write.WriteString("no\n"); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	_ = write.Close()

	cfg := targetActionCommandConfig{target: "collection", value: "openclaw"}
	if code := confirmTargetAction(cfg, "delete"); code != 1 {
		t.Fatalf("cancelled confirmTargetAction() = %d, want 1", code)
	}
}

func TestResolveScoopDirValidationPaths(t *testing.T) {
	root := t.TempDir()
	if _, err := resolveScoopDir(filepath.Join(root, "missing")); err == nil || !strings.Contains(err.Error(), "must contain backend/ and frontend/") {
		t.Fatalf("missing scoop root error = %v", err)
	}

	if err := os.Mkdir(filepath.Join(root, "backend"), 0o755); err != nil {
		t.Fatalf("mkdir backend: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "frontend"), 0o755); err != nil {
		t.Fatalf("mkdir frontend: %v", err)
	}

	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldCwd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	detected, err := autoDetectScoopDir()
	if err != nil {
		t.Fatalf("autoDetectScoopDir() error = %v", err)
	}
	if detected != root {
		t.Fatalf("autoDetectScoopDir() = %q, want %q", detected, root)
	}
}

func TestResolveNodeAndPnpmUsesPathExecutables(t *testing.T) {
	dir := t.TempDir()
	writeExecutable(t, filepath.Join(dir, "pnpm"))
	writeExecutable(t, filepath.Join(dir, "node"))
	t.Setenv("PATH", dir)

	pnpmPath, nodePath, err := resolveNodeAndPnpm()
	if err != nil {
		t.Fatalf("resolveNodeAndPnpm() error = %v", err)
	}
	if pnpmPath != filepath.Join(dir, "pnpm") || nodePath != filepath.Join(dir, "node") {
		t.Fatalf("resolveNodeAndPnpm() = %q, %q", pnpmPath, nodePath)
	}
}

func TestParseTargetActionCommandAcceptsDryRunAndForce(t *testing.T) {
	t.Parallel()

	cfg, exitCode, ok := parseTargetActionCommand(
		[]string{"collection", "--dry-run", "--force", "--timeout", "2s", "openclaw"},
		targetActionCommandSpec{
			commandName:  "delete",
			validTarget:  func(target string) bool { return target == "collection" },
			usage:        func() {},
			emptyMessage: "collection is required",
		},
	)
	if !ok || exitCode != 0 {
		t.Fatalf("parseTargetActionCommand() ok=%t exit=%d", ok, exitCode)
	}
	if cfg.target != "collection" || cfg.value != "openclaw" || !cfg.dryRun || !cfg.force || cfg.timeout != 2*time.Second {
		t.Fatalf("config = %#v", cfg)
	}
}

func TestParseTargetActionCommandRejectsInvalidShapes(t *testing.T) {
	t.Parallel()

	spec := targetActionCommandSpec{
		commandName:  "delete",
		validTarget:  func(target string) bool { return target == "story" },
		usage:        func() {},
		emptyMessage: "story UUID is required",
	}
	cases := [][]string{
		nil,
		{"article", "uuid"},
		{"story"},
		{"story", " "},
	}
	for _, args := range cases {
		if _, exitCode, ok := parseTargetActionCommand(args, spec); ok || exitCode != 2 {
			t.Fatalf("parseTargetActionCommand(%v) ok=%t exit=%d, want validation failure", args, ok, exitCode)
		}
	}
}

func TestParseAppFlagSetDistinguishesHelpFromParseErrors(t *testing.T) {
	t.Parallel()

	if exitCode, ok := parseAppFlagSet(newAppFlagSet("test"), []string{"--help"}); ok || exitCode != 0 {
		t.Fatalf("help parse exit=%d ok=%t, want help stop", exitCode, ok)
	}
	fs := newAppFlagSet("test")
	fs.Int("count", 0, "count")
	if exitCode, ok := parseAppFlagSet(fs, []string{"--count", "not-a-number"}); ok || exitCode != 2 {
		t.Fatalf("bad flag parse exit=%d ok=%t, want parse error", exitCode, ok)
	}
}

func TestRunSubcommandsDefaultAndFlagHandling(t *testing.T) {
	t.Parallel()

	var usageCalled bool
	defaultRun := func(args []string) int {
		if len(args) != 0 {
			t.Fatalf("default args = %#v, want none", args)
		}
		return 7
	}
	if code := runSubcommands("items", nil, nil, func() { usageCalled = true }, defaultRun); code != 7 || usageCalled {
		t.Fatalf("default subcommand code=%d usage=%t, want default run", code, usageCalled)
	}

	usageCalled = false
	if code := runSubcommands("items", []string{"--bad"}, nil, func() { usageCalled = true }, nil); code != 2 || !usageCalled {
		t.Fatalf("flag without default code=%d usage=%t, want usage error", code, usageCalled)
	}
}

func TestRunParsedCommandSkipsExecuteOnParseStop(t *testing.T) {
	t.Parallel()

	code := runParsedCommand(
		[]string{"bad"},
		func([]string) (string, int, bool) { return "", 2, false },
		func(string) int {
			t.Fatalf("execute should not run after parse stop")
			return 0
		},
	)
	if code != 2 {
		t.Fatalf("runParsedCommand() = %d, want parse exit code", code)
	}
}

func writeExecutable(t *testing.T, path string) {
	t.Helper()

	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", path, err)
	}
}
