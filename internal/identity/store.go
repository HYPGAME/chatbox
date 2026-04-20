package identity

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const fileName = "identity.json"

type Store struct {
	IdentityID string
	Path       string
}

type fileRecord struct {
	IdentityID string `json:"identity_id"`
}

func OpenOrCreate(baseDir string) (Store, error) {
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return Store{}, fmt.Errorf("create identity directory: %w", err)
	}

	path := filepath.Join(baseDir, fileName)
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		var record fileRecord
		if err := json.Unmarshal(data, &record); err != nil {
			return Store{}, fmt.Errorf("parse identity file: %w", err)
		}
		if record.IdentityID == "" {
			return Store{}, fmt.Errorf("parse identity file: missing identity_id")
		}
		return Store{
			IdentityID: record.IdentityID,
			Path:       path,
		}, nil
	case os.IsNotExist(err):
	default:
		return Store{}, fmt.Errorf("read identity file: %w", err)
	}

	record := fileRecord{
		IdentityID: randomIdentityID(),
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return Store{}, fmt.Errorf("marshal identity file: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return Store{}, fmt.Errorf("write identity file: %w", err)
	}
	return Store{
		IdentityID: record.IdentityID,
		Path:       path,
	}, nil
}

func Export(baseDir, outPath string) error {
	store, err := OpenOrCreate(baseDir)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(store.Path)
	if err != nil {
		return fmt.Errorf("read identity file: %w", err)
	}
	if err := os.WriteFile(outPath, data, 0o600); err != nil {
		return fmt.Errorf("write exported identity: %w", err)
	}
	return nil
}

func Import(baseDir, inPath string) (Store, error) {
	data, err := os.ReadFile(inPath)
	if err != nil {
		return Store{}, fmt.Errorf("read identity import: %w", err)
	}
	var record fileRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return Store{}, fmt.Errorf("parse identity import: %w", err)
	}
	if record.IdentityID == "" {
		return Store{}, fmt.Errorf("parse identity import: missing identity_id")
	}

	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return Store{}, fmt.Errorf("create identity directory: %w", err)
	}
	path := filepath.Join(baseDir, fileName)
	payload, err := json.Marshal(record)
	if err != nil {
		return Store{}, fmt.Errorf("marshal identity import: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return Store{}, fmt.Errorf("write identity import: %w", err)
	}
	return Store{
		IdentityID: record.IdentityID,
		Path:       path,
	}, nil
}

func DefaultBaseDir() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(root, "chatbox"), nil
}

func randomIdentityID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic("identity: crypto/rand failed")
	}
	return hex.EncodeToString(raw[:])
}
