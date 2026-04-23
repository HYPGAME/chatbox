//go:build darwin

package attachment

import "os/exec"

func openFile(path string) error {
	return exec.Command("open", path).Run()
}
