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
	"chatbox/internal/update"
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
	originalMallocStackLogging := os.Getenv("MallocStackLogging")
	originalMallocStackLoggingNoCompact := os.Getenv("MallocStackLoggingNoCompact")
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe returned error: %v", err)
	}
	stdout = w
	t.Cleanup(func() {
		stdout = originalStdout
		launchBackgroundUpdateCheck = originalLaunchBackgroundUpdateCheck
		if originalMallocStackLogging == "" {
			_ = os.Unsetenv("MallocStackLogging")
		} else {
			_ = os.Setenv("MallocStackLogging", originalMallocStackLogging)
		}
		if originalMallocStackLoggingNoCompact == "" {
			_ = os.Unsetenv("MallocStackLoggingNoCompact")
		} else {
			_ = os.Setenv("MallocStackLoggingNoCompact", originalMallocStackLoggingNoCompact)
		}
	})
	launchBackgroundUpdateCheck = func(context.Context) {}
	if err := os.Setenv("MallocStackLogging", "1"); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	if err := os.Setenv("MallocStackLoggingNoCompact", "1"); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}

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
	if got := os.Getenv("MallocStackLogging"); got != "" {
		t.Fatalf("expected MallocStackLogging to be cleared, got %q", got)
	}
	if got := os.Getenv("MallocStackLoggingNoCompact"); got != "" {
		t.Fatalf("expected MallocStackLoggingNoCompact to be cleared, got %q", got)
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

func TestPrintSelfUpdateResultShowsReleaseNotesForUpdatedVersion(t *testing.T) {
	var out bytes.Buffer

	err := printSelfUpdateResult(&out, update.SelfUpdateResult{
		LatestVersion: "v0.2.0",
		ExecutablePath: "/usr/local/bin/chatbox",
		ReleaseNotes:  "## What's New\n- slash command suggestions\n- router auto-update retries",
		Updated:       true,
	})

	if err != nil {
		t.Fatalf("printSelfUpdateResult returned error: %v", err)
	}
	rendered := out.String()
	if !strings.Contains(rendered, "updated chatbox to v0.2.0") {
		t.Fatalf("expected updated version line, got %q", rendered)
	}
	if !strings.Contains(rendered, "/usr/local/bin/chatbox") {
		t.Fatalf("expected updated binary path, got %q", rendered)
	}
	if !strings.Contains(rendered, "restart chatbox") {
		t.Fatalf("expected restart hint, got %q", rendered)
	}
	if !strings.Contains(rendered, "what's new:") {
		t.Fatalf("expected release notes heading, got %q", rendered)
	}
	if !strings.Contains(rendered, "slash command suggestions") {
		t.Fatalf("expected release notes body, got %q", rendered)
	}
}

func TestPrintSelfUpdateResultFallsBackToReleaseURLWhenNotesAreEmpty(t *testing.T) {
	var out bytes.Buffer

	err := printSelfUpdateResult(&out, update.SelfUpdateResult{
		LatestVersion: "v0.2.0",
		ExecutablePath: "/usr/local/bin/chatbox",
		ReleaseURL:    "https://github.com/HYPGAME/chatbox/releases/tag/v0.2.0",
		Updated:       true,
	})

	if err != nil {
		t.Fatalf("printSelfUpdateResult returned error: %v", err)
	}
	rendered := out.String()
	if !strings.Contains(rendered, "updated chatbox to v0.2.0") {
		t.Fatalf("expected updated version line, got %q", rendered)
	}
	if !strings.Contains(rendered, "restart chatbox") {
		t.Fatalf("expected restart hint, got %q", rendered)
	}
	if !strings.Contains(rendered, "release: https://github.com/HYPGAME/chatbox/releases/tag/v0.2.0") {
		t.Fatalf("expected release URL fallback, got %q", rendered)
	}
}

func TestRunIdentityExportAndImport(t *testing.T) {
	originalLaunchBackgroundUpdateCheck := launchBackgroundUpdateCheck
	t.Cleanup(func() {
		launchBackgroundUpdateCheck = originalLaunchBackgroundUpdateCheck
	})
	launchBackgroundUpdateCheck = func(context.Context) {}

	configDir := t.TempDir()
	originalConfigHome := os.Getenv("XDG_CONFIG_HOME")
	if err := os.Setenv("XDG_CONFIG_HOME", configDir); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	t.Cleanup(func() {
		if originalConfigHome == "" {
			_ = os.Unsetenv("XDG_CONFIG_HOME")
		} else {
			_ = os.Setenv("XDG_CONFIG_HOME", originalConfigHome)
		}
	})

	exportPath := filepath.Join(t.TempDir(), "identity.json")
	if err := run(context.Background(), []string{"identity", "export", "--out", exportPath}); err != nil {
		t.Fatalf("identity export returned error: %v", err)
	}
	first, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("ReadFile exported identity returned error: %v", err)
	}

	if err := os.RemoveAll(filepath.Join(configDir, "chatbox")); err != nil {
		t.Fatalf("RemoveAll returned error: %v", err)
	}
	if err := run(context.Background(), []string{"identity", "import", "--in", exportPath}); err != nil {
		t.Fatalf("identity import returned error: %v", err)
	}

	secondExport := filepath.Join(t.TempDir(), "identity-again.json")
	if err := run(context.Background(), []string{"identity", "export", "--out", secondExport}); err != nil {
		t.Fatalf("second identity export returned error: %v", err)
	}
	second, err := os.ReadFile(secondExport)
	if err != nil {
		t.Fatalf("ReadFile second exported identity returned error: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("expected imported identity to export identically, got %q vs %q", first, second)
	}
}

func TestRunIdentityImportRejectsMalformedFile(t *testing.T) {
	originalLaunchBackgroundUpdateCheck := launchBackgroundUpdateCheck
	t.Cleanup(func() {
		launchBackgroundUpdateCheck = originalLaunchBackgroundUpdateCheck
	})
	launchBackgroundUpdateCheck = func(context.Context) {}

	configDir := t.TempDir()
	originalConfigHome := os.Getenv("XDG_CONFIG_HOME")
	if err := os.Setenv("XDG_CONFIG_HOME", configDir); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	t.Cleanup(func() {
		if originalConfigHome == "" {
			_ = os.Unsetenv("XDG_CONFIG_HOME")
		} else {
			_ = os.Setenv("XDG_CONFIG_HOME", originalConfigHome)
		}
	})

	path := filepath.Join(t.TempDir(), "bad-identity.json")
	if err := os.WriteFile(path, []byte("{not-json"), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	if err := run(context.Background(), []string{"identity", "import", "--in", path}); err == nil {
		t.Fatal("expected malformed identity import to fail")
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

func TestRunHostHeadlessDelegatesToHeadlessLauncher(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "chatbox.psk")
	if err := run(context.Background(), []string{"keygen", "--out", path}); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	originalRunHostUI := runHostUI
	originalRunHostHeadless := runHostHeadless
	t.Cleanup(func() {
		runHostUI = originalRunHostUI
		runHostHeadless = originalRunHostHeadless
	})

	uiCalled := false
	headlessCalled := false
	runHostUI = func(_ *session.Host, _ string, _ []byte, _ string, _ string) error {
		uiCalled = true
		return nil
	}
	runHostHeadless = func(_ context.Context, _ *session.Host, _ string, _ []byte) error {
		headlessCalled = true
		return nil
	}

	if err := runHost(context.Background(), []string{"--listen", "127.0.0.1:0", "--psk-file", path, "--name", "tester", "--headless"}); err != nil {
		t.Fatalf("runHost returned error: %v", err)
	}

	if !headlessCalled {
		t.Fatal("expected headless launcher to be invoked")
	}
	if uiCalled {
		t.Fatal("expected ui launcher to stay unused in headless mode")
	}
}

func TestRunHostRejectsHeadlessWithUI(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "chatbox.psk")
	if err := run(context.Background(), []string{"keygen", "--out", path}); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	err := runHost(context.Background(), []string{"--listen", "127.0.0.1:0", "--psk-file", path, "--headless", "--ui", "tui"})
	if err == nil {
		t.Fatal("expected headless host to reject --ui")
	}
	if !strings.Contains(err.Error(), "headless") || !strings.Contains(err.Error(), "--ui") {
		t.Fatalf("expected headless/ui validation error, got %q", err.Error())
	}
}

func TestRunSkipsBackgroundUpdateCheckForHeadlessHost(t *testing.T) {
	originalLaunchBackgroundUpdateCheck := launchBackgroundUpdateCheck
	originalRunHostHeadless := runHostHeadless
	t.Cleanup(func() {
		launchBackgroundUpdateCheck = originalLaunchBackgroundUpdateCheck
		runHostHeadless = originalRunHostHeadless
	})

	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "chatbox.psk")
	if err := run(context.Background(), []string{"keygen", "--out", path}); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	launched := false
	launchBackgroundUpdateCheck = func(context.Context) {
		launched = true
	}
	runHostHeadless = func(_ context.Context, _ *session.Host, _ string, _ []byte) error {
		return nil
	}

	if err := run(context.Background(), []string{"host", "--listen", "127.0.0.1:0", "--psk-file", path, "--name", "tester", "--headless"}); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if launched {
		t.Fatal("expected headless host to skip background update checks")
	}
}

func TestRunSkipsStderrBackgroundUpdateCheckForTUIJoin(t *testing.T) {
	originalLaunchBackgroundUpdateCheck := launchBackgroundUpdateCheck
	originalRunJoinUI := runJoinUI
	originalRunJoinUIWithUpdates := runJoinUIWithUpdates
	t.Cleanup(func() {
		launchBackgroundUpdateCheck = originalLaunchBackgroundUpdateCheck
		runJoinUI = originalRunJoinUI
		runJoinUIWithUpdates = originalRunJoinUIWithUpdates
	})

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

	launched := false
	launchBackgroundUpdateCheck = func(context.Context) {
		launched = true
	}
	runJoinUIWithUpdates = func(_ *session.Session, _ string, _ string, _ session.Config, _ string, _ string, notices <-chan string) error {
		if notices == nil {
			t.Fatal("expected update notices channel for tui join")
		}
		return nil
	}
	runJoinUI = func(_ *session.Session, _ string, _ string, _ session.Config, _ string, _ string) error {
		t.Fatal("expected legacy join ui launcher to stay unused for tui mode")
		return nil
	}

	if err := run(context.Background(), []string{"join", "--peer", host.Addr(), "--psk-file", path, "--name", "tester", "--ui", "tui"}); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if err := <-acceptDone; err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if launched {
		t.Fatal("expected tui join to skip stderr background update checks")
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
