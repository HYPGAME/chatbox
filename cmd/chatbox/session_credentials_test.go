package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"chatbox/internal/keys"
)

func TestResolveSessionCredentialsLoadsPSKFileMode(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "chatbox.psk")
	if err := keys.GeneratePSKFile(path); err != nil {
		t.Fatalf("GeneratePSKFile returned error: %v", err)
	}

	got, err := resolveSessionCredentials(path, "", "", "")
	if err != nil {
		t.Fatalf("resolveSessionCredentials returned error: %v", err)
	}
	if len(got.psk) != 32 {
		t.Fatalf("expected 32-byte psk, got %d", len(got.psk))
	}
	if got.transcriptKey != "" {
		t.Fatalf("expected file mode transcript key to stay empty, got %q", got.transcriptKey)
	}
}

func TestResolveSessionCredentialsRejectsMixedModes(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "chatbox.psk")
	if err := keys.GeneratePSKFile(path); err != nil {
		t.Fatalf("GeneratePSKFile returned error: %v", err)
	}

	_, err := resolveSessionCredentials(path, "team-alpha", "abc123", "")
	if err == nil {
		t.Fatal("expected mixed psk-file and group mode to fail")
	}
	if !strings.Contains(err.Error(), "--psk-file") || !strings.Contains(err.Error(), "--group-name") {
		t.Fatalf("expected mixed-mode error, got %q", err.Error())
	}
}

func TestResolveSessionCredentialsPromptsForGroupPasswordWhenInteractive(t *testing.T) {
	originalPrompt := promptForGroupPassword
	originalInteractive := stdinIsTerminal
	t.Cleanup(func() {
		promptForGroupPassword = originalPrompt
		stdinIsTerminal = originalInteractive
	})

	stdinIsTerminal = func() bool { return true }
	promptForGroupPassword = func(groupName string) (string, error) {
		if groupName != "team-alpha" {
			t.Fatalf("expected normalized prompt group name, got %q", groupName)
		}
		return "abc123", nil
	}

	got, err := resolveSessionCredentials("", " team-alpha ", "", "")
	if err != nil {
		t.Fatalf("resolveSessionCredentials returned error: %v", err)
	}
	if len(got.psk) != 32 {
		t.Fatalf("expected derived 32-byte psk, got %d", len(got.psk))
	}
	if !strings.HasPrefix(got.transcriptKey, "group:team-alpha:") {
		t.Fatalf("expected stable group transcript key, got %q", got.transcriptKey)
	}
}

func TestResolveSessionCredentialsRejectsMissingPasswordWithoutTTY(t *testing.T) {
	originalInteractive := stdinIsTerminal
	t.Cleanup(func() {
		stdinIsTerminal = originalInteractive
	})
	stdinIsTerminal = func() bool { return false }

	_, err := resolveSessionCredentials("", "team-alpha", "", "")
	if err == nil {
		t.Fatal("expected non-interactive password resolution to fail")
	}
	if !strings.Contains(err.Error(), "--group-password") {
		t.Fatalf("expected missing-password guidance, got %q", err.Error())
	}
}

func TestResolveSessionCredentialsRejectsPasswordWithoutGroupName(t *testing.T) {
	t.Parallel()

	_, err := resolveSessionCredentials("", "", "abc123", "")
	if err == nil {
		t.Fatal("expected password without group name to fail")
	}
	if !strings.Contains(err.Error(), "--group-name") {
		t.Fatalf("expected group-name guidance, got %q", err.Error())
	}
}

func TestResolveSessionCredentialsLoadsGroupPasswordFromFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "group-password.txt")
	if err := os.WriteFile(path, []byte("abc123\nignored\n"), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	got, err := resolveSessionCredentials("", "team-alpha", "", path)
	if err != nil {
		t.Fatalf("resolveSessionCredentials returned error: %v", err)
	}

	want, err := keys.DeriveGroupCredentials("team-alpha", "abc123")
	if err != nil {
		t.Fatalf("DeriveGroupCredentials returned error: %v", err)
	}
	if !bytes.Equal(got.psk, want.PSK) {
		t.Fatal("expected password file to derive the same PSK as the first line")
	}
	if got.transcriptKey != want.RoomKey {
		t.Fatalf("expected transcript key %q, got %q", want.RoomKey, got.transcriptKey)
	}
}

func TestResolveSessionCredentialsPrefersExplicitPasswordOverPasswordFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "group-password.txt")
	if err := os.WriteFile(path, []byte("wrong-password\n"), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	got, err := resolveSessionCredentials("", "team-alpha", "abc123", path)
	if err != nil {
		t.Fatalf("resolveSessionCredentials returned error: %v", err)
	}

	want, err := keys.DeriveGroupCredentials("team-alpha", "abc123")
	if err != nil {
		t.Fatalf("DeriveGroupCredentials returned error: %v", err)
	}
	if !bytes.Equal(got.psk, want.PSK) {
		t.Fatal("expected explicit password to win over password file")
	}
}

func TestResolveSessionCredentialsRejectsPasswordFileWithoutGroupName(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "group-password.txt")
	if err := os.WriteFile(path, []byte("abc123\n"), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	_, err := resolveSessionCredentials("", "", "", path)
	if err == nil {
		t.Fatal("expected password file without group name to fail")
	}
	if !strings.Contains(err.Error(), "--group-name") {
		t.Fatalf("expected group-name guidance, got %q", err.Error())
	}
}
