package update

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestApplyUpdateReplacesBinaryAtomically(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	currentPath := filepath.Join(tempDir, "chatbox")
	if err := os.WriteFile(currentPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	result, err := applyUpdateAtPath(currentPath, []byte("new-binary"), defaultApplyFileOps())
	if err != nil {
		t.Fatalf("applyUpdateAtPath returned error: %v", err)
	}
	if result.FallbackPath != "" {
		t.Fatalf("expected in-place replacement, got fallback path %q", result.FallbackPath)
	}

	got, err := os.ReadFile(currentPath)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if string(got) != "new-binary" {
		t.Fatalf("expected binary to be replaced, got %q", string(got))
	}
}

func TestApplyUpdateFallsBackToSiblingFileWhenDirectReplaceFails(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	currentPath := filepath.Join(tempDir, "chatbox")
	if err := os.WriteFile(currentPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	ops := defaultApplyFileOps()
	ops.rename = func(oldPath string, newPath string) error {
		if newPath == currentPath+".old" {
			return errors.New("permission denied")
		}
		return os.Rename(oldPath, newPath)
	}

	result, err := applyUpdateAtPath(currentPath, []byte("new-binary"), ops)
	if err != nil {
		t.Fatalf("applyUpdateAtPath returned error: %v", err)
	}
	if result.FallbackPath == "" {
		t.Fatal("expected fallback path to be returned")
	}

	currentBytes, err := os.ReadFile(currentPath)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if string(currentBytes) != "old-binary" {
		t.Fatalf("expected original binary to remain untouched, got %q", string(currentBytes))
	}

	fallbackBytes, err := os.ReadFile(result.FallbackPath)
	if err != nil {
		t.Fatalf("ReadFile fallback returned error: %v", err)
	}
	if string(fallbackBytes) != "new-binary" {
		t.Fatalf("expected fallback binary to be written, got %q", string(fallbackBytes))
	}
}
