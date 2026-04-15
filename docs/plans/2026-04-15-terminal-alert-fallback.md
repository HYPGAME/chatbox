# Terminal Alert Fallback Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Finalize the macOS Terminal alert fallback so alerts still fire when Terminal is backgrounded, while preserving current-tab suppression.

**Architecture:** Keep the current AppleScript tab/TTY detector for in-Terminal precision, but gate it with a lightweight `lsappinfo` frontmost-app check. Tighten ASN parsing so bundle resolution only matches real `lsappinfo list` entry headers.

**Tech Stack:** Go, macOS `lsappinfo`, AppleScript/`osascript`, Go tests

---

### Task 1: Lock down the ASN parsing edge case

**Files:**
- Modify: `internal/tui/terminal_alert_test.go`
- Modify: `internal/tui/terminal_alert_darwin.go`

**Step 1: Write the failing test**

Add a regression test showing that `parseBundleIDForASN` must ignore `parentASN=...` lines inside unrelated app blocks.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run 'TestParseBundleIDForASNIgnoresParentASNReferences'`

Expected: FAIL because the parser currently matches any line containing the target ASN.

**Step 3: Write minimal implementation**

- only start an ASN block when the current line is a real `lsappinfo list` entry header
- keep existing bundleID extraction behavior unchanged

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run 'TestParseBundleIDForASNIgnoresParentASNReferences'`

Expected: PASS

### Task 2: Verify the foreground-fallback behavior stays correct

**Files:**
- Modify: `internal/tui/terminal_alert_test.go`

**Step 1: Add focused behavior coverage**

Add or keep tests that verify:

- matching selected TTY suppresses alerts
- non-Terminal frontmost app allows alerts when AppleScript is unavailable

**Step 2: Run targeted tests**

Run: `go test ./internal/tui -run 'TestTerminalAppForegroundDetector|TestParseBundleIDForASN'`

Expected: PASS

### Task 3: Full verification and commit

**Files:**
- Modify: `internal/tui/terminal_alert_darwin.go`
- Modify: `internal/tui/terminal_alert_test.go`
- Add: `docs/plans/2026-04-15-terminal-alert-fallback-design.md`
- Add: `docs/plans/2026-04-15-terminal-alert-fallback.md`

**Step 1: Run broader verification**

Run: `go test ./internal/tui`

Expected: PASS

**Step 2: Run full suite**

Run: `go test ./...`

Expected: PASS

**Step 3: Commit**

```bash
git add internal/tui/terminal_alert_darwin.go internal/tui/terminal_alert_test.go docs/plans/2026-04-15-terminal-alert-fallback-design.md docs/plans/2026-04-15-terminal-alert-fallback.md
git commit -m "fix: harden terminal alert foreground fallback"
```
