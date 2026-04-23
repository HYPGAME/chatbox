package attachment

import (
	"fmt"
	"os"
	"path/filepath"
)

func DefaultHostDir() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(root, "chatbox", "attachments", "host"), nil
}

func DefaultCacheDir() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(root, "chatbox", "attachments", "cache"), nil
}
