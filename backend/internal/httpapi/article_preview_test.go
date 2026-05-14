package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestBuildArticlePreviewTextUsesReaderWhenURLHasBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>Reader title</title></head><body><article><p>This reader body has enough detail for the preview.</p></article></body></html>`))
	}))
	t.Cleanup(server.Close)

	text, source, err := buildArticlePreviewText(context.Background(), &server.URL, "Reader title", "normalized body", "rss")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source != "reader" {
		t.Fatalf("unexpected source: %q", source)
	}
	if text == "" || text == "normalized body" {
		t.Fatalf("expected reader text, got %q", text)
	}
}

func TestBuildArticlePreviewTextFallsBackWhenReaderFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not available", http.StatusInternalServerError)
	}))
	server.Close()

	text, source, err := buildArticlePreviewText(context.Background(), &server.URL, "title", "normalized body", "rss")
	if err == nil {
		t.Fatalf("expected reader error")
	}
	if source != "normalized_text" {
		t.Fatalf("unexpected source: %q", source)
	}
	if text != "normalized body" {
		t.Fatalf("unexpected fallback text: %q", text)
	}
}
