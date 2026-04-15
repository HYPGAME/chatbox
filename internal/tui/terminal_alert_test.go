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
	}

	if detector.ShouldAlert() {
		t.Fatal("expected detector error to suppress alert")
	}
}
