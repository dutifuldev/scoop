package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"horse.fit/scoop/internal/cli"
	"horse.fit/scoop/internal/language"
	"horse.fit/scoop/internal/translation"
)

type translateCommandConfig struct {
	envLoader  *cli.EnvLoader
	timeout    time.Duration
	target     string
	identifier string
	targetLang string
	provider   string
	dryRun     bool
	force      bool
}

func runTranslate(args []string) int {
	return runParsedCommand(args, parseTranslateCommand, executeTranslateCommand)
}

func parseTranslateCommand(args []string) (translateCommandConfig, int, bool) {
	target, exitCode, ok := parseTranslateTarget(args)
	if !ok {
		return translateCommandConfig{}, exitCode, false
	}

	fs := newAppFlagSet("translate " + target)
	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 2*time.Minute, "Command timeout")
	lang := fs.String("lang", "", "Target language (ISO 639-1, for example: en, zh)")
	provider := fs.String("provider", "", "Translation provider name (for example: local, google)")
	dryRun := fs.Bool("dry-run", false, "Preview work without calling the translation provider")
	force := fs.Bool("force", false, "Retranslate even when cached translation exists")

	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return translateCommandConfig{}, 0, false
		}
		return translateCommandConfig{}, 2, false
	}
	return buildTranslateCommandConfig(target, fs, envLoader, *timeout, *lang, *provider, *dryRun, *force)
}

func parseTranslateTarget(args []string) (string, int, bool) {
	if len(args) == 0 {
		printTranslateUsage()
		return "", 2, false
	}
	target := strings.ToLower(strings.TrimSpace(args[0]))
	if !isTranslateTarget(target) {
		fmt.Fprintf(os.Stderr, "Unknown translate target: %s\n\n", args[0])
		printTranslateUsage()
		return "", 2, false
	}
	return target, 0, true
}

func buildTranslateCommandConfig(
	target string,
	fs *flag.FlagSet,
	envLoader *cli.EnvLoader,
	timeout time.Duration,
	lang string,
	provider string,
	dryRun bool,
	force bool,
) (translateCommandConfig, int, bool) {
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "translate requires one argument")
		printTranslateUsage()
		return translateCommandConfig{}, 2, false
	}
	targetLang := normalizeLanguageFlag(lang)
	if targetLang == "" {
		fmt.Fprintln(os.Stderr, "--lang is required and must be a valid language code")
		return translateCommandConfig{}, 2, false
	}

	identifier := strings.TrimSpace(fs.Arg(0))
	if identifier == "" {
		fmt.Fprintln(os.Stderr, "translate argument must not be empty")
		return translateCommandConfig{}, 2, false
	}
	return translateCommandConfig{
		envLoader:  envLoader,
		timeout:    timeout,
		target:     target,
		identifier: identifier,
		targetLang: targetLang,
		provider:   strings.TrimSpace(provider),
		dryRun:     dryRun,
		force:      force,
	}, 0, true
}

func isTranslateTarget(target string) bool {
	return stringSliceContains([]string{"story", "article", "collection"}, target)
}

func executeTranslateCommand(cfg translateCommandConfig) int {
	ctx, cancel, pool, err := connectReadPool(cfg.timeout, cfg.envLoader)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer cancel()
	defer pool.Close()

	registry := translation.NewRegistryFromEnv()
	var service translation.Service = translation.NewManager(pool, registry)

	runOpts := translation.RunOptions{
		TargetLang: cfg.targetLang,
		Provider:   cfg.provider,
		Force:      cfg.force,
		DryRun:     cfg.dryRun,
	}
	stats, exitCode, ok := runTranslateTarget(ctx, service, cfg, runOpts)
	if !ok {
		return exitCode
	}
	printTranslateResult(cfg, stats, resolvedTranslationProvider(runOpts.Provider, registry))
	return 0
}

func runTranslateTarget(
	ctx context.Context,
	service translation.Service,
	cfg translateCommandConfig,
	runOpts translation.RunOptions,
) (translation.RunStats, int, bool) {
	switch cfg.target {
	case "story":
		return translateSingleTarget(
			cfg.identifier,
			func() (translation.RunStats, error) {
				return service.TranslateStoryByUUID(ctx, cfg.identifier, runOpts)
			},
			translation.ErrStoryNotFound,
			"Story not found",
			"Translate story failed",
		)
	case "article":
		return translateSingleTarget(
			cfg.identifier,
			func() (translation.RunStats, error) {
				return service.TranslateArticleByUUID(ctx, cfg.identifier, runOpts)
			},
			translation.ErrArticleNotFound,
			"Article not found",
			"Translate article failed",
		)
	default:
		return translateCollectionTarget(ctx, service, cfg.identifier, runOpts)
	}
}

func translateSingleTarget(
	identifier string,
	translate func() (translation.RunStats, error),
	notFound error,
	notFoundMessage string,
	failureMessage string,
) (translation.RunStats, int, bool) {
	stats, err := translate()
	if err == nil {
		return stats, 0, true
	}
	if errors.Is(err, notFound) {
		fmt.Fprintf(os.Stderr, "%s: %s\n", notFoundMessage, identifier)
	} else {
		fmt.Fprintf(os.Stderr, "%s: %v\n", failureMessage, err)
	}
	return translation.RunStats{}, 1, false
}

func translateCollectionTarget(
	ctx context.Context,
	service translation.Service,
	identifier string,
	runOpts translation.RunOptions,
) (translation.RunStats, int, bool) {
	stats, err := service.TranslateCollection(ctx, identifier, translation.CollectionRunOptions{
		RunOptions: runOpts,
		Progress: func(p translation.CollectionProgress) {
			fmt.Printf("Translating %d/%d stories...\n", p.Current, p.Total)
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Translate collection failed: %v\n", err)
		return translation.RunStats{}, 1, false
	}
	return stats, 0, true
}

func resolvedTranslationProvider(provider string, registry *translation.Registry) string {
	resolvedProvider := strings.TrimSpace(provider)
	if resolvedProvider == "" {
		resolvedProvider = registry.DefaultProvider()
	}
	return resolvedProvider
}

func printTranslateResult(cfg translateCommandConfig, stats translation.RunStats, resolvedProvider string) {
	fmt.Printf(
		"translate target=%s id=%s lang=%s provider=%s total=%d translated=%d cached=%d skipped=%d dry_run=%t force=%t\n",
		cfg.target,
		cfg.identifier,
		cfg.targetLang,
		resolvedProvider,
		stats.Total,
		stats.Translated,
		stats.Cached,
		stats.Skipped,
		cfg.dryRun,
		cfg.force,
	)
}

func normalizeLanguageFlag(raw string) string {
	return language.NormalizeCode(raw)
}

func printTranslateUsage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  scoop translate story <story_uuid> --lang <lang> [--provider local] [--dry-run] [--force] [--env .env] [--timeout 2m]")
	fmt.Fprintln(os.Stderr, "  scoop translate article <article_uuid> --lang <lang> [--provider local] [--dry-run] [--force] [--env .env] [--timeout 2m]")
	fmt.Fprintln(os.Stderr, "  scoop translate collection <name> --lang <lang> [--provider local] [--dry-run] [--force] [--env .env] [--timeout 2m]")
}
