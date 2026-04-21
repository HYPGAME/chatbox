package admins

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReturnsHostOnlyWhenFileMissing(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	store, err := Load(filepath.Join(baseDir, "admins.json"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if store.Allows("identity-a") {
		t.Fatal("expected missing file not to authorize arbitrary identities")
	}
}

func TestLoadParsesAllowedUpdateIdentities(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	path := filepath.Join(baseDir, "admins.json")
	if err := os.WriteFile(path, []byte(`{"allowed_update_identities":["identity-a","identity-b"]}`), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	store, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !store.Allows("identity-a") || !store.Allows("identity-b") {
		t.Fatalf("expected configured identities to be allowed, got %#v", store)
	}
	if store.Allows("identity-c") {
		t.Fatal("expected unknown identity not to be allowed")
	}
}

func TestLoadRejectsMalformedWhitelistFile(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	path := filepath.Join(baseDir, "admins.json")
	if err := os.WriteFile(path, []byte(`{"allowed_update_identities":`), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("expected malformed config to be rejected")
	}
}
