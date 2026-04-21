package update

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type RestartSpec struct {
	Path string
	Args []string
}

var restartStarter = func(path string, args []string) error {
	cmd := exec.Command(path, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Start()
}

func BuildRestartSpec(executablePath string, startupArgs []string) (RestartSpec, error) {
	if strings.TrimSpace(executablePath) == "" {
		return RestartSpec{}, fmt.Errorf("restart executable path is required")
	}
	if len(startupArgs) == 0 || strings.TrimSpace(startupArgs[0]) != "join" {
		return RestartSpec{}, fmt.Errorf("restart is only supported for join")
	}
	return RestartSpec{
		Path: executablePath,
		Args: append([]string(nil), startupArgs...),
	}, nil
}

func LaunchRestart(spec RestartSpec) error {
	if strings.TrimSpace(spec.Path) == "" {
		return fmt.Errorf("restart executable path is required")
	}
	if len(spec.Args) == 0 {
		return fmt.Errorf("restart arguments are required")
	}
	return restartStarter(spec.Path, spec.Args)
}
