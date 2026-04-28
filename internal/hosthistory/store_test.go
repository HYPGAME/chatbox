package hosthistory

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"chatbox/internal/transcript"
)

func TestStoreAppendsEncryptedMessageAndLoadsWindow(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	psk := bytes.Repeat([]byte{0x41}, 32)
	store, err := OpenStore(baseDir, psk)
	if err != nil {
		t.Fatalf("OpenStore returned error: %v", err)
	}

	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	record := transcript.Record{
		MessageID:      "msg-1",
		Direction:      transcript.DirectionIncoming,
		From:           "alice",
		AuthorIdentity: "identity-a",
		Body:           "hello from host history",
		At:             now.Add(-time.Minute),
		Status:         transcript.StatusSent,
	}
	if err := store.AppendMessage("join:127.0.0.1:7331", record, now); err != nil {
		t.Fatalf("AppendMessage returned error: %v", err)
	}

	raw, err := os.ReadFile(store.path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if strings.Contains(string(raw), record.Body) {
		t.Fatal("expected host history file to stay encrypted")
	}

	window, err := store.LoadWindow("join:127.0.0.1:7331", now.Add(-2*time.Minute), now)
	if err != nil {
		t.Fatalf("LoadWindow returned error: %v", err)
	}
	if len(window.Records) != 1 || window.Records[0].MessageID != "msg-1" {
		t.Fatalf("expected one retained record, got %#v", window.Records)
	}
}

func TestStoreKeepsRevokesAndSeparatesRooms(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	psk := bytes.Repeat([]byte{0x42}, 32)
	store, err := OpenStore(baseDir, psk)
	if err != nil {
		t.Fatalf("OpenStore returned error: %v", err)
	}

	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	if err := store.AppendMessage("room-a", transcript.Record{
		MessageID:      "room-a-msg",
		Direction:      transcript.DirectionIncoming,
		From:           "alice",
		AuthorIdentity: "identity-a",
		Body:           "keep me",
		At:             now.Add(-time.Minute),
		Status:         transcript.StatusSent,
	}, now); err != nil {
		t.Fatalf("AppendMessage room-a returned error: %v", err)
	}
	if err := store.AppendMessage("room-b", transcript.Record{
		MessageID:      "room-b-msg",
		Direction:      transcript.DirectionIncoming,
		From:           "bob",
		AuthorIdentity: "identity-b",
		Body:           "hide me",
		At:             now.Add(-time.Minute),
		Status:         transcript.StatusSent,
	}, now); err != nil {
		t.Fatalf("AppendMessage room-b returned error: %v", err)
	}
	if err := store.AppendRevoke("room-a", transcript.RevokeRecord{
		TargetMessageID:  "room-a-msg",
		OperatorIdentity: "identity-a",
		At:               now,
	}, now); err != nil {
		t.Fatalf("AppendRevoke returned error: %v", err)
	}

	window, err := store.LoadWindow("room-a", now.Add(-10*time.Minute), now)
	if err != nil {
		t.Fatalf("LoadWindow returned error: %v", err)
	}
	if len(window.Records) != 1 || window.Records[0].MessageID != "room-a-msg" {
		t.Fatalf("expected room-a message only, got %#v", window.Records)
	}
	if len(window.Revokes) != 1 || window.Revokes[0].TargetMessageID != "room-a-msg" {
		t.Fatalf("expected room-a revoke only, got %#v", window.Revokes)
	}
}

func TestStoreCleanupExpiredDropsOldFrames(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	psk := bytes.Repeat([]byte{0x43}, 32)
	store, err := OpenStore(baseDir, psk)
	if err != nil {
		t.Fatalf("OpenStore returned error: %v", err)
	}

	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	oldAt := now.Add(-31 * 24 * time.Hour)
	if err := store.AppendMessage("room-a", transcript.Record{
		MessageID:      "expired-msg",
		Direction:      transcript.DirectionIncoming,
		From:           "alice",
		AuthorIdentity: "identity-a",
		Body:           "expired",
		At:             oldAt,
		Status:         transcript.StatusSent,
	}, oldAt); err != nil {
		t.Fatalf("AppendMessage returned error: %v", err)
	}
	if err := store.AppendMessage("room-a", transcript.Record{
		MessageID:      "fresh-msg",
		Direction:      transcript.DirectionIncoming,
		From:           "alice",
		AuthorIdentity: "identity-a",
		Body:           "fresh",
		At:             now.Add(-time.Minute),
		Status:         transcript.StatusSent,
	}, now); err != nil {
		t.Fatalf("AppendMessage returned error: %v", err)
	}

	removed, err := store.CleanupExpired(now)
	if err != nil {
		t.Fatalf("CleanupExpired returned error: %v", err)
	}
	if removed != 1 {
		t.Fatalf("expected one expired frame to be removed, got %d", removed)
	}

	window, err := store.LoadWindow("room-a", now.Add(-40*24*time.Hour), now)
	if err != nil {
		t.Fatalf("LoadWindow returned error: %v", err)
	}
	if len(window.Records) != 1 || window.Records[0].MessageID != "fresh-msg" {
		t.Fatalf("expected only fresh message after cleanup, got %#v", window.Records)
	}
}

func TestStoreSupportsDifferentPSKsInSameBaseDir(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	pskA := bytes.Repeat([]byte{0x51}, 32)
	pskB := bytes.Repeat([]byte{0x52}, 32)

	storeA, err := OpenStore(baseDir, pskA)
	if err != nil {
		t.Fatalf("OpenStore A returned error: %v", err)
	}
	storeB, err := OpenStore(baseDir, pskB)
	if err != nil {
		t.Fatalf("OpenStore B returned error: %v", err)
	}
	if storeA.path == storeB.path {
		t.Fatalf("expected distinct host history files per PSK, got %q", storeA.path)
	}

	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	if err := storeA.AppendMessage("room-a", transcript.Record{
		MessageID:      "msg-a",
		Direction:      transcript.DirectionIncoming,
		From:           "alice",
		AuthorIdentity: "identity-a",
		Body:           "from psk a",
		At:             now.Add(-2 * time.Minute),
		Status:         transcript.StatusSent,
	}, now); err != nil {
		t.Fatalf("AppendMessage A returned error: %v", err)
	}
	if err := storeB.AppendMessage("room-b", transcript.Record{
		MessageID:      "msg-b",
		Direction:      transcript.DirectionIncoming,
		From:           "bob",
		AuthorIdentity: "identity-b",
		Body:           "from psk b",
		At:             now.Add(-time.Minute),
		Status:         transcript.StatusSent,
	}, now); err != nil {
		t.Fatalf("AppendMessage B returned error: %v", err)
	}

	windowA, err := storeA.LoadWindow("room-a", now.Add(-10*time.Minute), now)
	if err != nil {
		t.Fatalf("LoadWindow A returned error: %v", err)
	}
	if len(windowA.Records) != 1 || windowA.Records[0].MessageID != "msg-a" {
		t.Fatalf("expected room-a record after mixed PSKs, got %#v", windowA.Records)
	}

	windowB, err := storeB.LoadWindow("room-b", now.Add(-10*time.Minute), now)
	if err != nil {
		t.Fatalf("LoadWindow B returned error: %v", err)
	}
	if len(windowB.Records) != 1 || windowB.Records[0].MessageID != "msg-b" {
		t.Fatalf("expected room-b record after mixed PSKs, got %#v", windowB.Records)
	}
}
