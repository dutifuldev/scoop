package app

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	daemonServeUnitName    = "scoop-serve.service"
	daemonFrontendUnitName = "scoop-frontend.service"
	systemdUnitDir         = "/etc/systemd/system"
)

var daemonUnitNames = []string{
	daemonServeUnitName,
	daemonFrontendUnitName,
}

type daemonInstallConfig struct {
	userName     string
	backendPort  int
	frontendPort int
	scoopDir     string
}

type daemonInstallDeps struct {
	requireRoot     func(string) error
	resolveScoopDir func(string) (string, error)
	resolveNodePnpm func() (string, string, error)
	writeUnit       func(string, string) error
	systemctl       func(...string) error
}

type daemonUninstallDeps struct {
	requireRoot func(string) error
	stopDisable func() int
	removeUnits func() int
	systemctl   func(...string) error
}

func runDaemon(args []string) int {
	action, remainingArgs, exitCode, ok := parseDaemonAction(args)
	if !ok {
		return exitCode
	}

	switch action {
	case "install":
		return runDaemonInstall(remainingArgs)
	case "uninstall":
		return runDaemonUninstall(remainingArgs)
	case "status":
		return runDaemonServiceAction(action, remainingArgs, false)
	default:
		return runDaemonServiceAction(action, remainingArgs, true)
	}
}

func parseDaemonAction(args []string) (string, []string, int, bool) {
	if len(args) == 0 {
		printDaemonUsage()
		return "", nil, 2, false
	}
	action := strings.ToLower(strings.TrimSpace(args[0]))
	switch action {
	case "help", "-h", "--help":
		printDaemonUsage()
		return "", nil, 0, false
	case "install", "uninstall", "start", "stop", "restart", "status":
		return action, args[1:], 0, true
	}
	fmt.Fprintf(os.Stderr, "unknown daemon action: %s\n\n", args[0])
	printDaemonUsage()
	return "", nil, 2, false
}

func runDaemonInstall(args []string) int {
	return runParsedCommand(args, parseDaemonInstallConfig, installDaemonServices)
}

func parseDaemonInstallConfig(args []string) (daemonInstallConfig, int, bool) {
	fs := newAppFlagSet("daemon install")

	userName := fs.String("user", defaultDaemonUser(), "Run services as this Linux user")
	backendPort := fs.Int("backend-port", 8090, "Port for scoop-serve")
	frontendPort := fs.Int("frontend-port", 5173, "Port for scoop-frontend")
	scoopDir := fs.String("scoop-dir", "", "Scoop root containing backend/ and frontend/ (auto-detected if empty)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return daemonInstallConfig{}, 0, false
		}
		return daemonInstallConfig{}, 2, false
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "daemon install does not accept positional args")
		return daemonInstallConfig{}, 2, false
	}
	cfg := daemonInstallConfig{
		userName:     strings.TrimSpace(*userName),
		backendPort:  *backendPort,
		frontendPort: *frontendPort,
		scoopDir:     *scoopDir,
	}
	if err := validateDaemonInstallConfig(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return daemonInstallConfig{}, 2, false
	}
	return cfg, 0, true
}

func defaultDaemonUser() string {
	defaultUser := strings.TrimSpace(os.Getenv("USER"))
	if defaultUser == "" {
		return "root"
	}
	return defaultUser
}

func validateDaemonInstallConfig(cfg daemonInstallConfig) error {
	if err := validatePort(cfg.backendPort, "--backend-port"); err != nil {
		return err
	}
	if err := validatePort(cfg.frontendPort, "--frontend-port"); err != nil {
		return err
	}
	if cfg.userName == "" {
		return fmt.Errorf("--user must not be empty")
	}
	return nil
}

func installDaemonServices(cfg daemonInstallConfig) int {
	return installDaemonServicesWithDeps(cfg, daemonInstallDeps{
		requireRoot:     requireRoot,
		resolveScoopDir: resolveScoopDir,
		resolveNodePnpm: resolveNodeAndPnpm,
		writeUnit:       writeUnitFile,
		systemctl:       runSystemctl,
	})
}

