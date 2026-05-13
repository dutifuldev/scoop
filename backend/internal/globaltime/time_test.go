package globaltime

import (
	"testing"
	"time"
)

func TestMockTime(t *testing.T) {
	fixed := time.Date(2026, 5, 13, 12, 0, 0, 0, time.FixedZone("TST", 2*60*60))
	SetMockTime(fixed)
	t.Cleanup(ResetTime)

	if got := Now(); !got.Equal(fixed) {
		t.Fatalf("Now mismatch: want %s, got %s", fixed, got)
	}
	if got := UTC(); !got.Equal(fixed.UTC()) {
		t.Fatalf("UTC mismatch: want %s, got %s", fixed.UTC(), got)
	}
}
