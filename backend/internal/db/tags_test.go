package db

import "testing"

func TestValidateTagSlugCanonicalFormat(t *testing.T) {
	valid := []string{
		"openclaw",
		"metal-news",
		"tag-123",
		"ABC-123",
		"  Needs-Review  ",
	}
	for _, raw := range valid {
		if err := ValidateTagSlug(NormalizeTagSlug(raw)); err != nil {
			t.Fatalf("expected %q to be valid: %v", raw, err)
		}
	}

	invalid := []string{
		"",
		"-openclaw",
		"openclaw-",
		"open--claw",
		"open_claw",
		"open claw",
		"open.claw",
		"this-tag-is-way-too-long-because-it-has-more-than-sixty-four-characters",
	}
	for _, raw := range invalid {
		if err := ValidateTagSlug(NormalizeTagSlug(raw)); err == nil {
			t.Fatalf("expected %q to be invalid", raw)
		}
	}
}
