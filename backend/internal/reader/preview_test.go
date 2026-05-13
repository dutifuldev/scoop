package reader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCleanTextCollapsesWhitespaceAndPreservesParagraphs(t *testing.T) {
	input := "  First   paragraph \n\n Second\tparagraph \r\n\r\nThird line "
	got := CleanText(input)
	want := "First paragraph\n\nSecond paragraph\n\nThird line"
	if got != want {
		t.Fatalf("CleanText mismatch\nwant: %q\ngot:  %q", want, got)
	}
}

func TestTruncateText(t *testing.T) {
	input := "abcdefghijklmnopqrstuvwxyz"

	got, truncated := TruncateText(input, 10)
	if !truncated {
		t.Fatalf("expected truncated=true")
	}
	if got != "abcdefghi…" {
		t.Fatalf("unexpected truncated text: %q", got)
	}

	full, wasTruncated := TruncateText("short", 10)
	if wasTruncated {
		t.Fatalf("expected truncated=false for short text")
	}
	if full != "short" {
		t.Fatalf("unexpected short text: %q", full)
	}
}

func TestFetchTextWithOptionsRequiresURL(t *testing.T) {
	t.Parallel()

	if _, err := FetchTextWithOptions(context.Background(), " ", "", FetchOptions{}); err == nil {
		t.Fatalf("expected missing URL error")
	}
}

func TestFetchTextWithOptionsPlainText(t *testing.T) {
	t.Parallel()

	var userAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgent = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(" first  line\n\nsecond\tline "))
	}))
	t.Cleanup(server.Close)

	got, err := FetchTextWithOptions(context.Background(), server.URL, "", FetchOptions{
		Timeout:       time.Second,
		BodyByteLimit: 1024,
		UserAgent:     "scoop-test",
	})
	if err != nil {
		t.Fatalf("FetchTextWithOptions: %v", err)
	}
	if got != "first line\n\nsecond line" {
		t.Fatalf("unexpected text: %q", got)
	}
	if userAgent != "scoop-test" {
		t.Fatalf("unexpected user agent: %q", userAgent)
	}
}

func TestFetchTextWithOptionsRejectsHTTPError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusTeapot)
	}))
	t.Cleanup(server.Close)

	_, err := FetchTextWithOptions(context.Background(), server.URL, "", FetchOptions{Timeout: time.Second})
	if err == nil || !strings.Contains(err.Error(), "fetch status 418") {
		t.Fatalf("expected status error, got %v", err)
	}
}

func TestFetchTextWithOptionsReportsUnreadableHTML(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><head><title></title></head><body></body></html>"))
	}))
	t.Cleanup(server.Close)

	_, err := FetchTextWithOptions(context.Background(), server.URL, "Fallback Title", FetchOptions{Timeout: time.Second})
	if err == nil || !strings.Contains(err.Error(), "render readability text") {
		t.Fatalf("expected readability render error, got %v", err)
	}
}
