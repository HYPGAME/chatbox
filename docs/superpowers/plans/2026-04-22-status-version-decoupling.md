# Status Version Decoupling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `/status` show connected client versions without depending on history sync control flow.

**Architecture:** Add a dedicated hidden room control message for client version advertisement, have join clients send it immediately after a successful session bind, and let the host record that version independently of `HistorySyncHello`. Keep the legacy `HistorySyncHello.client_version` path intact during rollout so mixed-version rooms keep working.

**Tech Stack:** Go, Bubble Tea TUI, existing `internal/room` hidden control-message pattern, existing `internal/tui` session startup flow, Go test.

---

### Task 1: Add Dedicated Version Control Message

**Files:**
- Create: `internal/room/version_control.go`
- Create: `internal/room/version_control_test.go`

- [ ] **Step 1: Write the failing protocol tests**

Create `internal/room/version_control_test.go` with these tests:

```go
package room

import "testing"

func TestVersionAnnounceRoundTrip(t *testing.T) {
	t.Parallel()

	body := VersionAnnounceBody(VersionAnnounce{
		Version:       1,
		ClientVersion: "v0.1.31",
	})

	parsed, ok := ParseVersionAnnounce(body)
	if !ok {
		t.Fatalf("expected version announce to parse, got %q", body)
	}
	if parsed.Version != 1 || parsed.ClientVersion != "v0.1.31" {
		t.Fatalf("unexpected parsed version announce: %#v", parsed)
	}
}

func TestParseVersionAnnounceRejectsOtherBodies(t *testing.T) {
	t.Parallel()

	if _, ok := ParseVersionAnnounce(StatusRequestBody()); ok {
		t.Fatal("expected status request not to parse as version announce")
	}
	if _, ok := ParseVersionAnnounce("version"); ok {
		t.Fatal("expected arbitrary body not to parse as version announce")
	}
}
```

- [ ] **Step 2: Run the room tests to verify they fail**

Run: `go test ./internal/room -run 'Test(VersionAnnounceRoundTrip|ParseVersionAnnounceRejectsOtherBodies)' -count=1`
Expected: FAIL with undefined `VersionAnnounceBody`, `VersionAnnounce`, and `ParseVersionAnnounce`.

- [ ] **Step 3: Implement the dedicated version control helper**

Create `internal/room/version_control.go`:

```go
package room

import (
	"encoding/json"
	"strings"
)

const versionControlPrefix = "\x00chatbox:version:"

type VersionAnnounce struct {
	Version       int    `json:"version"`
	ClientVersion string `json:"client_version,omitempty"`
}

func IsVersionControl(body string) bool {
	return strings.HasPrefix(body, versionControlPrefix)
}

func VersionAnnounceBody(announce VersionAnnounce) string {
	data, err := json.Marshal(announce)
	if err != nil {
		return versionControlPrefix + "announce:{}"
	}
	return versionControlPrefix + "announce:" + string(data)
}

func ParseVersionAnnounce(body string) (VersionAnnounce, bool) {
	var announce VersionAnnounce
	prefix := versionControlPrefix + "announce:"
	if !strings.HasPrefix(body, prefix) {
		return announce, false
	}
	return announce, json.Unmarshal([]byte(strings.TrimPrefix(body, prefix)), &announce) == nil
}
```

- [ ] **Step 4: Run the new room tests to verify they pass**

Run: `go test ./internal/room -run 'Test(VersionAnnounceRoundTrip|ParseVersionAnnounceRejectsOtherBodies)' -count=1`
Expected: PASS

- [ ] **Step 5: Commit the protocol helper**

```bash
git add internal/room/version_control.go internal/room/version_control_test.go
git commit -m "feat: add client version control message"
```

### Task 2: Teach HostRoom To Record Advertised Versions

**Files:**
- Modify: `internal/room/host.go`
- Modify: `internal/room/host_test.go`
- Test: `internal/room/version_control_test.go`

- [ ] **Step 1: Write the failing host behavior test**

Add this test to `internal/room/host_test.go` near the existing `/status` tests:

