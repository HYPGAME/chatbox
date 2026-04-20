# TUI Slash Command Suggestions Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Show slash-command suggestions with short descriptions while typing `/` commands in TUI mode.

**Architecture:** Keep the feature inside the TUI model/view layer. Add static command metadata plus a small filter helper, then render a suggestion block only when the current input begins with `/` in `uiModeTUI`.

**Tech Stack:** Go, Bubble Tea, Bubbles textarea/viewport, existing TUI model/view tests

---

### Task 1: Add failing TUI view tests

**Files:**
- Modify: `internal/tui/model_test.go`
- Reference: `internal/tui/model.go`

**Step 1: Write the failing test**

Add tests for:

- `/` shows all three commands with descriptions
- `/st` shows only `/status -- 查询在线成员信息`
- normal text hides the suggestion block
- scrollback mode hides the suggestion block

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run 'TestModelShowsSlashCommandSuggestions|TestScrollbackModeHidesSlashCommandSuggestions' -count=1`

Expected: FAIL because the current view does not render any suggestion block.

**Step 3: Commit**

```bash
git add internal/tui/model_test.go
git commit -m "test: cover tui slash command suggestions"
```

### Task 2: Implement TUI-only slash suggestion rendering

**Files:**
- Modify: `internal/tui/model.go`
- Test: `internal/tui/model_test.go`

**Step 1: Write minimal implementation**

Add:

- static slash command metadata with description text
- helper to match current input prefix
- view rendering block in TUI mode between status and viewport/input

Keep behavior read-only:

- no selection
- no new key bindings
- no submit behavior changes

**Step 2: Run focused tests**

Run: `go test ./internal/tui -run 'TestModelShowsSlashCommandSuggestions|TestScrollbackModeHidesSlashCommandSuggestions' -count=1`

Expected: PASS

**Step 3: Run broader TUI regression tests**

Run: `go test ./internal/tui -count=1`

Expected: PASS

**Step 4: Commit**

```bash
git add internal/tui/model.go internal/tui/model_test.go
git commit -m "feat: add tui slash command suggestions"
```

### Task 3: Verify repo-wide safety

**Files:**
- Modify: `README.md` if user-facing docs need command-entry note
- Verify: repo test suite

**Step 1: Decide whether docs change is needed**

Only document if the feature is visible enough to mention now. Skip if it would add noise.

**Step 2: Run full verification**

Run: `go test ./...`

Expected: PASS

**Step 3: Final commit if docs changed**

```bash
git add README.md
git commit -m "docs: mention tui slash command suggestions"
```
