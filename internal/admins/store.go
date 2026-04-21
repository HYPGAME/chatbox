package admins

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Store struct {
	AllowedUpdateIdentities map[string]struct{}
}

type fileConfig struct {
	AllowedUpdateIdentities []string `json:"allowed_update_identities"`
}

func Load(path string) (Store, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Store{AllowedUpdateIdentities: map[string]struct{}{}}, nil
		}
		return Store{}, fmt.Errorf("read admin config: %w", err)
	}

	var config fileConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return Store{}, fmt.Errorf("parse admin config: %w", err)
	}

	store := Store{AllowedUpdateIdentities: make(map[string]struct{}, len(config.AllowedUpdateIdentities))}
	for _, identityID := range config.AllowedUpdateIdentities {
		identityID = strings.TrimSpace(identityID)
		if identityID == "" {
			continue
		}
		store.AllowedUpdateIdentities[identityID] = struct{}{}
	}
	return store, nil
}

func (s Store) Allows(identityID string) bool {
	identityID = strings.TrimSpace(identityID)
	if identityID == "" {
		return false
	}
	_, ok := s.AllowedUpdateIdentities[identityID]
	return ok
}
