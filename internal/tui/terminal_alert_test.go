package tui

import (
	"errors"
	"testing"
)

func TestTerminalAppForegroundDetectorSuppressesWhenSelectedTTYMatches(t *testing.T) {
	t.Parallel()

	detector := terminalAppForegroundDetector{
		currentTTY: "/dev/ttys002",
		runScript: func(string) (string, error) {
			return "frontmost\t/dev/ttys002", nil
		},
		frontmostBundleID: func() (string, error) {
			return "com.apple.Terminal", nil
		},
	}

	if detector.ShouldAlert() {
		t.Fatal("expected matching selected tty to suppress alert")
	}
}

func TestTerminalAppForegroundDetectorAllowsAlertWhenTerminalNotFrontmost(t *testing.T) {
	t.Parallel()

	detector := terminalAppForegroundDetector{
		currentTTY: "/dev/ttys002",
		runScript: func(string) (string, error) {
			return "background\t", nil
		},
		frontmostBundleID: func() (string, error) {
			return "", errors.New("bundle lookup unavailable")
		},
	}

	if !detector.ShouldAlert() {
		t.Fatal("expected background terminal state to allow alert")
	}
}

func TestTerminalAppForegroundDetectorFailsClosedOnScriptError(t *testing.T) {
	t.Parallel()

	detector := terminalAppForegroundDetector{
		currentTTY: "/dev/ttys002",
		runScript: func(string) (string, error) {
			return "", errors.New("osascript failed")
		},
		frontmostBundleID: func() (string, error) {
			return "com.apple.Terminal", nil
		},
	}

	if detector.ShouldAlert() {
		t.Fatal("expected detector error to suppress alert")
	}
}

func TestTerminalAppForegroundDetectorFallsBackToFrontmostAppWhenScriptFails(t *testing.T) {
	t.Parallel()

	detector := terminalAppForegroundDetector{
		currentTTY: "/dev/ttys002",
		runScript: func(string) (string, error) {
			return "", errors.New("osascript failed")
		},
		frontmostBundleID: func() (string, error) {
			return "com.apple.finder", nil
		},
	}

	if !detector.ShouldAlert() {
		t.Fatal("expected non-terminal frontmost app to allow alert when AppleScript fails")
	}
}

func TestParseBundleIDForASNFromLSAppInfoList(t *testing.T) {
	t.Parallel()

	output := `11) "终端" ASN:0x0-0x1501500:
    bundleID="com.apple.Terminal"
    bundle path="/System/Applications/Utilities/Terminal.app"

12) "访达" ASN:0x0-0x26026:
    bundleID="com.apple.finder"
`

	bundleID, ok := parseBundleIDForASN(output, "0x0-0x1501500")
	if !ok {
		t.Fatal("expected bundleID to be parsed")
	}
	if bundleID != "com.apple.Terminal" {
		t.Fatalf("expected terminal bundleID, got %q", bundleID)
	}
}

func TestParseBundleIDForASNIgnoresParentASNReferences(t *testing.T) {
	t.Parallel()

	output := `10) "Child App" ASN:0x0-0x10010:
    parentASN="Terminal" ASN:0x0-0x1501500:
    bundleID="com.example.child"

11) "Terminal" ASN:0x0-0x1501500:
    bundleID="com.apple.Terminal"
`

	bundleID, ok := parseBundleIDForASN(output, "0x0-0x1501500")
	if !ok {
		t.Fatal("expected bundleID to be parsed from the real entry header")
	}
	if bundleID != "com.apple.Terminal" {
		t.Fatalf("expected terminal bundleID from the real ASN entry, got %q", bundleID)
	}
}
