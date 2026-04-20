package router_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenWrtAutoUpdateRetriesWithOpenClashBypass(t *testing.T) {
	t.Parallel()

	scriptPath, err := filepath.Abs("chatbox-openwrt-autoupdate.sh")
	if err != nil {
		t.Fatalf("resolve script path: %v", err)
	}

	tempDir := t.TempDir()
	binDir := filepath.Join(tempDir, "bin")
	initdDir := filepath.Join(tempDir, "init.d")
	stateDir := filepath.Join(tempDir, "state")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("create bin dir: %v", err)
	}
	if err := os.MkdirAll(initdDir, 0o755); err != nil {
		t.Fatalf("create initd dir: %v", err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("create state dir: %v", err)
	}

	mustWriteExecutable(t, filepath.Join(binDir, "chatbox"), `#!/bin/sh
set -eu
state="${CHATBOX_STATE_DIR:?}"
case "${1:-}" in
version)
	cat "$state/version"
	;;
self-update)
	count_file="$state/self-update-count"
	count=0
	if [ -f "$count_file" ]; then
		count="$(cat "$count_file")"
	fi
	count=$((count + 1))
	printf '%s' "$count" > "$count_file"
	if [ "$count" -eq 1 ]; then
		echo 'fetch latest release redirect: EOF' >&2
		exit 1
	fi
	printf '%s\n' 'v0.1.9' > "$state/version"
	printf '%s\n' 'updated to v0.1.9'
	;;
*)
	echo "unexpected command: $*" >&2
	exit 99
	;;
esac
`)
	mustWriteExecutable(t, filepath.Join(binDir, "uci"), `#!/bin/sh
set -eu
state="${CHATBOX_STATE_DIR:?}"
if [ "${1:-}" = "-q" ]; then
	shift
fi
if [ "${1:-}" != "get" ]; then
	exit 1
fi
case "${2:-}" in
openclash.config.enable)
	cat "$state/openclash-enable"
	;;
openclash.config.router_self_proxy)
	cat "$state/router-self-proxy"
	;;
*)
	exit 1
	;;
esac
`)
	mustWriteExecutable(t, filepath.Join(binDir, "service"), `#!/bin/sh
set -eu
printf '%s %s\n' "${1:-}" "${2:-}" >> "${CHATBOX_STATE_DIR:?}/service.log"
`)
	mustWriteExecutable(t, filepath.Join(binDir, "sleep"), `#!/bin/sh
exit 0
`)
	mustWriteExecutable(t, filepath.Join(binDir, "logger"), `#!/bin/sh
exit 0
`)
	mustWriteExecutable(t, filepath.Join(binDir, "curl"), `#!/bin/sh
exit 0
`)
	mustWriteExecutable(t, filepath.Join(initdDir, "chatbox"), `#!/bin/sh
set -eu
printf '%s\n' "${1:-}" >> "${CHATBOX_STATE_DIR:?}/chatbox-init.log"
`)

	mustWriteFile(t, filepath.Join(stateDir, "version"), []byte("v0.1.8\n"), 0o644)
	mustWriteFile(t, filepath.Join(stateDir, "openclash-enable"), []byte("1\n"), 0o644)
	mustWriteFile(t, filepath.Join(stateDir, "router-self-proxy"), []byte("1\n"), 0o644)

	cmd := exec.Command("/bin/sh", scriptPath)
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"CHATBOX_BIN="+filepath.Join(binDir, "chatbox"),
		"CHATBOX_INITD_DIR="+initdDir,
		"CHATBOX_STATE_DIR="+stateDir,
		"LOCKDIR="+filepath.Join(tempDir, "lock"),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run script: %v\n%s", err, output)
	}

	serviceLog := mustReadFile(t, filepath.Join(stateDir, "service.log"))
	if !strings.Contains(serviceLog, "openclash stop") || !strings.Contains(serviceLog, "openclash start") {
		t.Fatalf("expected openclash stop/start during retry, got %q", serviceLog)
	}

	initLog := mustReadFile(t, filepath.Join(stateDir, "chatbox-init.log"))
	if strings.TrimSpace(initLog) != "restart" {
		t.Fatalf("expected chatbox service restart, got %q", initLog)
	}

	version := strings.TrimSpace(mustReadFile(t, filepath.Join(stateDir, "version")))
	if version != "v0.1.9" {
		t.Fatalf("expected updated version, got %q", version)
	}
}

