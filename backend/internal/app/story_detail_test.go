package app

import "testing"

func TestParseStoryDetailCommand(t *testing.T) {
	t.Parallel()

	cfg, exitCode, ok := parseStoryDetailCommand([]string{"--format", "json", "--timeout", "5s", "story-uuid"})
	if !ok || exitCode != 0 {
		t.Fatalf("parseStoryDetailCommand() ok=%t exit=%d", ok, exitCode)
	}
	if cfg.storyUUID != "story-uuid" || cfg.format != outputFormatJSON {
		t.Fatalf("config = %#v", cfg)
	}

	cases := [][]string{
		nil,
		{"story-uuid", "extra"},
		{"--format", "yaml", "story-uuid"},
		{" "},
	}
	for _, args := range cases {
		if _, exitCode, ok := parseStoryDetailCommand(args); ok || exitCode != 2 {
			t.Fatalf("parseStoryDetailCommand(%v) ok=%t exit=%d, want validation failure", args, ok, exitCode)
		}
	}
}
