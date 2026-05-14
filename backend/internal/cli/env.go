package cli

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

// EnvLoader loads .env files with a predictable override order.
type EnvLoader struct {
	value       *string
	defaultPath string
}

// AddEnvFlag registers an --env flag and returns an EnvLoader.
func AddEnvFlag(fs *flag.FlagSet, defaultPath, description string) *EnvLoader {
	if fs == nil {
		fs = flag.CommandLine
	}
	if defaultPath == "" {
		defaultPath = ".env"
	}
	if description == "" {
		description = "Path to the .env file"
	}

	value := fs.String("env", defaultPath, description)
	return &EnvLoader{
		value:       value,
		defaultPath: defaultPath,
	}
}

// Load resolves and loads environment variables using the configured flag value.
func (l *EnvLoader) Load() (string, error) {
	if l == nil {
		return "", fmt.Errorf("env loader is nil")
	}

	log.SetOutput(os.Stderr)
	if path, ok := loadOverrideEnvFile([]string{"NEWS_PIPELINE_ENV_FILE", "HORSE_ENV_FILE"}); ok {
		return path, nil
	}

	requested := l.requestedPath()
	candidates := envFileCandidates(requested, l.defaultPath)
	for _, candidate := range candidates {
		if err := godotenv.Overload(candidate.path); err == nil {
			log.Printf("Loaded environment from %s: %s", candidate.label, candidate.path)
			return candidate.path, nil
		}
	}

	return "", fmt.Errorf("failed to load env file from %s", requested)
}

func loadOverrideEnvFile(envVars []string) (string, bool) {
	for _, envVar := range envVars {
		custom := strings.TrimSpace(os.Getenv(envVar))
		if custom == "" {
			continue
		}
		if err := godotenv.Overload(custom); err == nil {
			log.Printf("Loaded environment from %s: %s", envVar, custom)
			return custom, true
		}
		log.Printf("Warning: failed to load %s=%s", envVar, custom)
	}
	return "", false
}

func (l *EnvLoader) requestedPath() string {
	requested := strings.TrimSpace(derefString(l.value))
	if requested == "" {
		return l.defaultPath
	}
	return requested
}

type envFileCandidate struct {
	path  string
	label string
}

func envFileCandidates(requested string, defaultPath string) []envFileCandidate {
	candidates := []envFileCandidate{{path: requested, label: "requested path"}}
	if base := filepath.Base(requested); base != "" && base != requested {
		candidates = append(candidates, envFileCandidate{path: base, label: "basename fallback"})
	}
	if requested != defaultPath {
		candidates = append(candidates, envFileCandidate{path: defaultPath, label: "fallback"})
	}
	return candidates
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
