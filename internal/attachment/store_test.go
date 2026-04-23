package attachment

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreCreateBindLookupDelete(t *testing.T) {
	root := t.TempDir()
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore returned error: %v", err)
	}

	blobPath := filepath.Join(root, "blobs", "att_123.bin")
	if err := os.WriteFile(blobPath, []byte("ciphertext"), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	record, err := store.CreatePending(PendingRecord{
		ID:            "att_123",
		RoomKey:       "join:10.77.1.4:7331",
		OwnerName:     "alice",
		OwnerIdentity: "id-alice",
		FileName:      "cat.gif",
		Kind:          KindImage,
		Size:          123,
		ExpiresAt:     time.Now().Add(7 * 24 * time.Hour),
		BlobPath:      "blobs/att_123.bin",
	})
	if err != nil {
		t.Fatalf("CreatePending returned error: %v", err)
	}
	if err := store.BindMessage(record.ID, "msg-1"); err != nil {
		t.Fatalf("BindMessage returned error: %v", err)
	}
	byMessage, err := store.LookupByMessageID("msg-1")
	if err != nil {
		t.Fatalf("LookupByMessageID returned error: %v", err)
	}
	if byMessage.ID != record.ID {
		t.Fatalf("expected attachment id %q, got %#v", record.ID, byMessage)
	}
	if err := store.Delete(record.ID); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if _, err := store.Lookup(record.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
	if _, err := os.Stat(blobPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected blob file to be removed, got %v", err)
	}
}

func TestStoreCleanupExpired(t *testing.T) {
	root := t.TempDir()
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore returned error: %v", err)
	}

	expiredBlobPath := filepath.Join(root, "blobs", "att_old.bin")
	if err := os.WriteFile(expiredBlobPath, []byte("ciphertext"), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	if _, err := store.CreatePending(PendingRecord{
		ID:        "att_old",
		RoomKey:   "join:10.77.1.4:7331",
		FileName:  "old.txt",
		Kind:      KindFile,
		Size:      5,
		ExpiresAt: time.Now().Add(-time.Minute),
		BlobPath:  "blobs/att_old.bin",
	}); err != nil {
		t.Fatalf("CreatePending returned error: %v", err)
	}

	if removed, err := store.CleanupExpired(context.Background(), time.Now()); err != nil || removed != 1 {
		t.Fatalf("expected one expired record removed, got removed=%d err=%v", removed, err)
	}
	if _, err := store.Lookup("att_old"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected expired record to be removed, got %v", err)
	}
}
