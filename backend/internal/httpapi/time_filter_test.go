package httpapi

import (
	"testing"
	"time"
)

func TestParseTimeFilterInterpretsDateInViewerTimeZone(t *testing.T) {
	t.Parallel()

	location, _, err := parseClientTimeZone("Asia/Singapore")
	if err != nil {
		t.Fatalf("parse timezone: %v", err)
	}

	start, err := parseTimeFilter("2026-05-11", false, location)
	if err != nil {
		t.Fatalf("parse start: %v", err)
	}
	end, err := parseTimeFilter("2026-05-11", true, location)
	if err != nil {
		t.Fatalf("parse end: %v", err)
	}

	if want := "2026-05-10T16:00:00Z"; start.Format(time.RFC3339) != want {
		t.Fatalf("unexpected start: got %s want %s", start.Format(time.RFC3339), want)
	}
	if want := "2026-05-11T15:59:59.999999999Z"; end.Format(time.RFC3339Nano) != want {
		t.Fatalf("unexpected end: got %s want %s", end.Format(time.RFC3339Nano), want)
	}
}

func TestParseTimeFilterKeepsRFC3339Exact(t *testing.T) {
	t.Parallel()

	location, _, err := parseClientTimeZone("Asia/Singapore")
	if err != nil {
		t.Fatalf("parse timezone: %v", err)
	}

	got, err := parseTimeFilter("2026-05-11T01:30:00+08:00", false, location)
	if err != nil {
		t.Fatalf("parse timestamp: %v", err)
	}
	if want := "2026-05-10T17:30:00Z"; got.Format(time.RFC3339) != want {
		t.Fatalf("unexpected timestamp: got %s want %s", got.Format(time.RFC3339), want)
	}
}
