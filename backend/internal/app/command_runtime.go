package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"

	"horse.fit/scoop/internal/cli"
	"horse.fit/scoop/internal/config"
	"horse.fit/scoop/internal/db"
	"horse.fit/scoop/internal/logging"
)

type commandRuntime struct {
	ctx    context.Context
	cancel context.CancelFunc
	cfg    *config.Config
	logger zerolog.Logger
	pool   *db.Pool
}

func openCommandRuntime(timeout time.Duration, envLoader *cli.EnvLoader, dbFailureLog string) (*commandRuntime, int, bool) {
	base, exitCode, ok := newCommandRuntimeBase(timeout, envLoader)
	if !ok {
		return nil, exitCode, false
	}
	pool, err := db.NewPool(base.ctx, base.cfg)
	if err != nil {
		base.logger.Error().Err(err).Msg(dbFailureLog)
		fmt.Fprintf(os.Stderr, "Failed to connect to database: %v\n", err)
		base.cancel()
		return nil, 1, false
	}
	base.pool = pool
	return base, 0, true
}

func newCommandRuntimeBase(timeout time.Duration, envLoader *cli.EnvLoader) (*commandRuntime, int, bool) {
	loadEnvWithWarning(envLoader)

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		return nil, 1, false
	}
	logger, err := logging.New(cfg.Environment, cfg.LogLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		return nil, 1, false
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	return &commandRuntime{
		ctx:    ctx,
		cancel: cancel,
		cfg:    cfg,
		logger: logger,
	}, 0, true
}

func loadEnvWithWarning(envLoader *cli.EnvLoader) {
	if envLoader == nil {
		return
	}
	if _, err := envLoader.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}
}

func (r *commandRuntime) Close() {
	if r == nil {
		return
	}
	if r.pool != nil {
		_ = r.pool.Close()
	}
	if r.cancel != nil {
		r.cancel()
	}
}
