package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"horse.fit/scoop/internal/cli"
	"horse.fit/scoop/internal/db"
	"horse.fit/scoop/internal/globaltime"
)

type updateStorySnapshot struct {
	StoryUUID  string
	Title      string
	Status     string
	Collection string
	URL        *string
	UpdatedAt  time.Time
}

type updateArticleSnapshot struct {
	ArticleUUID  string
	Title        string
	Source       string
	Collection   string
	URL          *string
	SourceDomain *string
	UpdatedAt    time.Time
}

type updateTarget string

const (
	updateTargetStory   updateTarget = "story"
	updateTargetArticle updateTarget = "article"
)

type updateCommandConfig struct {
	target      updateTarget
	uuid        string
	timeout     time.Duration
	envLoader   *cli.EnvLoader
	dryRun      bool
	storyOpts   db.UpdateStoryOptions
	articleOpts db.UpdateArticleOptions
}

type updateFlagValues struct {
	title      *string
	status     *string
	collection *string
	url        *string
	source     *string
	dryRun     *bool
	timeout    *time.Duration
	envLoader  *cli.EnvLoader
}

type updateRunConfig[Snapshot any, Options any] struct {
	uuid               string
	options            Options
	now                time.Time
	dryRun             bool
	getSnapshot        func(context.Context, *db.Pool, string) (*Snapshot, error)
	apply              func(context.Context, *db.Pool, string, Options, time.Time) error
	preview            func(Snapshot, Options, time.Time) Snapshot
	write              func(Snapshot, Snapshot) int
	loadNotFound       string
	loadFailed         string
	applyNotFound      string
	applyFailed        string
	postUpdateLoadFail string
}

func runUpdate(args []string) int {
	cfg, exitCode, ok := parseUpdateCommand(args)
	if !ok {
		return exitCode
	}

	ctx, cancel, pool, err := connectReadPool(cfg.timeout, cfg.envLoader)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer cancel()
	defer pool.Close()

	now := globaltime.UTC()
	if cfg.target == updateTargetStory {
		return runUpdateStory(ctx, pool, cfg.uuid, cfg.storyOpts, now, cfg.dryRun)
	}
	return runUpdateArticle(ctx, pool, cfg.uuid, cfg.articleOpts, now, cfg.dryRun)
}

func parseUpdateCommand(args []string) (updateCommandConfig, int, bool) {
	target, exitCode, ok := parseUpdateTarget(args)
	if !ok {
		return updateCommandConfig{}, exitCode, false
	}

	fs, values := newUpdateFlagSet(target)
	if exitCode, ok := parseAppFlagSet(fs, args[1:]); !ok {
		return updateCommandConfig{}, exitCode, false
	}
	return buildUpdateCommandConfig(target, fs, values)
}

func buildUpdateCommandConfig(target updateTarget, fs *flag.FlagSet, values updateFlagValues) (updateCommandConfig, int, bool) {
	uuid, exitCode, ok := updateUUIDFromFlagSet(fs)
	if !ok {
		return updateCommandConfig{}, exitCode, false
	}
	cfg := newUpdateCommandConfig(target, uuid, values)
	if err := populateUpdateOptions(&cfg, values, visitedFlags(fs)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return updateCommandConfig{}, 2, false
	}
	return cfg, 0, true
}

func updateUUIDFromFlagSet(fs *flag.FlagSet) (string, int, bool) {
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "update requires exactly one UUID argument")
		printUpdateUsage()
		return "", 2, false
	}
	uuid, err := parseUpdateUUID(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "UUID must not be empty")
		return "", 2, false
	}
	return uuid, 0, true
}

func parseUpdateUUID(raw string) (string, error) {
	uuid := strings.TrimSpace(raw)
	if uuid == "" {
		return "", errors.New("empty UUID")
	}
	return uuid, nil
}

func newUpdateCommandConfig(target updateTarget, uuid string, values updateFlagValues) updateCommandConfig {
	return updateCommandConfig{
		target:    target,
		uuid:      uuid,
		timeout:   *values.timeout,
		envLoader: values.envLoader,
		dryRun:    *values.dryRun,
	}
}

