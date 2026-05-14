package app

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseDaemonAction(t *testing.T) {
	t.Parallel()

	if _, _, exitCode, ok := parseDaemonAction(nil); ok || exitCode != 2 {
		t.Fatalf("missing action ok=%t exit=%d, want usage failure", ok, exitCode)
	}
	if _, _, exitCode, ok := parseDaemonAction([]string{"help"}); ok || exitCode != 0 {
		t.Fatalf("help action ok=%t exit=%d, want help stop", ok, exitCode)
	}
	action, args, exitCode, ok := parseDaemonAction([]string{"restart", "--flag"})
	if !ok || exitCode != 0 || action != "restart" || len(args) != 1 || args[0] != "--flag" {
		t.Fatalf("parseDaemonAction() = action=%q args=%v exit=%d ok=%t", action, args, exitCode, ok)
	}
	if _, _, exitCode, ok := parseDaemonAction([]string{"missing"}); ok || exitCode != 2 {
		t.Fatalf("unknown action ok=%t exit=%d, want validation failure", ok, exitCode)
	}
}

func TestParseDaemonInstallConfig(t *testing.T) {
	t.Parallel()

	cfg, exitCode, ok := parseDaemonInstallConfig([]string{
		"--user", "scoop",
		"--backend-port", "8091",
		"--frontend-port", "5174",
		"--scoop-dir", "/tmp/scoop",
	})
	if !ok || exitCode != 0 {
		t.Fatalf("parseDaemonInstallConfig() ok=%t exit=%d", ok, exitCode)
	}
	if cfg.userName != "scoop" || cfg.backendPort != 8091 || cfg.frontendPort != 5174 || cfg.scoopDir != "/tmp/scoop" {
		t.Fatalf("config = %#v", cfg)
	}
	if _, exitCode, ok := parseDaemonInstallConfig([]string{"--backend-port", "0"}); ok || exitCode != 2 {
		t.Fatalf("invalid port ok=%t exit=%d, want validation failure", ok, exitCode)
	}
	if _, exitCode, ok := parseDaemonInstallConfig([]string{"--user", " "}); ok || exitCode != 2 {
		t.Fatalf("blank user ok=%t exit=%d, want validation failure", ok, exitCode)
	}
	if _, exitCode, ok := parseDaemonInstallConfig([]string{"extra"}); ok || exitCode != 2 {
		t.Fatalf("positional args ok=%t exit=%d, want validation failure", ok, exitCode)
	}
}

func TestResolveScoopDirAndUnitRendering(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "backend"), 0o755); err != nil {
		t.Fatalf("mkdir backend: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "frontend"), 0o755); err != nil {
		t.Fatalf("mkdir frontend: %v", err)
	}
	resolved, err := resolveScoopDir(root)
	if err != nil {
		t.Fatalf("resolveScoopDir() error = %v", err)
	}
	if resolved != root {
		t.Fatalf("resolved = %q, want %q", resolved, root)
	}

	serveUnit := buildServeUnitFile("scoop", root, 8091)
	if !strings.Contains(serveUnit, "User=scoop") || !strings.Contains(serveUnit, "--port 8091") {
		t.Fatalf("serve unit = %s", serveUnit)
	}
	frontendUnit := buildFrontendUnitFile("scoop", root, "/usr/local/bin/pnpm", "/usr/bin/node", 5174)
	if !strings.Contains(frontendUnit, "User=scoop") || !strings.Contains(frontendUnit, "--port 5174") {
		t.Fatalf("frontend unit = %s", frontendUnit)
	}
	if got := buildFrontendPathEnv("/usr/local/bin/pnpm", "/usr/bin/node"); !strings.Contains(got, "/usr/local/bin") {
		t.Fatalf("PATH = %q", got)
	}

	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(filepath.Dir(root)); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldCwd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
	relativeResolved, err := resolveScoopDir(filepath.Base(root))
	if err != nil {
		t.Fatalf("resolveScoopDir(relative) error = %v", err)
	}
	if relativeResolved != root {
		t.Fatalf("relative resolved = %q, want %q", relativeResolved, root)
	}
}

