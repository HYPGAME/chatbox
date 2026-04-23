package attachment

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"
)

func TestServiceUploadBindDownloadDelete(t *testing.T) {
	psk := bytes.Repeat([]byte{9}, 32)
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore returned error: %v", err)
	}

	svc := NewService(store, psk, time.Now)
	server := httptest.NewServer(svc.Handler())
	defer server.Close()

	cacheDir := t.TempDir()
	client := Client{
		BaseURL:    server.URL,
		PSK:        psk,
		HTTPClient: server.Client(),
		CacheDir:   cacheDir,
		OpenFile: func(string) error {
			return nil
		},
	}

	upload, err := client.UploadBytes(context.Background(), UploadRequest{
		RoomKey:       "join:10.77.1.4:7331",
		OwnerName:     "alice",
		OwnerIdentity: "id-alice",
		FileName:      "cat.gif",
		Kind:          KindImage,
		Body:          []byte("gif89a"),
	})
	if err != nil {
		t.Fatalf("UploadBytes returned error: %v", err)
	}
	if upload.ID == "" {
		t.Fatal("expected upload id to be set")
	}

	if err := client.BindMessage(context.Background(), upload.ID, "msg-1"); err != nil {
		t.Fatalf("BindMessage returned error: %v", err)
	}

	meta, err := client.FetchMeta(context.Background(), upload.ID)
	if err != nil {
		t.Fatalf("FetchMeta returned error: %v", err)
	}
	if meta.MessageID != "msg-1" {
		t.Fatalf("expected message id %q, got %#v", "msg-1", meta)
	}

	got, err := client.DownloadBytes(context.Background(), upload.ID)
	if err != nil {
		t.Fatalf("DownloadBytes returned error: %v", err)
	}
	if string(got) != "gif89a" {
		t.Fatalf("expected downloaded bytes %q, got %q", "gif89a", string(got))
	}

	if err := client.Delete(context.Background(), upload.ID); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if _, err := store.Lookup(upload.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected record removal after delete, got %v", err)
	}
}

func TestServiceRejectsBadSignature(t *testing.T) {
	psk := bytes.Repeat([]byte{9}, 32)
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore returned error: %v", err)
	}

	svc := NewService(store, psk, time.Now)
	server := httptest.NewServer(svc.Handler())
	defer server.Close()

	client := Client{
		BaseURL:    server.URL,
		PSK:        bytes.Repeat([]byte{3}, 32),
		HTTPClient: server.Client(),
		CacheDir:   t.TempDir(),
	}

	if _, err := client.UploadBytes(context.Background(), UploadRequest{
		RoomKey:  "join:10.77.1.4:7331",
		FileName: "bad.txt",
		Kind:     KindFile,
		Body:     []byte("nope"),
	}); err == nil {
		t.Fatal("expected upload with wrong psk to fail")
	}
}
