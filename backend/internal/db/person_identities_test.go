package db

import "testing"

func TestParseIdentityRef(t *testing.T) {
	tests := []struct {
		name           string
		raw            string
		provider       string
		providerUserID *string
		handle         *string
		canonical      string
	}{
		{
			name:           "stable id with handle",
			raw:            "id://Discord/id/123456789012345678?handle=@Alice",
			provider:       "discord",
			providerUserID: stringPtr("123456789012345678"),
			handle:         stringPtr("alice"),
			canonical:      "id://discord/id/123456789012345678?handle=alice",
		},
		{
			name:      "handle",
			raw:       "id://x/handle/@Alice_AI",
			provider:  "x",
			handle:    stringPtr("alice_ai"),
			canonical: "id://x/handle/alice_ai",
		},
		{
			name:           "escaped id",
			raw:            "id://github/id/user%2F42?handle=octocat",
			provider:       "github",
			providerUserID: stringPtr("user/42"),
			handle:         stringPtr("octocat"),
			canonical:      "id://github/id/user%2F42?handle=octocat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseIdentityRef(tt.raw)
			if err != nil {
				t.Fatalf("ParseIdentityRef() error = %v", err)
			}
			if got.Provider != tt.provider {
				t.Fatalf("provider = %q, want %q", got.Provider, tt.provider)
			}
			assertStringPtr(t, "provider user id", got.ProviderUserID, tt.providerUserID)
			assertStringPtr(t, "handle", got.Handle, tt.handle)
			if got.IdentityRef != tt.canonical {
				t.Fatalf("identity ref = %q, want %q", got.IdentityRef, tt.canonical)
			}
		})
	}
}

func TestParseIdentityRefRejectsInvalidRefs(t *testing.T) {
	for _, raw := range []string{
		"",
		"https://x.com/alice",
		"id://X Upper/handle/alice",
		"id://x/user/alice",
		"id://x/handle/",
		"id://x/handle/al ice",
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := ParseIdentityRef(raw); err == nil {
				t.Fatalf("ParseIdentityRef(%q) returned nil error", raw)
			}
		})
	}
}

func assertStringPtr(t *testing.T, label string, got *string, want *string) {
	t.Helper()
	if got == nil || want == nil {
		if got != want {
			t.Fatalf("%s = %v, want %v", label, got, want)
		}
		return
	}
	if *got != *want {
		t.Fatalf("%s = %q, want %q", label, *got, *want)
	}
}