func TestResolveDetectedScoopDirUsesCurrentWorkingDirectory(t *testing.T) {
	root := t.TempDir()
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
		t.Fatalf("chdir root: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldCwd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	resolved, err := resolveScoopDir("")
	if err != nil {
		t.Fatalf("resolveScoopDir(empty) error = %v", err)
	}
	if resolved != root {
		t.Fatalf("resolved = %q, want %q", resolved, root)
	}
}

func TestScoopDirCandidateNormalization(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "backend"), 0o755); err != nil {
		t.Fatalf("mkdir backend: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "frontend"), 0o755); err != nil {
		t.Fatalf("mkdir frontend: %v", err)
	}
	seen := map[string]struct{}{}
	resolved, ok := normalizedScoopCandidate(root, seen)
	if !ok {
		t.Fatalf("normalizedScoopCandidate(%q) ok = false, want true", root)
	}
	if resolved == "" {
		t.Fatalf("resolved candidate is empty")
	}
	if _, ok := normalizedScoopCandidate(root, seen); ok {
		t.Fatalf("duplicate candidate ok = true, want false")
	}
	if _, ok := normalizedScoopCandidate(" ", seen); ok {
		t.Fatalf("blank candidate ok = true, want false")
	}

	candidates := scoopDirCandidates()
	if len(candidates) == 0 {
		t.Fatalf("scoopDirCandidates() returned no candidates")
	}
}

func TestInstallDaemonServicesWithDepsWritesAndEnablesUnits(t *testing.T) {
	t.Parallel()

	written := map[string]string{}
	var systemctlCalls [][]string
	exitCode := installDaemonServicesWithDeps(daemonInstallConfig{
		userName:     "scoop",
		backendPort:  8091,
		frontendPort: 5174,
		scoopDir:     "/repo/scoop",
	}, daemonInstallDeps{
		requireRoot: func(string) error { return nil },
		resolveScoopDir: func(raw string) (string, error) {
			if raw != "/repo/scoop" {
				t.Fatalf("resolveScoopDir raw = %q", raw)
			}
			return "/repo/scoop", nil
		},
		resolveNodePnpm: func() (string, string, error) {
			return "/usr/bin/pnpm", "/usr/bin/node", nil
		},
		writeUnit: func(name, content string) error {
			written[name] = content
			return nil
		},
		systemctl: func(args ...string) error {
			systemctlCalls = append(systemctlCalls, append([]string(nil), args...))
			return nil
		},
	})

	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0", exitCode)
	}
	if len(written) != 2 {
		t.Fatalf("written unit count = %d, want 2", len(written))
	}
	wantSystemctl := [][]string{
		{"daemon-reload"},
		{"enable", daemonServeUnitName, daemonFrontendUnitName},
	}
	if !reflect.DeepEqual(systemctlCalls, wantSystemctl) {
		t.Fatalf("systemctl calls = %#v, want %#v", systemctlCalls, wantSystemctl)
	}
}

func TestInstallDaemonServicesWithDepsStopsOnPrepareFailure(t *testing.T) {
	t.Parallel()

	if exitCode := installDaemonServicesWithDeps(daemonInstallConfig{}, daemonInstallDeps{
		requireRoot: func(string) error { return errors.New("not root") },
		resolveScoopDir: func(string) (string, error) {
			t.Fatalf("resolveScoopDir should not be called")
			return "", nil
		},
		resolveNodePnpm: func() (string, string, error) {
			t.Fatalf("resolveNodePnpm should not be called")
			return "", "", nil
		},
		writeUnit: func(string, string) error {
			t.Fatalf("writeUnit should not be called")
			return nil
		},
		systemctl: func(...string) error {
			t.Fatalf("systemctl should not be called")
			return nil
		},
	}); exitCode != 1 {
		t.Fatalf("root failure exitCode = %d, want 1", exitCode)
	}

	exitCode := installDaemonServicesWithDeps(daemonInstallConfig{}, daemonInstallDeps{
		requireRoot: func(string) error { return nil },
		resolveScoopDir: func(string) (string, error) {
			return "", errors.New("missing repo")
		},
		resolveNodePnpm: func() (string, string, error) {
			t.Fatalf("resolveNodePnpm should not be called")
			return "", "", nil
		},
		writeUnit: func(string, string) error {
			t.Fatalf("writeUnit should not be called")
			return nil
		},
		systemctl: func(...string) error {
			t.Fatalf("systemctl should not be called")
			return nil
		},
	})
	if exitCode != 2 {
		t.Fatalf("exitCode = %d, want 2", exitCode)
	}

	exitCode = installDaemonServicesWithDeps(daemonInstallConfig{}, daemonInstallDeps{
		requireRoot:     func(string) error { return nil },
		resolveScoopDir: func(string) (string, error) { return "/repo/scoop", nil },
		resolveNodePnpm: func() (string, string, error) { return "", "", errors.New("missing pnpm") },
		writeUnit: func(string, string) error {
			t.Fatalf("writeUnit should not be called")
			return nil
		},
		systemctl: func(...string) error {
			t.Fatalf("systemctl should not be called")
			return nil
		},
	})
	if exitCode != 1 {
		t.Fatalf("node/pnpm failure exitCode = %d, want 1", exitCode)
	}
}

