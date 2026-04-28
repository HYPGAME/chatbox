package hosthistory

import (
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"chatbox/internal/transcript"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/sys/unix"
)

const (
	fileMagic      = "CBR1"
	fileName       = "host-history.cbh"
	retentionLimit = 30 * 24 * time.Hour
)

type Window struct {
	Records []transcript.Record
	Revokes []transcript.RevokeRecord
}

type Store struct {
	path string
	aead cipherState
}

type cipherState interface {
	NonceSize() int
	Seal(dst, nonce, plaintext, additionalData []byte) []byte
	Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error)
}

type fileEvent struct {
	Type      string                  `json:"type"`
	RoomKey   string                  `json:"room_key"`
	Ingested  time.Time               `json:"ingested_at"`
	ExpiresAt time.Time               `json:"expires_at"`
	Record    transcript.Record       `json:"record,omitempty"`
	Revoke    transcript.RevokeRecord `json:"revoke,omitempty"`
}

func OpenStore(baseDir string, psk []byte) (*Store, error) {
	if len(psk) != 32 {
		return nil, errors.New("host history store requires a 32-byte PSK")
	}
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return nil, fmt.Errorf("create host history directory: %w", err)
	}

	key, err := hkdf.Key(sha256.New, psk, nil, "chatbox host history key", chacha20poly1305.KeySize)
	if err != nil {
		return nil, fmt.Errorf("derive host history key: %w", err)
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("create host history cipher: %w", err)
	}

	store := &Store{
		path: filepath.Join(baseDir, fileName),
		aead: aead,
	}
	if err := store.ensureInitialized(); err != nil {
		return nil, err
	}
	return store, nil
}

func DefaultBaseDir() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(root, "chatbox", "hosthistory"), nil
}

func (s *Store) AppendMessage(roomKey string, record transcript.Record, now time.Time) error {
	return s.appendEvent(fileEvent{
		Type:      "message",
		RoomKey:   roomKey,
		Ingested:  now.UTC(),
		ExpiresAt: now.UTC().Add(retentionLimit),
		Record:    record,
	})
}

func (s *Store) AppendRevoke(roomKey string, revoke transcript.RevokeRecord, now time.Time) error {
	return s.appendEvent(fileEvent{
		Type:      "revoke",
		RoomKey:   roomKey,
		Ingested:  now.UTC(),
		ExpiresAt: now.UTC().Add(retentionLimit),
		Revoke:    revoke,
	})
}

func (s *Store) LoadWindow(roomKey string, since, now time.Time) (Window, error) {
	events, err := s.loadAll()
	if err != nil {
		return Window{}, err
	}

	nowUTC := now.UTC()
	cutoff := nowUTC.Add(-retentionLimit)
	window := Window{
		Records: make([]transcript.Record, 0, len(events)),
		Revokes: make([]transcript.RevokeRecord, 0, len(events)),
	}

	for _, event := range events {
		if event.RoomKey != roomKey || !event.ExpiresAt.After(nowUTC) {
			continue
		}
		switch event.Type {
		case "message":
			if event.Record.At.Before(since) || event.Record.At.Before(cutoff) {
				continue
			}
			window.Records = append(window.Records, event.Record)
		case "revoke":
			if event.Revoke.At.Before(since) || event.Revoke.At.Before(cutoff) {
				continue
			}
			window.Revokes = append(window.Revokes, event.Revoke)
		default:
			return Window{}, fmt.Errorf("unknown host history event type %q", event.Type)
		}
	}

	return window, nil
}

func (s *Store) CleanupExpired(now time.Time) (int, error) {
	events, err := s.loadAll()
	if err != nil {
		return 0, err
	}

	nowUTC := now.UTC()
	kept := make([]fileEvent, 0, len(events))
	removed := 0
	for _, event := range events {
		if !event.ExpiresAt.After(nowUTC) {
			removed++
			continue
		}
		kept = append(kept, event)
	}

	if err := s.rewriteAll(kept); err != nil {
		return 0, err
	}
	return removed, nil
}

