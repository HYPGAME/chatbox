# Offline History Sync Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Let modern clients recover authorized room history from other online clients while keeping the router host stateless for transcripts.

**Architecture:** Add a stable local identity, room-level authorization metadata, and a hidden control-message sync protocol layered on top of the existing encrypted room transport. Older clients remain compatible because sync traffic is only sent after an explicit capability announcement from another modern client.

**Tech Stack:** Go, existing `session`/`room` message transport, existing encrypted `transcript` store, local JSON metadata files, Bubble Tea TUI/scrollback UI

---

### Task 1: Add local identity persistence

**Files:**
- Create: `internal/identity/store.go`
- Create: `internal/identity/store_test.go`
- Modify: `cmd/chatbox/main.go`
- Modify: `internal/tui/model.go`

**Step 1: Write the failing tests**

Add tests covering:

- creating a new identity file when none exists
- reloading an existing identity and preserving the same `identity_id`
- rejecting malformed identity files

**Step 2: Run test to verify it fails**

Run: `go test ./internal/identity -run TestIdentity -count=1`
Expected: FAIL because the package does not exist yet.

**Step 3: Write minimal implementation**

Implement:

- stable identity record with `identity_id`
- local config path resolution
- create-if-missing behavior with `0600` permissions

Keep the first version simple and file-based.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/identity -run TestIdentity -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/identity/store.go internal/identity/store_test.go cmd/chatbox/main.go internal/tui/model.go
git commit -m "feat: add local identity persistence"
```

### Task 2: Add room authorization metadata

**Files:**
- Create: `internal/historymeta/store.go`
- Create: `internal/historymeta/store_test.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/model_test.go`

**Step 1: Write the failing tests**

Add tests covering:

- first join for a new identity records `joined_at`
- reopening the same room preserves the original `joined_at`
- a different identity in the same room gets a different authorization record

**Step 2: Run test to verify it fails**

Run: `go test ./internal/historymeta -run TestRoomAuthorization -count=1`
Expected: FAIL because the metadata store does not exist yet.

**Step 3: Write minimal implementation**

Implement a local metadata store keyed by:

- room key
- identity ID

Persist:

- `joined_at`
- optional summary placeholders for later sync work

Wire the TUI startup/session-ready path so room authorization is ensured when a room opens.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/historymeta -run TestRoomAuthorization -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/historymeta/store.go internal/historymeta/store_test.go internal/tui/model.go internal/tui/model_test.go
git commit -m "feat: persist per-room join authorization"
```

### Task 3: Define sync control message codec

**Files:**
- Create: `internal/room/history_sync_control.go`
- Create: `internal/room/history_sync_control_test.go`
- Modify: `internal/room/status_control.go`

**Step 1: Write the failing tests**

Add tests for:

- encoding and parsing `sync:hello`
- encoding and parsing `sync:offer`
- encoding and parsing `sync:request`
- encoding and parsing `sync:chunk`
- non-sync messages are ignored cleanly

**Step 2: Run test to verify it fails**

Run: `go test ./internal/room -run TestHistorySyncControl -count=1`
Expected: FAIL because the codec does not exist yet.

**Step 3: Write minimal implementation**

Define a reserved hidden prefix family and compact payload shapes.

Ensure:

- helper functions mirror the existing status-control style
- chunk payloads can carry transcript records without exceeding message limits

**Step 4: Run test to verify it passes**

Run: `go test ./internal/room -run TestHistorySyncControl -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/room/history_sync_control.go internal/room/history_sync_control_test.go internal/room/status_control.go
git commit -m "feat: add history sync control message codec"
```

### Task 4: Gate sync traffic to modern peers only

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/model_test.go`

**Step 1: Write the failing tests**

Add tests for:

- receiving `sync:hello` marks a peer as sync-capable
- sync control messages are not emitted before a peer has announced support
- sync control messages never appear in visible chat history

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run 'TestModel.*SyncCapability' -count=1`
Expected: FAIL because the capability bookkeeping does not exist yet.

**Step 3: Write minimal implementation**

In the TUI model:

- parse incoming hidden sync messages
- track which peers are sync-capable
- suppress rendering/persistence of sync control messages

Do not implement full replay yet.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run 'TestModel.*SyncCapability' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/model_test.go
git commit -m "feat: gate history sync to modern peers"
```

### Task 5: Advertise sync capability after room join

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/model_test.go`

**Step 1: Write the failing tests**

Add tests for:

- a modern client sends `sync:hello` after session readiness
- the payload includes identity ID, room key, and summary
- old behavior for normal chat startup remains intact

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run 'TestModel.*SyncHello' -count=1`
Expected: FAIL because hello emission does not exist yet.

**Step 3: Write minimal implementation**

Emit a hidden `sync:hello` once per room connection after:

- identity is loaded
- room authorization metadata exists
- transcript summary is computed

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run 'TestModel.*SyncHello' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/model_test.go
git commit -m "feat: announce history sync capability"
```