func TestInstallDaemonServicesWithDepsStopsOnWriteOrSystemctlFailure(t *testing.T) {
	t.Parallel()

	deps := daemonInstallDeps{
		requireRoot:     func(string) error { return nil },
		resolveScoopDir: func(string) (string, error) { return "/repo/scoop", nil },
		resolveNodePnpm: func() (string, string, error) { return "/usr/bin/pnpm", "/usr/bin/node", nil },
		writeUnit:       func(string, string) error { return errors.New("write failed") },
		systemctl: func(...string) error {
			t.Fatalf("systemctl should not run after write failure")
			return nil
		},
	}
	if exitCode := installDaemonServicesWithDeps(daemonInstallConfig{}, deps); exitCode != 1 {
		t.Fatalf("write failure exitCode = %d, want 1", exitCode)
	}

	deps.writeUnit = func(string, string) error { return nil }
	deps.systemctl = func(args ...string) error {
		if len(args) == 1 && args[0] == "daemon-reload" {
			return errors.New("reload failed")
		}
		return nil
	}
	if exitCode := installDaemonServicesWithDeps(daemonInstallConfig{}, deps); exitCode != 1 {
		t.Fatalf("reload failure exitCode = %d, want 1", exitCode)
	}

	deps.systemctl = func(args ...string) error {
		if len(args) > 0 && args[0] == "enable" {
			return errors.New("enable failed")
		}
		return nil
	}
	if exitCode := installDaemonServicesWithDeps(daemonInstallConfig{}, deps); exitCode != 1 {
		t.Fatalf("enable failure exitCode = %d, want 1", exitCode)
	}
}

func TestRunDaemonUninstallWithDeps(t *testing.T) {
	t.Parallel()

	var calls []string
	exitCode := runDaemonUninstallWithDeps(daemonUninstallDeps{
		requireRoot: func(action string) error {
			calls = append(calls, "root:"+action)
			return nil
		},
		stopDisable: func() int {
			calls = append(calls, "stop-disable")
			return 0
		},
		removeUnits: func() int {
			calls = append(calls, "remove")
			return 0
		},
		systemctl: func(args ...string) error {
			calls = append(calls, strings.Join(args, " "))
			return nil
		},
	})
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0", exitCode)
	}
	want := []string{"root:uninstall", "stop-disable", "remove", "daemon-reload"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

func TestRemoveDaemonUnitFilesInRemovesExistingAndIgnoresMissing(t *testing.T) {
	t.Parallel()

	unitDir := t.TempDir()
	existing := filepath.Join(unitDir, daemonServeUnitName)
	if err := os.WriteFile(existing, []byte("[Unit]\n"), 0o644); err != nil {
		t.Fatalf("write unit: %v", err)
	}
	if exitCode := removeDaemonUnitFilesIn(unitDir); exitCode != 0 {
		t.Fatalf("removeDaemonUnitFilesIn() = %d, want 0", exitCode)
	}
	if _, err := os.Stat(existing); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("serve unit still exists or unexpected stat error: %v", err)
	}
	if exitCode := removeDaemonUnitFilesIn(unitDir); exitCode != 0 {
		t.Fatalf("removeDaemonUnitFilesIn() with missing files = %d, want 0", exitCode)
	}
}

func TestRunDaemonUninstallWithDepsStopsOnRootFailure(t *testing.T) {
	t.Parallel()

	exitCode := runDaemonUninstallWithDeps(daemonUninstallDeps{
		requireRoot: func(string) error { return errors.New("not root") },
		stopDisable: func() int {
			t.Fatalf("stopDisable should not be called")
			return 0
		},
		removeUnits: func() int {
			t.Fatalf("removeUnits should not be called")
			return 0
		},
		systemctl: func(...string) error {
			t.Fatalf("systemctl should not be called")
			return nil
		},
	})
	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1", exitCode)
	}
}

