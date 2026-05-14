package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"horse.fit/scoop/internal/config"
)

func TestParseServeConfig(t *testing.T) {
	t.Parallel()

	cfg, exitCode, ok := parseServeConfig([]string{
		"--host", "127.0.0.1",
		"--port", "8091",
		"--read-timeout", "3s",
		"--write-timeout", "4s",
		"--shutdown-timeout", "5s",
	})
	if !ok || exitCode != 0 {
		t.Fatalf("parseServeConfig() ok=%t exit=%d", ok, exitCode)
	}
	if cfg.host != "127.0.0.1" || cfg.port != 8091 || cfg.readTimeout != 3*time.Second {
		t.Fatalf("config = %#v", cfg)
	}
	if _, exitCode, ok := parseServeConfig([]string{"--port", "0"}); ok || exitCode != 2 {
		t.Fatalf("invalid port ok=%t exit=%d, want validation failure", ok, exitCode)
	}
}

func TestServeOptions(t *testing.T) {
	t.Parallel()

	opts := serveOptions(serveConfig{
		host:            "127.0.0.1",
		port:            8091,
		readTimeout:     3 * time.Second,
		writeTimeout:    4 * time.Second,
		shutdownTimeout: 5 * time.Second,
	}, &config.Config{
		SessionTTLHours:     2,
		SessionCookieName:   "session",
		SessionCookieSecure: true,
		CORSAllowedOrigins:  "https://a.example, https://b.example",
	})
	if opts.Host != "127.0.0.1" || opts.Port != 8091 || opts.SessionTTL != 2*time.Hour {
		t.Fatalf("options = %#v", opts)
	}
	if len(opts.CORSAllowedOrigins) != 2 || opts.CORSAllowedOrigins[0] != "https://a.example" {
		t.Fatalf("cors origins = %#v", opts.CORSAllowedOrigins)
	}
}

func TestServeEarlyExitHelpers(t *testing.T) {
	t.Parallel()

	if code := loadServeEnv(serveConfig{}); code != 0 {
		t.Fatalf("loadServeEnv() = %d, want 0", code)
	}
	if code := runServeAfterEnv(serveConfig{}, 2); code != 2 {
		t.Fatalf("runServeAfterEnv(exit) = %d, want 2", code)
	}
	if code := runServeAfterDependencies(serveConfig{}, nil, zerolog.Nop(), 1); code != 1 {
		t.Fatalf("runServeAfterDependencies(exit) = %d, want 1", code)
	}

	_, _, exitCode := loadServeLogger(&config.Config{Environment: "test", LogLevel: "bad-level"})
	if exitCode != 1 {
		t.Fatalf("loadServeLogger(invalid log level) exit = %d, want 1", exitCode)
	}
	warnServeEnvLoad("", errors.New("missing env"))
}

func TestServeFailureWrappers(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("ENVIRONMENT", "test")
	t.Setenv("SESSION_COOKIE_NAME", "scoop_session")

	if code := runServeWithConfig(serveConfig{}); code != 1 {
		t.Fatalf("runServeWithConfig(missing config) = %d, want 1", code)
	}
	if code := runServeWithPool(serveConfig{}, &config.Config{}, zerolog.Nop(), nil); code != 1 {
		t.Fatalf("runServeWithPool(nil pool) = %d, want bootstrap failure", code)
	}
	cfg := &config.Config{
		DatabaseURL:       "://bad-url",
		Environment:       "test",
		LogLevel:          "error",
		SessionTTLHours:   1,
		SessionCookieName: "scoop_session",
	}
	if code := runServeWithDependencies(serveConfig{}, cfg, zerolog.Nop()); code != 1 {
		t.Fatalf("runServeWithDependencies(bad db) = %d, want 1", code)
	}
}

func TestServeDependencyFailures(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://news:test@localhost:1/scoop?sslmode=disable")
	t.Setenv("ENVIRONMENT", "test")
	t.Setenv("LOG_LEVEL", "error")
	t.Setenv("SESSION_COOKIE_NAME", "scoop_session")
	t.Setenv("SESSION_TTL_HOURS", "1")

	cfg, logger, exitCode := loadServeDependencies()
	if exitCode != 0 || cfg == nil {
		t.Fatalf("loadServeDependencies() cfg=%v exit=%d, want config and zero exit", cfg, exitCode)
	}
	cfg.DatabaseURL = "://bad-url"
	if pool, exitCode := openServePool(cfg, logger); exitCode != 1 || pool != nil {
		t.Fatalf("openServePool(invalid database) pool=%v exit=%d, want failure", pool, exitCode)
	}
}

func TestStartServeHTTPFailsFastWithUninitializedServer(t *testing.T) {
	cfg := &config.Config{SessionTTLHours: 1, SessionCookieName: "scoop_session"}
	code := startServeHTTP(serveConfig{host: "127.0.0.1", port: 8090}, cfg, zerolog.Nop(), nil)
	if code != 1 {
		t.Fatalf("startServeHTTP(nil pool) = %d, want failure", code)
	}
}

func TestLoadOptionalServeEnvHandlesNilAndWarning(t *testing.T) {
	loadOptionalServeEnv(nil)
	warnServeEnvLoad("ignored", errors.New("missing"))
}

func TestServeOptionsKeepsConfiguredOrigins(t *testing.T) {
	opts := serveOptions(serveConfig{host: "0.0.0.0", port: 8090}, &config.Config{
		SessionTTLHours:    3,
		SessionCookieName:  "session",
		CORSAllowedOrigins: " https://a.example,https://b.example ",
	})
	if strings.Join(opts.CORSAllowedOrigins, ",") != "https://a.example,https://b.example" {
		t.Fatalf("origins = %#v", opts.CORSAllowedOrigins)
	}
}
