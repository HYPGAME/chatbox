package identity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenOrCreateCreatesIdentityFile(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	store, err := OpenOrCreate(baseDir)
	if err != nil {
		t.Fatalf("OpenOrCreate returned error: %v", err)
	}
	if store.IdentityID == "" {
		t.Fatal("expected identity id to be generated")
	}
	if store.Path == "" {
		t.Fatal("expected identity path to be populated")
	}

	info, err := os.Stat(store.Path)
	if err != nil {
		t.Fatalf("Stat returned error: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected identity file permissions 0600, got %o", got)
	}
}

func TestOpenOrCreateReloadsExistingIdentity(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	first, err := OpenOrCreate(baseDir)
	if err != nil {
		t.Fatalf("OpenOrCreate returned error: %v", err)
	}

	second, err := OpenOrCreate(baseDir)
	if err != nil {
		t.Fatalf("OpenOrCreate reload returned error: %v", err)
	}
	if first.IdentityID != second.IdentityID {
		t.Fatalf("expected stable identity id %q, got %q", first.IdentityID, second.IdentityID)
	}
	if first.Path != second.Path {
		t.Fatalf("expected stable identity path %q, got %q", first.Path, second.Path)
	}
}

func TestOpenOrCreateRejectsMalformedIdentityFile(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	path := filepath.Join(baseDir, fileName)
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(path, []byte("{not-json"), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	if _, err := OpenOrCreate(baseDir); err == nil {
		t.Fatal("expected malformed identity file to fail")
	}
}
