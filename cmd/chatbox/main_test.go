package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"chatbox/internal/keys"
	"chatbox/internal/session"
)

func TestRunKeygenCreatesPSKFile(t *testing.T) {
	originalLaunchBackgroundUpdateCheck := launchBackgroundUpdateCheck
	launchBackgroundUpdateCheck = func(context.Context) {}
	t.Cleanup(func() {
		launchBackgroundUpdateCheck = originalLaunchBackgroundUpdateCheck
	})

	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "chatbox.psk")

	if err := run(context.Background(), []string{"keygen", "--out", path}); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat returned error: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected 0600 permissions, got %o", got)
	}
}

func TestRunVersionPrintsCurrentVersion(t *testing.T) {
	originalStdout := stdout
	originalLaunchBackgroundUpdateCheck := launchBackgroundUpdateCheck
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe returned error: %v", err)
	}
	stdout = w
	t.Cleanup(func() {
		stdout = originalStdout
		launchBackgroundUpdateCheck = originalLaunchBackgroundUpdateCheck
	})
	launchBackgroundUpdateCheck = func(context.Context) {}

	runErr := run(context.Background(), []string{"version"})
	_ = w.Close()
	output, readErr := io.ReadAll(r)
	_ = r.Close()
	if readErr != nil {
		t.Fatalf("ReadAll returned error: %v", readErr)
	}
	if runErr != nil {
		t.Fatalf("run returned error: %v", runErr)
	}
	if !strings.Contains(string(bytes.TrimSpace(output)), "dev") {
		t.Fatalf("expected version output to contain %q, got %q", "dev", string(output))
	}
}

func TestUsageIncludesVersionAndSelfUpdateCommands(t *testing.T) {
	t.Parallel()

	message := usageError().Error()
	if !strings.Contains(message, "version") {
		t.Fatalf("expected usage to mention version command, got %q", message)
	}
	if !strings.Contains(message, "self-update") {
		t.Fatalf("expected usage to mention self-update command, got %q", message)
	}
}

func TestRunSelfUpdateDelegatesToUpdater(t *testing.T) {
	originalRunSelfUpdateCommand := runSelfUpdateCommand
	t.Cleanup(func() {
		runSelfUpdateCommand = originalRunSelfUpdateCommand
	})

	called := false
	runSelfUpdateCommand = func(context.Context) error {
		called = true
		return nil
	}

	if err := run(context.Background(), []string{"self-update"}); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if !called {
		t.Fatal("expected self-update command handler to be invoked")
	}
}

func TestRunSkipsBackgroundUpdateCheckForSelfUpdate(t *testing.T) {
	originalLaunchBackgroundUpdateCheck := launchBackgroundUpdateCheck
	originalRunSelfUpdateCommand := runSelfUpdateCommand
	t.Cleanup(func() {
		launchBackgroundUpdateCheck = originalLaunchBackgroundUpdateCheck
		runSelfUpdateCommand = originalRunSelfUpdateCommand
	})

	launched := false
	launchBackgroundUpdateCheck = func(context.Context) {
		launched = true
	}
	runSelfUpdateCommand = func(context.Context) error { return nil }

	if err := run(context.Background(), []string{"self-update"}); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if launched {
		t.Fatal("expected self-update command to skip background update checks")
	}
}

func TestResolveUIDefaultsToScrollback(t *testing.T) {
	t.Parallel()

	ui, err := resolveUI("")
	if err != nil {
		t.Fatalf("resolveUI returned error: %v", err)
	}
	if ui != "scrollback" {
		t.Fatalf("expected default ui %q, got %q", "scrollback", ui)
	}
}

func TestResolveUIAcceptsExplicitTUI(t *testing.T) {
	t.Parallel()

	ui, err := resolveUI("tui")
	if err != nil {
		t.Fatalf("resolveUI returned error: %v", err)
	}
	if ui != "tui" {
		t.Fatalf("expected explicit ui %q, got %q", "tui", ui)
	}
}

