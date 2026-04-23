package attachment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrNotFound = errors.New("attachment not found")

type Record struct {
	ID            string    `json:"id"`
	RoomKey       string    `json:"room_key"`
	OwnerName     string    `json:"owner_name"`
	OwnerIdentity string    `json:"owner_identity,omitempty"`
	FileName      string    `json:"file_name"`
	Kind          string    `json:"kind"`
	Size          int64     `json:"size"`
	DigestHex     string    `json:"digest_hex,omitempty"`
	BlobPath      string    `json:"blob_path"`
	MessageID     string    `json:"message_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	ExpiresAt     time.Time `json:"expires_at"`
}

type PendingRecord = Record

type Store struct {
	root    string
	metaDir string
	blobDir string
}

func OpenStore(root string) (*Store, error) {
	store := &Store{
		root:    filepath.Clean(root),
		metaDir: filepath.Join(root, "meta"),
		blobDir: filepath.Join(root, "blobs"),
	}
	for _, dir := range []string{store.metaDir, store.blobDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create attachment store dir %q: %w", dir, err)
		}
	}
	return store, nil
}

func (s *Store) BlobPath(id string) string {
	return filepath.Join(s.blobDir, strings.TrimSpace(id)+".bin")
}

func (s *Store) CreatePending(record PendingRecord) (Record, error) {
	record.ID = strings.TrimSpace(record.ID)
	if record.ID == "" {
		return Record{}, errors.New("attachment id is required")
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}
	record.BlobPath = s.normalizeBlobPath(record.BlobPath)
	if err := s.writeRecord(record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (s *Store) BindMessage(id, messageID string) error {
	record, err := s.Lookup(id)
	if err != nil {
		return err
	}
	record.MessageID = strings.TrimSpace(messageID)
	return s.writeRecord(record)
}

func (s *Store) Lookup(id string) (Record, error) {
	payload, err := os.ReadFile(s.metaPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("read attachment record: %w", err)
	}

	var record Record
	if err := json.Unmarshal(payload, &record); err != nil {
		return Record{}, fmt.Errorf("unmarshal attachment record: %w", err)
	}
	record.BlobPath = s.normalizeBlobPath(record.BlobPath)
	return record, nil
}

func (s *Store) LookupByMessageID(messageID string) (Record, error) {
	messageID = strings.TrimSpace(messageID)
	entries, err := os.ReadDir(s.metaDir)
	if err != nil {
		return Record{}, fmt.Errorf("read attachment meta dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		record, err := s.Lookup(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return Record{}, err
		}
		if record.MessageID == messageID {
			return record, nil
		}
	}
	return Record{}, ErrNotFound
}

func (s *Store) Delete(id string) error {
	record, err := s.Lookup(id)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	if err == nil {
		_ = os.Remove(record.BlobPath)
	}
	_ = os.Remove(s.metaPath(id))
	return nil
}

func (s *Store) DeleteByMessageID(messageID string) error {
	record, err := s.LookupByMessageID(messageID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}
	return s.Delete(record.ID)
}

func (s *Store) CleanupExpired(ctx context.Context, now time.Time) (int, error) {
	entries, err := os.ReadDir(s.metaDir)
	if err != nil {
		return 0, fmt.Errorf("read attachment meta dir: %w", err)
	}

	var removed int
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return removed, ctx.Err()
		default:
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		record, err := s.Lookup(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return removed, err
		}
		if !record.ExpiresAt.IsZero() && !record.ExpiresAt.After(now) {
			if err := s.Delete(record.ID); err != nil {
				return removed, err
			}
			removed++
		}
	}

	return removed, nil
}

func (s *Store) writeRecord(record Record) error {
	payload, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal attachment record: %w", err)
	}
	if err := os.WriteFile(s.metaPath(record.ID), payload, 0o600); err != nil {
		return fmt.Errorf("write attachment record: %w", err)
	}
	return nil
}

func (s *Store) metaPath(id string) string {
	return filepath.Join(s.metaDir, strings.TrimSpace(id)+".json")
}

func (s *Store) normalizeBlobPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(s.root, path)
}
