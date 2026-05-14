package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"

	"horse.fit/scoop/internal/cli"
	"horse.fit/scoop/internal/pipeline"
)

type embedCommandConfig struct {
	envLoader      *cli.EnvLoader
	timeout        time.Duration
	limit          int
	batchSize      int
	endpoint       string
	modelName      string
	modelVersion   string
	maxLength      int
	requestTimeout time.Duration
}

type normalizeCommandConfig struct {
	envLoader *cli.EnvLoader
	timeout   time.Duration
	limit     int
}

type dedupCommandConfig struct {
	envLoader    *cli.EnvLoader
	timeout      time.Duration
	limit        int
	modelName    string
	modelVersion string
	lookbackDays int
}

type processCommandConfig struct {
	envLoader           *cli.EnvLoader
	timeout             time.Duration
	normalizeLimit      int
	embedLimit          int
	embedBatchSize      int
	embedEndpoint       string
	modelName           string
	modelVersion        string
	embedMaxLength      int
	embedRequestTimeout time.Duration
	dedupLimit          int
	dedupLookbackDays   int
	untilEmpty          bool
	maxCycles           int
}

type processCycleResult struct {
	normalize pipeline.NormalizeResult
	embed     pipeline.EmbedResult
	dedup     pipeline.DedupResult
}

type processTotals struct {
	normalize pipeline.NormalizeResult
	embed     pipeline.EmbedResult
	dedup     pipeline.DedupResult
	cyclesRun int
	drained   bool
}

type positiveIntFlag struct {
	name  string
	value int
}

func runNormalize(args []string) int {
	return runParsedCommand(args, parseNormalizeCommand, executeNormalizeCommand)
}

func parseNormalizeCommand(args []string) (normalizeCommandConfig, int, bool) {
	fs := newAppFlagSet("normalize")

	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 60*time.Second, "Command timeout")
	limit := fs.Int("limit", 1000, "Maximum pending raw arrivals to normalize")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return normalizeCommandConfig{}, 0, false
		}
		return normalizeCommandConfig{}, 2, false
	}
	cfg := normalizeCommandConfig{envLoader: envLoader, timeout: *timeout, limit: *limit}
	if err := validatePositiveIntFlags([]positiveIntFlag{{name: "--limit", value: cfg.limit}}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return normalizeCommandConfig{}, 2, false
	}
	return cfg, 0, true
}

func executeNormalizeCommand(normalizeCfg normalizeCommandConfig) int {
	runtime, exitCode, ok := openCommandRuntime(normalizeCfg.timeout, normalizeCfg.envLoader, "normalize command failed to connect to database")
	if !ok {
		return exitCode
	}
	defer runtime.Close()

	svc := pipeline.NewService(runtime.pool, runtime.logger)
	result, err := svc.NormalizePending(runtime.ctx, normalizeCfg.limit)
	if err != nil {
		runtime.logger.Error().Err(err).Int("limit", normalizeCfg.limit).Msg("normalize failed")
		fmt.Fprintf(os.Stderr, "Normalize failed: %v\n", err)
		return 1
	}

	runtime.logger.Info().
		Int("limit", normalizeCfg.limit).
		Int("processed", result.Processed).
		Int("inserted", result.Inserted).
		Msg("normalize completed")
	printNormalizeResult(result, normalizeCfg)
	return 0
}

func printNormalizeResult(result pipeline.NormalizeResult, cfg normalizeCommandConfig) {
	fmt.Printf("normalize processed=%d inserted=%d limit=%d\n", result.Processed, result.Inserted, cfg.limit)
}

func runEmbed(args []string) int {
	embedCfg, exitCode, ok := parseEmbedCommand(args)
	if !ok {
		return exitCode
	}
	runtime, exitCode, ok := openCommandRuntime(embedCfg.timeout, embedCfg.envLoader, "embed command failed to connect to database")
	if !ok {
		return exitCode
	}
	defer runtime.Close()

	result, err := pipeline.NewService(runtime.pool, runtime.logger).EmbedPending(runtime.ctx, embedOptions(embedCfg))
	if err != nil {
		runtime.logger.Error().Err(err).Int("limit", embedCfg.limit).Msg("embed failed")
		fmt.Fprintf(os.Stderr, "Embed failed: %v\n", err)
		return 1
	}

	runtime.logger.Info().
		Int("limit", embedCfg.limit).
		Int("processed", result.Processed).
		Int("embedded", result.Embedded).
		Int("skipped", result.Skipped).
		Int("failed", result.Failed).
		Msg("embed completed")
	printEmbedResult(result, embedCfg)
	return 0
}

