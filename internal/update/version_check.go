package update

import (
	"fmt"
	"os/exec"
	"strings"
)

func readBinaryVersion(path string) (string, error) {
	output, err := exec.Command(path, "version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("run %s version: %w", path, err)
	}
	return strings.TrimSpace(string(output)), nil
}
