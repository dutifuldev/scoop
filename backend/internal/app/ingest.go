package app

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"horse.fit/scoop/internal/cli"
	"horse.fit/scoop/internal/db"
	"horse.fit/scoop/internal/ingest"
	payloadschema "horse.fit/scoop/schema"
)

type ingestCommandConfig struct {
	envLoader        *cli.EnvLoader
	timeout          time.Duration
	payloadJSON      json.RawMessage
	checkpointJSON   json.RawMessage
	triggeredByTopic string
}

func runIngest(args []string) int {
	return runParsedCommand(args, parseIngestCommand, executeIngestCommand)
}

func parseIngestCommand(args []string) (ingestCommandConfig, int, bool) {
	fs := newAppFlagSet("ingest")

	envLoader := cli.AddEnvFlag(fs, ".env", "Path to the .env file")
	timeout := fs.Duration("timeout", 20*time.Second, "Command timeout")
	payload := fs.String("payload", `{"payload_version":"v1","source":"manual_cli","source_item_id":"manual-1","title":"manual ingest event","source_metadata":{"collection":"manual_cli","job_name":"manual_cli","job_run_id":"manual-1","scraped_at":"2026-02-14T00:00:00Z","kind":"manual"}}`, "Canonical news-article payload JSON")
	payloadFile := fs.String("payload-file", "", "Path to payload JSON file (overrides --payload)")
	checkpoint := fs.String("checkpoint", `{"cursor":"manual"}`, "Checkpoint JSON")
	checkpointFile := fs.String("checkpoint-file", "", "Path to checkpoint JSON file (overrides --checkpoint)")
	triggeredByTopic := fs.String("triggered-by-topic", "manual_cli", "Topic trace for ingest_runs.triggered_by_topic")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return ingestCommandConfig{}, 0, false
		}
		return ingestCommandConfig{}, 2, false
	}

	payloadJSON, err := loadJSONInput(*payload, *payloadFile, "payload")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid payload: %v\n", err)
		return ingestCommandConfig{}, 2, false
	}
	checkpointJSON, err := loadJSONInput(*checkpoint, *checkpointFile, "checkpoint")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid checkpoint: %v\n", err)
		return ingestCommandConfig{}, 2, false
	}
	return ingestCommandConfig{
		envLoader:        envLoader,
		timeout:          *timeout,
		payloadJSON:      payloadJSON,
		checkpointJSON:   checkpointJSON,
		triggeredByTopic: strings.TrimSpace(*triggeredByTopic),
	}, 0, true
}

func executeIngestCommand(commandCfg ingestCommandConfig) int {
	runtime, exitCode, ok := newCommandRuntimeBase(commandCfg.timeout, commandCfg.envLoader)
	if !ok {
		return exitCode
	}
	defer runtime.Close()

	request, err := ingestRequestFromPayload(commandCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid payload: %v\n", err)
		return 2
	}

	pool, err := db.NewPool(runtime.ctx, runtime.cfg)
	if err != nil {
		runtime.logger.Error().Err(err).Msg("database connection failed")
		fmt.Fprintf(os.Stderr, "Failed to connect to database: %v\n", err)
		return 1
	}
	runtime.pool = pool

	svc := ingest.NewService(pool, runtime.logger)
	result, err := svc.IngestOne(runtime.ctx, request)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Ingest failed: %v\n", err)
		return 1
	}

	printIngestResult(result)
	return 0
}

func ingestRequestFromPayload(commandCfg ingestCommandConfig) (ingest.Request, error) {
	article, err := payloadschema.ValidateNewsItemPayload(commandCfg.payloadJSON)
	if err != nil {
		return ingest.Request{}, err
	}

	payloadPublishedAt, err := parseOptionalRFC3339("payload.published_at", pointerStringOrEmpty(article.PublishedAt))
	if err != nil {
		return ingest.Request{}, err
	}
	collection, err := requiredMetadataString(article.SourceMetadata, "collection")
	if err != nil {
		return ingest.Request{}, err
	}

	return ingest.Request{
		Source:            strings.TrimSpace(article.Source),
		SourceItemID:      strings.TrimSpace(article.SourceItemID),
		Collection:        collection,
		SourceItemURL:     pointerStringOrEmpty(article.CanonicalURL),
		SourcePublishedAt: payloadPublishedAt,
		RawPayload:        commandCfg.payloadJSON,
		CursorCheckpoint:  commandCfg.checkpointJSON,
		TriggeredByTopic:  commandCfg.triggeredByTopic,
		ResponseHeaders:   nil,
	}, nil
}

func printIngestResult(result ingest.Result) {
	fmt.Printf("run_id=%d status=%s inserted=%t payload_hash=%s\n", result.RunID, result.Status, result.Inserted, result.PayloadHashHex)
	fmt.Printf("run_uuid=%s\n", result.RunUUID)
	if result.RawArrivalID != nil {
		fmt.Printf("raw_arrival_id=%d\n", *result.RawArrivalID)
	}
	if result.RawArrivalUUID != nil {
		fmt.Printf("raw_arrival_uuid=%s\n", *result.RawArrivalUUID)
	}
}

func loadJSONInput(inlineValue, filePath, label string) (json.RawMessage, error) {
	if path := strings.TrimSpace(filePath); path != "" {
		payload, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s file %q: %w", label, path, err)
		}
		trimmed := strings.TrimSpace(string(payload))
		if trimmed == "" {
			return nil, fmt.Errorf("%s file %q is empty", label, path)
		}
		return json.RawMessage(trimmed), nil
	}

	trimmed := strings.TrimSpace(inlineValue)
	if trimmed == "" {
		return nil, fmt.Errorf("%s JSON is empty", label)
	}
	return json.RawMessage(trimmed), nil
}

func parseOptionalRFC3339(fieldName, raw string) (*time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	ts, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return nil, fmt.Errorf("%s must be RFC3339: %w", fieldName, err)
	}
	utc := ts.UTC()
	return &utc, nil
}

func requiredMetadataString(metadata map[string]any, key string) (string, error) {
	if len(metadata) == 0 {
		return "", fmt.Errorf("source_metadata.%s is required", key)
	}
	raw, ok := metadata[key]
	if !ok {
		return "", fmt.Errorf("source_metadata.%s is required", key)
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("source_metadata.%s must be a string", key)
	}
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return "", fmt.Errorf("source_metadata.%s must not be empty", key)
	}
	return trimmed, nil
}