func parseEmbedCommand(args []string) (embedCommandConfig, int, bool) {
	fs := newAppFlagSet("embed")

	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 2*time.Minute, "Command timeout")
	limit := fs.Int("limit", 1000, "Maximum pending articles to embed")
	batchSize := fs.Int("batch-size", pipeline.DefaultEmbeddingBatchSize, "Embedding request batch size")
	endpoint := fs.String("endpoint", pipeline.DefaultEmbeddingEndpoint, "Embedding HTTP endpoint")
	modelName := fs.String("model-name", pipeline.DefaultEmbeddingModelName, "Embedding model name key for storage")
	modelVersion := fs.String("model-version", pipeline.DefaultEmbeddingModelVersion, "Embedding model version key for storage")
	maxLength := fs.Int("max-length", pipeline.DefaultEmbeddingMaxLength, "Embedding max token length per text")
	requestTimeout := fs.Duration("request-timeout", pipeline.DefaultEmbeddingRequestTimeout, "Per-request timeout for embedding API")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return embedCommandConfig{}, 0, false
		}
		return embedCommandConfig{}, 2, false
	}
	cfg := embedCommandConfig{
		envLoader:      envLoader,
		timeout:        *timeout,
		limit:          *limit,
		batchSize:      *batchSize,
		endpoint:       *endpoint,
		modelName:      *modelName,
		modelVersion:   *modelVersion,
		maxLength:      *maxLength,
		requestTimeout: *requestTimeout,
	}
	if err := validateEmbedCommandConfig(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return embedCommandConfig{}, 2, false
	}
	return cfg, 0, true
}

func validateEmbedCommandConfig(cfg embedCommandConfig) error {
	if cfg.limit <= 0 {
		return fmt.Errorf("--limit must be > 0")
	}
	if cfg.batchSize <= 0 {
		return fmt.Errorf("--batch-size must be > 0")
	}
	if cfg.maxLength <= 0 {
		return fmt.Errorf("--max-length must be > 0")
	}
	return nil
}

func embedOptions(cfg embedCommandConfig) pipeline.EmbedOptions {
	return pipeline.EmbedOptions{
		Limit:          cfg.limit,
		BatchSize:      cfg.batchSize,
		Endpoint:       cfg.endpoint,
		ModelName:      cfg.modelName,
		ModelVersion:   cfg.modelVersion,
		MaxLength:      cfg.maxLength,
		RequestTimeout: cfg.requestTimeout,
	}
}

func printEmbedResult(result pipeline.EmbedResult, cfg embedCommandConfig) {
	fmt.Printf(
		"embed processed=%d embedded=%d skipped=%d failed=%d limit=%d model=%s model_version=%s\n",
		result.Processed,
		result.Embedded,
		result.Skipped,
		result.Failed,
		cfg.limit,
		cfg.modelName,
		cfg.modelVersion,
	)
}

func runDedup(args []string) int {
	return runParsedCommand(args, parseDedupCommand, executeDedupCommand)
}

func parseDedupCommand(args []string) (dedupCommandConfig, int, bool) {
	fs := newAppFlagSet("dedup")
	flags := registerDedupCommandFlags(fs)

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return dedupCommandConfig{}, 0, false
		}
		return dedupCommandConfig{}, 2, false
	}
	cfg := flags.config()
	if err := validateDedupCommandConfig(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return dedupCommandConfig{}, 2, false
	}
	return cfg, 0, true
}

type dedupCommandFlags struct {
	envLoader    *cli.EnvLoader
	timeout      *time.Duration
	limit        *int
	modelName    *string
	modelVersion *string
	lookbackDays *int
}

func registerDedupCommandFlags(fs *flag.FlagSet) dedupCommandFlags {
	return dedupCommandFlags{
		envLoader:    cli.AddEnvFlag(fs, ".env", "Path to the .env file"),
		timeout:      fs.Duration("timeout", 90*time.Second, "Command timeout"),
		limit:        fs.Int("limit", 1000, "Maximum pending articles to deduplicate"),
		modelName:    fs.String("model-name", pipeline.DefaultEmbeddingModelName, "Embedding model name used for semantic dedup"),
		modelVersion: fs.String("model-version", pipeline.DefaultEmbeddingModelVersion, "Embedding model version used for semantic dedup"),
		lookbackDays: fs.Int("lookback-days", pipeline.DefaultDedupLookbackDays, "How many days of stories to search for lexical/semantic candidates"),
	}
}

