package update

import (
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/charmbracelet/x/term"
	"golang.org/x/sys/unix"
)

type RestartSpec struct {
	Path string
	Args []string
}

var restartStarter = func(path string, args []string, env []string) error {
	return syscall.Exec(path, append([]string{path}, args...), env)
}

var restartInputDrainer = drainRestartInput

func buildRestartInvocation(spec RestartSpec) (string, []string, error) {
	if strings.TrimSpace(spec.Path) == "" {
		return "", nil, fmt.Errorf("restart executable path is required")
	}
	if len(spec.Args) == 0 {
		return "", nil, fmt.Errorf("restart arguments are required")
	}
	return spec.Path, append([]string{spec.Path}, spec.Args...), nil
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
	path, argv, err := buildRestartInvocation(spec)
	if err != nil {
		return err
	}
	if len(argv) == 0 {
		return fmt.Errorf("restart arguments are required")
	}
	_ = restartInputDrainer()
	return restartStarter(path, argv[1:], os.Environ())
}

func drainRestartInput() error {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(uintptr(fd)) {
		return nil
	}
	if err := unix.SetNonblock(fd, true); err != nil {
		return nil
	}
	defer func() {
		_ = unix.SetNonblock(fd, false)
	}()
	buf := make([]byte, 256)
	for {
		_, err := unix.Read(fd, buf)
		if err == nil {
			continue
		}
		if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
			return nil
		}
		break
	}
	return nil
}
