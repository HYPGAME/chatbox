package historymeta

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Record struct {
	RoomKey    string    `json:"room_key"`
	IdentityID string    `json:"identity_id"`
	JoinedAt   time.Time `json:"joined_at"`
}

func OpenOrCreateRoomAuthorization(baseDir, roomKey, identityID string, now func() time.Time) (Record, error) {
	return OpenOrCreateFirstSeenRecord(baseDir, roomKey, identityID, now)
}

func OpenOrCreateFirstSeenRecord(baseDir, roomKey, identityID string, now func() time.Time) (Record, error) {
	if strings.TrimSpace(roomKey) == "" {
		return Record{}, fmt.Errorf("room authorization: missing room key")
	}
	if strings.TrimSpace(identityID) == "" {
		return Record{}, fmt.Errorf("room authorization: missing identity id")
	}
	if now == nil {
		now = time.Now
	}
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return Record{}, fmt.Errorf("create history metadata directory: %w", err)
	}

	path := filepath.Join(baseDir, fileName(roomKey, identityID))
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		var record Record
		if err := json.Unmarshal(data, &record); err != nil {
			return Record{}, fmt.Errorf("parse room authorization: %w", err)
		}
		if record.RoomKey == "" || record.IdentityID == "" || record.JoinedAt.IsZero() {
			return Record{}, fmt.Errorf("parse room authorization: incomplete record")
		}
		return record, nil
	case os.IsNotExist(err):
	default:
		return Record{}, fmt.Errorf("read room authorization: %w", err)
	}

	record := Record{
		RoomKey:    roomKey,
		IdentityID: identityID,
		JoinedAt:   now().UTC(),
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return Record{}, fmt.Errorf("marshal room authorization: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return Record{}, fmt.Errorf("write room authorization: %w", err)
	}
	return record, nil
}

func DefaultBaseDir() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(root, "chatbox", "historymeta"), nil
}

func fileName(roomKey, identityID string) string {
	safeRoomKey := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_").Replace(strings.TrimSpace(roomKey))
	safeIdentityID := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_").Replace(strings.TrimSpace(identityID))
	return safeRoomKey + "__" + safeIdentityID + ".json"
}