func (f dedupCommandFlags) config() dedupCommandConfig {
	return dedupCommandConfig{
		envLoader:    f.envLoader,
		timeout:      *f.timeout,
		limit:        *f.limit,
		modelName:    *f.modelName,
		modelVersion: *f.modelVersion,
		lookbackDays: *f.lookbackDays,
	}
}

func validateDedupCommandConfig(cfg dedupCommandConfig) error {
	return validatePositiveIntFlags([]positiveIntFlag{
		{name: "--limit", value: cfg.limit},
		{name: "--lookback-days", value: cfg.lookbackDays},
	})
}

func executeDedupCommand(dedupCfg dedupCommandConfig) int {
	runtime, exitCode, ok := openCommandRuntime(dedupCfg.timeout, dedupCfg.envLoader, "dedup command failed to connect to database")
	if !ok {
		return exitCode
	}
	defer runtime.Close()

	svc := pipeline.NewService(runtime.pool, runtime.logger)
	result, err := svc.DedupPending(runtime.ctx, dedupOptions(dedupCfg))
	if err != nil {
		runtime.logger.Error().Err(err).Int("limit", dedupCfg.limit).Msg("dedup failed")
		fmt.Fprintf(os.Stderr, "Dedup failed: %v\n", err)
		return 1
	}

	runtime.logger.Info().
		Int("limit", dedupCfg.limit).
		Int("lookback_days", dedupCfg.lookbackDays).
		Int("processed", result.Processed).
		Int("new_stories", result.NewStories).
		Int("auto_merges", result.AutoMerges).
		Int("gray_zones", result.GrayZones).
		Msg("dedup completed")
	printDedupResult(result, dedupCfg)
	return 0
}

func dedupOptions(cfg dedupCommandConfig) pipeline.DedupOptions {
	return pipeline.DedupOptions{
		Limit:        cfg.limit,
		ModelName:    cfg.modelName,
		ModelVersion: cfg.modelVersion,
		LookbackDays: cfg.lookbackDays,
	}
}

func printDedupResult(result pipeline.DedupResult, cfg dedupCommandConfig) {
	fmt.Printf(
		"dedup processed=%d new_stories=%d auto_merges=%d gray_zones=%d limit=%d lookback_days=%d model=%s model_version=%s\n",
		result.Processed,
		result.NewStories,
		result.AutoMerges,
		result.GrayZones,
		cfg.limit,
		cfg.lookbackDays,
		cfg.modelName,
		cfg.modelVersion,
	)
}

func runProcess(args []string) int {
	return runParsedCommand(args, parseProcessCommand, executeProcessCommand)
}

func parseProcessCommand(args []string) (processCommandConfig, int, bool) {
	fs := newAppFlagSet("process")

	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 5*time.Minute, "Command timeout")
	normalizeLimit := fs.Int("normalize-limit", 1000, "Maximum raw arrivals to normalize per cycle")
	embedLimit := fs.Int("embed-limit", 1000, "Maximum articles to embed per cycle")
	embedBatchSize := fs.Int("embed-batch-size", pipeline.DefaultEmbeddingBatchSize, "Embedding request batch size")
	embedEndpoint := fs.String("embed-endpoint", pipeline.DefaultEmbeddingEndpoint, "Embedding HTTP endpoint")
	modelName := fs.String("model-name", pipeline.DefaultEmbeddingModelName, "Embedding model name")
	modelVersion := fs.String("model-version", pipeline.DefaultEmbeddingModelVersion, "Embedding model version")
	embedMaxLength := fs.Int("embed-max-length", pipeline.DefaultEmbeddingMaxLength, "Embedding max token length per text")
	embedRequestTimeout := fs.Duration("embed-request-timeout", pipeline.DefaultEmbeddingRequestTimeout, "Per-request timeout for embedding API")
	dedupLimit := fs.Int("dedup-limit", 1000, "Maximum articles to deduplicate per cycle")
	dedupLookbackDays := fs.Int("dedup-lookback-days", pipeline.DefaultDedupLookbackDays, "How many days of stories to search for lexical/semantic candidates")
	untilEmpty := fs.Bool("until-empty", true, "Repeat cycles until no work remains")
	maxCycles := fs.Int("max-cycles", 25, "Maximum normalize+embed+dedup cycles when --until-empty=true")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return processCommandConfig{}, 0, false
		}
		return processCommandConfig{}, 2, false
	}
	cfg := processCommandConfig{
		envLoader:           envLoader,
		timeout:             *timeout,
		normalizeLimit:      *normalizeLimit,
		embedLimit:          *embedLimit,
		embedBatchSize:      *embedBatchSize,
		embedEndpoint:       *embedEndpoint,
		modelName:           *modelName,
		modelVersion:        *modelVersion,
		embedMaxLength:      *embedMaxLength,
		embedRequestTimeout: *embedRequestTimeout,
		dedupLimit:          *dedupLimit,
		dedupLookbackDays:   *dedupLookbackDays,
		untilEmpty:          *untilEmpty,
		maxCycles:           *maxCycles,
	}
	if err := validateProcessCommandConfig(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return processCommandConfig{}, 2, false
	}
	return cfg, 0, true
}

