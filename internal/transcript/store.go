package transcript

import (
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/sys/unix"
)

const (
	fileMagic = "CBH1"

	DirectionIncoming = "incoming"
	DirectionOutgoing = "outgoing"

	StatusSending = "sending"
	StatusSent    = "sent"
	StatusFailed  = "failed"
)

type Record struct {
	MessageID string    `json:"message_id"`
	Direction string    `json:"direction"`
	From      string    `json:"from"`
	Body      string    `json:"body"`
	At        time.Time `json:"at"`
	Status    string    `json:"status"`
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
	Type      string `json:"type"`
	MessageID string `json:"message_id,omitempty"`
	Status    string `json:"status,omitempty"`
	Record    Record `json:"record,omitempty"`
}

func OpenStore(baseDir, localName, conversationKey string, psk []byte) (*Store, error) {
	if len(psk) != 32 {
		return nil, errors.New("transcript store requires a 32-byte PSK")
	}
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return nil, fmt.Errorf("create transcript directory: %w", err)
	}

	aead, err := newTranscriptCipher(psk)
	if err != nil {
		return nil, err
	}

	store := &Store{
		path: filepath.Join(baseDir, conversationFileName(localName, conversationKey, psk)),
		aead: aead,
	}
	if err := store.ensureInitialized(); err != nil {
		return nil, err
	}
	if err := store.importLegacyDisplayNameTranscripts(baseDir, localName, conversationKey, psk); err != nil {
		return nil, err
	}
	return store, nil
}

func newTranscriptCipher(psk []byte) (cipherState, error) {
	key, err := hkdf.Key(sha256.New, psk, nil, "chatbox transcript key", chacha20poly1305.KeySize)
	if err != nil {
		return nil, fmt.Errorf("derive transcript key: %w", err)
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("create transcript cipher: %w", err)
	}
	return aead, nil
}

func DefaultBaseDir() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(root, "chatbox", "history"), nil
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) Load() ([]Record, error) {
	file, err := os.Open(s.path)
	if err != nil {
		return nil, fmt.Errorf("open transcript: %w", err)
	}
	defer file.Close()
	if err := lockShared(file); err != nil {
		return nil, fmt.Errorf("lock transcript for read: %w", err)
	}
	defer unlockFile(file)

	if err := verifyMagic(file); err != nil {
		return nil, err
	}

	records := make([]Record, 0, 64)
	indexByID := make(map[string]int)

	for {
		event, err := s.readEvent(file)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}

		switch event.Type {
		case "message":
			records = append(records, event.Record)
			if event.Record.MessageID != "" {
				indexByID[event.Record.MessageID] = len(records) - 1
			}
		case "status":
			idx, ok := indexByID[event.MessageID]
			if ok {
				records[idx].Status = event.Status
			}
		default:
			return nil, fmt.Errorf("unknown transcript event type %q", event.Type)
		}
	}

	return records, nil
}

func (s *Store) AppendMessage(record Record) error {
	return s.appendEvent(fileEvent{
		Type:   "message",
		Record: record,
	})
}

func (s *Store) UpdateStatus(messageID, status string) error {
	return s.appendEvent(fileEvent{
		Type:      "status",
		MessageID: messageID,
		Status:    status,
	})
}

func (s *Store) importLegacyDisplayNameTranscripts(baseDir, localName, conversationKey string, psk []byte) error {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return fmt.Errorf("read transcript directory: %w", err)
	}

	currentPath := filepath.Clean(s.path)
	legacySuffix := "--" + sanitizeName(conversationKey) + "--" + pskFingerprint(psk) + ".cbh"
	importedIDs := make(map[string]struct{})
	existing, err := s.Load()
	if err != nil {
		return err
	}
	for _, record := range existing {
		if record.MessageID != "" {
			importedIDs[record.MessageID] = struct{}{}
		}
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		path := filepath.Join(baseDir, name)
		if filepath.Clean(path) == currentPath {
			continue
		}
		if !isLegacyConversationFileName(name, legacySuffix) {
			continue
		}
		legacy := &Store{path: path, aead: s.aead}
		records, err := legacy.Load()
		if err != nil {
			continue
		}
		for _, record := range records {
			if record.MessageID != "" {
				if _, ok := importedIDs[record.MessageID]; ok {
					continue
				}
				importedIDs[record.MessageID] = struct{}{}
			}
			if err := s.AppendMessage(record); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) ensureInitialized() error {
	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open transcript file: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat transcript file: %w", err)
	}
	if info.Size() == 0 {
		if _, err := file.Write([]byte(fileMagic)); err != nil {
			return fmt.Errorf("write transcript header: %w", err)
		}
		return nil
	}
	return verifyMagic(file)
}

