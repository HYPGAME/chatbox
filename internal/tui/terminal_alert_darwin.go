//go:build darwin

package tui

import (
	"os"
	"os/exec"
	"strings"
)

const terminalAppForegroundScript = `
tell application "System Events"
	if not (exists process "Terminal") then
		return "background\t"
	end if
	set terminalFrontmost to frontmost of process "Terminal"
end tell

if terminalFrontmost is false then
	return "background\t"
end if

tell application "Terminal"
	if (count of windows) is 0 then
		return "background\t"
	end if
	return "frontmost\t" & (tty of selected tab of front window)
end tell
`

type terminalAppForegroundDetector struct {
	currentTTY string
	runScript  func(string) (string, error)
}

func newTerminalAppForegroundDetector(currentTTY string) terminalAppForegroundDetector {
	return terminalAppForegroundDetector{
		currentTTY: strings.TrimSpace(currentTTY),
		runScript:  runAppleScript,
	}
}

func (d terminalAppForegroundDetector) ShouldAlert() bool {
	if d.currentTTY == "" {
		return false
	}

	runScript := d.runScript
	if runScript == nil {
		runScript = runAppleScript
	}

	output, err := runScript(terminalAppForegroundScript)
	if err != nil {
		return false
	}

	state, selectedTTY, ok := parseTerminalAppForegroundState(output)
	if !ok {
		return false
	}
	if state == "background" {
		return true
	}
	if state != "frontmost" {
		return false
	}

	selectedTTY = strings.TrimSpace(selectedTTY)
	if selectedTTY == "" {
		return false
	}
	return selectedTTY != d.currentTTY
}

func parseTerminalAppForegroundState(output string) (state string, selectedTTY string, ok bool) {
	parts := strings.SplitN(strings.TrimSpace(output), "\t", 2)
	if len(parts) == 0 || parts[0] == "" {
		return "", "", false
	}
	if parts[0] == "background" {
		return "background", "", true
	}
	if parts[0] != "frontmost" || len(parts) != 2 {
		return "", "", false
	}
	return "frontmost", parts[1], true
}

func runAppleScript(script string) (string, error) {
	output, err := exec.Command("osascript", "-e", script).CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func newTerminalBellAlertNotifier(console *promptConsole) alertNotifierFunc {
	if console == nil {
		return nil
	}
	if os.Getenv("TERM_PROGRAM") != "Apple_Terminal" {
		return nil
	}

	currentTTY, err := currentTerminalTTY()
	if err != nil {
		return nil
	}

	detector := newTerminalAppForegroundDetector(currentTTY)
	return func() {
		if detector.ShouldAlert() {
			console.bell()
		}
	}
}

func currentTerminalTTY() (string, error) {
	cmd := exec.Command("tty")
	cmd.Stdin = os.Stdin
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