func TestOpenWrtAutoUpdateRecoversFromStaleLock(t *testing.T) {
	t.Parallel()

	scriptPath, err := filepath.Abs("chatbox-openwrt-autoupdate.sh")
	if err != nil {
		t.Fatalf("resolve script path: %v", err)
	}

	tempDir := t.TempDir()
	binDir := filepath.Join(tempDir, "bin")
	initdDir := filepath.Join(tempDir, "init.d")
	stateDir := filepath.Join(tempDir, "state")
	lockDir := filepath.Join(tempDir, "lock")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("create bin dir: %v", err)
	}
	if err := os.MkdirAll(initdDir, 0o755); err != nil {
		t.Fatalf("create initd dir: %v", err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("create state dir: %v", err)
	}
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatalf("create lock dir: %v", err)
	}

	mustWriteExecutable(t, filepath.Join(binDir, "chatbox"), `#!/bin/sh
set -eu
state="${CHATBOX_STATE_DIR:?}"
case "${1:-}" in
version)
	cat "$state/version"
	;;
self-update)
	printf '%s\n' 'chatbox is already up to date (v0.1.9)'
	;;
*)
	echo "unexpected command: $*" >&2
	exit 99
	;;
esac
`)
	mustWriteExecutable(t, filepath.Join(initdDir, "chatbox"), `#!/bin/sh
exit 0
`)
	mustWriteFile(t, filepath.Join(stateDir, "version"), []byte("v0.1.9\n"), 0o644)
	mustWriteFile(t, filepath.Join(lockDir, "pid"), []byte("999999\n"), 0o644)

	cmd := exec.Command("/bin/sh", scriptPath)
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"CHATBOX_BIN="+filepath.Join(binDir, "chatbox"),
		"CHATBOX_INITD_DIR="+initdDir,
		"CHATBOX_STATE_DIR="+stateDir,
		"LOCKDIR="+lockDir,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run script with stale lock: %v\n%s", err, output)
	}
	if strings.Contains(string(output), "another update is already running") {
		t.Fatalf("expected stale lock recovery, got %q", output)
	}

	if _, err := os.Stat(lockDir); !os.IsNotExist(err) {
		t.Fatalf("expected lock dir to be removed, stat err=%v", err)
	}
}

