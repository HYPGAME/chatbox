# Terminal.app Bell Alert Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add Terminal.app-native background bell alerts for new incoming messages in scrollback mode, only when the current chatbox tab is not frontmost.

**Architecture:** Extend the CLI and scrollback runtime with an alert mode and a small foreground-detector abstraction. On each live inbound message, scrollback mode will ask the detector whether the current Terminal.app tab is frontmost and emit `\a` only when it is not. All replay/system/ACK/retry flows remain silent.

**Tech Stack:** Go, existing CLI/TUI code, macOS `osascript`, Terminal tty detection

---

### Task 1: CLI alert mode selection

**Files:**
- Modify: `cmd/chatbox/main.go`
- Test: `cmd/chatbox/main_test.go`

**Step 1: Write the failing test**

Add tests that verify:

```go
func TestResolveAlertDefaultsToBell(t *testing.T) { ... }
func TestResolveAlertAcceptsOff(t *testing.T) { ... }
func TestResolveAlertRejectsUnknownMode(t *testing.T) { ... }
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/chatbox -run 'TestResolveAlert'`

Expected: FAIL because no alert parser exists.

**Step 3: Write minimal implementation**

Add `resolveAlert`, parse `--alert` for `host` and `join`, and pass the chosen mode into the UI layer.

**Step 4: Run test to verify it passes**

Run: `go test ./cmd/chatbox -run 'TestResolveAlert'`

Expected: PASS

**Step 5: Commit**

Run if repo is initialized:

```bash
git add cmd/chatbox/main.go cmd/chatbox/main_test.go
git commit -m "feat: add alert mode flag"
```

If git is not initialized, record that the commit step was skipped.

### Task 2: Alert abstraction and scrollback routing

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/scrollback.go`
- Test: `internal/tui/model_test.go`

**Step 1: Write the failing test**

Add tests that verify:

```go
func TestScrollbackAlertsOnlyForLiveInboundMessages(t *testing.T) { ... }
func TestScrollbackDoesNotAlertForTranscriptReplay(t *testing.T) { ... }
func TestScrollbackDoesNotAlertForOutgoingReceiptOrRetry(t *testing.T) { ... }
```

Use an injected fake notifier so the tests can count alert attempts without calling the OS.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run 'TestScrollback(AlertsOnlyForLiveInboundMessages|DoesNotAlertForTranscriptReplay|DoesNotAlertForOutgoingReceiptOrRetry)'`

Expected: FAIL because no alert routing exists.

**Step 3: Write minimal implementation**

- add alert mode to `modelOptions` / `model`
- add a notifier abstraction that can emit bell output
- trigger alert checks only from the real-time inbound message path
- do not call alert logic from transcript load, receipt, outgoing send, or retry marker paths

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run 'TestScrollback(AlertsOnlyForLiveInboundMessages|DoesNotAlertForTranscriptReplay|DoesNotAlertForOutgoingReceiptOrRetry)'`

Expected: PASS

**Step 5: Commit**

Run if repo is initialized:

```bash
git add internal/tui/model.go internal/tui/scrollback.go internal/tui/model_test.go
git commit -m "feat: route bell alerts for live inbound messages"
```

If git is not initialized, record that the commit step was skipped.

### Task 3: Terminal.app foreground detection

**Files:**
- Create: `internal/tui/terminal_alert_darwin.go`
- Create: `internal/tui/terminal_alert_other.go`
- Test: `internal/tui/terminal_alert_test.go`

**Step 1: Write the failing test**

Add tests around a detector abstraction, for example:

```go
func TestTerminalAppForegroundDetectorSuppressesWhenSelectedTTYMatches(t *testing.T) { ... }
func TestTerminalAppForegroundDetectorAllowsAlertWhenTerminalNotFrontmost(t *testing.T) { ... }
func TestTerminalAppForegroundDetectorFailsClosedOnScriptError(t *testing.T) { ... }
```

Design the detector so script execution is injected and testable.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run 'TestTerminalAppForegroundDetector'`

Expected: FAIL because no detector exists.

**Step 3: Write minimal implementation**

- on Darwin, implement a detector that:
  - captures current tty
  - invokes `osascript`
  - determines whether Terminal.app is frontmost and whether selected tab tty matches
- on non-Darwin, return a detector that never alerts
- fail closed on any detection error

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run 'TestTerminalAppForegroundDetector'`

Expected: PASS

**Step 5: Commit**

Run if repo is initialized:

```bash
git add internal/tui/terminal_alert_darwin.go internal/tui/terminal_alert_other.go internal/tui/terminal_alert_test.go
git commit -m "feat: detect terminal foreground tab for alerts"
```

If git is not initialized, record that the commit step was skipped.

### Task 4: End-to-end wiring and docs

**Files:**
- Modify: `cmd/chatbox/main.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/scrollback.go`
- Modify: `README.md`

**Step 1: Write the failing test**

Add or extend tests to verify:

```go
func TestRunHostPassesAlertModeToLauncher(t *testing.T) { ... }
func TestRunJoinPassesAlertModeToLauncher(t *testing.T) { ... }
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/chatbox -run 'TestRun(Host|Join)PassesAlertMode'`

Expected: FAIL because alert mode is not yet fully wired to the launcher.

**Step 3: Write minimal implementation**

- pass alert mode from CLI into UI construction
- hook the detector/notifier into scrollback startup
- document Terminal.app-only behavior and the need for Terminal bell settings

**Step 4: Run test to verify it passes**

Run: `go test ./cmd/chatbox -run 'TestRun(Host|Join)PassesAlertMode'`

Expected: PASS

**Step 5: Commit**

Run if repo is initialized:

```bash
git add cmd/chatbox/main.go internal/tui/model.go internal/tui/scrollback.go README.md
git commit -m "docs: document terminal bell alerts"
```

If git is not initialized, record that the commit step was skipped.

### Task 5: Full verification

**Files:**
- No code changes required unless verification finds defects.

**Step 1: Run complete test suite**

Run: `go test ./...`

Expected: PASS

**Step 2: Run race checks for sensitive packages**

Run: `go test -race ./internal/session ./internal/tui`

Expected: PASS

**Step 3: Rebuild the local binary**

Run: `go build -o ./chatbox ./cmd/chatbox`

Expected: PASS

**Step 4: Manual Terminal.app validation**

Run:

1. Start `./chatbox host --psk-file ... --alert bell`
2. Open a different Terminal.app tab
3. Send a message from peer
4. Confirm Terminal.app shows configured bell attention behavior
5. Return to the chatbox tab
6. Confirm Terminal clears the reminder normally

Expected: reminder appears only for background live inbound messages.

**Step 5: Commit**

Run if repo is initialized:

```bash
git add .
git commit -m "feat: add terminal bell alerts for background messages"
```

If git is not initialized, record that the commit step was skipped.
