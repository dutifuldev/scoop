package app

import (
	"errors"
	"flag"
	"fmt"
	"time"

	"horse.fit/scoop/internal/cli"
)

func runHealth(args []string) int {
	fs := newAppFlagSet("health")

	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 5*time.Second, "Database ping timeout")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	runtime, exitCode, ok := openCommandRuntime(*timeout, envLoader, "health check failed")
	if !ok {
		return exitCode
	}
	defer runtime.Close()

	runtime.logger.Info().
		Dur("timeout", *timeout).
		Msg("database health check passed")
	fmt.Println("ok: database ping successful")
	return 0
}
