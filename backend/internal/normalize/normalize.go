package normalize

import (
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
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", ""
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", ""
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", ""
	}

	parsed.Scheme = strings.ToLower(parsed.Scheme)
	hostname := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	parsed.Host = hostname
	if port != "" {
		defaultPort := (parsed.Scheme == "http" && port == "80") || (parsed.Scheme == "https" && port == "443")
		if !defaultPort {
			parsed.Host = hostname + ":" + port
		}
	}

	parsed.Fragment = ""
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

	q := parsed.Query()
	for key := range q {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "utm_") {
			q.Del(key)
			continue
		}
		if _, ok := trackingQueryKeys[lower]; ok {
			q.Del(key)
		}
	}
	parsed.RawQuery = sortedQuery(q)

	return parsed.String(), parsed.Hostname()
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
