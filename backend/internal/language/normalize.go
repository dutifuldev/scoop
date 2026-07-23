package language

import "strings"

// NormalizeTag normalizes a language tag to lowercase and "-" separators.
// Returns an empty string when the value is blank or contains invalid characters.
func NormalizeTag(raw string) string {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return ""
	}

	trimmed = strings.ReplaceAll(trimmed, "_", "-")
	parts := strings.Split(trimmed, "-")
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !isAlphaLower(part) {
			return ""
		}
		normalized = append(normalized, part)
	}

	if len(normalized) == 0 {
		return ""
	}
	return strings.Join(normalized, "-")
}

// NormalizeCode returns the primary language subtag (for example, "en" from "en-US").
func NormalizeCode(raw string) string {
	tag := NormalizeTag(raw)
	if tag == "" {
		return ""
	}
	primary, _, _ := strings.Cut(tag, "-")
	return primary
}

func isAlphaLower(value string) bool {
	for _, r := range value {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

// mutate4go-manifest-begin
// {"version":1,"tested_at":"2026-07-23T14:04:30+08:00","module_hash":"51a646dff95e66b12567f0205019e660eabd282342edf9160dad81bdefd32a31","functions":[{"id":"func/NormalizeTag","name":"NormalizeTag","line":7,"end_line":31,"hash":"314d77634ca4bcc74b69c9aa12af669f157f8ad83d111e15f6c4ea29cc59990d"},{"id":"func/NormalizeCode","name":"NormalizeCode","line":34,"end_line":41,"hash":"a65a49e4bb3fa6e996a9e136ef303e76112ab2ebf2650bee7641d35a9f87a365"},{"id":"func/isAlphaLower","name":"isAlphaLower","line":43,"end_line":50,"hash":"01b03996e8f65c7ba5383b9479781f218cc7160e70e899cc2ba121c663bd1d45"}]}
// mutate4go-manifest-end
