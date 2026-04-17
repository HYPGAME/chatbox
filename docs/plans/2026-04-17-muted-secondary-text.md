# Muted Secondary Text Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Dim timestamps and `system`/`error` lines while preserving sender-name emphasis and message-body readability.

**Architecture:** Add dedicated secondary-text styles in the TUI renderer and update message/system/error formatting to use them. Cover the behavior with focused rendering tests and keep semantic tests ANSI-tolerant.

**Tech Stack:** Go, Bubble Tea, Lip Gloss, Go test

---

### Task 1: Add failing rendering tests

**Files:**
- Modify: `internal/tui/model_test.go`

**Step 1: Write the failing tests**

- Add a test asserting rendered message timestamps are ANSI-styled separately from sender/body text.
- Add a test asserting `system` and `error` entries no longer render as plain/default bright output.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run 'TestRenderEntryWithStatus(ColorsOnlySenderLabel|UsesMutedTimestampAndSecondaryLines)'`

Expected: FAIL because timestamps and secondary lines are not yet rendered with the new muted styles.

### Task 2: Implement muted secondary styles

**Files:**
- Modify: `internal/tui/model.go`

**Step 1: Write minimal implementation**

- Introduce dedicated muted styles for timestamps and `system`/`error` lines.
- Render timestamps separately from sender/body text.
- Render `system` and `error` lines with dimmer dedicated styles.

**Step 2: Run targeted tests to verify they pass**

Run: `go test ./internal/tui -run 'TestRenderEntryWithStatus(ColorsOnlySenderLabel|UsesMutedTimestampAndSecondaryLines)'`

Expected: PASS

### Task 3: Verify existing behavior remains intact

**Files:**
- Modify: `internal/tui/model_test.go`

**Step 1: Update any brittle assertions**

- Keep semantic assertions based on stripped ANSI output where needed.

**Step 2: Run package tests**

Run: `go test ./internal/tui`

Expected: PASS

### Task 4: Rebuild local binary

**Files:**
- Modify: `./chatbox`

**Step 1: Rebuild local binary in place**

Run: `go build -ldflags "-X chatbox/internal/version.Version=v0.1.7" -o ./chatbox ./cmd/chatbox`

**Step 2: Verify binary**

Run: `./chatbox version`

Expected: `v0.1.7`
