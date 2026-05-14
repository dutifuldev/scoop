package normalize

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"unicode"
)

var trackingQueryKeys = map[string]struct{}{
	"fbclid":  {},
	"gclid":   {},
	"mc_cid":  {},
	"mc_eid":  {},
	"ref":     {},
	"ref_src": {},
}

func URL(raw string) (canonical string, host string) {
	parsed, err := parseAbsoluteURL(raw)
	if err != nil {
		return "", ""
	}

	normalizeURLSchemeAndHost(parsed)
	normalizeURLPath(parsed)
	normalizeURLQuery(parsed)

	parsed.Fragment = ""
	return parsed.String(), parsed.Hostname()
}

func parseAbsoluteURL(raw string) (*url.URL, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("url is empty")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("url must be absolute")
	}
	return parsed, nil
}

func normalizeURLSchemeAndHost(parsed *url.URL) {
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	hostname := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	parsed.Host = hostname
	if shouldKeepURLPort(parsed.Scheme, port) {
		parsed.Host = hostname + ":" + port
	}
}

func shouldKeepURLPort(scheme string, port string) bool {
	if port == "" {
		return false
	}
	return (scheme != "http" || port != "80") && (scheme != "https" || port != "443")
}

func normalizeURLPath(parsed *url.URL) {
	path := strings.TrimSpace(parsed.EscapedPath())
	if path == "" {
		path = "/"
	}
	path = strings.ReplaceAll(path, "//", "/")
	if strings.HasSuffix(path, "/") && path != "/" {
		path = strings.TrimSuffix(path, "/")
	}
	parsed.Path = path
	parsed.RawPath = ""
}

func normalizeURLQuery(parsed *url.URL) {
	q := parsed.Query()
	for key := range q {
		if isTrackingQueryKey(key) {
			q.Del(key)
		}
	}
	parsed.RawQuery = sortedQuery(q)
}

func isTrackingQueryKey(key string) bool {
	lower := strings.ToLower(key)
	if strings.HasPrefix(lower, "utm_") {
		return true
	}
	_, ok := trackingQueryKeys[lower]
	return ok
}

func Text(input string) string {
	trimmed := strings.TrimSpace(strings.ToLower(input))
	if trimmed == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(trimmed))
	lastSpace := false
	for _, r := range trimmed {
		if unicode.IsSpace(r) {
			if !lastSpace {
				b.WriteRune(' ')
				lastSpace = true
			}
			continue
		}
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
		lastSpace = false
	}
	return strings.TrimSpace(b.String())
}

func sortedQuery(q url.Values) string {
	if len(q) == 0 {
		return ""
	}

	keys := make([]string, 0, len(q))
	for key := range q {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	reordered := url.Values{}
	for _, key := range keys {
		values := append([]string(nil), q[key]...)
		sort.Strings(values)
		for _, value := range values {
			reordered.Add(key, value)
		}
	}
	return reordered.Encode()
}
