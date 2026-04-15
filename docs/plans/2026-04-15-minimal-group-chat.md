# Minimal Group Chat Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Extend `chatbox` from one-to-one chat to a minimal room model where one host can relay messages for multiple joiners.

**Architecture:** Keep `internal/session.Session` as a single encrypted connection and add a host-side room broker above it. Joiners stay single-connection clients. The host UI will consume broker output, render join/leave system events, and show live peer counts without changing the wire protocol.

**Tech Stack:** Go, existing `internal/session` transport, Bubble Tea TUI/scrollback UI, encrypted transcript store

---

### Task 1: Add room transcript key support

**Files:**
- Modify: `internal/transcript/store.go`
- Test: `internal/transcript/store_test.go`

**Step 1: Write the failing test**

Add tests that verify room-scoped transcript keys are stable and no longer depend on a single peer name:

```go
func TestConversationFileNameUsesRoomKeyForGroupChat(t *testing.T) { ... }
func TestConversationFileNameStillSeparatesDifferentRooms(t *testing.T) { ... }
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/transcript -run 'TestConversationFileName(UsesRoomKeyForGroupChat|StillSeparatesDifferentRooms)'`

Expected: FAIL because transcript identity is still peer-scoped.

**Step 3: Write minimal implementation**

- add a room key based transcript naming helper
- keep existing encrypted file format unchanged
- make `OpenStore` accept a generalized conversation key instead of assuming a single peer name

**Step 4: Run test to verify it passes**

Run: `go test ./internal/transcript -run 'TestConversationFileName(UsesRoomKeyForGroupChat|StillSeparatesDifferentRooms)'`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/transcript/store.go internal/transcript/store_test.go
git commit -m "refactor: key transcripts by conversation room"
```

### Task 2: Build the host-side room broker

**Files:**
- Create: `internal/room/host.go`
- Create: `internal/room/host_test.go`
- Modify: `internal/session/session.go` only if small helper access is required

**Step 1: Write the failing test**

Add broker tests that verify:

```go
func TestHostRoomBroadcastsJoinerMessagesToOtherMembersAndHost(t *testing.T) { ... }
func TestHostRoomBroadcastsHostMessagesToAllMembers(t *testing.T) { ... }
func TestHostRoomEmitsJoinAndLeaveEvents(t *testing.T) { ... }
```

Use fake session clients in the broker tests so broadcast behavior is testable without live sockets.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/room -run 'TestHostRoom'`

Expected: FAIL because the room broker package does not exist yet.

**Step 3: Write minimal implementation**

- add a host room type that tracks connected member sessions
- accept member sessions from `session.Host`
- collect inbound messages from each member
- relay joiner messages to all other members and to the host-local stream
- relay host-local sends to all members
- emit system events for `joined` / `left`
- expose current peer count

**Step 4: Run test to verify it passes**

Run: `go test ./internal/room -run 'TestHostRoom'`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/room/host.go internal/room/host_test.go
git commit -m "feat: add host room broker for group chat"
```

### Task 3: Wire the host runner to the room broker

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/scrollback.go`
- Modify: `cmd/chatbox/main.go` only if host startup needs a new adapter
- Test: `internal/tui/model_test.go`

**Step 1: Write the failing test**

Add tests that verify host mode can consume room events:

```go
func TestHostModelShowsPeerCountInStatus(t *testing.T) { ... }
func TestHostModelRendersJoinAndLeaveSystemEvents(t *testing.T) { ... }
func TestHostModelShowsRelayedMessagesFromOriginalSender(t *testing.T) { ... }
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run 'TestHostModel(ShowsPeerCountInStatus|RendersJoinAndLeaveSystemEvents|ShowsRelayedMessagesFromOriginalSender)'`

Expected: FAIL because the model only understands a single peer session today.

**Step 3: Write minimal implementation**

- extend the model with an optional host room event source
- keep join mode behavior unchanged
- update host status to `hosting on <addr> (<n> peers)`
- render room system events as normal system lines
- keep relayed chat lines on the existing `[time] from: body` path

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run 'TestHostModel(ShowsPeerCountInStatus|RendersJoinAndLeaveSystemEvents|ShowsRelayedMessagesFromOriginalSender)'`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/scrollback.go internal/tui/model_test.go cmd/chatbox/main.go
git commit -m "feat: wire host ui to room broker"
```

