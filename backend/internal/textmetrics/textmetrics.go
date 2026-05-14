package textmetrics

import (
	"hash/fnv"
	"strings"
	"unicode"

	textnormalize "horse.fit/scoop/internal/normalize"
)

func CountTokens(text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	return len(strings.Fields(text))
}

func Simhash64(text string) (uint64, bool) {
	tokens := Tokenize(text)
	if len(tokens) == 0 {
		return 0, false
	}

	var bitWeights [64]int
	for _, token := range tokens {
		h := hashToken64(token)
		for bit := range 64 {
			mask := uint64(1) << bit
			if h&mask != 0 {
				bitWeights[bit]++
			} else {
				bitWeights[bit]--
			}
		}
	}

	var result uint64
	for bit := range 64 {
		if bitWeights[bit] > 0 {
			result |= uint64(1) << bit
		}
	}
	return result, true
}

func Tokenize(text string) []string {
	normalized := textnormalize.Text(text)
	if normalized == "" {
		return nil
	}

	parts := strings.FieldsFunc(normalized, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		tokens = append(tokens, part)
	}
	return tokens
}

func TitleTokenJaccard(left, right string) float64 {
	return jaccard(TokenSet(left), TokenSet(right))
}

func TokenSet(text string) map[string]struct{} {
	tokens := Tokenize(text)
	if len(tokens) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		set[token] = struct{}{}
	}
	return set
}

func TitleTrigramJaccard(left, right string) float64 {
	return jaccard(TrigramSet(left), TrigramSet(right))
}

func TrigramSet(text string) map[string]struct{} {
	normalized := textnormalize.Text(text)
	if normalized == "" {
		return nil
	}

	runes := []rune(normalized)
	if len(runes) < 3 {
		return map[string]struct{}{string(runes): {}}
	}

	set := make(map[string]struct{}, len(runes)-2)
	for i := 0; i <= len(runes)-3; i++ {
		set[string(runes[i:i+3])] = struct{}{}
	}
	return set
}

func jaccard(leftSet, rightSet map[string]struct{}) float64 {
	if len(leftSet) == 0 || len(rightSet) == 0 {
		return 0
	}

	intersection := 0
	for token := range leftSet {
		if _, ok := rightSet[token]; ok {
			intersection++
		}
	}
	if intersection == 0 {
		return 0
	}

	union := len(leftSet) + len(rightSet) - intersection
	if union <= 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func hashToken64(token string) uint64 {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(token))
	return hasher.Sum64()
}
