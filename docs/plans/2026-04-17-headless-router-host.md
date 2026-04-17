# Headless Router Host and Stable Auto-Update Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a true non-interactive host mode for router deployment and support daily stable self-updates on `linux/arm64`.

**Architecture:** Introduce a dedicated headless host runtime that starts `HostRoom`, drains message and receipt channels without printing chat content, and logs only system-level lifecycle events. Keep daily update orchestration outside the main process by extending stable release/self-update support to `linux/arm64` and supplying a router-local cron script that restarts the service only after a real version change.

**Tech Stack:** Go, existing `internal/session` and `internal/room` packages, Go test, GitHub Releases, OpenWrt/iStoreOS shell scripts, cron

---

### Task 1: Add failing CLI tests for headless host mode

**Files:**
- Modify: `cmd/chatbox/main_test.go`
- Modify: `cmd/chatbox/main.go`

**Step 1: Write the failing tests**

- Add a test showing `chatbox host --headless` dispatches to a dedicated headless runner instead of the UI runner.
- Add a test showing `--headless` rejects `--ui`.
- Add a test showing `run()` skips the background update notifier for headless host invocations.

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/chatbox -run 'TestRun(HostHeadless|SkipsBackgroundUpdateCheckForHeadlessHost)|TestRunHostRejectsHeadlessWithUI'`

Expected: FAIL because there is no headless runner, no `--headless` parsing, and no headless-specific background-check bypass.

**Step 3: Write minimal implementation**

- Add a `runHostHeadless` launcher variable alongside the existing UI launcher.
- Add `--headless` parsing and validation in `runHost`.
- Add a helper that decides whether startup update checks should run, and make headless host skip them.

**Step 4: Run test to verify it passes**

Run: `go test ./cmd/chatbox -run 'TestRun(HostHeadless|SkipsBackgroundUpdateCheckForHeadlessHost)|TestRunHostRejectsHeadlessWithUI'`

Expected: PASS

**Step 5: Commit**

```bash
git add cmd/chatbox/main.go cmd/chatbox/main_test.go
git commit -m "feat: add headless host cli mode"
```

### Task 2: Build the headless host runtime

**Files:**
- Create: `internal/headless/host.go`
- Create: `internal/headless/host_test.go`
- Modify: `cmd/chatbox/main.go`

**Step 1: Write the failing tests**

- Add a test showing the headless runtime logs startup and join/leave events.
- Add a test showing it drains message and receipt channels without emitting message bodies.
- Add a test showing it exits cleanly when the context is cancelled or the host closes.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/headless -run 'TestHeadlessHost(RuntimeLogsLifecycle|DoesNotLogChatBodies|StopsOnContextCancel)'`

Expected: FAIL because the `internal/headless` package does not exist.

**Step 3: Write minimal implementation**

- Start a `room.HostRoom`, call `Serve`, and watch its `Events`, `Messages`, `Receipts`, and `Done` channels.
- Log startup, shutdown, and join/leave events to an injected writer.
- Drain `Messages` and `Receipts` silently so the host relay cannot stall on buffered channels.
- Return errors only for real startup/runtime failures.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/headless -run 'TestHeadlessHost(RuntimeLogsLifecycle|DoesNotLogChatBodies|StopsOnContextCancel)'`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/headless/host.go internal/headless/host_test.go cmd/chatbox/main.go
git commit -m "feat: add headless host runtime"
```

### Task 3: Extend stable release and self-update support to linux arm64

**Files:**
- Modify: `internal/update/release.go`
- Modify: `internal/update/release_test.go`
- Modify: `internal/update/client.go`
- Modify: `internal/update/client_test.go`
- Modify: `internal/release/manual.go`
- Modify: `internal/release/manual_test.go`
- Modify: `.github/workflows/release.yml`
- Modify: `scripts/release-manual.sh`

**Step 1: Write the failing tests**

- Add a release-selection test for `linux/arm64`.
- Add a self-update test that downloads and applies `chatbox_linux_arm64.tar.gz`.
- Add a release-artifact test showing versioned releases include the Linux ARM64 archive.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/update ./internal/release -run 'Test(SelectAssetNameSupportsLinuxArm64|SelfUpdateDownloadsAndAppliesLatestLinuxArm64Release|ReleaseArtifactsIncludeLinuxArm64Archive)'`

Expected: FAIL because the updater and release metadata do not yet know about `linux/arm64`.

**Step 3: Write minimal implementation**

- Teach `selectAssetName()` about `linux/arm64`.
- Extend redirect-based latest-release fallback assets to include the Linux ARM64 archive.
- Update release metadata, GitHub Actions, and the manual release script so stable releases publish `chatbox_linux_arm64.tar.gz`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/update ./internal/release -run 'Test(SelectAssetNameSupportsLinuxArm64|SelfUpdateDownloadsAndAppliesLatestLinuxArm64Release|ReleaseArtifactsIncludeLinuxArm64Archive)'`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/update/release.go internal/update/release_test.go internal/update/client.go internal/update/client_test.go internal/release/manual.go internal/release/manual_test.go .github/workflows/release.yml scripts/release-manual.sh
git commit -m "feat: support linux arm64 self-update"
```

### Task 4: Add router auto-update scripts and documentation

**Files:**
- Create: `scripts/router/chatbox-openwrt-autoupdate.sh`
- Create: `scripts/router/chatbox-openwrt-cron.txt`
- Modify: `README.md`

**Step 1: Write the failing docs/check expectation**

- Add a README check target in the plan by searching for `headless`, `linux_arm64`, and router auto-update instructions after the docs change.

**Step 2: Write minimal implementation**

- Add an OpenWrt-friendly update script that:
  - uses a lock file under `/tmp`
  - captures `before` and `after` versions
  - runs `chatbox self-update`
  - restarts `/etc/init.d/chatbox` only when the version actually changes
  - exits without restart when the updater reports a fallback file or any error
- Add a sample cron line that runs once per day.
- Document `host --headless`, Linux ARM64 release/update support, and the router auto-update flow in the README.

**Step 3: Run verification checks**

Run: `rg -n 'headless|chatbox_linux_arm64|auto-update|cron|linux/arm64' README.md scripts/router`

Expected: matching lines for the new headless and router-update documentation.

**Step 4: Commit**

```bash
git add scripts/router/chatbox-openwrt-autoupdate.sh scripts/router/chatbox-openwrt-cron.txt README.md
git commit -m "docs: add router headless update guidance"
```

### Task 5: Run full verification and rebuild router deployable binary

**Files:**
- Modify: `./chatbox`

**Step 1: Run full verification**

Run: `go test ./...`

Expected: PASS

**Step 2: Build the local developer binary**

Run: `go build -o ./chatbox ./cmd/chatbox`

Expected: exit code 0

**Step 3: Build the router deployment binary**

Run: `GOOS=linux GOARCH=arm64 go build -o ./dist/chatbox_linux_arm64/chatbox ./cmd/chatbox`

Expected: exit code 0

**Step 4: Verify the router binary format**

Run: `file ./dist/chatbox_linux_arm64/chatbox`

Expected: output includes `ELF 64-bit` and `ARM aarch64`

**Step 5: Commit**

```bash
git add .github/workflows/release.yml scripts/release-manual.sh README.md scripts/router cmd/chatbox internal/headless internal/update internal/release
git commit -m "feat: add headless router host updates"
```