func TestOpenWrtAutoUpdateRestoresDNSBeforeBypassRetry(t *testing.T) {
	t.Parallel()

	scriptPath, err := filepath.Abs("chatbox-openwrt-autoupdate.sh")
	if err != nil {
		t.Fatalf("resolve script path: %v", err)
	}

	tempDir := t.TempDir()
	binDir := filepath.Join(tempDir, "bin")
	initdDir := filepath.Join(tempDir, "init.d")
	stateDir := filepath.Join(tempDir, "state")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("create bin dir: %v", err)
	}
	if err := os.MkdirAll(initdDir, 0o755); err != nil {
		t.Fatalf("create initd dir: %v", err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("create state dir: %v", err)
	}

	mustWriteExecutable(t, filepath.Join(binDir, "chatbox"), `#!/bin/sh
set -eu
state="${CHATBOX_STATE_DIR:?}"
case "${1:-}" in
version)
	cat "$state/version"
	;;
self-update)
	count_file="$state/self-update-count"
	count=0
	if [ -f "$count_file" ]; then
		count="$(cat "$count_file")"
	fi
	count=$((count + 1))
	printf '%s' "$count" > "$count_file"
	if [ "$count" -eq 1 ]; then
		echo 'fetch latest release redirect: EOF' >&2
		exit 1
	fi
	printf '%s\n' 'chatbox is already up to date (v0.1.11)'
	;;
*)
	echo "unexpected command: $*" >&2
	exit 99
	;;
esac
`)
	mustWriteExecutable(t, filepath.Join(binDir, "uci"), `#!/bin/sh
set -eu
state="${CHATBOX_STATE_DIR:?}"
if [ "${1:-}" = "-q" ]; then
	shift
fi
if [ "${1:-}" != "get" ]; then
	exit 1
fi
case "${2:-}" in
openclash.config.enable)
	cat "$state/openclash-enable"
	;;
openclash.config.router_self_proxy)
	cat "$state/router-self-proxy"
	;;
*)
	exit 1
	;;
esac
`)
	mustWriteExecutable(t, filepath.Join(binDir, "service"), `#!/bin/sh
set -eu
state="${CHATBOX_STATE_DIR:?}"
printf '%s %s\n' "${1:-}" "${2:-}" >> "$state/service.log"
if [ "${1:-}" = "dnsmasq" ] && [ "${2:-}" = "restart" ]; then
	printf '%s\n' '1' > "$state/dns-restored"
fi
`)
	mustWriteExecutable(t, filepath.Join(binDir, "sleep"), `#!/bin/sh
exit 0
`)
	mustWriteExecutable(t, filepath.Join(binDir, "logger"), `#!/bin/sh
exit 0
`)
	mustWriteExecutable(t, filepath.Join(binDir, "curl"), `#!/bin/sh
exit 0
`)
	mustWriteExecutable(t, filepath.Join(initdDir, "chatbox"), `#!/bin/sh
exit 0
`)

	mustWriteFile(t, filepath.Join(stateDir, "version"), []byte("v0.1.11\n"), 0o644)
	mustWriteFile(t, filepath.Join(stateDir, "openclash-enable"), []byte("1\n"), 0o644)
	mustWriteFile(t, filepath.Join(stateDir, "router-self-proxy"), []byte("1\n"), 0o644)

	cmd := exec.Command("/bin/sh", scriptPath)
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"CHATBOX_BIN="+filepath.Join(binDir, "chatbox"),
		"CHATBOX_INITD_DIR="+initdDir,
		"CHATBOX_STATE_DIR="+stateDir,
		"LOCKDIR="+filepath.Join(tempDir, "lock"),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run script: %v\n%s", err, output)
	}

	serviceLog := mustReadFile(t, filepath.Join(stateDir, "service.log"))
	stopIndex := strings.Index(serviceLog, "openclash stop")
	dnsRestartIndex := strings.Index(serviceLog, "dnsmasq restart")
	if stopIndex == -1 || dnsRestartIndex == -1 {
		t.Fatalf("expected both openclash stop and dnsmasq restart, got %q", serviceLog)
	}
	if dnsRestartIndex < stopIndex {
		t.Fatalf("expected dnsmasq restart after openclash stop, got %q", serviceLog)
	}
}

