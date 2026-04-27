package keys

import (
	"bytes"
	"strings"
	"testing"
)

func TestDeriveGroupCredentialsNormalizesNameAndBuildsStableRoomKey(t *testing.T) {
	t.Parallel()

	first, err := DeriveGroupCredentials("  team-alpha  ", "abc123")
	if err != nil {
		t.Fatalf("DeriveGroupCredentials returned error: %v", err)
	}
	second, err := DeriveGroupCredentials("team-alpha", "abc123")
	if err != nil {
		t.Fatalf("DeriveGroupCredentials returned error: %v", err)
	}

	if first.GroupName != "team-alpha" {
		t.Fatalf("expected normalized group name %q, got %q", "team-alpha", first.GroupName)
	}
	if len(first.PSK) != 32 {
		t.Fatalf("expected 32-byte PSK, got %d", len(first.PSK))
	}
	if !bytes.Equal(first.PSK, second.PSK) {
		t.Fatal("expected trimmed group names to derive the same PSK")
	}
	if first.RoomKey != second.RoomKey {
		t.Fatalf("expected identical room keys, got %q vs %q", first.RoomKey, second.RoomKey)
	}
	if !strings.HasPrefix(first.RoomKey, "group:team-alpha:") {
		t.Fatalf("expected group room key prefix, got %q", first.RoomKey)
	}
}

func TestDeriveGroupCredentialsChangesWhenPasswordChanges(t *testing.T) {
	t.Parallel()

	first, err := DeriveGroupCredentials("team-alpha", "abc123")
	if err != nil {
		t.Fatalf("DeriveGroupCredentials returned error: %v", err)
	}
	second, err := DeriveGroupCredentials("team-alpha", "abc124")
	if err != nil {
		t.Fatalf("DeriveGroupCredentials returned error: %v", err)
	}

	if bytes.Equal(first.PSK, second.PSK) {
		t.Fatal("expected different passwords to derive different PSKs")
	}
	if first.RoomKey == second.RoomKey {
		t.Fatalf("expected different passwords to produce different room keys, got %q", first.RoomKey)
	}
}

func TestDeriveGroupCredentialsRejectsEmptyInputs(t *testing.T) {
	t.Parallel()

	if _, err := DeriveGroupCredentials("   ", "abc123"); err == nil {
		t.Fatal("expected blank group name to fail")
	}
	if _, err := DeriveGroupCredentials("team-alpha", ""); err == nil {
		t.Fatal("expected blank password to fail")
	}
}
