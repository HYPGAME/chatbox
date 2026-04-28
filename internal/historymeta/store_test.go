package historymeta

import (
	"testing"
	"time"
)

func TestOpenOrCreateRoomAuthorizationCreatesJoinedAt(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	now := time.Date(2026, 4, 20, 20, 0, 0, 0, time.UTC)

	record, err := OpenOrCreateRoomAuthorization(baseDir, "room:abc", "identity-1", func() time.Time {
		return now
	})
	if err != nil {
		t.Fatalf("OpenOrCreateRoomAuthorization returned error: %v", err)
	}
	if record.RoomKey != "room:abc" {
		t.Fatalf("expected room key %q, got %q", "room:abc", record.RoomKey)
	}
	if record.IdentityID != "identity-1" {
		t.Fatalf("expected identity id %q, got %q", "identity-1", record.IdentityID)
	}
	if !record.JoinedAt.Equal(now) {
		t.Fatalf("expected joined_at %v, got %v", now, record.JoinedAt)
	}
}

func TestOpenOrCreateRoomAuthorizationPreservesExistingJoinedAt(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	first := time.Date(2026, 4, 20, 20, 0, 0, 0, time.UTC)
	second := first.Add(2 * time.Hour)

	initial, err := OpenOrCreateRoomAuthorization(baseDir, "room:abc", "identity-1", func() time.Time {
		return first
	})
	if err != nil {
		t.Fatalf("first OpenOrCreateRoomAuthorization returned error: %v", err)
	}
	reopened, err := OpenOrCreateRoomAuthorization(baseDir, "room:abc", "identity-1", func() time.Time {
		return second
	})
	if err != nil {
		t.Fatalf("second OpenOrCreateRoomAuthorization returned error: %v", err)
	}
	if !reopened.JoinedAt.Equal(initial.JoinedAt) {
		t.Fatalf("expected joined_at %v to be preserved, got %v", initial.JoinedAt, reopened.JoinedAt)
	}
}

func TestOpenOrCreateRoomAuthorizationSeparatesIdentities(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	first := time.Date(2026, 4, 20, 20, 0, 0, 0, time.UTC)
	second := first.Add(time.Minute)

	recordA, err := OpenOrCreateRoomAuthorization(baseDir, "room:abc", "identity-a", func() time.Time {
		return first
	})
	if err != nil {
		t.Fatalf("OpenOrCreateRoomAuthorization for identity-a returned error: %v", err)
	}
	recordB, err := OpenOrCreateRoomAuthorization(baseDir, "room:abc", "identity-b", func() time.Time {
		return second
	})
	if err != nil {
		t.Fatalf("OpenOrCreateRoomAuthorization for identity-b returned error: %v", err)
	}
	if !recordA.JoinedAt.Equal(first) {
		t.Fatalf("expected identity-a joined_at %v, got %v", first, recordA.JoinedAt)
	}
	if !recordB.JoinedAt.Equal(second) {
		t.Fatalf("expected identity-b joined_at %v, got %v", second, recordB.JoinedAt)
	}
}

func TestOpenOrCreateFirstSeenRecordPreservesExistingTimestamp(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	first := time.Date(2026, 4, 20, 20, 0, 0, 0, time.UTC)
	second := first.Add(24 * time.Hour)

	created, err := OpenOrCreateFirstSeenRecord(baseDir, "room:abc", "identity-1", func() time.Time {
		return first
	})
	if err != nil {
		t.Fatalf("OpenOrCreateFirstSeenRecord returned error: %v", err)
	}
	reopened, err := OpenOrCreateFirstSeenRecord(baseDir, "room:abc", "identity-1", func() time.Time {
		return second
	})
	if err != nil {
		t.Fatalf("OpenOrCreateFirstSeenRecord returned error: %v", err)
	}
	if !reopened.JoinedAt.Equal(created.JoinedAt) {
		t.Fatalf("expected first-seen timestamp to persist, got %v vs %v", created.JoinedAt, reopened.JoinedAt)
	}
}

func TestOpenOrCreateRoomAuthorizationAliasesFirstSeenRecord(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	now := time.Date(2026, 4, 20, 20, 0, 0, 0, time.UTC)

	record, err := OpenOrCreateRoomAuthorization(baseDir, "room:abc", "identity-1", func() time.Time {
		return now
	})
	if err != nil {
		t.Fatalf("OpenOrCreateRoomAuthorization returned error: %v", err)
	}
	if !record.JoinedAt.Equal(now) {
		t.Fatalf("expected alias to keep joined_at %v, got %v", now, record.JoinedAt)
	}
}