```go
func TestHostRoomUsesVersionAnnouncementsInStatusResponses(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	memberA := newFakeMember("bbb")
	memberB := newFakeMember("aaa")
	room.AddMember(memberA)
	room.AddMember(memberB)
	drainJoinEvents(t, room, 2)

	memberA.messages <- session.Message{
		ID:   "version-1",
		From: "bbb",
		Body: VersionAnnounceBody(VersionAnnounce{
			Version:       1,
			ClientVersion: "v0.1.31",
		}),
		At: time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC),
	}

	memberA.messages <- session.Message{
		ID:   "status-1",
		From: "bbb",
		Body: StatusRequestBody(),
		At:   time.Date(2026, 4, 22, 12, 0, 1, 0, time.UTC),
	}

	response := waitForResentMessage(t, memberA.resent)
	line, ok := ParseStatusResponse(response.Body)
	if !ok {
		t.Fatalf("expected hidden status response, got %#v", response)
	}
	if !strings.Contains(line, "bbb [v0.1.31]") {
		t.Fatalf("expected advertised version in roster, got %q", line)
	}
	if !strings.Contains(line, "aaa [unknown]") {
		t.Fatalf("expected untouched legacy peer in roster, got %q", line)
	}
}
```

- [ ] **Step 2: Run the host tests to verify they fail**

Run: `go test ./internal/room -run 'Test(HostRoomUsesVersionAnnouncementsInStatusResponses|HostRoomParticipantNamesIncludeKnownVersionsAndUnknownLegacyPeers)' -count=1`
Expected: FAIL because the host ignores the new version control message.

- [ ] **Step 3: Intercept the new control message in HostRoom**

Update `internal/room/host.go` in two places.

First, add the new handler call in `runMember` before history sync handling:

```go
			if r.handleVersionControl(member, message) {
				continue
			}
			if r.handleHistorySyncControl(member, message) {
				continue
			}
```

Then add the handler alongside the other control-message helpers:

```go
func (r *HostRoom) handleVersionControl(member trackedMember, message session.Message) bool {
	announce, ok := ParseVersionAnnounce(message.Body)
	if !ok {
		return false
	}
	r.rememberMemberVersion(member.id, announce.ClientVersion)
	return true
}
```

Do not remove the existing legacy update inside `handleHistorySyncControl`; keep this line in place for mixed-version rooms:

```go
r.rememberMemberVersion(member.id, hello.ClientVersion)
```

- [ ] **Step 4: Run the host tests to verify both new and legacy paths pass**

Run: `go test ./internal/room -run 'Test(HostRoomUsesVersionAnnouncementsInStatusResponses|HostRoomParticipantNamesIncludeKnownVersionsAndUnknownLegacyPeers)' -count=1`
Expected: PASS

- [ ] **Step 5: Commit the host-side wiring**

```bash
git add internal/room/host.go internal/room/host_test.go
git commit -m "fix: record peer versions outside history sync"
```