func (s *Store) appendEvent(event fileEvent) error {
	file, err := os.OpenFile(s.path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open transcript for append: %w", err)
	}
	defer file.Close()
	if err := lockExclusive(file); err != nil {
		return fmt.Errorf("lock transcript for append: %w", err)
	}
	defer unlockFile(file)

	plaintext, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal transcript event: %w", err)
	}

	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("generate transcript nonce: %w", err)
	}

	ciphertext := s.aead.Seal(nil, nonce, plaintext, []byte(fileMagic))
	frameLength := uint32(len(nonce) + len(ciphertext))
	lengthPrefix := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthPrefix, frameLength)

	if _, err := file.Write(lengthPrefix); err != nil {
		return fmt.Errorf("write transcript frame length: %w", err)
	}
	if _, err := file.Write(nonce); err != nil {
		return fmt.Errorf("write transcript nonce: %w", err)
	}
	if _, err := file.Write(ciphertext); err != nil {
		return fmt.Errorf("write transcript ciphertext: %w", err)
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
		return fileEvent{}, errors.New("invalid transcript frame length")
	}

	frame := make([]byte, frameLength)
	if _, err := io.ReadFull(r, frame); err != nil {
		return fileEvent{}, fmt.Errorf("read transcript frame: %w", err)
	}

	nonce := frame[:s.aead.NonceSize()]
	ciphertext := frame[s.aead.NonceSize():]
	plaintext, err := s.aead.Open(nil, nonce, ciphertext, []byte(fileMagic))
	if err != nil {
		return fileEvent{}, fmt.Errorf("decrypt transcript frame: %w", err)
	}

	var event fileEvent
	if err := json.Unmarshal(plaintext, &event); err != nil {
		return fileEvent{}, fmt.Errorf("unmarshal transcript event: %w", err)
	}
	return event, nil
}

func verifyMagic(file *os.File) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek transcript header: %w", err)
	}

	header := make([]byte, len(fileMagic))
	if _, err := io.ReadFull(file, header); err != nil {
		return fmt.Errorf("read transcript header: %w", err)
	}
	if string(header) != fileMagic {
		return errors.New("invalid transcript file header")
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

func HostRoomKey(listenAddr string) string {
	return "host:" + strings.TrimSpace(listenAddr)
}

func JoinRoomKey(targetAddr string) string {
	return "join:" + strings.TrimSpace(targetAddr)
}

func conversationFileName(localName, conversationKey string, psk []byte) string {
	return strings.Join([]string{sanitizeName(conversationKey), pskFingerprint(psk)}, "--") + ".cbh"
}

func legacyConversationFileName(localName, conversationKey string, psk []byte) string {
	return strings.Join([]string{sanitizeName(localName), sanitizeName(conversationKey), pskFingerprint(psk)}, "--") + ".cbh"
}

func isLegacyConversationFileName(name, legacySuffix string) bool {
	return strings.HasSuffix(name, legacySuffix) && strings.Count(name, "--") >= 2
}

func pskFingerprint(psk []byte) string {
	fingerprintBytes := sha256.Sum256(psk)
	return hex.EncodeToString(fingerprintBytes[:6])
}

func sanitizeName(name string) string {
	if name == "" {
		return "unknown"
	}

	lower := strings.ToLower(strings.TrimSpace(name))
	var builder strings.Builder
	for _, r := range lower {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
		}
	}
	if builder.Len() == 0 {
		return "unknown"
	}
	return builder.String()
}
