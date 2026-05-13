package app

import "testing"

func TestRootCommandLookup(t *testing.T) {
	t.Parallel()

	if got := normalizeCommandName("  STORIES "); got != "stories" {
		t.Fatalf("normalizeCommandName() = %q, want stories", got)
	}
	if !isHelpCommand("help") || !isHelpCommand("--help") || !isHelpCommand("-h") {
		t.Fatal("expected help aliases to be recognized")
	}
	if isHelpCommand("stories") {
		t.Fatal("stories should not be recognized as help")
	}

	for _, name := range []string{"stories", "process", "run-once"} {
		command, ok := findRootCommand(name)
		if !ok {
			t.Fatalf("findRootCommand(%q) ok = false", name)
		}
		if len(command.names) == 0 {
			t.Fatalf("findRootCommand(%q) returned an unnamed command", name)
		}
	}

	if _, ok := findRootCommand("missing"); ok {
		t.Fatal("findRootCommand(missing) ok = true")
	}
}