### Task 3: Send Version Advertisement On Join Session Startup

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/model_test.go`

- [ ] **Step 1: Write the failing TUI tests**

Update `internal/tui/model_test.go` in two places.

Add a new test that proves version advertisement happens even when history sync prerequisites are absent:

```go
func TestModelSendsVersionAnnouncementAfterSessionReadyWithoutHistorySyncPrereqs(t *testing.T) {
	t.Parallel()

	fake := &fakeSession{peerName: "host", localName: "alice"}
	uiModel := newModel(modelOptions{
		mode:          "join",
		listeningAddr: "203.0.113.10:7331",
		session:       fake,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
		identityLoader: func() (identity.Store, error) {
			return identity.Store{IdentityID: "", Path: "/tmp/identity.json"}, nil
		},
		roomAuthLoader: func(string, string) (historymeta.Record, error) {
			t.Fatal("roomAuthLoader should not run when identity is empty")
			return historymeta.Record{}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: fake})
	uiModel = updated.(model)

	if len(fake.sent) == 0 {
		t.Fatal("expected version announce to be sent after session ready")
	}
	announce, ok := room.ParseVersionAnnounce(fake.sent[0].Body)
	if !ok {
		t.Fatalf("expected first payload to be version announce, got %#v", fake.sent[0])
	}
	if announce.ClientVersion != version.Version {
		t.Fatalf("expected advertised version %q, got %#v", version.Version, announce)
	}
}
```

Then replace the assertion block in `TestModelSendsHistorySyncHelloAfterSessionReady` so it expects both messages in order:

```go
	if len(fake.sent) != 2 {
		t.Fatalf("expected version announce and sync hello after session ready, got %#v", fake.sent)
	}
	announce, ok := room.ParseVersionAnnounce(fake.sent[0].Body)
	if !ok {
		t.Fatalf("expected first payload to be version announce, got %#v", fake.sent[0])
	}
	if announce.ClientVersion != version.Version {
		t.Fatalf("expected version announce %q, got %#v", version.Version, announce)
	}
	hello, ok := room.ParseHistorySyncHello(fake.sent[1].Body)
	if !ok {
		t.Fatalf("expected second payload to be sync hello, got %#v", fake.sent[1])
	}
```

- [ ] **Step 2: Run the TUI tests to verify they fail**

Run: `go test ./internal/tui -run 'Test(ModelSendsVersionAnnouncementAfterSessionReadyWithoutHistorySyncPrereqs|ModelSendsHistorySyncHelloAfterSessionReady)' -count=1`
Expected: FAIL because no version announcement is sent yet.

- [ ] **Step 3: Add an explicit version announcement step to the join startup flow**

Update `internal/tui/model.go`.

First, call the new helper before history sync announcement in `handleSessionReady`:

```go
	m.announceClientVersion()
	m.announceHistorySyncCapability()
```

Then add the helper near `announceHistorySyncCapability`:

```go
func (m *model) announceClientVersion() {
	if m.mode != "join" || m.session == nil {
		return
	}
	_, _ = m.session.Send(room.VersionAnnounceBody(room.VersionAnnounce{
		Version:       1,
		ClientVersion: version.Version,
	}))
}
```

Do not add identity or room authorization guards to `announceClientVersion`; that is the behavioral change this task is introducing.

- [ ] **Step 4: Run the focused TUI tests to verify they pass**

Run: `go test ./internal/tui -run 'Test(ModelSendsVersionAnnouncementAfterSessionReadyWithoutHistorySyncPrereqs|ModelSendsHistorySyncHelloAfterSessionReady)' -count=1`
Expected: PASS

- [ ] **Step 5: Run the full project test suite and build**

Run: `go test ./... -count=1`
Expected: PASS

Run: `go build ./cmd/chatbox`
Expected: exit 0 with no output

- [ ] **Step 6: Commit the startup wiring**

```bash
git add internal/tui/model.go internal/tui/model_test.go
git commit -m "fix: advertise client version on join connect"
```

### Task 4: Final Verification

**Files:**
- Modify: none
- Test: `internal/room/*.go`, `internal/tui/*.go`, `cmd/chatbox`

- [ ] **Step 1: Run the targeted regression suite**

Run: `go test ./internal/room -run 'Test(VersionAnnounceRoundTrip|ParseVersionAnnounceRejectsOtherBodies|HostRoomUsesVersionAnnouncementsInStatusResponses|HostRoomParticipantNamesIncludeKnownVersionsAndUnknownLegacyPeers)' -count=1`
Expected: PASS

Run: `go test ./internal/tui -run 'Test(ModelSendsVersionAnnouncementAfterSessionReadyWithoutHistorySyncPrereqs|ModelSendsHistorySyncHelloAfterSessionReady|JoinStatusCommandSendsHiddenRequestAndRendersRosterResponse)' -count=1`
Expected: PASS

- [ ] **Step 2: Run the full project verification again**

Run: `go test ./... -count=1`
Expected: PASS

Run: `go build ./cmd/chatbox`
Expected: exit 0 with no output

- [ ] **Step 3: Confirm the worktree is clean**

Run: `git status --short`
Expected: no output

- [ ] **Step 4: Prepare release work**

```bash
git log --oneline -n 5
```

Expected: the three implementation commits appear at the top and are ready for release packaging or squash handling, depending on the chosen release workflow.