func TestRunDaemonUninstallWithDepsStopsOnStepFailures(t *testing.T) {
	t.Parallel()

	if exitCode := runDaemonUninstallWithDeps(daemonUninstallDeps{
		requireRoot: func(string) error { return nil },
		stopDisable: func() int {
			return 1
		},
		removeUnits: func() int {
			t.Fatalf("removeUnits should not run after stop failure")
			return 0
		},
		systemctl: func(...string) error {
			t.Fatalf("systemctl should not run after stop failure")
			return nil
		},
	}); exitCode != 1 {
		t.Fatalf("stop failure exitCode = %d, want 1", exitCode)
	}

	if exitCode := runDaemonUninstallWithDeps(daemonUninstallDeps{
		requireRoot: func(string) error { return nil },
		stopDisable: func() int {
			return 0
		},
		removeUnits: func() int {
			return 1
		},
		systemctl: func(...string) error {
			t.Fatalf("systemctl should not run after remove failure")
			return nil
		},
	}); exitCode != 1 {
		t.Fatalf("remove failure exitCode = %d, want 1", exitCode)
	}

	if exitCode := runDaemonUninstallWithDeps(daemonUninstallDeps{
		requireRoot: func(string) error { return nil },
		stopDisable: func() int {
			return 0
		},
		removeUnits: func() int {
			return 0
		},
		systemctl: func(...string) error {
			return errors.New("reload failed")
		},
	}); exitCode != 1 {
		t.Fatalf("reload failure exitCode = %d, want 1", exitCode)
	}
}

func TestRunDaemonServiceActionWithDeps(t *testing.T) {
	t.Parallel()

	var rootActions []string
	var systemctlCalls [][]string
	exitCode := runDaemonServiceActionWithDeps(
		"restart",
		nil,
		true,
		func(action string) error {
			rootActions = append(rootActions, action)
			return nil
		},
		func(args ...string) error {
			systemctlCalls = append(systemctlCalls, append([]string(nil), args...))
			return nil
		},
	)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0", exitCode)
	}
	if !reflect.DeepEqual(rootActions, []string{"restart"}) {
		t.Fatalf("rootActions = %#v, want restart", rootActions)
	}
	want := [][]string{{"restart", daemonServeUnitName, daemonFrontendUnitName}}
	if !reflect.DeepEqual(systemctlCalls, want) {
		t.Fatalf("systemctlCalls = %#v, want %#v", systemctlCalls, want)
	}

	systemctlCalls = nil
	exitCode = runDaemonServiceActionWithDeps(
		"status",
		nil,
		false,
		func(string) error {
			t.Fatalf("status should not require root")
			return nil
		},
		func(args ...string) error {
			systemctlCalls = append(systemctlCalls, append([]string(nil), args...))
			return nil
		},
	)
	if exitCode != 0 {
		t.Fatalf("status exitCode = %d, want 0", exitCode)
	}
	want = [][]string{{"status", "--no-pager", daemonServeUnitName, daemonFrontendUnitName}}
	if !reflect.DeepEqual(systemctlCalls, want) {
		t.Fatalf("status systemctlCalls = %#v, want %#v", systemctlCalls, want)
	}
}

func TestRunDaemonServiceActionWithDepsStopsOnValidationAndRootFailure(t *testing.T) {
	t.Parallel()

	if exitCode := runDaemonServiceActionWithDeps("start", []string{"extra"}, true, nil, nil); exitCode != 2 {
		t.Fatalf("positional exitCode = %d, want 2", exitCode)
	}
	exitCode := runDaemonServiceActionWithDeps(
		"start",
		nil,
		true,
		func(string) error { return errors.New("not root") },
		func(...string) error {
			t.Fatalf("systemctl should not run after root failure")
			return nil
		},
	)
	if exitCode != 1 {
		t.Fatalf("root failure exitCode = %d, want 1", exitCode)
	}

	exitCode = runDaemonServiceActionWithDeps(
		"start",
		nil,
		true,
		func(string) error { return nil },
		func(...string) error { return errors.New("systemctl failed") },
	)
	if exitCode != 1 {
		t.Fatalf("systemctl failure exitCode = %d, want 1", exitCode)
	}
}
