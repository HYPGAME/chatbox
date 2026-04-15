# Scrollback UI Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make terminal-native scrollback the default chat UI while keeping the existing full-screen TUI available as an explicit option.

**Architecture:** Split UI launch into selectable modes. Keep the existing Bubble Tea viewport model for `tui` mode, and add a new non-alt-screen `scrollback` mode that prints chat history as unmanaged terminal lines so the terminal scrollbar and wheel can access past messages. Reuse the same session, transcript, ack, and resend behavior underneath the UI mode boundary.

**Tech Stack:** Go, Bubble Tea, Bubbles textarea/viewport, existing session/transcript packages

---

### Task 1: UI mode selection

**Files:**
- Modify: `cmd/chatbox/main.go`
- Test: `cmd/chatbox/main_test.go`

**Step 1: Write the failing test**

```go
func TestRunHostDefaultsToScrollbackUI(t *testing.T) { ... }
func TestRunHostAcceptsExplicitTUI(t *testing.T) { ... }
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/chatbox -run 'TestRun(Host|Join).*UI'`
Expected: FAIL because host/join do not parse `--ui` and do not route to the correct launcher.

**Step 3: Write minimal implementation**

Add `--ui` with default `scrollback`, validate `scrollback|tui`, and pass the selection into the TUI package launcher.

**Step 4: Run test to verify it passes**

Run: `go test ./cmd/chatbox -run 'TestRun(Host|Join).*UI'`
Expected: PASS

### Task 2: Scrollback rendering behavior

**Files:**
- Modify: `internal/tui/model.go`
- Test: `internal/tui/model_test.go`

**Step 1: Write the failing test**

```go
func TestProgramOptionsUseAltScreenOnlyForTUI(t *testing.T) { ... }
func TestScrollbackSessionReadyPrintsTranscriptAndNewMessages(t *testing.T) { ... }
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run 'Test(ProgramOptionsUseAltScreenOnlyForTUI|ScrollbackSessionReadyPrintsTranscriptAndNewMessages)'`
Expected: FAIL because the program always enables alt screen and has no scrollback print path.

**Step 3: Write minimal implementation**

Extract UI mode aware program options, add a scrollback-oriented model path that emits unmanaged terminal output for history, transcript replay, system status, and new messages, and leave the existing viewport behavior intact for `tui`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run 'Test(ProgramOptionsUseAltScreenOnlyForTUI|ScrollbackSessionReadyPrintsTranscriptAndNewMessages)'`
Expected: PASS

### Task 3: Documentation and regression verification

**Files:**
- Modify: `README.md`

**Step 1: Write the failing test**

No code test. Use verification-only task after implementation.

**Step 2: Run focused verification**

Run: `go test ./...`
Expected: PASS

**Step 3: Run build verification**

Run: `GOOS=darwin GOARCH=arm64 go build -o /tmp/chatbox-arm64 ./cmd/chatbox`
Expected: PASS

**Step 4: Document behavior**

Update README usage examples and mention `--ui tui` for the old full-screen interface.
