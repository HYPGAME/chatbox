package main

import (
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

	got, err := resolveSessionCredentials(path, "", "")
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

	_, err := resolveSessionCredentials(path, "team-alpha", "abc123")
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

	got, err := resolveSessionCredentials("", " team-alpha ", "")
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

	_, err := resolveSessionCredentials("", "team-alpha", "")
	if err == nil {
		t.Fatal("expected non-interactive password resolution to fail")
	}
	if !strings.Contains(err.Error(), "--group-password") {
		t.Fatalf("expected missing-password guidance, got %q", err.Error())
	}
}

func TestResolveSessionCredentialsRejectsPasswordWithoutGroupName(t *testing.T) {
	t.Parallel()

	_, err := resolveSessionCredentials("", "", "abc123")
	if err == nil {
		t.Fatal("expected password without group name to fail")
	}
	if !strings.Contains(err.Error(), "--group-name") {
		t.Fatalf("expected group-name guidance, got %q", err.Error())
	}
}