func validateProcessCommandConfig(cfg processCommandConfig) error {
	return validatePositiveIntFlags([]positiveIntFlag{
		{name: "--normalize-limit", value: cfg.normalizeLimit},
		{name: "--embed-limit", value: cfg.embedLimit},
		{name: "--embed-batch-size", value: cfg.embedBatchSize},
		{name: "--embed-max-length", value: cfg.embedMaxLength},
		{name: "--dedup-limit", value: cfg.dedupLimit},
		{name: "--dedup-lookback-days", value: cfg.dedupLookbackDays},
		{name: "--max-cycles", value: cfg.maxCycles},
	})
}

func validatePositiveIntFlags(flags []positiveIntFlag) error {
	for _, flag := range flags {
		if flag.value <= 0 {
			return fmt.Errorf("%s must be > 0", flag.name)
		}
	}
	return nil
}

func executeProcessCommand(processCfg processCommandConfig) int {
	runtime, exitCode, ok := openCommandRuntime(processCfg.timeout, processCfg.envLoader, "process command failed to connect to database")
	if !ok {
		return exitCode
	}
	defer runtime.Close()

	svc := pipeline.NewService(runtime.pool, runtime.logger)
	totals, exitCode := runProcessCycles(runtime.ctx, svc, runtime.logger, processCfg)
	if exitCode != 0 {
		return exitCode
	}

	logProcessTotals(runtime.logger, totals)
	printProcessTotals(totals)
	return processDrainExitCode(totals, processCfg)
}

func runProcessCycles(
	ctx context.Context,
	svc *pipeline.Service,
	logger zerolog.Logger,
	cfg processCommandConfig,
) (processTotals, int) {
	var totals processTotals
	for cycle := 1; cycle <= cfg.maxCycles; cycle++ {
		result, exitCode, ok := runProcessCycle(ctx, svc, logger, cfg, cycle)
		if !ok {
			return totals, exitCode
		}
		totals = accumulateProcessCycle(totals, cycle, result)
		printProcessCycle(cycle, result)
		if shouldStopProcessCycles(result, cfg) {
			totals.drained = processCycleDrained(result)
			break
		}
	}
	return totals, 0
}

func runProcessCycle(
	ctx context.Context,
	svc *pipeline.Service,
	logger zerolog.Logger,
	cfg processCommandConfig,
	cycle int,
) (processCycleResult, int, bool) {
	normalizeResult, err := svc.NormalizePending(ctx, cfg.normalizeLimit)
	if err != nil {
		logger.Error().Err(err).Int("cycle", cycle).Msg("normalize stage failed")
		fmt.Fprintf(os.Stderr, "Process failed during normalize cycle %d: %v\n", cycle, err)
		return processCycleResult{}, 1, false
	}
	embedResult, err := svc.EmbedPending(ctx, processEmbedOptions(cfg))
	if err != nil {
		logger.Error().Err(err).Int("cycle", cycle).Msg("embed stage failed")
		fmt.Fprintf(os.Stderr, "Process failed during embed cycle %d: %v\n", cycle, err)
		return processCycleResult{}, 1, false
	}
	dedupResult, err := svc.DedupPending(ctx, processDedupOptions(cfg))
	if err != nil {
		logger.Error().Err(err).Int("cycle", cycle).Msg("dedup stage failed")
		fmt.Fprintf(os.Stderr, "Process failed during dedup cycle %d: %v\n", cycle, err)
		return processCycleResult{}, 1, false
	}
	return processCycleResult{normalize: normalizeResult, embed: embedResult, dedup: dedupResult}, 0, true
}

