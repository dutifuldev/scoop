package app

import (
	"fmt"
	"os"
	"time"

	"horse.fit/scoop/internal/cli"
)

type noArgFormatCommandConfig struct {
	envLoader *cli.EnvLoader
	timeout   time.Duration
	format    string
}

func parseNoArgFormatCommand(args []string, name string, defaultFormat string) (noArgFormatCommandConfig, int, bool) {
	fs := newAppFlagSet(name)

	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 30*time.Second, "Command timeout")
	format := fs.String("format", defaultFormat, "Output format: table or json")

	if exitCode, ok := parseAppFlagSet(fs, args); !ok {
		return noArgFormatCommandConfig{}, exitCode, false
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "%s does not accept positional arguments\n", name)
		return noArgFormatCommandConfig{}, 2, false
	}

	outputFormat, err := parseOutputFormat(*format, defaultFormat)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid format: %v\n", err)
		return noArgFormatCommandConfig{}, 2, false
	}
	return noArgFormatCommandConfig{envLoader: envLoader, timeout: *timeout, format: outputFormat}, 0, true
}