func parseUpdateTarget(args []string) (updateTarget, int, bool) {
	if len(args) == 0 {
		printUpdateUsage()
		return "", 2, false
	}
	target := updateTarget(strings.ToLower(strings.TrimSpace(args[0])))
	if target != updateTargetStory && target != updateTargetArticle {
		fmt.Fprintf(os.Stderr, "Unknown update target: %s\n\n", args[0])
		printUpdateUsage()
		return "", 2, false
	}
	return target, 0, true
}

func newUpdateFlagSet(target updateTarget) (*flag.FlagSet, updateFlagValues) {
	fs := newAppFlagSet("update " + string(target))

	values := updateFlagValues{
		envLoader:  cli.AddEnvFlag(fs, ".env", "Path to the .env file"),
		timeout:    fs.Duration("timeout", 30*time.Second, "Command timeout"),
		title:      fs.String("title", "", "Updated title"),
		status:     fs.String("status", "", "Updated story status (story only)"),
		collection: fs.String("collection", "", "Updated collection"),
		url:        fs.String("url", "", "Updated canonical URL"),
		source:     fs.String("source", "", "Updated source (article only)"),
		dryRun:     fs.Bool("dry-run", false, "Preview changes without applying updates"),
	}
	return fs, values
}

func visitedFlags(fs *flag.FlagSet) map[string]bool {
	visited := map[string]bool{}
	fs.Visit(func(f *flag.Flag) {
		visited[f.Name] = true
	})
	return visited
}

func populateUpdateOptions(cfg *updateCommandConfig, values updateFlagValues, visited map[string]bool) error {
	if cfg.target == updateTargetStory {
		return populateTypedUpdateOptions(
			cfg,
			values,
			visited,
			"source",
			"--source is only supported for `scoop update article`",
			storyUpdateOptionsFromFlags,
			func(cfg *updateCommandConfig, opts db.UpdateStoryOptions) { cfg.storyOpts = opts },
			validateStoryUpdateOptions,
			"Invalid story update",
		)
	}
	return populateTypedUpdateOptions(
		cfg,
		values,
		visited,
		"status",
		"--status is only supported for `scoop update story`",
		articleUpdateOptionsFromFlags,
		func(cfg *updateCommandConfig, opts db.UpdateArticleOptions) { cfg.articleOpts = opts },
		validateArticleUpdateOptions,
		"Invalid article update",
	)
}

func populateTypedUpdateOptions[Options any](
	cfg *updateCommandConfig,
	values updateFlagValues,
	visited map[string]bool,
	unsupportedFlag string,
	unsupportedMessage string,
	build func(updateFlagValues, map[string]bool) Options,
	assign func(*updateCommandConfig, Options),
	validate func(Options) error,
	validationPrefix string,
) error {
	if visited[unsupportedFlag] {
		return errors.New(unsupportedMessage)
	}
	opts := build(values, visited)
	if err := validate(opts); err != nil {
		return fmt.Errorf("%s: %v", validationPrefix, err)
	}
	assign(cfg, opts)
	return nil
}

func storyUpdateOptionsFromFlags(values updateFlagValues, visited map[string]bool) db.UpdateStoryOptions {
	common := commonUpdateOptionsFromFlags(values, visited)
	opts := db.UpdateStoryOptions{
		Title:      common.title,
		Collection: common.collection,
		URL:        common.url,
	}
	if visited["status"] {
		opts.Status = lowerTrimmedStringPtr(*values.status)
	}
	return opts
}

func articleUpdateOptionsFromFlags(values updateFlagValues, visited map[string]bool) db.UpdateArticleOptions {
	common := commonUpdateOptionsFromFlags(values, visited)
	opts := db.UpdateArticleOptions{}
	opts.Title = common.title
	opts.Collection = common.collection
	opts.URL = common.url
	if visited["source"] {
		opts.Source = trimmedStringPtr(*values.source)
	}
	return opts
}

func commonUpdateOptionsFromFlags(values updateFlagValues, visited map[string]bool) commonUpdateOptions {
	opts := commonUpdateOptions{}
	if visited["title"] {
		opts.title = trimmedStringPtr(*values.title)
	}
	if visited["collection"] {
		opts.collection = normalizedCollectionPtr(*values.collection)
	}
	if visited["url"] {
		opts.url = trimmedStringPtr(*values.url)
	}
	return opts
}

func trimmedStringPtr(raw string) *string {
	value := strings.TrimSpace(raw)
	return &value
}

