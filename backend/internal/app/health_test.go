package app

import "testing"

func TestRunHealthValidationAndConfigFailures(t *testing.T) {
	if code := runHealth([]string{"--help"}); code != 0 {
		t.Fatalf("runHealth(--help) = %d, want 0", code)
	}
	if code := runHealth([]string{"--timeout", "not-a-duration"}); code != 2 {
		t.Fatalf("runHealth(bad timeout) = %d, want 2", code)
	}

	t.Setenv("DATABASE_URL", "")
	if code := runHealth(nil); code != 1 {
		t.Fatalf("runHealth(missing config) = %d, want 1", code)
	}
}
