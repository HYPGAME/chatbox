package keys

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGeneratePSKFileCreates32ByteKeyWith0600Permissions(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "chatbox.psk")

	if err := GeneratePSKFile(path); err != nil {
		t.Fatalf("GeneratePSKFile returned error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if len(data) != 32 {
		t.Fatalf("expected 32-byte PSK, got %d bytes", len(data))
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat returned error: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected permissions 0600, got %o", got)
	}
}

func TestLoadPSKFromFileRejectsInsecurePermissions(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "chatbox.psk")

	if err := os.WriteFile(path, make([]byte, 32), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	if _, err := LoadPSKFromFile(path); err == nil {
		t.Fatal("expected LoadPSKFromFile to reject insecure permissions")
	}
}
