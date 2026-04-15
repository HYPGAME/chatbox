//go:build darwin

package tui

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const terminalBundleID = "com.apple.Terminal"

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
	currentTTY        string
	runScript         func(string) (string, error)
	frontmostBundleID func() (string, error)
}

func newTerminalAppForegroundDetector(currentTTY string) terminalAppForegroundDetector {
	return terminalAppForegroundDetector{
		currentTTY:        strings.TrimSpace(currentTTY),
		runScript:         runAppleScript,
		frontmostBundleID: currentFrontmostAppBundleID,
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

	frontmostBundleID := d.frontmostBundleID
	if frontmostBundleID == nil {
		frontmostBundleID = currentFrontmostAppBundleID
	}
	if bundleID, err := frontmostBundleID(); err == nil && bundleID != "" && bundleID != terminalBundleID {
		return true
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

func currentFrontmostAppBundleID() (string, error) {
	frontOutput, err := exec.Command("lsappinfo", "front").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("lsappinfo front: %w", err)
	}
	asn, ok := parseFrontmostASN(string(frontOutput))
	if !ok {
		return "", fmt.Errorf("parse lsappinfo front output: %q", strings.TrimSpace(string(frontOutput)))
	}

	listOutput, err := exec.Command("lsappinfo", "list").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("lsappinfo list: %w", err)
	}
	bundleID, ok := parseBundleIDForASN(string(listOutput), asn)
	if !ok {
		return "", fmt.Errorf("resolve bundleID for ASN %s", asn)
	}
	return bundleID, nil
}

func parseFrontmostASN(output string) (string, bool) {
	line := strings.TrimSpace(output)
	if !strings.HasPrefix(line, "ASN:") {
		return "", false
	}
	asn := strings.TrimSuffix(strings.TrimPrefix(line, "ASN:"), ":")
	if asn == "" {
		return "", false
	}
	return asn, true
}

func parseBundleIDForASN(output string, asn string) (string, bool) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	target := "ASN:" + asn + ":"
	inBlock := false
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if isLSAppInfoEntryHeader(trimmed) && strings.Contains(trimmed, target) {
			inBlock = true
			continue
		}
		if !inBlock {
			continue
		}
		if isLSAppInfoEntryHeader(trimmed) {
			inBlock = false
			continue
		}
		if !strings.HasPrefix(trimmed, "bundleID=") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(trimmed, "bundleID="))
		if value == "" || value == "[ NULL ]" {
			return "", false
		}
		return strings.Trim(value, `"`), true
	}
	return "", false
}

func isLSAppInfoEntryHeader(line string) bool {
	if line == "" {
		return false
	}
	index := strings.Index(line, ")")
	if index <= 0 {
		return false
	}
	for _, r := range line[:index] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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
