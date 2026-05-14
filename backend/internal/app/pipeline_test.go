package app

import "testing"

func TestValidatePositiveIntFlags(t *testing.T) {
	t.Parallel()

	if err := validatePositiveIntFlags([]positiveIntFlag{{name: "--limit", value: 1}}); err != nil {
		t.Fatalf("validatePositiveIntFlags() error = %v", err)
	}
	err := validatePositiveIntFlags([]positiveIntFlag{{name: "--limit", value: 0}})
	if err == nil || err.Error() != "--limit must be > 0" {
		t.Fatalf("validatePositiveIntFlags() error = %v, want --limit validation", err)
	}
}

func TestParseEmbedCommand(t *testing.T) {
	t.Parallel()

	cfg, exitCode, ok := parseEmbedCommand([]string{
		"--limit", "2",
		"--batch-size", "3",
		"--endpoint", "http://127.0.0.1:9999/embed",
		"--model-name", "model",
		"--model-version", "v1",
		"--max-length", "400",
		"--request-timeout", "2s",
	})
	if !ok || exitCode != 0 {
		t.Fatalf("parseEmbedCommand() ok=%t exit=%d", ok, exitCode)
	}
	if cfg.limit != 2 || cfg.batchSize != 3 || cfg.modelName != "model" || cfg.modelVersion != "v1" {
		t.Fatalf("unexpected embed config: %#v", cfg)
	}

	if _, exitCode, ok := parseEmbedCommand([]string{"--limit", "0"}); ok || exitCode != 2 {
		t.Fatalf("invalid limit ok=%t exit=%d, want validation failure", ok, exitCode)
	}
}

func TestParseProcessCommand(t *testing.T) {
	t.Parallel()

	cfg, exitCode, ok := parseProcessCommand([]string{
		"--normalize-limit", "2",
		"--embed-limit", "3",
		"--embed-batch-size", "4",
		"--embed-endpoint", "http://127.0.0.1:9999/embed",
		"--model-name", "model",
		"--model-version", "v1",
		"--embed-max-length", "400",
		"--dedup-limit", "5",
		"--dedup-lookback-days", "6",
		"--until-empty=false",
		"--max-cycles", "7",
	})
	if !ok || exitCode != 0 {
		t.Fatalf("parseProcessCommand() ok=%t exit=%d", ok, exitCode)
	}
	if cfg.normalizeLimit != 2 || cfg.embedLimit != 3 || cfg.dedupLimit != 5 || cfg.maxCycles != 7 || cfg.untilEmpty {
		t.Fatalf("unexpected process config: %#v", cfg)
	}

	if _, exitCode, ok := parseProcessCommand([]string{"--max-cycles", "0"}); ok || exitCode != 2 {
		t.Fatalf("invalid max cycles ok=%t exit=%d, want validation failure", ok, exitCode)
	}
}

func TestProcessDrainExitCode(t *testing.T) {
	t.Parallel()

	if code := processDrainExitCode(processTotals{drained: true}, processCommandConfig{untilEmpty: true, maxCycles: 1}); code != 0 {
		t.Fatalf("drained exit code = %d, want 0", code)
	}
	if code := processDrainExitCode(processTotals{drained: false}, processCommandConfig{untilEmpty: false, maxCycles: 1}); code != 0 {
		t.Fatalf("single-cycle exit code = %d, want 0", code)
	}
	if code := processDrainExitCode(processTotals{drained: false}, processCommandConfig{untilEmpty: true, maxCycles: 1}); code != 1 {
		t.Fatalf("not drained exit code = %d, want 1", code)
	}
}
