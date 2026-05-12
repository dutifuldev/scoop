package httpapi

import (
	"context"
	"testing"
)

func TestTruncatePreviewText(t *testing.T) {
	input := "abcdefghijklmnopqrstuvwxyz"

	got, truncated := truncatePreviewText(input, 10)
	if !truncated {
		t.Fatalf("expected truncated=true")
	}
	if got != "abcdefghi…" {
		t.Fatalf("unexpected truncated text: %q", got)
	}

	full, wasTruncated := truncatePreviewText("short", 10)
	if wasTruncated {
		t.Fatalf("expected truncated=false for short text")
	}
	if full != "short" {
		t.Fatalf("unexpected short text: %q", full)
	}
}

func TestBuildArticlePreviewTextFallsBackToNormalizedTextWhenNoURL(t *testing.T) {
	text, source, err := buildArticlePreviewText(context.Background(), nil, "title", "normalized body", "rss")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source != "normalized_text" {
		t.Fatalf("unexpected source: %q", source)
	}
	if text != "normalized body" {
		t.Fatalf("unexpected text: %q", text)
	}
}
