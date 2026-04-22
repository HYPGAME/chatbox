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

var restartStarter = func(cmd *exec.Cmd) error {
	return cmd.Start()
}

func buildRestartCommand(spec RestartSpec) (*exec.Cmd, error) {
	if strings.TrimSpace(spec.Path) == "" {
		return nil, fmt.Errorf("restart executable path is required")
	}
	if len(spec.Args) == 0 {
		return nil, fmt.Errorf("restart arguments are required")
	}
	cmd := exec.Command(spec.Path, spec.Args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd, nil
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
	cmd, err := buildRestartCommand(spec)
	if err != nil {
		return err
	}
	return restartStarter(cmd)
}
