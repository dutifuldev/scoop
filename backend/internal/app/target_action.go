package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"horse.fit/scoop/internal/cli"
	"horse.fit/scoop/internal/db"
	"horse.fit/scoop/internal/globaltime"
)

type targetActionCommandConfig struct {
	envLoader *cli.EnvLoader
	timeout   time.Duration
	target    string
	value     string
	dryRun    bool
	force     bool
}

type targetActionCommandSpec struct {
	commandName  string
	validTarget  func(string) bool
	usage        func()
	emptyMessage string
}

type targetActionRunner func(context.Context, *db.Pool, targetActionCommandConfig, time.Time) int

func parseTargetActionCommand(args []string, spec targetActionCommandSpec) (targetActionCommandConfig, int, bool) {
	if len(args) == 0 {
		spec.usage()
		return targetActionCommandConfig{}, 2, false
	}

	target := strings.ToLower(strings.TrimSpace(args[0]))
	if !spec.validTarget(target) {
		fmt.Fprintf(os.Stderr, "Unknown %s target: %s\n\n", spec.commandName, args[0])
		spec.usage()
		return targetActionCommandConfig{}, 2, false
	}

	fs := newAppFlagSet(spec.commandName + " " + target)
	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 30*time.Second, "Command timeout")
	dryRun := fs.Bool("dry-run", false, "Preview affected rows without applying changes")
	force := fs.Bool("force", false, "Skip confirmation prompt")

	if exitCode, ok := parseAppFlagSet(fs, args[1:]); !ok {
		return targetActionCommandConfig{}, exitCode, false
	}
	if fs.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "%s requires one argument\n", spec.commandName)
		spec.usage()
		return targetActionCommandConfig{}, 2, false
	}

	value := strings.TrimSpace(fs.Arg(0))
	if value == "" {
		fmt.Fprintln(os.Stderr, spec.emptyMessage)
		return targetActionCommandConfig{}, 2, false
	}
	return targetActionCommandConfig{
		envLoader: envLoader,
		timeout:   *timeout,
		target:    target,
		value:     value,
		dryRun:    *dryRun,
		force:     *force,
	}, 0, true
}

func executeConfirmedTargetAction(cfg targetActionCommandConfig, action string, runner targetActionRunner) int {
	if exitCode := confirmTargetAction(cfg, action); exitCode != 0 {
		return exitCode
	}
	now := globaltime.UTC()
	return runWithReadPool(cfg.timeout, cfg.envLoader, func(ctx context.Context, pool *db.Pool) int {
		return runner(ctx, pool, cfg, now)
	})
}

func confirmTargetAction(cfg targetActionCommandConfig, action string) int {
	if cfg.force {
		return 0
	}
	ok, err := confirmDangerousAction(fmt.Sprintf("Proceed with %s %s %q?", action, cfg.target, cfg.value))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read confirmation: %v\n", err)
		return 1
	}
	if !ok {
		fmt.Fprintln(os.Stderr, "Cancelled")
		return 1
	}
	return 0
}
