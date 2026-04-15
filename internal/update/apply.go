package update

import (
	"fmt"
	"os"
)

type ApplyResult struct {
	FallbackPath string
}

type applyFileOps struct {
	writeFile func(string, []byte, os.FileMode) error
	rename    func(string, string) error
	remove    func(string) error
}

func defaultApplyFileOps() applyFileOps {
	return applyFileOps{
		writeFile: os.WriteFile,
		rename:    os.Rename,
		remove:    os.Remove,
	}
}

func applyUpdateAtPath(currentPath string, binary []byte, ops applyFileOps) (ApplyResult, error) {
	if ops.writeFile == nil {
		ops.writeFile = os.WriteFile
	}
	if ops.rename == nil {
		ops.rename = os.Rename
	}
	if ops.remove == nil {
		ops.remove = os.Remove
	}

	tempPath := currentPath + ".tmp"
	backupPath := currentPath + ".old"
	fallbackPath := currentPath + ".new"

	if err := ops.writeFile(tempPath, binary, 0o755); err != nil {
		return ApplyResult{}, fmt.Errorf("write temp binary: %w", err)
	}

	if err := ops.rename(currentPath, backupPath); err != nil {
		if fallbackErr := ops.writeFile(fallbackPath, binary, 0o755); fallbackErr != nil {
			return ApplyResult{}, fmt.Errorf("backup current binary: %v; write fallback binary: %w", err, fallbackErr)
		}
		_ = ops.remove(tempPath)
		return ApplyResult{FallbackPath: fallbackPath}, nil
	}

	if err := ops.rename(tempPath, currentPath); err != nil {
		_ = ops.rename(backupPath, currentPath)
		if fallbackErr := ops.writeFile(fallbackPath, binary, 0o755); fallbackErr != nil {
			return ApplyResult{}, fmt.Errorf("activate new binary: %v; write fallback binary: %w", err, fallbackErr)
		}
		_ = ops.remove(tempPath)
		return ApplyResult{FallbackPath: fallbackPath}, nil
	}

	_ = ops.remove(backupPath)
	return ApplyResult{}, nil
}
