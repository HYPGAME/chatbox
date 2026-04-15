package transcript

import (
	"bytes"
	"os"
	"testing"
	"time"
)

func TestStoreEncryptsAndReloadsConversationRecords(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	psk := bytes.Repeat([]byte{0x52}, 32)

	store, err := OpenStore(baseDir, "alice", "bob", psk)
	if err != nil {
		t.Fatalf("OpenStore returned error: %v", err)
	}

	record := Record{
		MessageID: "msg-1",
		Direction: DirectionOutgoing,
		From:      "alice",
		Body:      "hello from transcript",
		At:        time.Date(2026, 4, 14, 21, 30, 0, 0, time.UTC),
		Status:    StatusSending,
	}
	if err := store.AppendMessage(record); err != nil {
		t.Fatalf("AppendMessage returned error: %v", err)
	}
	if err := store.UpdateStatus(record.MessageID, StatusSent); err != nil {
		t.Fatalf("UpdateStatus returned error: %v", err)
	}

	raw, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if bytes.Contains(raw, []byte(record.Body)) {
		t.Fatal("expected transcript file to be encrypted, but plaintext body was found")
	}

	reopened, err := OpenStore(baseDir, "alice", "bob", psk)
	if err != nil {
		t.Fatalf("reopening store returned error: %v", err)
	}

	loaded, err := reopened.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 loaded record, got %d", len(loaded))
	}
	if loaded[0].Body != record.Body {
		t.Fatalf("expected body %q, got %q", record.Body, loaded[0].Body)
	}
	if loaded[0].Status != StatusSent {
		t.Fatalf("expected status %q, got %q", StatusSent, loaded[0].Status)
	}
}

func TestConversationFileNameUsesRoomKeyForGroupChat(t *testing.T) {
	t.Parallel()

	psk := bytes.Repeat([]byte{0x52}, 32)

	hostRoom := HostRoomKey("0.0.0.0:7331")
	joinRoom := JoinRoomKey("127.0.0.1:7331")

	if hostRoom == "" || joinRoom == "" {
		t.Fatalf("expected non-empty room keys, got host=%q join=%q", hostRoom, joinRoom)
	}

	hostFile := conversationFileName("alice", hostRoom, psk)
	joinFile := conversationFileName("alice", joinRoom, psk)
	if hostFile == joinFile {
		t.Fatalf("expected host and join room keys to remain distinct, got %q", hostFile)
	}

	sameHostFile := conversationFileName("alice", HostRoomKey("0.0.0.0:7331"), psk)
	if hostFile != sameHostFile {
		t.Fatalf("expected stable file name for identical room keys, got %q vs %q", hostFile, sameHostFile)
	}
}

func TestConversationFileNameStillSeparatesDifferentRooms(t *testing.T) {
	t.Parallel()

	psk := bytes.Repeat([]byte{0x52}, 32)

	roomA := conversationFileName("alice", JoinRoomKey("127.0.0.1:7331"), psk)
	roomB := conversationFileName("alice", JoinRoomKey("127.0.0.1:7444"), psk)
	if roomA == roomB {
		t.Fatalf("expected different room addresses to produce different files, got %q", roomA)
	}
}
