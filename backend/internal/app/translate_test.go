package app

import "testing"

func TestParseTranslateCommandValidStory(t *testing.T) {
	t.Parallel()

	cfg, exitCode, ok := parseTranslateCommand([]string{"story", "--lang", "zh", "--provider", "local", "--dry-run", "--force", "story-uuid"})
	if !ok {
		t.Fatalf("parseTranslateCommand() ok=false exit=%d", exitCode)
	}
	if cfg.target != "story" || cfg.identifier != "story-uuid" {
		t.Fatalf("unexpected target/identifier: %#v", cfg)
	}
	if cfg.targetLang != "zh" || cfg.provider != "local" || !cfg.dryRun || !cfg.force {
		t.Fatalf("unexpected translate flags: %#v", cfg)
	}
}

func TestParseTranslateCommandRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{name: "missing target", args: nil},
		{name: "bad target", args: []string{"feed", "--lang", "zh", "item"}},
		{name: "missing lang", args: []string{"story", "story-uuid"}},
		{name: "missing identifier", args: []string{"story", "--lang", "zh"}},
		{name: "empty identifier", args: []string{"story", "--lang", "zh", "   "}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, exitCode, ok := parseTranslateCommand(tt.args); ok || exitCode == 0 {
				t.Fatalf("parseTranslateCommand(%v) ok=%t exit=%d, want nonzero failure", tt.args, ok, exitCode)
			}
		})
	}
}
