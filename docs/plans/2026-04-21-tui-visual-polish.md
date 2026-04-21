# TUI Visual Polish Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Improve the TUI chat appearance without changing protocol, storage, or command behavior.

**Architecture:** Keep rendering changes local to `internal/tui/model.go`. Add small formatting helpers for header/status, date separators, compact timestamps, command suggestions, and input frame styling. Preserve scrollback mode behavior and keep stripped text assertions stable where compatibility matters.

**Tech Stack:** Go, Bubble Tea, Bubbles viewport/textarea, Lip Gloss, Go tests.

---

### Task 1: Compact the TUI header into a status bar

**Files:**
- Modify: `internal/tui/model.go`
- Test: `internal/tui/model_test.go`

**Step 1: Write the failing test**

Add a test that renders a host model and expects a single top line containing `chatbox host`, the current status, and `/help`.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run TestModelRendersCompactStatusBar -count=1`

Expected: FAIL because header and status are currently separate lines.

**Step 3: Implement minimal rendering**

Replace the separate `header` and `status` lines in TUI mode with `renderStatusBar`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run TestModelRendersCompactStatusBar -count=1`

Expected: PASS

### Task 2: Add compact message timestamps and date separators

**Files:**
- Modify: `internal/tui/model.go`
- Test: `internal/tui/model_test.go`

**Step 1: Write the failing tests**

Add tests that expect:
- message lines use `[15:10] alice: hello`
- history spanning multiple dates includes `--- 2026-04-17 ---`

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run 'TestRenderEntryWithStatusUsesCompactTime|TestRefreshViewportAddsDateSeparators' -count=1`

Expected: FAIL because the renderer currently prints full timestamps on every line.

**Step 3: Implement minimal rendering**

Add a `renderEntryCompact` helper for TUI mode and insert muted date separator lines in `refreshViewport`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run 'TestRenderEntryWithStatusUsesCompactTime|TestRefreshViewportAddsDateSeparators' -count=1`

Expected: PASS

### Task 3: Render slash suggestions as a command panel

**Files:**
- Modify: `internal/tui/model.go`
- Test: `internal/tui/model_test.go`

**Step 1: Write the failing test**

Add a test that expects `/` suggestions to include a `commands` title and still show `/status -- 查询在线成员信息`.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run TestSlashCommandSuggestionsRenderAsPanel -count=1`

Expected: FAIL because suggestions are currently plain lines.

**Step 3: Implement minimal rendering**

Wrap suggestions in a Lip Gloss panel with a `commands` title.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run TestSlashCommandSuggestionsRenderAsPanel -count=1`

Expected: PASS

### Task 4: Make the input area read as an input box

**Files:**
- Modify: `internal/tui/model.go`
- Test: `internal/tui/model_test.go`

**Step 1: Write the failing test**

Add a test that expects the TUI view to include `Enter send` help text near the input.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run TestInputAreaShowsSendHint -count=1`

Expected: FAIL because input currently only has a top border.

**Step 3: Implement minimal rendering**

Add a small input box renderer that includes `Enter send / Esc quit` beneath the textarea.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run TestInputAreaShowsSendHint -count=1`

Expected: PASS

### Task 5: Verify all TUI and project tests

**Files:**
- Modify: none

**Step 1: Run focused TUI tests**

Run: `go test ./internal/tui -count=1`

Expected: PASS

**Step 2: Run full test suite**

Run: `go test ./... -count=1`

Expected: PASS
