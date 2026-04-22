//go:build darwin

package tui

import (
	"fmt"
	"os/exec"
	"strings"
)

var runPbcopy = func(text string) error {
	trimmed := strings.ReplaceAll(text, "\r\n", "\n")
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(trimmed)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pbcopy: %w", err)
	}
	return nil
}

func defaultClipboardWriter() clipboardWriterFunc {
	return func(text string) error {
		return runPbcopy(text)
	}
}