func installDaemonServicesWithDeps(cfg daemonInstallConfig, deps daemonInstallDeps) int {
	unitFiles, exitCode, ok := prepareDaemonInstall(cfg, deps)
	if !ok {
		return exitCode
	}
	if exitCode := writeDaemonUnitFiles(unitFiles, deps.writeUnit); exitCode != 0 {
		return exitCode
	}
	if exitCode := reloadAndEnableDaemonUnits(deps.systemctl); exitCode != 0 {
		return exitCode
	}
	fmt.Printf("Installed %s and %s\n", daemonServeUnitName, daemonFrontendUnitName)
	fmt.Println("Services are enabled on boot. Run `scoop daemon start` to start them now.")
	return 0
}

func prepareDaemonInstall(cfg daemonInstallConfig, deps daemonInstallDeps) (map[string]string, int, bool) {
	if err := deps.requireRoot("install"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return nil, 1, false
	}
	resolvedScoopDir, err := deps.resolveScoopDir(cfg.scoopDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to resolve --scoop-dir: %v\n", err)
		return nil, 2, false
	}
	pnpmPath, nodePath, err := deps.resolveNodePnpm()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to locate node/pnpm in PATH: %v\n", err)
		return nil, 1, false
	}
	return map[string]string{
		daemonServeUnitName:    buildServeUnitFile(cfg.userName, resolvedScoopDir, cfg.backendPort),
		daemonFrontendUnitName: buildFrontendUnitFile(cfg.userName, resolvedScoopDir, pnpmPath, nodePath, cfg.frontendPort),
	}, 0, true
}

func writeDaemonUnitFiles(unitFiles map[string]string, writeUnit func(string, string) error) int {
	for unitName, unitContent := range unitFiles {
		if err := writeUnit(unitName, unitContent); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write %s: %v\n", unitName, err)
			return 1
		}
	}
	return 0
}

func reloadAndEnableDaemonUnits(systemctl func(...string) error) int {
	if err := systemctl("daemon-reload"); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to reload systemd units: %v\n", err)
		return 1
	}
	enableArgs := append([]string{"enable"}, daemonUnitNames...)
	if err := systemctl(enableArgs...); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to enable services: %v\n", err)
		return 1
	}
	return 0
}

func runDaemonUninstall(args []string) int {
	exitCode, ok := parseDaemonNoArgCommand("daemon uninstall", args, "daemon uninstall does not accept positional args")
	if !ok {
		return exitCode
	}
	return runDaemonUninstallWithDeps(daemonUninstallDeps{
		requireRoot: requireRoot,
		stopDisable: stopAndDisableDaemonUnits,
		removeUnits: removeDaemonUnitFiles,
		systemctl:   runSystemctl,
	})
}

func runDaemonUninstallWithDeps(deps daemonUninstallDeps) int {
	if err := deps.requireRoot("uninstall"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if exitCode := deps.stopDisable(); exitCode != 0 {
		return exitCode
	}
	if exitCode := deps.removeUnits(); exitCode != 0 {
		return exitCode
	}
	if err := deps.systemctl("daemon-reload"); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to reload systemd units: %v\n", err)
		return 1
	}

	fmt.Printf("Removed %s and %s\n", daemonServeUnitName, daemonFrontendUnitName)
	return 0
}

func stopAndDisableDaemonUnits() int {
	runSystemctlWithWarning("stop", "stop")
	runSystemctlWithWarning("disable", "disable")
	return 0
}

func runSystemctlWithWarning(action, label string) {
	args := append([]string{action}, daemonUnitNames...)
	if err := runSystemctl(args...); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to %s one or more services: %v\n", label, err)
	}
}

func removeDaemonUnitFiles() int {
	return removeDaemonUnitFilesIn(systemdUnitDir)
}

