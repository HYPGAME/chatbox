package update

import (
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
	var gotEnv []string
	restartStarter = func(path string, args []string, env []string) error {
		gotPath = path
		gotArgs = append([]string(nil), args...)
		gotEnv = append([]string(nil), env...)
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
	if len(gotEnv) == 0 {
		t.Fatal("expected restart to inherit environment")
	}
}

func TestBuildRestartInvocationUsesExecutablePathAsArgv0(t *testing.T) {
	t.Parallel()

	spec := RestartSpec{
		Path: "/tmp/chatbox",
		Args: []string{"join", "--peer", "127.0.0.1:7331"},
	}
	path, argv, err := buildRestartInvocation(spec)
	if err != nil {
		t.Fatalf("buildRestartInvocation returned error: %v", err)
	}
	if path != spec.Path {
		t.Fatalf("expected restart path %q, got %q", spec.Path, path)
	}
	if len(argv) != len(spec.Args)+1 {
		t.Fatalf("expected argv to include argv0 plus args, got %#v", argv)
	}
	if argv[0] != spec.Path {
		t.Fatalf("expected argv0 to be executable path, got %#v", argv)
	}
	if argv[1] != "join" {
		t.Fatalf("expected restart args to follow argv0, got %#v", argv)
	}
}