func TestOpenWrtAutoUpdateWaitsForProbeBeforeBypassRetry(t *testing.T) {
	t.Parallel()

	scriptPath, err := filepath.Abs("chatbox-openwrt-autoupdate.sh")
	if err != nil {
		t.Fatalf("resolve script path: %v", err)
	}

	tempDir := t.TempDir()
	binDir := filepath.Join(tempDir, "bin")
	initdDir := filepath.Join(tempDir, "init.d")
	stateDir := filepath.Join(tempDir, "state")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("create bin dir: %v", err)
	}
	if err := os.MkdirAll(initdDir, 0o755); err != nil {
		t.Fatalf("create initd dir: %v", err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("create state dir: %v", err)
	}

	mustWriteExecutable(t, filepath.Join(binDir, "chatbox"), `#!/bin/sh
set -eu
state="${CHATBOX_STATE_DIR:?}"
case "${1:-}" in
version)
	cat "$state/version"
	;;
self-update)
	count_file="$state/self-update-count"
	count=0
	if [ -f "$count_file" ]; then
		count="$(cat "$count_file")"
	fi
	count=$((count + 1))
	printf '%s' "$count" > "$count_file"
	if [ "$count" -eq 1 ]; then
		echo 'fetch latest release redirect: EOF' >&2
		exit 1
	fi
	printf '%s\n' 'chatbox is already up to date (v0.1.11)'
	;;
*)
	echo "unexpected command: $*" >&2
	exit 99
	;;
esac
`)
	mustWriteExecutable(t, filepath.Join(binDir, "uci"), `#!/bin/sh
set -eu
state="${CHATBOX_STATE_DIR:?}"
if [ "${1:-}" = "-q" ]; then
	shift
fi
if [ "${1:-}" != "get" ]; then
	exit 1
fi
case "${2:-}" in
openclash.config.enable)
	cat "$state/openclash-enable"
	;;
openclash.config.router_self_proxy)
	cat "$state/router-self-proxy"
	;;
*)
	exit 1
	;;
esac
`)
	mustWriteExecutable(t, filepath.Join(binDir, "service"), `#!/bin/sh
set -eu
printf '%s %s\n' "${1:-}" "${2:-}" >> "${CHATBOX_STATE_DIR:?}/service.log"
`)
	mustWriteExecutable(t, filepath.Join(binDir, "sleep"), `#!/bin/sh
exit 0
`)
	mustWriteExecutable(t, filepath.Join(binDir, "logger"), `#!/bin/sh
exit 0
`)
	mustWriteExecutable(t, filepath.Join(binDir, "curl"), `#!/bin/sh
set -eu
state="${CHATBOX_STATE_DIR:?}"
count_file="$state/curl-count"
count=0
if [ -f "$count_file" ]; then
	count="$(cat "$count_file")"
fi
count=$((count + 1))
printf '%s' "$count" > "$count_file"
if [ "$count" -lt 3 ]; then
	exit 1
fi
exit 0
`)
	mustWriteExecutable(t, filepath.Join(binDir, "wget"), `#!/bin/sh
set -eu
state="${CHATBOX_STATE_DIR:?}"
count_file="$state/wget-count"
count=0
if [ -f "$count_file" ]; then
	count="$(cat "$count_file")"
fi
count=$((count + 1))
printf '%s' "$count" > "$count_file"
if [ "$count" -lt 3 ]; then
	exit 1
fi
exit 0
`)
	mustWriteExecutable(t, filepath.Join(initdDir, "chatbox"), `#!/bin/sh
exit 0
`)

	mustWriteFile(t, filepath.Join(stateDir, "version"), []byte("v0.1.11\n"), 0o644)
	mustWriteFile(t, filepath.Join(stateDir, "openclash-enable"), []byte("1\n"), 0o644)
	mustWriteFile(t, filepath.Join(stateDir, "router-self-proxy"), []byte("1\n"), 0o644)

	cmd := exec.Command("/bin/sh", scriptPath)
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"CHATBOX_BIN="+filepath.Join(binDir, "chatbox"),
		"CHATBOX_INITD_DIR="+initdDir,
		"CHATBOX_STATE_DIR="+stateDir,
		"LOCKDIR="+filepath.Join(tempDir, "lock"),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run script: %v\n%s", err, output)
	}

	if got := strings.TrimSpace(mustReadFile(t, filepath.Join(stateDir, "curl-count"))); got != "3" {
		t.Fatalf("expected 3 probe attempts before retry, got %q", got)
	}
}

func mustWriteExecutable(t *testing.T, path string, content string) {
	t.Helper()
	mustWriteFile(t, path, []byte(content), 0o755)
}

func mustWriteFile(t *testing.T, path string, content []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, content, mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(payload)
}
