package app

import "testing"

func TestAdditionalReadCommandParsers(t *testing.T) {
	t.Parallel()

	if cfg, exitCode, ok := parseSearchCommand([]string{"--query", " OpenClaw ", "--collection", " OpenClaw ", "--limit", "3", "--format", "json"}); !ok || exitCode != 0 || cfg.query != "OpenClaw" || cfg.collection != "openclaw" || cfg.limit != 3 || cfg.format != outputFormatJSON {
		t.Fatalf("parseSearchCommand() cfg=%#v exit=%d ok=%t", cfg, exitCode, ok)
	}
	searchFailures := [][]string{
		{"--query", "openclaw", "extra"},
		{"--query", " "},
		{"--query", "openclaw", "--limit", "0"},
		{"--query", "openclaw", "--format", "yaml"},
	}
	for _, args := range searchFailures {
		if _, exitCode, ok := parseSearchCommand(args); ok || exitCode != 2 {
			t.Fatalf("parseSearchCommand(%v) ok=%t exit=%d, want validation failure", args, ok, exitCode)
		}
	}

	if cfg, exitCode, ok := parseCollectionsCommand([]string{"--format", "json"}); !ok || exitCode != 0 || cfg.format != outputFormatJSON {
		t.Fatalf("parseCollectionsCommand() cfg=%#v exit=%d ok=%t", cfg, exitCode, ok)
	}
	if _, exitCode, ok := parseCollectionsCommand([]string{"extra"}); ok || exitCode != 2 {
		t.Fatalf("parseCollectionsCommand(extra) ok=%t exit=%d, want validation failure", ok, exitCode)
	}
	if _, exitCode, ok := parseCollectionsCommand([]string{"--format", "yaml"}); ok || exitCode != 2 {
		t.Fatalf("parseCollectionsCommand(format) ok=%t exit=%d, want validation failure", ok, exitCode)
	}

	if cfg, exitCode, ok := parseArticlesListCommand([]string{"--collection", " OpenClaw ", "--from", "2026-05-14", "--to", "2026-05-15", "--limit", "4", "--format", "json"}); !ok || exitCode != 0 || cfg.collection != "openclaw" || cfg.limit != 4 || cfg.format != outputFormatJSON {
		t.Fatalf("parseArticlesListCommand() cfg=%#v exit=%d ok=%t", cfg, exitCode, ok)
	}
	if _, exitCode, ok := parseArticlesListCommand([]string{"--limit", "0"}); ok || exitCode != 2 {
		t.Fatalf("parseArticlesListCommand(limit) ok=%t exit=%d, want validation failure", ok, exitCode)
	}
}

func TestAdditionalIngestAndDigestParsers(t *testing.T) {
	t.Parallel()

	if cfg, exitCode, ok := parseDigestCommand([]string{"--collection", " OpenClaw ", "--date", "2026-05-14", "--format", "json"}); !ok || exitCode != 0 || cfg.collection != "openclaw" || cfg.date.Format("2006-01-02") != "2026-05-14" || cfg.format != outputFormatJSON {
		t.Fatalf("parseDigestCommand() cfg=%#v exit=%d ok=%t", cfg, exitCode, ok)
	}
	digestFailures := [][]string{
		{"--collection", " "},
		{"--collection", "openclaw", "--date", "bad-date"},
		{"--collection", "openclaw", "--format", "yaml"},
		{"--collection", "openclaw", "extra"},
	}
	for _, args := range digestFailures {
		if _, exitCode, ok := parseDigestCommand(args); ok || exitCode != 2 {
			t.Fatalf("parseDigestCommand(%v) ok=%t exit=%d, want validation failure", args, ok, exitCode)
		}
	}

	if cfg, exitCode, ok := parseIngestCommand([]string{"--payload", `{"ok":true}`, "--checkpoint", `{"cursor":"1"}`, "--triggered-by-topic", " openclaw ", "--timeout", "2s"}); !ok || exitCode != 0 || string(cfg.payloadJSON) != `{"ok":true}` || string(cfg.checkpointJSON) != `{"cursor":"1"}` || cfg.triggeredByTopic != "openclaw" {
		t.Fatalf("parseIngestCommand() cfg=%#v exit=%d ok=%t", cfg, exitCode, ok)
	}
	if _, exitCode, ok := parseIngestCommand([]string{"--payload", " "}); ok || exitCode != 2 {
		t.Fatalf("parseIngestCommand(blank payload) ok=%t exit=%d, want validation failure", ok, exitCode)
	}
}
