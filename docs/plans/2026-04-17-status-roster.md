# Status Roster Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make `/status` show the current online participant roster for both host and joiners.

**Architecture:** Reuse the current encrypted message path with a hidden host-mediated control request/response. Keep the roster source authoritative on `HostRoom`, and render results in the UI as `system` lines.

**Tech Stack:** Go, Bubble Tea, existing room/session transport, Go test

---

### Task 1: Add failing tests for the new `/status` behavior

**Files:**
- Modify: `internal/tui/model_test.go`
- Modify: `internal/room/host_test.go`

**Step 1: Write the failing tests**

- Add a host-side `/status` test asserting the roster line contains all current participants.
- Add a joiner-side `/status` test asserting the command sends a hidden internal request instead of a visible chat line.
- Add a room test asserting a hidden status request is intercepted and answered only to the requesting member.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui ./internal/room -run 'Test(HostStatus|JoinStatus|HostRoomStatus)'`

Expected: FAIL because `/status` currently only prints local status and does not support hidden roster exchange.

### Task 2: Implement host roster snapshot and hidden control helpers

**Files:**
- Create: `internal/room/status_control.go`
- Modify: `internal/room/host.go`

**Step 1: Write minimal implementation**

- Add helpers for the hidden status request/response payloads.
- Add a sorted participant snapshot method on `HostRoom`.
- Intercept hidden status requests in `HostRoom` and reply only to the requester.
- Drain member receipts so request/response traffic cannot clog the host-side session.

**Step 2: Run targeted room tests**

Run: `go test ./internal/room`

Expected: PASS

### Task 3: Implement `/status` UI behavior for host and joiner

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/scrollback.go`

**Step 1: Write minimal implementation**

- Add a host roster callback to the model options.
- For host `/status`, print local status plus the roster line immediately.
- For joiner `/status`, send the hidden status request and keep it out of visible chat history.
- Intercept hidden status responses and render them as `system` lines.

**Step 2: Run targeted TUI tests**

Run: `go test ./internal/tui`

Expected: PASS

### Task 4: Verify end-to-end and rebuild local binary

**Files:**
- Modify: `./chatbox`

**Step 1: Run full verification**

Run: `go test ./...`

Expected: PASS

**Step 2: Rebuild local binary**

Run: `go build -ldflags "-X chatbox/internal/version.Version=v0.1.8" -o ./chatbox ./cmd/chatbox`

**Step 3: Verify binary version**

Run: `./chatbox version`

Expected: `v0.1.8`
