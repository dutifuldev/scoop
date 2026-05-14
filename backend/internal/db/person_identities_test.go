package db

import (
	"testing"
	"time"
)

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

func TestPersonIdentityUpsertUpdates(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	oldHandle := "old"
	newHandle := "new"
	parsed := ParsedIdentityRef{
		Handle:      &newHandle,
		IdentityRef: "id://github/handle/new",
	}
	updates := personIdentityUpsertUpdates(PersonIdentity{
		Handle:      &oldHandle,
		IdentityRef: "id://github/handle/old",
	}, parsed, now)

	if updates["handle"] != "new" {
		t.Fatalf("handle update = %#v", updates["handle"])
	}
	if updates["identity_ref"] != "id://github/handle/new" {
		t.Fatalf("identity_ref update = %#v", updates["identity_ref"])
	}
	if _, ok := updates["updated_at"]; !ok {
		t.Fatalf("updated_at missing from updates")
	}

	same := personIdentityUpsertUpdates(PersonIdentity{
		Handle:      &newHandle,
		IdentityRef: "id://github/handle/new",
	}, parsed, now)
	if len(same) != 1 {
		t.Fatalf("same identity updates = %#v, want updated_at only", same)
	}
}

func TestPersonIdentityDisplayValuePriority(t *testing.T) {
	t.Parallel()

	handle := "octocat"
	providerUserID := "42"
	if got := personIdentityDisplayValue(PersonIdentityRecord{Handle: &handle, ProviderUserID: &providerUserID, IdentityRef: "id://github/id/42"}); got != "octocat" {
		t.Fatalf("display with handle = %q, want octocat", got)
	}
	if got := personIdentityDisplayValue(PersonIdentityRecord{ProviderUserID: &providerUserID, IdentityRef: "id://github/id/42"}); got != "42" {
		t.Fatalf("display with provider id = %q, want 42", got)
	}
	if got := personIdentityDisplayValue(PersonIdentityRecord{IdentityRef: "id://github/handle/octocat"}); got != "id://github/handle/octocat" {
		t.Fatalf("display fallback = %q, want identity ref", got)
	}
}

func TestSortPersonIdentityRecords(t *testing.T) {
	t.Parallel()

	githubHandle := "octocat"
	discordID := "42"
	records := []PersonIdentityRecord{
		{Provider: "github", Handle: &githubHandle, IdentityRef: "id://github/handle/octocat"},
		{Provider: "discord", ProviderUserID: &discordID, IdentityRef: "id://discord/id/42"},
		{Provider: "discord", IdentityRef: "id://discord/handle/alice"},
	}
	SortPersonIdentityRecords(records)
	got := []string{records[0].IdentityRef, records[1].IdentityRef, records[2].IdentityRef}
	want := []string{"id://discord/id/42", "id://discord/handle/alice", "id://github/handle/octocat"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sorted identities = %#v, want %#v", got, want)
		}
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
