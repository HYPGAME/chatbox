//go:build darwin

package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var runClipboardScript = func(ctx context.Context, lines ...string) ([]byte, error) {
	args := make([]string, 0, len(lines)*2)
	for _, line := range lines {
		args = append(args, "-e", line)
	}
	cmd := exec.CommandContext(ctx, "osascript", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("osascript: %w", err)
	}
	return output, nil
}

func defaultClipboardReader() clipboardReaderFunc {
	return readClipboardAttachment
}

func readClipboardAttachment(ctx context.Context) (clipboardAttachment, error) {
	paths, err := readClipboardFilePaths(ctx)
	if err != nil {
		return clipboardAttachment{}, err
	}
	if len(paths) > 1 {
		return clipboardAttachment{}, fmt.Errorf("clipboard contains multiple files; copy one item")
	}
	if len(paths) == 1 {
		return clipboardAttachment{
			Path: paths[0],
			Kind: attachmentKindFromPath(paths[0]),
		}, nil
	}

	path, err := exportClipboardImage(ctx, ".png", []string{
		"try",
		"set imageData to the clipboard as «class PNGf»",
		"on error",
		"return \"\"",
		"end try",
	})
	if err == nil {
		return clipboardAttachment{
			Path: path,
			Kind: "image",
			Cleanup: func() {
				_ = os.Remove(path)
			},
		}, nil
	}

	path, err = exportClipboardImage(ctx, ".tiff", []string{
		"try",
		"set imageData to the clipboard as TIFF picture",
		"on error",
		"return \"\"",
		"end try",
	})
	if err == nil {
		return clipboardAttachment{
			Path: path,
			Kind: "image",
			Cleanup: func() {
				_ = os.Remove(path)
			},
		}, nil
	}
	if err == errPasteEmpty {
		return clipboardAttachment{}, err
	}
	return clipboardAttachment{}, err
}

func readClipboardFilePaths(ctx context.Context) ([]string, error) {
	output, err := runClipboardScript(ctx,
		"try",
		"set clipboardItems to the clipboard as alias list",
		"set pathLines to {}",
		"repeat with itemAlias in clipboardItems",
		"set end of pathLines to POSIX path of itemAlias",
		"end repeat",
		"set AppleScript's text item delimiters to linefeed",
		"return pathLines as text",
		"on error",
		"return \"\"",
		"end try",
	)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return nil, nil
	}
	lines := strings.Split(trimmed, "\n")
	paths := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			paths = append(paths, line)
		}
	}
	return paths, nil
}

func exportClipboardImage(ctx context.Context, ext string, prelude []string) (string, error) {
	file, err := os.CreateTemp("", "chatbox-paste-*"+ext)
	if err != nil {
		return "", fmt.Errorf("create temp paste file: %w", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close temp paste file: %w", err)
	}
	_ = os.Remove(path)

	script := append([]string{}, prelude...)
	script = append(script,
		"set outputFile to POSIX file "+quotedAppleScriptString(path),
		"set fileRef to open for access outputFile with write permission",
		"try",
		"set eof of fileRef to 0",
		"write imageData to fileRef",
		"close access fileRef",
		"return POSIX path of outputFile",
		"on error errMsg number errNum",
		"try",
		"close access fileRef",
		"end try",
		"error errMsg number errNum",
		"end try",
	)

	output, err := runClipboardScript(ctx, script...)
	if err != nil {
		_ = os.Remove(path)
		return "", err
	}
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		_ = os.Remove(path)
		return "", errPasteEmpty
	}
	return filepath.Clean(trimmed), nil
}

func quotedAppleScriptString(value string) string {
	return "\"" + strings.ReplaceAll(value, "\"", "\\\"") + "\""
}
