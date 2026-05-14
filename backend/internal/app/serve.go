package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"horse.fit/scoop/internal/cli"
	"horse.fit/scoop/internal/config"
	"horse.fit/scoop/internal/db"
	"horse.fit/scoop/internal/httpapi"
	"horse.fit/scoop/internal/logging"
)

type serveConfig struct {
	envLoader       *cli.EnvLoader
	host            string
	port            int
	readTimeout     time.Duration
	writeTimeout    time.Duration
	shutdownTimeout time.Duration
}

func runServe(args []string) int {
	return runParsedCommand(args, parseServeConfig, runServeWithConfig)
}

func runServeWithConfig(serveCfg serveConfig) int {
	return runServeAfterEnv(serveCfg, loadServeEnv(serveCfg))
}

func runServeAfterEnv(serveCfg serveConfig, exitCode int) int {
	if exitCode != 0 {
		return exitCode
	}
	cfg, logger, exitCode := loadServeDependencies()
	return runServeAfterDependencies(serveCfg, cfg, logger, exitCode)
}

func runServeAfterDependencies(serveCfg serveConfig, cfg *config.Config, logger zerolog.Logger, exitCode int) int {
	if exitCode != 0 {
		return exitCode
	}
	return runServeWithDependencies(serveCfg, cfg, logger)
}

func loadServeEnv(serveCfg serveConfig) int {
	loadOptionalServeEnv(serveCfg.envLoader)
	return 0
}

func loadOptionalServeEnv(envLoader *cli.EnvLoader) {
	if envLoader == nil {
		return
	}
	warnServeEnvLoad(envLoader.Load())
}

func warnServeEnvLoad(_ string, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}
}

func runServeWithDependencies(serveCfg serveConfig, cfg *config.Config, logger zerolog.Logger) int {
	pool, exitCode := openServePool(cfg, logger)
	if exitCode != 0 {
		return exitCode
	}
	defer pool.Close()
	return runServeWithPool(serveCfg, cfg, logger, pool)
}

func runServeWithPool(serveCfg serveConfig, cfg *config.Config, logger zerolog.Logger, pool *db.Pool) int {
	if exitCode := bootstrapServeDefaultAdmin(cfg, logger, pool); exitCode != 0 {
		return exitCode
	}
	return startServeHTTP(serveCfg, cfg, logger, pool)
}

func bootstrapServeDefaultAdmin(cfg *config.Config, logger zerolog.Logger, pool *db.Pool) int {
	if err := ensureDefaultAdmin(context.Background(), pool, cfg, logger); err != nil {
		logger.Error().Err(err).Msg("serve failed to bootstrap default admin")
		fmt.Fprintf(os.Stderr, "Failed to bootstrap default admin: %v\n", err)
		return 1
	}
	return 0
}

func startServeHTTP(serveCfg serveConfig, cfg *config.Config, logger zerolog.Logger, pool *db.Pool) int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	notifyServeShutdown(cancel)

	srv := httpapi.NewServer(pool, logger, serveOptions(serveCfg, cfg))
	if err := srv.Start(ctx); err != nil {
		logger.Error().Err(err).Str("host", serveCfg.host).Int("port", serveCfg.port).Msg("server failed")
		fmt.Fprintf(os.Stderr, "Server failed: %v\n", err)
		return 1
	}
	return 0
}

func loadServeDependencies() (*config.Config, zerolog.Logger, int) {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		return nil, zerolog.Logger{}, 1
	}
	return loadServeLogger(cfg)
}

func loadServeLogger(cfg *config.Config) (*config.Config, zerolog.Logger, int) {
	logger, err := logging.New(cfg.Environment, cfg.LogLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		return nil, zerolog.Logger{}, 1
	}
	return cfg, logger, 0
}

func parseServeConfig(args []string) (serveConfig, int, bool) {
	fs := newAppFlagSet("serve")

	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	host := fs.String("host", "0.0.0.0", "Host interface to bind")
	port := fs.Int("port", 8090, "HTTP port")
	readTimeout := fs.Duration("read-timeout", 10*time.Second, "HTTP read timeout")
	writeTimeout := fs.Duration("write-timeout", 30*time.Second, "HTTP write timeout")
	shutdownTimeout := fs.Duration("shutdown-timeout", 10*time.Second, "Graceful shutdown timeout")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return serveConfig{}, 0, false
		}
		return serveConfig{}, 2, false
	}
	cfg := serveConfig{
		envLoader:       envLoader,
		host:            *host,
		port:            *port,
		readTimeout:     *readTimeout,
		writeTimeout:    *writeTimeout,
		shutdownTimeout: *shutdownTimeout,
	}
	if err := validatePort(cfg.port, "--port"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return serveConfig{}, 2, false
	}
	return cfg, 0, true
}

func openServePool(cfg *config.Config, logger zerolog.Logger) (*db.Pool, int) {
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer dbCancel()

	pool, err := db.NewPool(dbCtx, cfg)
	if err != nil {
		logger.Error().Err(err).Msg("serve failed to connect to database")
		fmt.Fprintf(os.Stderr, "Failed to connect to database: %v\n", err)
		return nil, 1
	}
	return pool, 0
}

func notifyServeShutdown(cancel context.CancelFunc) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		defer signal.Stop(sigCh)
		<-sigCh
		cancel()
	}()
}

func serveOptions(serveCfg serveConfig, cfg *config.Config) httpapi.Options {
	return httpapi.Options{
		Host:               serveCfg.host,
		Port:               serveCfg.port,
		ReadTimeout:        serveCfg.readTimeout,
		WriteTimeout:       serveCfg.writeTimeout,
		ShutdownTimeout:    serveCfg.shutdownTimeout,
		SessionTTL:         time.Duration(cfg.SessionTTLHours) * time.Hour,
		SessionCookie:      cfg.SessionCookieName,
		SessionSecure:      cfg.SessionCookieSecure,
		CORSAllowedOrigins: cfg.CORSAllowedOriginsList(),
	}
}