func lowerTrimmedStringPtr(raw string) *string {
	value := strings.TrimSpace(strings.ToLower(raw))
	return &value
}

func normalizedCollectionPtr(raw string) *string {
	value := normalizeCollectionFlag(raw)
	return &value
}

func runUpdateStory(ctx context.Context, pool *db.Pool, storyUUID string, opts db.UpdateStoryOptions, now time.Time, dryRun bool) int {
	cfg := updateRunConfig[updateStorySnapshot, db.UpdateStoryOptions]{}
	cfg.uuid = storyUUID
	cfg.options = opts
	cfg.now = now
	cfg.dryRun = dryRun
	cfg.getSnapshot = getStoryUpdateSnapshot
	cfg.apply = updateStoryRecord
	cfg.preview = buildStoryUpdatePreview
	cfg.write = writeStoryUpdateResult
	setUpdateRunMessages(&cfg, "Story")
	return runUpdateEntity(ctx, pool, cfg)
}

func buildStoryUpdatePreview(before updateStorySnapshot, opts db.UpdateStoryOptions, now time.Time) updateStorySnapshot {
	after := before
	if opts.Title != nil {
		after.Title = strings.TrimSpace(*opts.Title)
	}
	if opts.Status != nil {
		after.Status = strings.TrimSpace(strings.ToLower(*opts.Status))
	}
	if opts.Collection != nil {
		after.Collection = normalizeCollectionFlag(*opts.Collection)
	}
	if opts.URL != nil {
		after.URL = previewURL(*opts.URL)
	}
	after.UpdatedAt = now.UTC()
	return after
}

func writeStoryUpdateResult(before, after updateStorySnapshot) int {
	return writeUpdateResult(before, after, writeStoryUpdateDiff)
}

func runUpdateEntity[Snapshot any, Options any](
	ctx context.Context,
	pool *db.Pool,
	cfg updateRunConfig[Snapshot, Options],
) int {
	before, err := cfg.getSnapshot(ctx, pool, cfg.uuid)
	if err != nil {
		return printUpdateLoadError(cfg.uuid, err, cfg.loadNotFound, cfg.loadFailed)
	}

	if cfg.dryRun {
		fmt.Println("dry_run=true")
		return cfg.write(*before, cfg.preview(*before, cfg.options, cfg.now))
	}

	if err := cfg.apply(ctx, pool, cfg.uuid, cfg.options, cfg.now); err != nil {
		return printUpdateApplyError(cfg.uuid, err, cfg.applyNotFound, cfg.applyFailed)
	}

	after, err := cfg.getSnapshot(ctx, pool, cfg.uuid)
	if err != nil {
		fmt.Fprintf(os.Stderr, cfg.postUpdateLoadFail, err)
		return 1
	}
	return cfg.write(*before, *after)
}

func printUpdateLoadError(uuid string, err error, notFoundFormat string, failedFormat string) int {
	return printUpdateError(uuid, err, notFoundFormat, failedFormat)
}

func printUpdateApplyError(uuid string, err error, notFoundFormat string, failedFormat string) int {
	return printUpdateError(uuid, err, notFoundFormat, failedFormat)
}

func printUpdateError(uuid string, err error, notFoundFormat string, failedFormat string) int {
	if errors.Is(err, db.ErrNoRows) {
		fmt.Fprintf(os.Stderr, notFoundFormat, uuid)
		return 1
	}
	fmt.Fprintf(os.Stderr, failedFormat, err)
	return 1
}

func updateStoryRecord(ctx context.Context, pool *db.Pool, storyUUID string, opts db.UpdateStoryOptions, now time.Time) error {
	return pool.UpdateStory(ctx, storyUUID, opts, now)
}

func updateArticleRecord(ctx context.Context, pool *db.Pool, articleUUID string, opts db.UpdateArticleOptions, now time.Time) error {
	return pool.UpdateArticle(ctx, articleUUID, opts, now)
}

