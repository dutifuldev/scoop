package logging

import (
	"testing"

	"github.com/rs/zerolog"
)

func TestNewLoggerParsesLevel(t *testing.T) {
	t.Parallel()

	logger, err := New("production", "warn")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if logger.GetLevel() != zerolog.WarnLevel {
		t.Fatalf("expected warn level, got %s", logger.GetLevel())
	}
}

func TestNewLoggerRejectsInvalidLevel(t *testing.T) {
	t.Parallel()

	if _, err := New("production", "not-a-level"); err == nil {
		t.Fatalf("expected invalid log level error")
	}
}
