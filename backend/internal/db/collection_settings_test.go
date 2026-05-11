package db

import "testing"

func TestDefaultCollectionTranslationMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		collection string
		want       string
	}{
		{collection: "china_news", want: TranslationModeEnabled},
		{collection: "metal_news", want: TranslationModeEnabled},
		{collection: "openclaw", want: TranslationModeDisabled},
		{collection: "ai_news", want: TranslationModeDisabled},
		{collection: "world_news", want: TranslationModeDisabled},
		{collection: "", want: TranslationModeDisabled},
	}

	for _, test := range tests {
		if got := DefaultCollectionTranslationMode(test.collection); got != test.want {
			t.Fatalf("DefaultCollectionTranslationMode(%q) = %q, want %q", test.collection, got, test.want)
		}
	}
}

func TestNormalizeCollectionTranslationMode(t *testing.T) {
	t.Parallel()

	if got := NormalizeCollectionTranslationMode(" enabled "); got != TranslationModeEnabled {
		t.Fatalf("enabled mode normalized to %q", got)
	}
	if got := NormalizeCollectionTranslationMode("manual_only"); got != TranslationModeDisabled {
		t.Fatalf("invalid mode normalized to %q", got)
	}
}