func runUpdateArticle(ctx context.Context, pool *db.Pool, articleUUID string, opts db.UpdateArticleOptions, now time.Time, dryRun bool) int {
	cfg := updateRunConfig[updateArticleSnapshot, db.UpdateArticleOptions]{
		uuid:        articleUUID,
		options:     opts,
		now:         now,
		dryRun:      dryRun,
		getSnapshot: getArticleUpdateSnapshot,
		apply:       updateArticleRecord,
		preview:     buildArticleUpdatePreview,
		write:       writeArticleUpdateResult,
	}
	setUpdateRunMessages(&cfg, "Article")
	return runUpdateEntity(ctx, pool, cfg)
}

func setUpdateRunMessages[Snapshot any, Options any](cfg *updateRunConfig[Snapshot, Options], entity string) {
	cfg.loadNotFound = entity + " not found: %s\n"
	cfg.loadFailed = "Failed to load " + strings.ToLower(entity) + " before update: %v\n"
	cfg.applyNotFound = entity + " not found or already deleted: %s\n"
	cfg.applyFailed = "Failed to update " + strings.ToLower(entity) + ": %v\n"
	cfg.postUpdateLoadFail = entity + " updated but failed to load post-update state: %v\n"
}

func buildArticleUpdatePreview(before updateArticleSnapshot, opts db.UpdateArticleOptions, now time.Time) updateArticleSnapshot {
	after := before
	if opts.Title != nil {
		after.Title = normalizePreviewTitle(*opts.Title)
	}
	if opts.Source != nil {
		after.Source = strings.TrimSpace(*opts.Source)
	}
	if opts.Collection != nil {
		after.Collection = normalizeCollectionFlag(*opts.Collection)
	}
	if opts.URL != nil {
		after.URL = previewURL(*opts.URL)
		after.SourceDomain = previewSourceDomain(*opts.URL)
	}
	after.UpdatedAt = now.UTC()
	return after
}

func previewURL(raw string) *string {
	trimmed := strings.TrimSpace(raw)
	switch trimmed {
	case "":
		return nil
	default:
		return &trimmed
	}
}

func previewSourceDomain(raw string) *string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return hostFromURL(raw)
}

func writeArticleUpdateResult(before, after updateArticleSnapshot) int {
	return writeUpdateResult(before, after, writeArticleUpdateDiff)
}

func writeUpdateResult[Snapshot any](before, after Snapshot, write func(Snapshot, Snapshot) error) int {
	if err := write(before, after); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to render diff: %v\n", err)
		return 1
	}
	return 0
}

func getStoryUpdateSnapshot(ctx context.Context, pool *db.Pool, storyUUID string) (*updateStorySnapshot, error) {
	const q = `
SELECT
	s.story_uuid::text,
	s.canonical_title,
	s.status,
	s.collection,
	s.canonical_url,
	s.updated_at
FROM news.stories s
WHERE s.story_uuid = $1::uuid
  AND s.deleted_at IS NULL
LIMIT 1
`

	var snap updateStorySnapshot
	if err := pool.QueryRow(ctx, q, strings.TrimSpace(storyUUID)).Scan(
		&snap.StoryUUID,
		&snap.Title,
		&snap.Status,
		&snap.Collection,
		&snap.URL,
		&snap.UpdatedAt,
	); err != nil {
		if errors.Is(err, db.ErrNoRows) {
			return nil, db.ErrNoRows
		}
		return nil, err
	}
	return &snap, nil
}

func getArticleUpdateSnapshot(ctx context.Context, pool *db.Pool, articleUUID string) (*updateArticleSnapshot, error) {
	const q = `
SELECT
	a.article_uuid::text,
	a.normalized_title,
	a.source,
	a.collection,
	a.canonical_url,
	a.source_domain,
	a.updated_at
FROM news.articles a
WHERE a.article_uuid = $1::uuid
  AND a.deleted_at IS NULL
LIMIT 1
`

	var snap updateArticleSnapshot
	if err := pool.QueryRow(ctx, q, strings.TrimSpace(articleUUID)).Scan(
		&snap.ArticleUUID,
		&snap.Title,
		&snap.Source,
		&snap.Collection,
		&snap.URL,
		&snap.SourceDomain,
		&snap.UpdatedAt,
	); err != nil {
		if errors.Is(err, db.ErrNoRows) {
			return nil, db.ErrNoRows
		}
		return nil, err
	}
	return &snap, nil
}

