package reader

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	readability "codeberg.org/readeck/go-readability/v2"
)

const (
	DefaultFetchTimeout  = 12 * time.Second
	DefaultBodyByteLimit = 2 * 1024 * 1024

	defaultUserAgent = "SCOOP-ReaderPreview/1.0 (+https://github.com/janitrai/scoop)"
)

// FetchOptions controls HTTP behavior for reader preview extraction.
type FetchOptions struct {
	Timeout       time.Duration
	BodyByteLimit int64
	UserAgent     string
	HTTPClient    *http.Client
}

// FetchText retrieves and extracts readable text content for a canonical URL.
func FetchText(ctx context.Context, canonicalURL string, title string) (string, error) {
	return FetchTextWithOptions(ctx, canonicalURL, title, FetchOptions{})
}

// FetchTextWithOptions retrieves and extracts readable text content for a canonical URL.
func FetchTextWithOptions(ctx context.Context, canonicalURL string, title string, opts FetchOptions) (string, error) {
	page := strings.TrimSpace(canonicalURL)
	if page == "" {
		return "", fmt.Errorf("canonical URL is required")
	}

	opts = normalizeFetchOptions(opts)

	fetchCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	resp, err := fetchPage(fetchCtx, page, opts)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if !isSuccessStatus(resp.StatusCode) {
		return "", fmt.Errorf("fetch status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, opts.BodyByteLimit))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	return extractFetchedText(page, title, resp.Header.Get("Content-Type"), body)
}

func normalizeFetchOptions(opts FetchOptions) FetchOptions {
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultFetchTimeout
	}
	if opts.BodyByteLimit <= 0 {
		opts.BodyByteLimit = DefaultBodyByteLimit
	}
	if strings.TrimSpace(opts.UserAgent) == "" {
		opts.UserAgent = defaultUserAgent
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: opts.Timeout}
	}
	return opts
}

func fetchPage(ctx context.Context, page string, opts FetchOptions) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, page, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("User-Agent", strings.TrimSpace(opts.UserAgent))
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.8")

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch url: %w", err)
	}
	return resp, nil
}

func isSuccessStatus(statusCode int) bool {
	return statusCode >= 200 && statusCode < 300
}

func extractFetchedText(page string, title string, contentType string, body []byte) (string, error) {
	if isPlainText(contentType) {
		return CleanText(string(body)), nil
	}

	text, excerpt, err := parseReadableHTML(page, body)
	if err != nil {
		return "", err
	}
	return bestExtractedText(text, excerpt, title)
}

func isPlainText(contentType string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "text/plain")
}

func parseReadableHTML(page string, body []byte) (string, string, error) {
	pageURL, err := url.Parse(page)
	if err != nil {
		return "", "", fmt.Errorf("parse page url: %w", err)
	}
	article, err := readability.FromReader(bytes.NewReader(body), pageURL)
	if err != nil {
		return "", "", fmt.Errorf("readability parse: %w", err)
	}

	var renderedText bytes.Buffer
	if err := article.RenderText(&renderedText); err != nil {
		return "", "", fmt.Errorf("render readability text: %w", err)
	}
	return CleanText(renderedText.String()), CleanText(article.Excerpt()), nil
}

func bestExtractedText(text string, excerpt string, title string) (string, error) {
	if text == "" {
		text = excerpt
	}
	if text == "" {
		text = strings.TrimSpace(title)
	}
	if text == "" {
		return "", fmt.Errorf("reader extracted empty content")
	}

	return text, nil
}

// CleanText normalizes line endings and collapses extra in-line whitespace.
func CleanText(raw string) string {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")

	lines := strings.Split(normalized, "\n")
	paragraphs := make([]string, 0, len(lines))
	for _, line := range lines {
		clean := strings.Join(strings.Fields(strings.TrimSpace(line)), " ")
		if clean == "" {
			continue
		}
		paragraphs = append(paragraphs, clean)
	}

	return strings.TrimSpace(strings.Join(paragraphs, "\n\n"))
}

// TruncateText clips text to maxChars runes and appends a single ellipsis rune when truncated.
func TruncateText(raw string, maxChars int) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}
	if maxChars <= 0 {
		return trimmed, false
	}

	runes := []rune(trimmed)
	if len(runes) <= maxChars {
		return trimmed, false
	}
	if maxChars == 1 {
		return "…", true
	}

	clipped := strings.TrimSpace(string(runes[:maxChars-1]))
	if clipped == "" {
		return "…", true
	}

	return clipped + "…", true
}
