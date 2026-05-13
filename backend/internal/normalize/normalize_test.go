package normalize

import "testing"

func TestURLStripsTrackingAndNormalizes(t *testing.T) {
	t.Parallel()

	canonical, host := URL("https://Example.COM:443/news//path/?utm_source=abc&fbclid=123&b=2&a=1#frag")
	if canonical != "https://example.com/news/path?a=1&b=2" {
		t.Fatalf("unexpected canonical url: %q", canonical)
	}
	if host != "example.com" {
		t.Fatalf("unexpected host: %q", host)
	}
}

func TestURLKeepsNonDefaultPort(t *testing.T) {
	t.Parallel()

	canonical, host := URL("http://Example.COM:8080")
	if canonical != "http://example.com:8080/" {
		t.Fatalf("unexpected canonical url: %q", canonical)
	}
	if host != "example.com" {
		t.Fatalf("unexpected host: %q", host)
	}
}

func TestURLInvalid(t *testing.T) {
	t.Parallel()

	canonical, host := URL("not a url")
	if canonical != "" || host != "" {
		t.Fatalf("expected empty result for invalid URL, got canonical=%q host=%q", canonical, host)
	}
}

func TestText(t *testing.T) {
	t.Parallel()

	got := Text("  Hello\tWORLD\nwith\u0000controls  ")
	if got != "hello world withcontrols" {
		t.Fatalf("unexpected normalized text: %q", got)
	}
}
