package app

import (
	"testing"
	"time"
)

func TestParseSingleTagCommand(t *testing.T) {
	t.Parallel()

	cfg, exitCode, ok := parseSingleTagCommand([]string{"i0", "--format", "json", "--timeout", "3s"}, "tags archive", true)
	if !ok || exitCode != 0 {
		t.Fatalf("parseSingleTagCommand() ok=%t exit=%d", ok, exitCode)
	}
	if cfg.slug != "i0" || cfg.format != outputFormatJSON || cfg.timeout != 3*time.Second {
		t.Fatalf("config = %#v", cfg)
	}

	cfg, exitCode, ok = parseSingleTagCommand([]string{"i0", "--format", "json"}, "tags delete", false)
	if ok || exitCode != 2 {
		t.Fatalf("format-disabled parse ok=%t exit=%d cfg=%#v, want flag error", ok, exitCode, cfg)
	}
}

func TestParseSingleTagCommandRejectsInvalidArgs(t *testing.T) {
	t.Parallel()

	cases := [][]string{
		nil,
		{"i0", "extra"},
		{"i0", "--format", "yaml"},
	}
	for _, args := range cases {
		if _, exitCode, ok := parseSingleTagCommand(args, "tags archive", true); ok || exitCode != 2 {
			t.Fatalf("parseSingleTagCommand(%v) ok=%t exit=%d, want validation failure", args, ok, exitCode)
		}
	}
}

func TestParseTagsCreateCommandCapturesOptionalFields(t *testing.T) {
	t.Parallel()

	cfg, exitCode, ok := parseTagsCreateCommand([]string{
		"i0",
		"--description", "most interesting",
		"--color", "#ff0000",
		"--highlight-color", "#fff3b0",
		"--format", "json",
	})
	if !ok || exitCode != 0 {
		t.Fatalf("parseTagsCreateCommand() ok=%t exit=%d", ok, exitCode)
	}
	if cfg.opts.Slug != "i0" || cfg.format != outputFormatJSON {
		t.Fatalf("config = %#v", cfg)
	}
	if cfg.opts.Description == nil || *cfg.opts.Description != "most interesting" {
		t.Fatalf("description = %v", cfg.opts.Description)
	}
	if cfg.opts.Color == nil || *cfg.opts.Color != "#ff0000" {
		t.Fatalf("color = %v", cfg.opts.Color)
	}
	if cfg.opts.HighlightColor == nil || *cfg.opts.HighlightColor != "#fff3b0" {
		t.Fatalf("highlight color = %v", cfg.opts.HighlightColor)
	}
}

func TestParseTagsCreateCommandRejectsInvalidArgs(t *testing.T) {
	t.Parallel()

	cases := [][]string{
		nil,
		{"i0", "extra"},
		{"i0", "--format", "yaml"},
	}
	for _, args := range cases {
		if _, exitCode, ok := parseTagsCreateCommand(args); ok || exitCode != 2 {
			t.Fatalf("parseTagsCreateCommand(%v) ok=%t exit=%d, want validation failure", args, ok, exitCode)
		}
	}
}

func TestParseTagsListCommand(t *testing.T) {
	t.Parallel()

	cfg, exitCode, ok := parseTagsListCommand([]string{"--include-archived", "--format", "json", "--timeout", "4s"})
	if !ok || exitCode != 0 {
		t.Fatalf("parseTagsListCommand() ok=%t exit=%d", ok, exitCode)
	}
	if !cfg.includeArchived || cfg.format != outputFormatJSON || cfg.timeout != 4*time.Second {
		t.Fatalf("config = %#v", cfg)
	}
	if _, exitCode, ok := parseTagsListCommand([]string{"extra"}); ok || exitCode != 2 {
		t.Fatalf("positional ok=%t exit=%d, want validation failure", ok, exitCode)
	}
}

func TestRunTagsRenameAndUpdateValidation(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{{"old-tag"}, {"old-tag", "new-tag", "extra"}} {
		if exitCode := runTagsRename(args); exitCode != 2 {
			t.Fatalf("runTagsRename(%v) = %d, want validation failure", args, exitCode)
		}
	}
	if exitCode := runTagsUpdate([]string{}); exitCode != 2 {
		t.Fatalf("runTagsUpdate(empty) = %d, want validation failure", exitCode)
	}
	if exitCode := runTagsUpdate([]string{"i0", "extra"}); exitCode != 2 {
		t.Fatalf("runTagsUpdate(extra) = %d, want validation failure", exitCode)
	}
}

func TestParseTagsArticleMutationCommand(t *testing.T) {
	t.Parallel()

	cfg, exitCode, ok := parseArticleValueCommand([]string{"article-uuid", "i0", "--timeout", "2s"}, "tags add-article", "usage", false)
	if !ok || exitCode != 0 {
		t.Fatalf("parseArticleValueCommand() ok=%t exit=%d", ok, exitCode)
	}
	if cfg.articleUUID != "article-uuid" || cfg.value != "i0" || cfg.timeout != 2*time.Second {
		t.Fatalf("config = %#v", cfg)
	}
	for _, args := range [][]string{nil, {"article"}, {"article", "i0", "extra"}} {
		if _, exitCode, ok := parseArticleValueCommand(args, "tags add-article", "usage", false); ok || exitCode != 2 {
			t.Fatalf("parseArticleValueCommand(%v) ok=%t exit=%d, want validation failure", args, ok, exitCode)
		}
	}
}
