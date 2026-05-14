package textmetrics

import "testing"

func TestCountTokensNormalizesWhitespace(t *testing.T) {
	t.Parallel()

	if got := CountTokens("  alpha\tbeta\n gamma  "); got != 3 {
		t.Fatalf("CountTokens() = %d, want 3", got)
	}
	if got := CountTokens(" \n "); got != 0 {
		t.Fatalf("blank CountTokens() = %d, want 0", got)
	}
}

func TestSimhash64AndTokenizeUseNormalizedWords(t *testing.T) {
	t.Parallel()

	tokens := Tokenize("OpenClaw, OpenClaw! setup-v2")
	if len(tokens) != 4 || tokens[0] != "openclaw" || tokens[3] != "v2" {
		t.Fatalf("Tokenize() = %#v", tokens)
	}
	if _, ok := Simhash64("  "); ok {
		t.Fatalf("blank Simhash64() ok = true, want false")
	}
	left, ok := Simhash64("OpenClaw setup guide")
	if !ok {
		t.Fatalf("Simhash64(left) ok = false")
	}
	right, ok := Simhash64("OpenClaw setup guide")
	if !ok {
		t.Fatalf("Simhash64(right) ok = false")
	}
	if left != right {
		t.Fatalf("same text simhash differs: %d != %d", left, right)
	}
}

func TestTitleJaccardScores(t *testing.T) {
	t.Parallel()

	tokenScore := TitleTokenJaccard("Acme launches orbital drone", "Acme launches drone platform")
	if tokenScore <= 0 || tokenScore >= 1 {
		t.Fatalf("TitleTokenJaccard() = %f, want partial overlap", tokenScore)
	}
	if got := TitleTokenJaccard("", "Acme"); got != 0 {
		t.Fatalf("empty TitleTokenJaccard() = %f, want 0", got)
	}

	trigramScore := TitleTrigramJaccard("OpenAI releases model", "OpenAI released model")
	if trigramScore <= 0 || trigramScore >= 1 {
		t.Fatalf("TitleTrigramJaccard() = %f, want partial overlap", trigramScore)
	}
	if got := TrigramSet("AI"); len(got) != 1 {
		t.Fatalf("short TrigramSet() = %#v, want one gram", got)
	}
}
