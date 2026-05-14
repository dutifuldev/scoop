package reader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchTextWithOptionsPlainTextUsesInjectedClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "test-agent" {
			t.Fatalf("User-Agent = %q, want test-agent", got)
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("Hello\n\nworld"))
	}))
	t.Cleanup(server.Close)

	got, err := FetchTextWithOptions(context.Background(), server.URL, "ignored", FetchOptions{
		Timeout:       time.Second,
		BodyByteLimit: 1024,
		UserAgent:     "test-agent",
		HTTPClient:    server.Client(),
	})
	if err != nil {
		t.Fatalf("FetchTextWithOptions() error = %v", err)
	}
	if got != "Hello\n\nworld" {
		t.Fatalf("text = %q, want cleaned plain text", got)
	}
}

func TestFetchTextUsesDefaultOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("Default options body"))
	}))
	t.Cleanup(server.Close)

	got, err := FetchText(context.Background(), server.URL, "")
	if err != nil {
		t.Fatalf("FetchText() error = %v", err)
	}
	if got != "Default options body" {
		t.Fatalf("FetchText() = %q, want plain text", got)
	}
}

func TestFetchTextWithOptionsRejectsNotFoundStatus(t *testing.T) {
	if _, err := FetchTextWithOptions(context.Background(), " ", "", FetchOptions{}); err == nil {
		t.Fatalf("blank canonical URL should fail")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	_, err := FetchTextWithOptions(context.Background(), server.URL, "", FetchOptions{HTTPClient: server.Client()})
	if err == nil || !strings.Contains(err.Error(), "fetch status 404") {
		t.Fatalf("status error = %v, want 404", err)
	}
}

func TestFetchTextWithOptionsReadableHTML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>Article</title></head><body><article><h1>Article</h1><p>This is a readable article body with enough useful text to extract.</p></article></body></html>`))
	}))
	t.Cleanup(server.Close)

	got, err := FetchTextWithOptions(context.Background(), server.URL, "Article", FetchOptions{HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("FetchTextWithOptions(html) error = %v", err)
	}
	if !strings.Contains(got, "readable article body") {
		t.Fatalf("html text = %q", got)
	}
}