func (s *Store) ensureInitialized() error {
	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open host history file: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat host history file: %w", err)
	}
	if info.Size() == 0 {
		if _, err := file.Write([]byte(fileMagic)); err != nil {
			return fmt.Errorf("write host history header: %w", err)
		}
		return nil
	}
	return verifyMagic(file)
}

func (s *Store) appendEvent(event fileEvent) error {
	file, err := os.OpenFile(s.path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open host history for append: %w", err)
	}
	defer file.Close()
	if err := lockExclusive(file); err != nil {
		return fmt.Errorf("lock host history for append: %w", err)
	}
	defer unlockFile(file)

	if err := s.writeEvent(file, event); err != nil {
		return err
	}
	return nil
}

func (s *Store) loadAll() ([]fileEvent, error) {
	file, err := os.Open(s.path)
	if err != nil {
		return nil, fmt.Errorf("open host history: %w", err)
	}
	defer file.Close()
	if err := lockShared(file); err != nil {
		return nil, fmt.Errorf("lock host history for read: %w", err)
	}
	defer unlockFile(file)

	if err := verifyMagic(file); err != nil {
		return nil, err
	}

	events := make([]fileEvent, 0, 64)
	for {
		event, err := s.readEvent(file)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func (s *Store) rewriteAll(events []fileEvent) error {
	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open host history for rewrite: %w", err)
	}
	defer file.Close()
	if err := lockExclusive(file); err != nil {
		return fmt.Errorf("lock host history for rewrite: %w", err)
	}
	defer unlockFile(file)

	if err := file.Truncate(0); err != nil {
		return fmt.Errorf("truncate host history: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek host history: %w", err)
	}
	if _, err := file.Write([]byte(fileMagic)); err != nil {
		return fmt.Errorf("rewrite host history header: %w", err)
	}
	for _, event := range events {
		if err := s.writeEvent(file, event); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) writeEvent(w io.Writer, event fileEvent) error {
	plaintext, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal host history event: %w", err)
	}

	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("generate host history nonce: %w", err)
	}

	ciphertext := s.aead.Seal(nil, nonce, plaintext, []byte(fileMagic))
	frameLength := uint32(len(nonce) + len(ciphertext))
	lengthPrefix := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthPrefix, frameLength)

	if _, err := w.Write(lengthPrefix); err != nil {
		return fmt.Errorf("write host history frame length: %w", err)
	}
	if _, err := w.Write(nonce); err != nil {
		return fmt.Errorf("write host history nonce: %w", err)
	}
	if _, err := w.Write(ciphertext); err != nil {
		return fmt.Errorf("write host history ciphertext: %w", err)
	}
	return nil
}

func (s *Store) readEvent(r io.Reader) (fileEvent, error) {
	var lengthPrefix [4]byte
	if _, err := io.ReadFull(r, lengthPrefix[:]); err != nil {
		return fileEvent{}, err
	}

	frameLength := binary.BigEndian.Uint32(lengthPrefix[:])
	if frameLength < uint32(s.aead.NonceSize()+1) {
		return fileEvent{}, errors.New("invalid host history frame length")
	}

	frame := make([]byte, frameLength)
	if _, err := io.ReadFull(r, frame); err != nil {
		return fileEvent{}, fmt.Errorf("read host history frame: %w", err)
	}

	nonce := frame[:s.aead.NonceSize()]
	ciphertext := frame[s.aead.NonceSize():]
	plaintext, err := s.aead.Open(nil, nonce, ciphertext, []byte(fileMagic))
	if err != nil {
		return fileEvent{}, fmt.Errorf("decrypt host history frame: %w", err)
	}

	var event fileEvent
	if err := json.Unmarshal(plaintext, &event); err != nil {
		return fileEvent{}, fmt.Errorf("unmarshal host history event: %w", err)
	}
	return event, nil
}

func verifyMagic(file *os.File) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek host history header: %w", err)
	}

	header := make([]byte, len(fileMagic))
	if _, err := io.ReadFull(file, header); err != nil {
		return fmt.Errorf("read host history header: %w", err)
	}
	if string(header) != fileMagic {
		return errors.New("invalid host history file header")
	}
	return nil
}

func lockShared(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_SH)
}

func lockExclusive(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_EX)
}

func unlockFile(file *os.File) {
	_ = unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