func removeDaemonUnitFilesIn(unitDir string) int {
	for _, unitName := range daemonUnitNames {
		unitPath := filepath.Join(unitDir, unitName)
		if err := os.Remove(unitPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "Failed to remove %s: %v\n", unitPath, err)
			return 1
		}
	}
	return 0
}

func runDaemonServiceAction(action string, args []string, requireRootPrivileges bool) int {
	return runDaemonServiceActionWithDeps(action, args, requireRootPrivileges, requireRoot, runSystemctl)
}

func runDaemonServiceActionWithDeps(
	action string,
	args []string,
	requireRootPrivileges bool,
	requireRootFunc func(string) error,
	systemctlFunc func(...string) error,
) int {
	exitCode, ok := parseDaemonNoArgCommand("daemon "+action, args, fmt.Sprintf("daemon %s does not accept positional args", action))
	if !ok {
		return exitCode
	}
	if requireRootPrivileges {
		if err := requireRootFunc(action); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}

	systemctlArgs := make([]string, 0, 3+len(daemonUnitNames))
	systemctlArgs = append(systemctlArgs, action)
	if action == "status" {
		systemctlArgs = append(systemctlArgs, "--no-pager")
	}
	systemctlArgs = append(systemctlArgs, daemonUnitNames...)

	if err := systemctlFunc(systemctlArgs...); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to %s services: %v\n", action, err)
		return 1
	}
	return 0
}

func parseDaemonNoArgCommand(name string, args []string, positionalMessage string) (int, bool) {
	fs := newAppFlagSet(name)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, false
		}
		return 2, false
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, positionalMessage)
		return 2, false
	}
	return 0, true
}

func validatePort(port int, flagName string) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("%s must be between 1 and 65535", flagName)
	}
	return nil
}

func requireRoot(action string) error {
	if os.Geteuid() == 0 {
		return nil
	}
	return fmt.Errorf("daemon %s requires root privileges; run with sudo: sudo scoop daemon %s", action, action)
}

func resolveScoopDir(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed != "" {
		return resolveExplicitScoopDir(trimmed)
	}
	return resolveDetectedScoopDir()
}

func resolveExplicitScoopDir(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("normalize path %q: %w", path, err)
	}
	if !isScoopRoot(absPath) {
		return "", fmt.Errorf("%q must contain backend/ and frontend/ directories", absPath)
	}
	return absPath, nil
}

func resolveDetectedScoopDir() (string, error) {
	detected, err := autoDetectScoopDir()
	if err != nil {
		return "", err
	}
	if !isScoopRoot(detected) {
		return "", fmt.Errorf("auto-detected path %q does not contain backend/ and frontend/", detected)
	}
	return detected, nil
}

func autoDetectScoopDir() (string, error) {
	candidates := scoopDirCandidates()
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		if found, ok := normalizedScoopCandidate(candidate, seen); ok {
			return found, nil
		}
	}

	return "", errors.New("unable to auto-detect scoop directory from executable location or cwd parent; use --scoop-dir")
}

func scoopDirCandidates() []string {
	candidates := make([]string, 0, 6)
	if exePath, err := os.Executable(); err == nil {
		resolvedExePath := exePath
		if resolvedPath, err := filepath.EvalSymlinks(exePath); err == nil {
			resolvedExePath = resolvedPath
		}

		exeDir := filepath.Dir(resolvedExePath)
		candidates = append(candidates, exeDir, filepath.Dir(exeDir))
	}

	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd, filepath.Dir(cwd))
	}
	return candidates
}

func normalizedScoopCandidate(candidate string, seen map[string]struct{}) (string, bool) {
	if strings.TrimSpace(candidate) == "" {
		return "", false
	}
	absPath, err := filepath.Abs(candidate)
	if err != nil {
		return "", false
	}
	if _, exists := seen[absPath]; exists {
		return "", false
	}
	seen[absPath] = struct{}{}
	return absPath, isScoopRoot(absPath)
}