func writeStoryUpdateDiff(before, after updateStorySnapshot) error {
	rows := [][]string{
		{"title", before.Title, after.Title},
		{"status", before.Status, after.Status},
		{"collection", before.Collection, after.Collection},
		{"url", pointerStringOrEmpty(before.URL), pointerStringOrEmpty(after.URL)},
		{"updated_at", formatUTCTimestamp(before.UpdatedAt), formatUTCTimestamp(after.UpdatedAt)},
	}
	return writeTable([]string{"field", "before", "after"}, rows)
}

func writeArticleUpdateDiff(before, after updateArticleSnapshot) error {
	rows := [][]string{
		{"title", before.Title, after.Title},
		{"source", before.Source, after.Source},
		{"collection", before.Collection, after.Collection},
		{"url", pointerStringOrEmpty(before.URL), pointerStringOrEmpty(after.URL)},
		{"source_domain", pointerStringOrEmpty(before.SourceDomain), pointerStringOrEmpty(after.SourceDomain)},
		{"updated_at", formatUTCTimestamp(before.UpdatedAt), formatUTCTimestamp(after.UpdatedAt)},
	}
	return writeTable([]string{"field", "before", "after"}, rows)
}

func validateStoryUpdateOptions(opts db.UpdateStoryOptions) error {
	return validateCommonUpdateOptions(commonUpdateOptions{
		title:      opts.Title,
		status:     opts.Status,
		collection: opts.Collection,
		url:        opts.URL,
	})
}

func validateArticleUpdateOptions(opts db.UpdateArticleOptions) error {
	common := commonUpdateOptions{}
	common.title = opts.Title
	common.source = opts.Source
	common.collection = opts.Collection
	common.url = opts.URL
	return validateCommonUpdateOptions(common)
}

type commonUpdateOptions struct {
	title      *string
	source     *string
	status     *string
	collection *string
	url        *string
}

func validateCommonUpdateOptions(opts commonUpdateOptions) error {
	return validateUpdateFields([]updateFieldValidation{
		requiredStringField(opts.title, "--title must not be empty"),
		requiredStringField(opts.source, "--source must not be empty"),
		requiredStringField(opts.status, "--status must not be empty"),
		collectionField(opts.collection),
		urlField(opts.url),
	})
}

type updateFieldValidation struct {
	isSet   bool
	isValid bool
	message string
}

func validateUpdateFields(fields []updateFieldValidation) error {
	hasField := false
	for _, field := range fields {
		if !field.isSet {
			continue
		}
		hasField = true
		if !field.isValid {
			return fmt.Errorf("%s", field.message)
		}
	}
	if !hasField {
		return fmt.Errorf("at least one update flag is required")
	}
	return nil
}

func requiredStringField(value *string, message string) updateFieldValidation {
	if value == nil {
		return updateFieldValidation{}
	}
	return updateFieldValidation{
		isSet:   true,
		isValid: strings.TrimSpace(*value) != "",
		message: message,
	}
}

func collectionField(value *string) updateFieldValidation {
	if value == nil {
		return updateFieldValidation{}
	}
	return updateFieldValidation{
		isSet:   true,
		isValid: normalizeCollectionFlag(*value) != "",
		message: "--collection must not be empty",
	}
}

func urlField(value *string) updateFieldValidation {
	if value == nil {
		return updateFieldValidation{}
	}
	return updateFieldValidation{
		isSet:   true,
		isValid: isFullyQualifiedURL(*value),
		message: "--url must be a fully-qualified URL",
	}
}

func isFullyQualifiedURL(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return false
	}
	return parsed.Scheme != "" && parsed.Host != ""
}

func normalizePreviewTitle(raw string) string {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	if trimmed == "" {
		return ""
	}
	return strings.Join(strings.Fields(trimmed), " ")
}

func hostFromURL(raw string) *string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil || strings.TrimSpace(parsed.Host) == "" {
		return nil
	}
	host := strings.TrimSpace(strings.ToLower(parsed.Hostname()))
	if host == "" {
		return nil
	}
	return &host
}

func printUpdateUsage() {
	printUsageLines(
		"Usage:",
		"  scoop update story <story_uuid> [--title ...] [--status ...] [--collection ...] [--url ...] [--dry-run] [--env .env] [--timeout 30s]",
		"  scoop update article <article_uuid> [--title ...] [--source ...] [--collection ...] [--url ...] [--dry-run] [--env .env] [--timeout 30s]",
	)
}