func TestResolveAlertDefaultsToBell(t *testing.T) {
	t.Parallel()

	alert, err := resolveAlert("")
	if err != nil {
		t.Fatalf("resolveAlert returned error: %v", err)
	}
	if alert != "bell" {
		t.Fatalf("expected default alert %q, got %q", "bell", alert)
	}
}

func TestResolveAlertAcceptsOff(t *testing.T) {
	t.Parallel()

	alert, err := resolveAlert("off")
	if err != nil {
		t.Fatalf("resolveAlert returned error: %v", err)
	}
	if alert != "off" {
		t.Fatalf("expected explicit alert %q, got %q", "off", alert)
	}
}

func TestResolveAlertRejectsUnknownMode(t *testing.T) {
	t.Parallel()

	if _, err := resolveAlert("beep"); err == nil {
		t.Fatal("expected unknown alert mode to fail")
	}
}

func TestRunHostPassesDefaultScrollbackUIToLauncher(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "chatbox.psk")
	if err := run(context.Background(), []string{"keygen", "--out", path}); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	originalRunHostUI := runHostUI
	t.Cleanup(func() {
		runHostUI = originalRunHostUI
	})

	var gotUI string
	runHostUI = func(_ *session.Host, _ string, _ []byte, ui string, _ string) error {
		gotUI = ui
		return nil
	}

	if err := runHost(context.Background(), []string{"--listen", "127.0.0.1:0", "--psk-file", path, "--name", "tester"}); err != nil {
		t.Fatalf("runHost returned error: %v", err)
	}

	if gotUI != "scrollback" {
		t.Fatalf("expected launcher ui %q, got %q", "scrollback", gotUI)
	}
}

func TestRunHostPassesAlertModeToLauncher(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "chatbox.psk")
	if err := run(context.Background(), []string{"keygen", "--out", path}); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	originalRunHostUI := runHostUI
	t.Cleanup(func() {
		runHostUI = originalRunHostUI
	})

	var gotAlert string
	runHostUI = func(_ *session.Host, _ string, _ []byte, _ string, alert string) error {
		gotAlert = alert
		return nil
	}

	if err := runHost(context.Background(), []string{"--listen", "127.0.0.1:0", "--psk-file", path, "--name", "tester", "--alert", "off"}); err != nil {
		t.Fatalf("runHost returned error: %v", err)
	}

	if gotAlert != "off" {
		t.Fatalf("expected launcher alert %q, got %q", "off", gotAlert)
	}
}

func TestRunJoinPassesAlertModeToLauncher(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "chatbox.psk")
	if err := run(context.Background(), []string{"keygen", "--out", path}); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	psk, err := keys.LoadPSKFromFile(path)
	if err != nil {
		t.Fatalf("LoadPSKFromFile returned error: %v", err)
	}

	host, err := session.Listen("127.0.0.1:0", session.Config{
		Name: "host",
		PSK:  psk,
	})
	if err != nil {
		t.Fatalf("Listen returned error: %v", err)
	}
	defer host.Close()

	acceptDone := make(chan error, 1)
	go func() {
		conn, err := host.Accept(context.Background())
		if err == nil && conn != nil {
			_ = conn.Close()
		}
		acceptDone <- err
	}()

	originalRunJoinUI := runJoinUI
	t.Cleanup(func() {
		runJoinUI = originalRunJoinUI
	})

	var gotAlert string
	runJoinUI = func(_ *session.Session, _ string, _ string, _ session.Config, _ string, alert string) error {
		gotAlert = alert
		return nil
	}

	if err := runJoin(context.Background(), []string{"--peer", host.Addr(), "--psk-file", path, "--name", "tester", "--alert", "off"}); err != nil {
		t.Fatalf("runJoin returned error: %v", err)
	}

	if err := <-acceptDone; err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if gotAlert != "off" {
		t.Fatalf("expected launcher alert %q, got %q", "off", gotAlert)
	}
}