func processEmbedOptions(cfg processCommandConfig) pipeline.EmbedOptions {
	return embedOptions(embedCommandConfig{
		limit:          cfg.embedLimit,
		batchSize:      cfg.embedBatchSize,
		endpoint:       cfg.embedEndpoint,
		modelName:      cfg.modelName,
		modelVersion:   cfg.modelVersion,
		maxLength:      cfg.embedMaxLength,
		requestTimeout: cfg.embedRequestTimeout,
	})
}

func processDedupOptions(cfg processCommandConfig) pipeline.DedupOptions {
	return dedupOptions(dedupCommandConfig{
		limit:        cfg.dedupLimit,
		modelName:    cfg.modelName,
		modelVersion: cfg.modelVersion,
		lookbackDays: cfg.dedupLookbackDays,
	})
}

func accumulateProcessCycle(totals processTotals, cycle int, result processCycleResult) processTotals {
	totals.cyclesRun = cycle
	totals.normalize.Processed += result.normalize.Processed
	totals.normalize.Inserted += result.normalize.Inserted
	totals.embed.Processed += result.embed.Processed
	totals.embed.Embedded += result.embed.Embedded
	totals.embed.Skipped += result.embed.Skipped
	totals.embed.Failed += result.embed.Failed
	totals.dedup.Processed += result.dedup.Processed
	totals.dedup.NewStories += result.dedup.NewStories
	totals.dedup.AutoMerges += result.dedup.AutoMerges
	totals.dedup.GrayZones += result.dedup.GrayZones
	return totals
}

func printProcessCycle(cycle int, result processCycleResult) {
	fmt.Printf(
		"cycle=%d normalize_processed=%d normalize_inserted=%d embed_processed=%d embedded=%d skipped=%d failed=%d dedup_processed=%d new_stories=%d auto_merges=%d gray_zones=%d\n",
		cycle,
		result.normalize.Processed,
		result.normalize.Inserted,
		result.embed.Processed,
		result.embed.Embedded,
		result.embed.Skipped,
		result.embed.Failed,
		result.dedup.Processed,
		result.dedup.NewStories,
		result.dedup.AutoMerges,
		result.dedup.GrayZones,
	)
}

func shouldStopProcessCycles(result processCycleResult, cfg processCommandConfig) bool {
	return !cfg.untilEmpty || processCycleDrained(result)
}

func processCycleDrained(result processCycleResult) bool {
	return result.normalize.Processed == 0 && result.embed.Processed == 0 && result.dedup.Processed == 0
}

func logProcessTotals(logger zerolog.Logger, totals processTotals) {
	logger.Info().
		Int("cycles", totals.cyclesRun).
		Bool("drained", totals.drained).
		Int("normalize_processed", totals.normalize.Processed).
		Int("normalize_inserted", totals.normalize.Inserted).
		Int("embed_processed", totals.embed.Processed).
		Int("embedded", totals.embed.Embedded).
		Int("embed_skipped", totals.embed.Skipped).
		Int("embed_failed", totals.embed.Failed).
		Int("dedup_processed", totals.dedup.Processed).
		Int("new_stories", totals.dedup.NewStories).
		Int("auto_merges", totals.dedup.AutoMerges).
		Int("gray_zones", totals.dedup.GrayZones).
		Msg("process completed")
}

func printProcessTotals(totals processTotals) {
	fmt.Printf(
		"process_total cycles=%d drained=%t normalize_processed=%d normalize_inserted=%d embed_processed=%d embedded=%d skipped=%d failed=%d dedup_processed=%d new_stories=%d auto_merges=%d gray_zones=%d\n",
		totals.cyclesRun,
		totals.drained,
		totals.normalize.Processed,
		totals.normalize.Inserted,
		totals.embed.Processed,
		totals.embed.Embedded,
		totals.embed.Skipped,
		totals.embed.Failed,
		totals.dedup.Processed,
		totals.dedup.NewStories,
		totals.dedup.AutoMerges,
		totals.dedup.GrayZones,
	)
}

func processDrainExitCode(totals processTotals, cfg processCommandConfig) int {
	if cfg.untilEmpty && !totals.drained {
		fmt.Fprintf(
			os.Stderr,
			"Process stopped after max cycles (%d) before draining queue; rerun with higher --max-cycles or limits\n",
			cfg.maxCycles,
		)
		return 1
	}
	return 0
}
