package update

import (
	"os"
	"os/exec"
	"testing"
)

func TestBuildRestartSpecPreservesJoinArguments(t *testing.T) {
	t.Parallel()

	spec, err := BuildRestartSpec("/tmp/chatbox", []string{
		"join",
		"--peer", "127.0.0.1:7331",
		"--psk-file", "/tmp/test.psk",
		"--name", "alice",
		"--ui", "tui",
	})
	if err != nil {
		t.Fatalf("BuildRestartSpec returned error: %v", err)
	}
	if spec.Path != "/tmp/chatbox" {
		t.Fatalf("expected restart path to be preserved, got %#v", spec)
	}
	if len(spec.Args) != 9 || spec.Args[0] != "join" {
		t.Fatalf("expected join args to be preserved, got %#v", spec)
	}
}

func TestBuildRestartSpecRejectsNonJoinCommands(t *testing.T) {
	t.Parallel()

	if _, err := BuildRestartSpec("/tmp/chatbox", []string{"host", "--listen", "0.0.0.0:7331"}); err == nil {
		t.Fatal("expected non-join restart to be rejected")
	}
}

func TestLaunchRestartUsesBuiltSpec(t *testing.T) {
	t.Parallel()

	previous := restartStarter
	defer func() { restartStarter = previous }()

	var gotPath string
	var gotArgs []string
	var gotStdin any
	restartStarter = func(cmd *exec.Cmd) error {
		gotPath = cmd.Path
		gotArgs = append([]string(nil), cmd.Args[1:]...)
		gotStdin = cmd.Stdin
		return nil
	}

	spec := RestartSpec{
		Path: "/tmp/chatbox",
		Args: []string{"join", "--peer", "127.0.0.1:7331"},
	}
	if err := LaunchRestart(spec); err != nil {
		t.Fatalf("LaunchRestart returned error: %v", err)
	}
	if gotPath != spec.Path {
		t.Fatalf("expected restart path %q, got %q", spec.Path, gotPath)
	}
	if len(gotArgs) != len(spec.Args) || gotArgs[0] != "join" {
		t.Fatalf("expected restart args %#v, got %#v", spec.Args, gotArgs)
	}
	if gotStdin != os.Stdin {
		t.Fatalf("expected restart stdin to be inherited, got %#v", gotStdin)
	}
}

func TestBuildRestartCommandInheritsTerminalStreams(t *testing.T) {
	t.Parallel()

	spec := RestartSpec{
		Path: "/tmp/chatbox",
		Args: []string{"join", "--peer", "127.0.0.1:7331"},
	}
	cmd, err := buildRestartCommand(spec)
	if err != nil {
		t.Fatalf("buildRestartCommand returned error: %v", err)
	}
	if cmd.Stdin != os.Stdin {
		t.Fatalf("expected restart stdin to inherit terminal stdin, got %#v", cmd.Stdin)
	}
	if cmd.Stdout != os.Stdout {
		t.Fatalf("expected restart stdout to inherit terminal stdout, got %#v", cmd.Stdout)
	}
	if cmd.Stderr != os.Stderr {
		t.Fatalf("expected restart stderr to inherit terminal stderr, got %#v", cmd.Stderr)
	}
}
