//go:build !darwin

package attachment

import (
	"errors"
	"os/exec"
	"runtime"
)

func openFile(path string) error {
	if runtime.GOOS == "android" {
		if _, err := exec.LookPath("termux-open"); err == nil {
			return exec.Command("termux-open", path).Run()
		}
		return errors.New("termux-open is not available")
	}
	return exec.Command("xdg-open", path).Run()
}