### Task 6: Implement offer selection and request flow

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/model_test.go`

**Step 1: Write the failing tests**

Add tests for:

- eligible peer responds to `sync:hello` with `sync:offer`
- requester chooses one source deterministically
- requester sends `sync:request` using stored `joined_at`

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run 'TestModel.*SyncOffer|TestModel.*SyncRequest' -count=1`
Expected: FAIL because negotiation logic does not exist yet.

**Step 3: Write minimal implementation**

Implement:

- offer eligibility checks
- single-source selection
- request emission bounded by authorization lower bound

Keep summaries simple; prefer correctness over optimization.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run 'TestModel.*SyncOffer|TestModel.*SyncRequest' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/model_test.go
git commit -m "feat: negotiate history sync source"
```

### Task 7: Replay chunked history into encrypted local transcripts

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/model_test.go`
- Modify: `internal/transcript/store.go`
- Modify: `internal/transcript/store_test.go`

**Step 1: Write the failing tests**

Add tests for:

- `sync:chunk` messages append recovered transcript records locally
- replayed message IDs are de-duplicated
- recovered records do not overwrite unauthorized older history
- transcript files remain encrypted on disk

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui ./internal/transcript -run 'Test.*SyncChunk|TestStore.*Encrypted' -count=1`
Expected: FAIL because replay support does not exist yet.

**Step 3: Write minimal implementation**

Implement:

- chunk parsing
- transcript append for recovered messages
- in-memory history refresh
- message ID de-duplication

Only recover normal chat records.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui ./internal/transcript -run 'Test.*SyncChunk|TestStore.*Encrypted' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/model_test.go internal/transcript/store.go internal/transcript/store_test.go
git commit -m "feat: replay synced history into local transcripts"
```

### Task 8: Enforce authorization boundary for new identities

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/model_test.go`
- Modify: `internal/historymeta/store.go`
- Modify: `internal/historymeta/store_test.go`

**Step 1: Write the failing tests**

Add tests for:

- a newly introduced identity does not receive messages older than its `joined_at`
- the same imported identity on a second device does receive messages from its original `joined_at`

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui ./internal/historymeta -run 'Test.*AuthorizationBoundary|Test.*RestoredIdentity' -count=1`
Expected: FAIL because the full boundary enforcement is incomplete.

**Step 3: Write minimal implementation**

Apply authorization filtering during offer generation and chunk replay.

Use the stored `joined_at` as the lower bound.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui ./internal/historymeta -run 'Test.*AuthorizationBoundary|Test.*RestoredIdentity' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/model_test.go internal/historymeta/store.go internal/historymeta/store_test.go
git commit -m "feat: enforce per-identity history boundary"
```

### Task 9: Add muted sync status lines and failure handling

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/model_test.go`
- Modify: `internal/tui/scrollback.go`

**Step 1: Write the failing tests**

Add tests for:

- successful sync shows one muted status line
- failed sync shows one muted error line
- control messages still never appear directly in history output

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run 'TestModel.*HistorySyncStatus' -count=1`
Expected: FAIL because status surfacing does not exist yet.

**Step 3: Write minimal implementation**

Show concise local-only status lines:

- `history sync in progress`
- `history synced: N messages`
- `history sync failed`

Avoid noisy repeated lines.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run 'TestModel.*HistorySyncStatus' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/model_test.go internal/tui/scrollback.go
git commit -m "feat: surface muted history sync status"
```

### Task 10: Add CLI support for importing/exporting identity

**Files:**
- Modify: `cmd/chatbox/main.go`
- Modify: `cmd/chatbox/main_test.go`
- Modify: `internal/identity/store.go`
- Create: `docs/identity-migration.md`

**Step 1: Write the failing tests**

Add tests for:

- exporting current identity to a file
- importing identity from a file
- invalid import file is rejected

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/chatbox -run 'TestIdentityImport|TestIdentityExport' -count=1`
Expected: FAIL because import/export commands do not exist yet.

**Step 3: Write minimal implementation**

Add a small CLI surface such as:

- `chatbox identity export --out <path>`
- `chatbox identity import --in <path>`

Document the manual transfer flow for switching devices.

**Step 4: Run test to verify it passes**

Run: `go test ./cmd/chatbox -run 'TestIdentityImport|TestIdentityExport' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/chatbox/main.go cmd/chatbox/main_test.go internal/identity/store.go docs/identity-migration.md
git commit -m "feat: add identity import and export commands"
```

### Task 11: Run full verification and review compatibility behavior

**Files:**
- Modify as needed based on failures discovered during verification

**Step 1: Run focused package tests**

Run:

```bash
go test ./internal/identity ./internal/historymeta ./internal/room ./internal/transcript ./internal/tui -count=1
```

Expected: PASS

**Step 2: Run full test suite**

Run:

```bash
go test ./... -count=1
```

Expected: PASS

**Step 3: Manually verify compatibility logic**

Review that:

- sync control messages require prior `sync:hello`
- unauthorized history is not replayed
- host remains transcript-stateless

**Step 4: Commit final fixes**

```bash
git add .
git commit -m "test: verify offline history sync end to end"
```