func isScoopRoot(root string) bool {
	return isDir(filepath.Join(root, "backend")) && isDir(filepath.Join(root, "frontend"))
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func resolveNodeAndPnpm() (string, string, error) {
	pnpmPath, err := exec.LookPath("pnpm")
	if err != nil {
		return "", "", err
	}
	nodePath, err := exec.LookPath("node")
	if err != nil {
		return "", "", err
	}

	return pnpmPath, nodePath, nil
}

func buildServeUnitFile(userName, scoopDir string, backendPort int) string {
	lines := []string{
		"[Unit]",
		"Description=Scoop backend API service",
		"After=network.target postgresql.service",
		"",
		"[Service]",
		"Type=simple",
		"User=" + userName,
		"WorkingDirectory=" + filepath.Join(scoopDir, "backend"),
		"ExecStart=/usr/local/bin/scoop serve --host 0.0.0.0 --port " + strconv.Itoa(backendPort),
		"Restart=on-failure",
		"RestartSec=5",
		"",
		"[Install]",
		"WantedBy=multi-user.target",
		"",
	}
	return strings.Join(lines, "\n")
}

func buildFrontendUnitFile(userName, scoopDir, pnpmPath, nodePath string, frontendPort int) string {
	lines := []string{
		"[Unit]",
		"Description=Scoop frontend dev server",
		"After=scoop-serve.service",
		"",
		"[Service]",
		"Type=simple",
		"User=" + userName,
		"WorkingDirectory=" + filepath.Join(scoopDir, "frontend"),
		fmt.Sprintf("Environment=\"PATH=%s\"", buildFrontendPathEnv(pnpmPath, nodePath)),
		fmt.Sprintf("ExecStart=%s run dev --host 0.0.0.0 --port %d --strictPort", pnpmPath, frontendPort),
		"Restart=on-failure",
		"RestartSec=5",
		"",
		"[Install]",
		"WantedBy=multi-user.target",
		"",
	}
	return strings.Join(lines, "\n")
}

func buildFrontendPathEnv(pnpmPath, nodePath string) string {
	pathParts := []string{
		filepath.Dir(pnpmPath),
		filepath.Dir(nodePath),
		"/usr/local/sbin",
		"/usr/local/bin",
		"/usr/sbin",
		"/usr/bin",
		"/sbin",
		"/bin",
	}

	deduped := make([]string, 0, len(pathParts))
	seen := map[string]struct{}{}
	for _, part := range pathParts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		if _, exists := seen[part]; exists {
			continue
		}
		seen[part] = struct{}{}
		deduped = append(deduped, part)
	}

	return strings.Join(deduped, ":")
}

func writeUnitFile(name, content string) error {
	unitPath := filepath.Join(systemdUnitDir, name)
	return os.WriteFile(unitPath, []byte(content), 0o644)
}

func runSystemctl(args ...string) error {
	cmd := exec.Command("systemctl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("systemctl %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

func printDaemonUsage() {
	fmt.Fprintln(os.Stderr, "scoop daemon")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  scoop daemon <action> [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Actions:")
	fmt.Fprintln(os.Stderr, "  install     Write unit files, daemon-reload, and enable services on boot")
	fmt.Fprintln(os.Stderr, "  uninstall   Stop, disable, and remove unit files")
	fmt.Fprintln(os.Stderr, "  start       Start both services")
	fmt.Fprintln(os.Stderr, "  stop        Stop both services")
	fmt.Fprintln(os.Stderr, "  restart     Restart both services")
	fmt.Fprintln(os.Stderr, "  status      Show status for both services")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Install flags:")
	fmt.Fprintln(os.Stderr, "  --user <name>          Service user (default: $USER)")
	fmt.Fprintln(os.Stderr, "  --backend-port <n>     Backend port (default: 8090)")
	fmt.Fprintln(os.Stderr, "  --frontend-port <n>    Frontend port (default: 5173)")
	fmt.Fprintln(os.Stderr, "  --scoop-dir <path>     Scoop root directory (auto-detect by default)")
}