### Task 4: Add live integration coverage for multi-member host chat

**Files:**
- Modify: `internal/session/session_test.go`
- Create: `internal/room/integration_test.go` if a dedicated integration test file is cleaner

**Step 1: Write the failing test**

Add live-socket integration tests that verify:

```go
func TestGroupHostRelaysMessagesBetweenTwoJoiners(t *testing.T) { ... }
func TestGroupHostBroadcastsHostMessagesToAllJoiners(t *testing.T) { ... }
```

Use a real `session.Host` plus two real `session.Dial` joiners so the broker is exercised on top of the actual encrypted transport.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/session ./internal/room -run 'TestGroupHost'`

Expected: FAIL because the host still only supports a single peer flow.

**Step 3: Write minimal implementation**

- connect the broker accept loop to real `session.Host.Accept`
- ensure host messages broadcast to every connected joiner
- ensure joiner receipts stay sender-to-host scoped

**Step 4: Run test to verify it passes**

Run: `go test ./internal/session ./internal/room -run 'TestGroupHost'`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/session/session_test.go internal/room/integration_test.go internal/room/host.go
git commit -m "test: cover multi-member host relay flow"
```

### Task 5: Make transcript opening room-aware in host and join flows

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `cmd/chatbox/main.go` if conversation key plumbing belongs there
- Test: `internal/tui/model_test.go`

**Step 1: Write the failing test**

Add tests that verify:

```go
func TestHostTranscriptUsesRoomScopedKey(t *testing.T) { ... }
func TestJoinTranscriptUsesTargetScopedKey(t *testing.T) { ... }
```

Use fake transcript openers that capture the requested conversation key.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run 'Test(HostTranscriptUsesRoomScopedKey|JoinTranscriptUsesTargetScopedKey)'`

Expected: FAIL because transcript opening still passes only peer names.

**Step 3: Write minimal implementation**

- derive a stable room conversation key for host mode from the listen address
- derive a stable room conversation key for join mode from the target address
- ensure host transcript history stays in one room transcript even as members join and leave

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run 'Test(HostTranscriptUsesRoomScopedKey|JoinTranscriptUsesTargetScopedKey)'`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/model_test.go cmd/chatbox/main.go internal/transcript/store.go
git commit -m "feat: persist group chat history by room"
```

### Task 6: Document the minimal group chat workflow

**Files:**
- Modify: `README.md`

**Step 1: Write the failing check**

Confirm the README does not yet explain multi-join host chat:

Run: `rg -n "group chat|multiple join|joined|left" README.md`

Expected: no matches relevant to group chat.

**Step 2: Write minimal implementation**

- add a short section showing one host and multiple joiners
- explain that this is a host-relayed room, not mesh
- mention `joined` / `left` system messages

**Step 3: Run the documentation check**

Run: `rg -n "group chat|multiple join|joined|left" README.md`

Expected: matches for the new section.

**Step 4: Commit**

```bash
git add README.md
git commit -m "docs: add minimal group chat usage"
```

### Task 7: Full verification and local binary update

**Files:**
- No code changes required unless verification finds defects.

**Step 1: Run the full test suite**

Run: `go test ./...`

Expected: PASS

**Step 2: Run race checks on the most concurrent paths**

Run: `go test -race ./internal/session ./internal/room ./internal/tui`

Expected: PASS

**Step 3: Rebuild the local binary**

Run: `go build -ldflags "-X chatbox/internal/version.Version=v0.1.7-dev" -o ./chatbox ./cmd/chatbox`

Expected: PASS

**Step 4: Manual room validation**

Run in separate terminals:

1. `./chatbox host --psk-file ...`
2. `./chatbox join --peer <host-addr> --psk-file ... --name aaa`
3. `./chatbox join --peer <host-addr> --psk-file ... --name bbb`
4. Send messages from all three participants
5. Confirm every participant sees the same room traffic
6. Close one joiner and confirm `left` is rendered

Expected: host and both joiners share one live room conversation.

**Step 5: Commit**

```bash
git add .
git commit -m "feat: add minimal host-relayed group chat"
```
