package db

import (
	"context"
	"testing"
	"time"
)

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

func TestTagMutationsRejectInvalidOptionsBeforeDatabase(t *testing.T) {
	t.Parallel()

	pool := &Pool{}
	badColor := "red"
	ctx := context.Background()
	if _, err := pool.CreateTag(ctx, UpsertTagOptions{Slug: "i0", Color: &badColor}, nowForTagTest()); err == nil || err.Error() != "color must be a #RRGGBB hex value" {
		t.Fatalf("CreateTag() error = %v", err)
	}
	if _, err := pool.UpdateTag(ctx, "i0", UpdateTagOptions{}, nowForTagTest()); err == nil || err.Error() != "at least one update field is required" {
		t.Fatalf("UpdateTag() empty error = %v", err)
	}
	if _, err := pool.UpdateTag(ctx, "i0", UpdateTagOptions{Color: &badColor}, nowForTagTest()); err == nil || err.Error() != "color must be a #RRGGBB hex value" {
		t.Fatalf("UpdateTag() color error = %v", err)
	}
	if err := pool.DeleteTag(ctx, "bad tag"); err == nil {
		t.Fatalf("DeleteTag() error = nil, want validation error")
	}
	if err := pool.AddArticleTag(ctx, "article", "bad tag", nil, nowForTagTest()); err == nil {
		t.Fatalf("AddArticleTag() error = nil, want validation error")
	}
	if err := pool.RemoveArticleTag(ctx, "article", "bad tag", nil); err == nil {
		t.Fatalf("RemoveArticleTag() error = nil, want validation error")
	}
}

func nowForTagTest() time.Time {
	return time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)
}
