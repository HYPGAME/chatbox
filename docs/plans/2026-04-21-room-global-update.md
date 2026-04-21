# Room Global Update Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a room-scoped `/update-all [version]` flow where `host` or an authorized `join` can trigger silent verified updates for all connected `join` clients, and successful `join` updates restart automatically.

**Architecture:** Introduce a new hidden room control family under `\x00chatbox:update:` with `request`, `execute`, and `result` messages. Keep `host` as the sole authority by loading a host-local admin whitelist, authorizing requests, broadcasting execution orders, and aggregating results. Reuse the existing updater for download and checksum verification, but extend it with explicit-version update support and a `join`-only restart hook that relaunches the current process with preserved CLI args.

**Tech Stack:** Go, Bubble Tea TUI, existing `internal/room`, `internal/tui`, `internal/update`, `cmd/chatbox`, Go testing package.

---

## File Structure

### New Files

- `internal/room/update_control.go`
  - Hidden control payloads for room-scoped update request / execute / result messages.
- `internal/room/update_control_test.go`
  - Round-trip parsing tests for update control payloads.
- `internal/admins/store.go`
  - Host-local admin whitelist loading from `~/.config/chatbox/admins.json`.
- `internal/admins/store_test.go`
  - Config loading tests, including missing file and malformed file handling.
- `internal/update/perform.go`
  - Version-targeted update entrypoint and update outcome classification shared by TUI and command code.
- `internal/update/perform_test.go`
  - Explicit-version update selection and outcome tests.
- `internal/update/restart.go`
  - Relaunch helper for `join` clients after successful in-place replacement.
- `internal/update/restart_test.go`
  - Restart argument and command construction tests.

### Modified Files

- `internal/room/host.go`
  - Accept update request/result control messages, authorize with whitelist, broadcast execute control, and collect per-request summaries.
- `internal/room/host_test.go`
  - Authorization and broadcast behavior tests.
- `internal/tui/model.go`
  - Add `/update-all` command handling, host-local feedback, join-side execution, and result rendering.
- `internal/tui/model_test.go`
  - End-to-end TUI command and execution tests.
- `cmd/chatbox/main.go`
  - Pass original startup args and new update hooks into join UI instances.
- `cmd/chatbox/main_test.go`
  - Validate argument preservation wiring for `join`.
- `internal/update/client.go`
  - Refactor current self-update flow to support “latest release” and “specific release” through a shared helper.
- `internal/update/client_test.go`
  - Keep current latest-release behavior green after refactor.

## Task 1: Add Room Update Control Messages

**Files:**
- Create: `internal/room/update_control.go`
- Create: `internal/room/update_control_test.go`
- Modify: `internal/room/host.go`
- Test: `internal/room/update_control_test.go`

- [ ] **Step 1: Write the failing tests for update request / execute / result payloads**

```go
package room

import (
	"testing"
	"time"
)

func TestUpdateControlRequestRoundTrip(t *testing.T) {
	t.Parallel()

	request := UpdateRequest{
		Version:           1,
		RequestID:         "req-1",
		RoomKey:           "join:203.0.113.10:7331",
		RequesterIdentity: "identity-a",
		RequesterName:     "alice",
		TargetVersion:     "v0.1.24",
		At:                time.Date(2026, 4, 21, 13, 0, 0, 0, time.UTC),
	}

	parsed, ok := ParseUpdateRequest(UpdateRequestBody(request))
	if !ok {
		t.Fatal("expected update request to parse")
	}
	if parsed.RequestID != request.RequestID || parsed.TargetVersion != request.TargetVersion {
		t.Fatalf("expected request to round-trip, got %#v", parsed)
	}
}

func TestUpdateControlExecuteRoundTrip(t *testing.T) {
	t.Parallel()

	execute := UpdateExecute{
		Version:           1,
		RequestID:         "req-1",
		RoomKey:           "join:203.0.113.10:7331",
		InitiatorIdentity: "identity-a",
		InitiatorName:     "alice",
		TargetVersion:     "v0.1.24",
		At:                time.Date(2026, 4, 21, 13, 1, 0, 0, time.UTC),
	}

	parsed, ok := ParseUpdateExecute(UpdateExecuteBody(execute))
	if !ok {
		t.Fatal("expected update execute to parse")
	}
	if parsed.RequestID != execute.RequestID || parsed.InitiatorIdentity != execute.InitiatorIdentity {
		t.Fatalf("expected execute to round-trip, got %#v", parsed)
	}
}

func TestUpdateControlResultRoundTrip(t *testing.T) {
	t.Parallel()

	result := UpdateResult{
		Version:        1,
		RequestID:      "req-1",
		RoomKey:        "join:203.0.113.10:7331",
		ReporterName:   "bob",
		ReporterID:     "identity-b",
		TargetVersion:  "v0.1.24",
		Status:         "success",
		Detail:         "",
		CurrentVersion: "v0.1.23",
		At:             time.Date(2026, 4, 21, 13, 2, 0, 0, time.UTC),
	}

	parsed, ok := ParseUpdateResult(UpdateResultBody(result))
	if !ok {
		t.Fatal("expected update result to parse")
	}
	if parsed.Status != result.Status || parsed.ReporterID != result.ReporterID {
		t.Fatalf("expected result to round-trip, got %#v", parsed)
	}
}

func TestUpdateControlIgnoresRegularMessages(t *testing.T) {
	t.Parallel()

	if IsUpdateControl("hello") {
		t.Fatal("expected regular message not to be update control")
	}
	if _, ok := ParseUpdateRequest("hello"); ok {
		t.Fatal("expected regular message not to parse as update request")
	}
}
```

- [ ] **Step 2: Run the package tests to confirm the new tests fail**

Run: `go test ./internal/room -run 'TestUpdateControl' -count=1`

Expected: FAIL with undefined `UpdateRequest`, `ParseUpdateRequest`, or related symbols.

- [ ] **Step 3: Implement the update control helpers**

```go
package room

import (
	"encoding/json"
	"strings"
	"time"
)

const updateControlPrefix = "\x00chatbox:update:"

type UpdateRequest struct {
	Version           int       `json:"version"`
	RequestID         string    `json:"request_id"`
	RoomKey           string    `json:"room_key"`
	RequesterIdentity string    `json:"requester_identity"`
	RequesterName     string    `json:"requester_name"`
	TargetVersion     string    `json:"target_version,omitempty"`
	At                time.Time `json:"at"`
}

type UpdateExecute struct {
	Version           int       `json:"version"`
	RequestID         string    `json:"request_id"`
	RoomKey           string    `json:"room_key"`
	InitiatorIdentity string    `json:"initiator_identity"`
	InitiatorName     string    `json:"initiator_name"`
	TargetVersion     string    `json:"target_version"`
	At                time.Time `json:"at"`
}

type UpdateResult struct {
	Version        int       `json:"version"`
	RequestID      string    `json:"request_id"`
	RoomKey        string    `json:"room_key"`
	ReporterName   string    `json:"reporter_name"`
	ReporterID     string    `json:"reporter_id"`
	TargetVersion  string    `json:"target_version"`
	Status         string    `json:"status"`
	Detail         string    `json:"detail,omitempty"`
	CurrentVersion string    `json:"current_version,omitempty"`
	At             time.Time `json:"at"`
}

func IsUpdateControl(body string) bool {
	return strings.HasPrefix(body, updateControlPrefix)
}

func UpdateRequestBody(request UpdateRequest) string { return marshalUpdateControl("request", request) }
func UpdateExecuteBody(execute UpdateExecute) string { return marshalUpdateControl("execute", execute) }
func UpdateResultBody(result UpdateResult) string   { return marshalUpdateControl("result", result) }

func ParseUpdateRequest(body string) (UpdateRequest, bool) {
	var request UpdateRequest
	return request, parseUpdateControl(body, "request", &request)
}

func ParseUpdateExecute(body string) (UpdateExecute, bool) {
	var execute UpdateExecute
	return execute, parseUpdateControl(body, "execute", &execute)
}

func ParseUpdateResult(body string) (UpdateResult, bool) {
	var result UpdateResult
	return result, parseUpdateControl(body, "result", &result)
}

func marshalUpdateControl(kind string, payload any) string {
	data, err := json.Marshal(payload)
	if err != nil {
		return updateControlPrefix + kind + ":{}"
	}
	return updateControlPrefix + kind + ":" + string(data)
}

func parseUpdateControl(body, kind string, out any) bool {
	prefix := updateControlPrefix + kind + ":"
	if !strings.HasPrefix(body, prefix) {
		return false
	}
	return json.Unmarshal([]byte(strings.TrimPrefix(body, prefix)), out) == nil
}
```

- [ ] **Step 4: Run the new control tests**

Run: `go test ./internal/room -run 'TestUpdateControl' -count=1`

Expected: PASS

- [ ] **Step 5: Commit the control message scaffolding**

```bash
git add internal/room/update_control.go internal/room/update_control_test.go
git commit -m "feat: add room update control messages"
```

## Task 2: Add Host Admin Whitelist Loading

**Files:**
- Create: `internal/admins/store.go`
- Create: `internal/admins/store_test.go`
- Test: `internal/admins/store_test.go`

- [ ] **Step 1: Write failing tests for host-local whitelist loading**

```go
package admins

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReturnsHostOnlyWhenFileMissing(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	store, err := Load(filepath.Join(baseDir, "admins.json"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if store.Allows("identity-a") {
		t.Fatal("expected missing file not to authorize arbitrary identities")
	}
}

func TestLoadParsesAllowedUpdateIdentities(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	path := filepath.Join(baseDir, "admins.json")
	if err := os.WriteFile(path, []byte(`{"allowed_update_identities":["identity-a","identity-b"]}`), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	store, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !store.Allows("identity-a") || !store.Allows("identity-b") {
		t.Fatalf("expected configured identities to be allowed, got %#v", store)
	}
	if store.Allows("identity-c") {
		t.Fatal("expected unknown identity not to be allowed")
	}
}

func TestLoadRejectsMalformedWhitelistFile(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	path := filepath.Join(baseDir, "admins.json")
	if err := os.WriteFile(path, []byte(`{"allowed_update_identities":`), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("expected malformed config to be rejected")
	}
}
```

- [ ] **Step 2: Run the whitelist tests to confirm they fail**

Run: `go test ./internal/admins -count=1`

Expected: FAIL because `Load` and `Allows` do not exist yet.

- [ ] **Step 3: Implement whitelist loading**

```go
package admins

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Store struct {
	AllowedUpdateIdentities map[string]struct{}
}

type fileConfig struct {
	AllowedUpdateIdentities []string `json:"allowed_update_identities"`
}

func Load(path string) (Store, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Store{AllowedUpdateIdentities: map[string]struct{}{}}, nil
		}
		return Store{}, fmt.Errorf("read admin config: %w", err)
	}

	var config fileConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return Store{}, fmt.Errorf("parse admin config: %w", err)
	}

	store := Store{AllowedUpdateIdentities: make(map[string]struct{}, len(config.AllowedUpdateIdentities))}
	for _, identityID := range config.AllowedUpdateIdentities {
		identityID = strings.TrimSpace(identityID)
		if identityID == "" {
			continue
		}
		store.AllowedUpdateIdentities[identityID] = struct{}{}
	}
	return store, nil
}

func (s Store) Allows(identityID string) bool {
	identityID = strings.TrimSpace(identityID)
	if identityID == "" {
		return false
	}
	_, ok := s.AllowedUpdateIdentities[identityID]
	return ok
}
```

- [ ] **Step 4: Run the whitelist tests**

Run: `go test ./internal/admins -count=1`

Expected: PASS

- [ ] **Step 5: Commit the whitelist loader**

```bash
git add internal/admins/store.go internal/admins/store_test.go
git commit -m "feat: add host admin whitelist loader"
```

## Task 3: Authorize Requests and Broadcast Through HostRoom

**Files:**
- Modify: `internal/room/host.go`
- Modify: `internal/room/host_test.go`
- Modify: `internal/room/update_control.go`
- Test: `internal/room/host_test.go`

- [ ] **Step 1: Add failing host tests for authorized and unauthorized room updates**

```go
func TestHostRoomAcceptsAuthorizedUpdateRequestAndBroadcastsExecute(t *testing.T) {
	t.Parallel()

	member := &fakeMemberSession{name: "alice"}
	room := NewHostRoom("host")
	room.admins = admins.Store{
		AllowedUpdateIdentities: map[string]struct{}{"identity-a": {}},
	}
	room.identityByPeerName = map[string]string{"alice": "identity-a"}
	room.releaseResolver = func(context.Context, string) (string, error) { return "v0.1.24", nil }
	room.AddMember(member)

	message := session.Message{
		ID:   "req-1",
		From: "alice",
		Body: UpdateRequestBody(UpdateRequest{
			Version:           1,
			RequestID:         "update-1",
			RoomKey:           JoinRoomKey("203.0.113.10:7331"),
			RequesterIdentity: "identity-a",
			RequesterName:     "alice",
			TargetVersion:     "",
		}),
		At: time.Date(2026, 4, 21, 14, 0, 0, 0, time.UTC),
	}

	if !room.handleUpdateControl(trackedMember{id: 1, session: member}, message) {
		t.Fatal("expected request to be handled")
	}
	if len(member.resent) == 0 {
		t.Fatal("expected execute message to be broadcast to members")
	}
	execute, ok := ParseUpdateExecute(member.resent[len(member.resent)-1].Body)
	if !ok || execute.TargetVersion != "v0.1.24" {
		t.Fatalf("expected execute broadcast with resolved version, got %#v", member.resent)
	}
}

func TestHostRoomRejectsUnauthorizedUpdateRequest(t *testing.T) {
	t.Parallel()

	member := &fakeMemberSession{name: "eve"}
	room := NewHostRoom("host")
	room.identityByPeerName = map[string]string{"eve": "identity-eve"}
	room.AddMember(member)

	message := session.Message{
		ID:   "req-1",
		From: "eve",
		Body: UpdateRequestBody(UpdateRequest{
			Version:           1,
			RequestID:         "update-1",
			RoomKey:           JoinRoomKey("203.0.113.10:7331"),
			RequesterIdentity: "identity-eve",
			RequesterName:     "eve",
			TargetVersion:     "v0.1.24",
		}),
	}

	if !room.handleUpdateControl(trackedMember{id: 1, session: member}, message) {
		t.Fatal("expected unauthorized request to still be consumed")
	}
	if len(member.resent) == 0 {
		t.Fatal("expected denial result to be sent back to requester")
	}
	result, ok := ParseUpdateResult(member.resent[len(member.resent)-1].Body)
	if !ok || result.Status != "permission-denied" {
		t.Fatalf("expected permission-denied result, got %#v", member.resent)
	}
}
```

- [ ] **Step 2: Run the host tests to confirm they fail**

Run: `go test ./internal/room -run 'TestHostRoom(AcceptsAuthorizedUpdateRequestAndBroadcastsExecute|RejectsUnauthorizedUpdateRequest)' -count=1`

Expected: FAIL with missing `handleUpdateControl`, `admins`, or `releaseResolver` support in `HostRoom`.

- [ ] **Step 3: Extend `HostRoom` with update authorization and summary state**

```go
type HostRoom struct {
	localName string

	// existing fields...

	admins             admins.Store
	releaseResolver    func(context.Context, string) (string, error)
	identityByPeerName map[string]string
	processedUpdates   map[string]struct{}
	activeUpdateStatus map[string]map[string]string
}

func (r *HostRoom) handleUpdateControl(member trackedMember, message session.Message) bool {
	if request, ok := ParseUpdateRequest(message.Body); ok {
		r.handleUpdateRequest(member, request)
		return true
	}
	if result, ok := ParseUpdateResult(message.Body); ok {
		r.handleUpdateResult(result)
		return true
	}
	return IsUpdateControl(message.Body)
}

func (r *HostRoom) handleUpdateRequest(member trackedMember, request UpdateRequest) {
	if _, seen := r.processedUpdates[request.RequestID]; seen {
		return
	}
	requesterIdentity := strings.TrimSpace(request.RequesterIdentity)
	requesterName := strings.TrimSpace(request.RequesterName)
	allowed := requesterName == r.localName || r.admins.Allows(requesterIdentity)
	if !allowed {
		r.sendUpdateResult(member.session, UpdateResult{
			Version:       1,
			RequestID:     request.RequestID,
			RoomKey:       request.RoomKey,
			ReporterName:  r.localName,
			ReporterID:    "",
			TargetVersion: strings.TrimSpace(request.TargetVersion),
			Status:        "permission-denied",
			At:            time.Now(),
		})
		return
	}

	targetVersion, err := r.resolveTargetVersion(request.TargetVersion)
	if err != nil {
		r.sendUpdateResult(member.session, UpdateResult{
			Version:       1,
			RequestID:     request.RequestID,
			RoomKey:       request.RoomKey,
			ReporterName:  r.localName,
			TargetVersion: strings.TrimSpace(request.TargetVersion),
			Status:        "resolve-latest-failed",
			Detail:        err.Error(),
			At:            time.Now(),
		})
		return
	}

	r.processedUpdates[request.RequestID] = struct{}{}
	r.activeUpdateStatus[request.RequestID] = map[string]string{}
	_ = r.broadcast(session.Message{
		ID:   request.RequestID + "-execute",
		From: r.localName,
		Body: UpdateExecuteBody(UpdateExecute{
			Version:           1,
			RequestID:         request.RequestID,
			RoomKey:           request.RoomKey,
			InitiatorIdentity: requesterIdentity,
			InitiatorName:     requesterName,
			TargetVersion:     targetVersion,
			At:                time.Now(),
		}),
		At: time.Now(),
	}, 0)
}

func (r *HostRoom) resolveTargetVersion(targetVersion string) (string, error) {
	targetVersion = strings.TrimSpace(targetVersion)
	if targetVersion != "" {
		return targetVersion, nil
	}
	if r.releaseResolver == nil {
		return "", fmt.Errorf("latest release resolver is not configured")
	}
	return r.releaseResolver(context.Background(), "")
}

func (r *HostRoom) handleUpdateResult(result UpdateResult) {
	if _, ok := r.activeUpdateStatus[result.RequestID]; !ok {
		r.activeUpdateStatus[result.RequestID] = map[string]string{}
	}
	r.activeUpdateStatus[result.RequestID][result.ReporterName] = result.Status
}

func (r *HostRoom) sendUpdateResult(member memberSession, result UpdateResult) {
	_ = member.Resend(session.Message{
		ID:   result.RequestID + "-result",
		From: r.localName,
		Body: UpdateResultBody(result),
		At:   result.At,
	})
}

func renderUpdateSummaryLine(requestID string, statuses map[string]string) string {
	success := 0
	for _, status := range statuses {
		if status == "success" {
			success++
		}
	}
	return fmt.Sprintf("update summary %s: success=%d", requestID, success)
}
```

- [ ] **Step 4: Wire update control handling into `runMember` and keep the tests green**

Run: `go test ./internal/room -run 'TestHostRoom(AcceptsAuthorizedUpdateRequestAndBroadcastsExecute|RejectsUnauthorizedUpdateRequest)' -count=1`

Expected: PASS

- [ ] **Step 5: Add one more host result-summary regression test**

```go
func TestHostRoomPublishesUpdateResultSummary(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	room.activeUpdateStatus = map[string]map[string]string{"update-1": {}}
	room.handleUpdateResult(UpdateResult{
		Version:       1,
		RequestID:     "update-1",
		ReporterName:  "alice",
		ReporterID:    "identity-a",
		TargetVersion: "v0.1.24",
		Status:        "success",
		At:            time.Date(2026, 4, 21, 14, 5, 0, 0, time.UTC),
	})

	events := room.EventLog()
	_ = events
	if !strings.Contains(renderUpdateSummaryLine("update-1", room.activeUpdateStatus["update-1"]), "success=1") {
		t.Fatal("expected summary line to include success count")
	}
}
```

- [ ] **Step 6: Run the full room package**

Run: `go test ./internal/room -count=1`

Expected: PASS

- [ ] **Step 7: Commit the host authorization flow**

```bash
git add internal/room/host.go internal/room/host_test.go internal/room/update_control.go
git commit -m "feat: authorize room-wide update requests"
```

## Task 4: Add Explicit-Version Update and Join Restart Support

**Files:**
- Create: `internal/update/perform.go`
- Create: `internal/update/perform_test.go`
- Create: `internal/update/restart.go`
- Create: `internal/update/restart_test.go`
- Modify: `internal/update/client.go`
- Modify: `internal/update/client_test.go`
- Test: `internal/update/perform_test.go`

- [ ] **Step 1: Write failing tests for version-targeted update and restart rules**

```go
package update

import (
	"context"
	"testing"
)

func TestPerformUpdateToVersionUsesExplicitRelease(t *testing.T) {
	t.Parallel()

	client := Client{
		Repository:     "HYPGAME/chatbox",
		CurrentVersion: "v0.1.23",
		ReleaseByTag: func(context.Context, string) (Release, error) {
			return Release{
				TagName: "v0.1.24",
				Assets: []ReleaseAsset{
					{Name: "chatbox_darwin_arm64.tar.gz", DownloadURL: "https://example.invalid/chatbox_darwin_arm64.tar.gz"},
					{Name: "checksums.txt", DownloadURL: "https://example.invalid/checksums.txt"},
				},
			}, nil
		},
	}

	release, err := client.resolveRelease(context.Background(), "v0.1.24")
	if err != nil {
		t.Fatalf("resolveRelease returned error: %v", err)
	}
	if release.TagName != "v0.1.24" {
		t.Fatalf("expected explicit release tag %q, got %#v", "v0.1.24", release)
	}
}

func TestBuildRestartSpecPreservesJoinArguments(t *testing.T) {
	t.Parallel()

	spec, err := BuildRestartSpec("/tmp/chatbox", []string{
		"join",
		"--peer", "127.0.0.1:7331",
		"--psk-file", "/tmp/test.psk",
		"--name", "alice",
		"--ui", "tui",
	})
	if err != nil {
		t.Fatalf("BuildRestartSpec returned error: %v", err)
	}
	if spec.Path != "/tmp/chatbox" {
		t.Fatalf("expected restart path to be preserved, got %#v", spec)
	}
	if len(spec.Args) != 7 || spec.Args[0] != "join" {
		t.Fatalf("expected join args to be preserved, got %#v", spec)
	}
}

func TestBuildRestartSpecRejectsNonJoinCommands(t *testing.T) {
	t.Parallel()

	if _, err := BuildRestartSpec("/tmp/chatbox", []string{"host", "--listen", "0.0.0.0:7331"}); err == nil {
		t.Fatal("expected non-join restart to be rejected")
	}
}
```

- [ ] **Step 2: Run update package tests to confirm failure**

Run: `go test ./internal/update -run 'Test(PerformUpdateToVersionUsesExplicitRelease|BuildRestartSpec)' -count=1`

Expected: FAIL because `resolveRelease`, `ReleaseByTag`, and `BuildRestartSpec` do not exist yet.

- [ ] **Step 3: Implement targeted update execution and restart spec helpers**

```go
package update

import "context"

type Outcome struct {
	Status         string
	Detail         string
	CurrentVersion string
	LatestVersion  string
	FallbackPath   string
	Restartable    bool
}

type ReleaseByTagFunc func(context.Context, string) (Release, error)

type Client struct {
	// existing fields...
	ReleaseByTag ReleaseByTagFunc
}

func (c Client) PerformUpdate(ctx context.Context, targetVersion string) (Outcome, error) {
	if c.goos() == "android" {
		return Outcome{Status: "android-manual-required", CurrentVersion: c.CurrentVersion}, nil
	}

	release, err := c.resolveRelease(ctx, targetVersion)
	if err != nil {
		return Outcome{Status: "resolve-latest-failed", CurrentVersion: c.CurrentVersion, Detail: err.Error()}, nil
	}
	if c.CurrentVersion != "" && !isNewerRelease(c.CurrentVersion, release.TagName) {
		return Outcome{
			Status:         "already-up-to-date",
			CurrentVersion: c.CurrentVersion,
			LatestVersion:  release.TagName,
		}, nil
	}

	result, err := c.applyResolvedRelease(ctx, release)
	if err != nil {
		return Outcome{Status: classifyUpdateError(err), CurrentVersion: c.CurrentVersion, LatestVersion: release.TagName, Detail: err.Error()}, nil
	}
	return Outcome{
		Status:         map[bool]string{true: "success", false: "fallback-written"}[result.FallbackPath == ""],
		CurrentVersion: c.CurrentVersion,
		LatestVersion:  release.TagName,
		FallbackPath:   result.FallbackPath,
		Restartable:    result.FallbackPath == "",
	}, nil
}

func (c Client) resolveRelease(ctx context.Context, targetVersion string) (Release, error) {
	targetVersion = strings.TrimSpace(targetVersion)
	if targetVersion == "" {
		return c.LatestRelease(ctx)
	}
	if c.ReleaseByTag != nil {
		return c.ReleaseByTag(ctx, targetVersion)
	}
	return Release{}, fmt.Errorf("explicit release lookup is not configured")
}

func (c Client) applyResolvedRelease(ctx context.Context, release Release) (ApplyResult, error) {
	return c.applyRelease(ctx, release)
}

func classifyUpdateError(err error) string {
	if err == nil {
		return "success"
	}
	text := err.Error()
	switch {
	case strings.Contains(text, "checksum"):
		return "checksum-failed"
	case strings.Contains(text, "extract"):
		return "extract-failed"
	case strings.Contains(text, "replace"), strings.Contains(text, "activate"):
		return "replace-failed"
	default:
		return "download-failed"
	}
}

type RestartSpec struct {
	Path string
	Args []string
}

func BuildRestartSpec(executablePath string, startupArgs []string) (RestartSpec, error) {
	if len(startupArgs) == 0 || strings.TrimSpace(startupArgs[0]) != "join" {
		return RestartSpec{}, fmt.Errorf("restart is only supported for join")
	}
	return RestartSpec{
		Path: executablePath,
		Args: append([]string(nil), startupArgs...),
	}, nil
}
```

- [ ] **Step 4: Add a restart launcher helper test and implementation**

```go
func LaunchRestart(spec RestartSpec) error {
	cmd := exec.Command(spec.Path, spec.Args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Start()
}
```

Run: `go test ./internal/update -run 'Test(PerformUpdateToVersionUsesExplicitRelease|BuildRestartSpec)' -count=1`

Expected: PASS

- [ ] **Step 5: Run the full update package**

Run: `go test ./internal/update -count=1`

Expected: PASS, including current `SelfUpdate` tests.

- [ ] **Step 6: Commit the update engine changes**

```bash
git add internal/update/client.go internal/update/client_test.go internal/update/perform.go internal/update/perform_test.go internal/update/restart.go internal/update/restart_test.go
git commit -m "feat: add explicit room update execution support"
```

## Task 5: Wire `/update-all` Into Join and Host TUI Flows

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/model_test.go`
- Modify: `cmd/chatbox/main.go`
- Modify: `cmd/chatbox/main_test.go`
- Test: `internal/tui/model_test.go`

- [ ] **Step 1: Write failing TUI tests for `/update-all` request, permission denial, and join auto-execution**

```go
func TestModelJoinUpdateAllSendsHiddenRequest(t *testing.T) {
	t.Parallel()

	fake := &fakeSession{peerName: "host", localName: "alice"}
	uiModel := newModel(modelOptions{
		mode:          "join",
		uiMode:        uiModeTUI,
		listeningAddr: "203.0.113.10:7331",
		session:       fake,
		identityLoader: func() (identity.Store, error) {
			return identity.Store{IdentityID: "identity-a", Path: "/tmp/identity.json"}, nil
		},
		roomAuthLoader: func(roomKey, identityID string) (historymeta.Record, error) {
			return historymeta.Record{RoomKey: roomKey, IdentityID: identityID}, nil
		},
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: fake})
	uiModel = updated.(model)
	uiModel.input.SetValue("/update-all v0.1.24")
	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	uiModel = updated.(model)

	if len(fake.sent) == 0 {
		t.Fatal("expected update request to be sent")
	}
	request, ok := room.ParseUpdateRequest(fake.sent[len(fake.sent)-1].Body)
	if !ok || request.TargetVersion != "v0.1.24" {
		t.Fatalf("expected hidden update request, got %#v", fake.sent)
	}
}

func TestModelRendersPermissionDeniedUpdateResult(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:          "join",
		uiMode:        uiModeTUI,
		listeningAddr: "203.0.113.10:7331",
		session:       &fakeSession{peerName: "host"},
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
		identityLoader: func() (identity.Store, error) {
			return identity.Store{IdentityID: "identity-a", Path: "/tmp/identity.json"}, nil
		},
		roomAuthLoader: func(roomKey, identityID string) (historymeta.Record, error) {
			return historymeta.Record{RoomKey: roomKey, IdentityID: identityID}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: &fakeSession{peerName: "host"}})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "result-1",
			From: "host",
			Body: room.UpdateResultBody(room.UpdateResult{
				Version:       1,
				RequestID:     "update-1",
				RoomKey:       transcript.JoinRoomKey("203.0.113.10:7331"),
				ReporterName:  "host",
				TargetVersion: "v0.1.24",
				Status:        "permission-denied",
			}),
		},
	})
	uiModel = updated.(model)

	if !strings.Contains(stripANSI(uiModel.View()), "permission-denied") {
		t.Fatalf("expected readable denial in view, got %q", stripANSI(uiModel.View()))
	}
}

func TestModelJoinExecutesApprovedUpdateAndReportsSuccess(t *testing.T) {
	t.Parallel()

	fake := &fakeSession{peerName: "host", localName: "alice"}
	var restarted bool
	uiModel := newModel(modelOptions{
		mode:          "join",
		uiMode:        uiModeTUI,
		listeningAddr: "203.0.113.10:7331",
		session:       fake,
		startupArgs:   []string{"join", "--peer", "203.0.113.10:7331", "--psk-file", "/tmp/test.psk"},
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
		identityLoader: func() (identity.Store, error) {
			return identity.Store{IdentityID: "identity-a", Path: "/tmp/identity.json"}, nil
		},
		roomAuthLoader: func(roomKey, identityID string) (historymeta.Record, error) {
			return historymeta.Record{RoomKey: roomKey, IdentityID: identityID}, nil
		},
		updatePerformer: func(context.Context, string) (update.Outcome, error) {
			return update.Outcome{Status: "success", LatestVersion: "v0.1.24", Restartable: true}, nil
		},
		restartLauncher: func(update.RestartSpec) error {
			restarted = true
			return nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: fake})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "execute-1",
			From: "host",
			Body: room.UpdateExecuteBody(room.UpdateExecute{
				Version:           1,
				RequestID:         "update-1",
				RoomKey:           transcript.JoinRoomKey("203.0.113.10:7331"),
				InitiatorIdentity: "identity-host",
				InitiatorName:     "host",
				TargetVersion:     "v0.1.24",
			}),
		},
	})
	uiModel = updated.(model)

	if !restarted {
		t.Fatal("expected successful in-place update to trigger restart")
	}
	result, ok := room.ParseUpdateResult(fake.sent[len(fake.sent)-1].Body)
	if !ok || result.Status != "success" {
		t.Fatalf("expected success update result, got %#v", fake.sent)
	}
}
```

- [ ] **Step 2: Run the targeted TUI tests to confirm failure**

Run: `go test ./internal/tui -run 'TestModel(JoinUpdateAllSendsHiddenRequest|RendersPermissionDeniedUpdateResult|JoinExecutesApprovedUpdateAndReportsSuccess)' -count=1`

Expected: FAIL because `/update-all`, update performer hooks, and restart hooks do not exist.

- [ ] **Step 3: Extend `modelOptions`, `model`, and command parsing for room updates**

```go
type modelOptions struct {
	// existing fields...
	startupArgs         []string
	hostUpdateRequester func(room.UpdateRequest) error
	updatePerformer     func(context.Context, string) (update.Outcome, error)
	restartLauncher     func(update.RestartSpec) error
}

func (m *model) handleSubmit(text string) (tea.Model, tea.Cmd) {
	if strings.HasPrefix(text, "/update-all") {
		return m.handleUpdateAllCommand(text)
	}
	// existing command handling...
}

func (m *model) handleUpdateAllCommand(text string) (tea.Model, tea.Cmd) {
	fields := strings.Fields(text)
	targetVersion := ""
	if len(fields) > 1 {
		targetVersion = fields[1]
	}
	request := room.UpdateRequest{
		Version:           1,
		RequestID:         fmt.Sprintf("update-%d", time.Now().UnixNano()),
		RoomKey:           m.roomAuthorization.RoomKey,
		RequesterIdentity: m.identityID,
		RequesterName:     m.localDisplayName(),
		TargetVersion:     targetVersion,
		At:                time.Now(),
	}
	if m.mode == "host" {
		m.handleLocalHostUpdateRequest(request)
		return *m, m.flushScrollbackCmd()
	}
	_, err := m.session.Send(room.UpdateRequestBody(request))
	if err != nil {
		m.addErrorEntry(err.Error())
	}
	return *m, m.flushScrollbackCmd()
}

func (m *model) localDisplayName() string {
	if len(m.startupArgs) > 0 {
		for i := 0; i < len(m.startupArgs)-1; i++ {
			if m.startupArgs[i] == "--name" {
				return m.startupArgs[i+1]
			}
		}
	}
	if m.mode == "host" {
		return "host"
	}
	return "chatbox"
}

func (m *model) handleLocalHostUpdateRequest(request room.UpdateRequest) {
	if m.hostUpdateRequester != nil {
		_ = m.hostUpdateRequester(request)
	}
}
```

- [ ] **Step 4: Add join-side execution and result reporting**

```go
func (m *model) handleControlMessage(message session.Message) bool {
	if execute, ok := room.ParseUpdateExecute(message.Body); ok {
		m.handleUpdateExecute(execute)
		return true
	}
	if result, ok := room.ParseUpdateResult(message.Body); ok {
		m.addSystemEntry(renderUpdateResultLine(result))
		return true
	}
	// existing control handlers...
}

func (m *model) handleUpdateExecute(execute room.UpdateExecute) {
	if m.mode != "join" || execute.RoomKey != m.roomAuthorization.RoomKey {
		return
	}
	outcome, err := m.updatePerformer(context.Background(), execute.TargetVersion)
	if err != nil {
		outcome = update.Outcome{Status: "replace-failed", Detail: err.Error()}
	}
	if m.session != nil {
		_, _ = m.session.Send(room.UpdateResultBody(room.UpdateResult{
			Version:        1,
			RequestID:      execute.RequestID,
			RoomKey:        execute.RoomKey,
			ReporterName:   m.localDisplayName(),
			ReporterID:     m.identityID,
			TargetVersion:  execute.TargetVersion,
			Status:         outcome.Status,
			Detail:         outcome.Detail,
			CurrentVersion: version.Version,
			At:             time.Now(),
		}))
	}
	m.addSystemEntry(renderUpdateResultLine(room.UpdateResult{
		RequestID:     execute.RequestID,
		ReporterName:  m.localDisplayName(),
		TargetVersion: execute.TargetVersion,
		Status:        outcome.Status,
	}))
	if outcome.Restartable {
		spec, err := update.BuildRestartSpec(m.executablePath, m.startupArgs)
		if err == nil {
			_ = m.restartLauncher(spec)
		}
	}
}

func renderUpdateResultLine(result room.UpdateResult) string {
	return fmt.Sprintf("update result: %s %s (%s)", result.ReporterName, result.Status, result.TargetVersion)
}
```

- [ ] **Step 5: Wire startup args from `cmd/chatbox/main.go` into join UI creation**

```go
func runJoin(ctx context.Context, args []string) error {
	// existing flag parsing...
	originalArgs := append([]string{"join"}, args...)
	if uiMode == "tui" {
		return runJoinUIWithUpdates(conn, *name, *peer, cfg, uiMode, alertMode, backgroundUpdateNoticeChannel(ctx), originalArgs)
	}
	return runJoinUI(conn, *name, *peer, cfg, uiMode, alertMode, originalArgs)
}
```

- [ ] **Step 6: Run the targeted TUI and main command tests**

Run: `go test ./internal/tui ./cmd/chatbox -run 'Test(Model|Run)' -count=1`

Expected: PASS for the new `/update-all` and startup-arg wiring tests, with no regressions in current slash-command behavior.

- [ ] **Step 7: Commit the TUI and CLI wiring**

```bash
git add internal/tui/model.go internal/tui/model_test.go cmd/chatbox/main.go cmd/chatbox/main_test.go
git commit -m "feat: wire room update command into join and host flows"
```

## Task 6: End-to-End Regression and Documentation Updates

**Files:**
- Modify: `internal/room/host_test.go`
- Modify: `internal/tui/model_test.go`
- Modify: `cmd/chatbox/main_test.go`
- Modify: `docs/plans/2026-04-21-room-global-update-design.md` (only if implementation diverges)

- [ ] **Step 1: Add regression tests for duplicate requests, Android manual fallback, and non-restart fallback updates**

```go
func TestModelJoinReportsFallbackWrittenWithoutRestart(t *testing.T) {
	t.Parallel()

	fake := &fakeSession{peerName: "host", localName: "alice"}
	var restarted bool
	uiModel := newModel(modelOptions{
		mode:          "join",
		uiMode:        uiModeTUI,
		listeningAddr: "203.0.113.10:7331",
		session:       fake,
		startupArgs:   []string{"join", "--peer", "203.0.113.10:7331"},
		transcriptOpener: func(string) (transcriptStore, error) { return &fakeTranscriptStore{}, nil },
		identityLoader:   func() (identity.Store, error) { return identity.Store{IdentityID: "identity-a", Path: "/tmp/identity.json"}, nil },
		roomAuthLoader:   func(roomKey, identityID string) (historymeta.Record, error) { return historymeta.Record{RoomKey: roomKey, IdentityID: identityID}, nil },
		updatePerformer: func(context.Context, string) (update.Outcome, error) {
			return update.Outcome{Status: "fallback-written", LatestVersion: "v0.1.24", Restartable: false}, nil
		},
		restartLauncher: func(update.RestartSpec) error {
			restarted = true
			return nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: fake})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "execute-1",
			From: "host",
			Body: room.UpdateExecuteBody(room.UpdateExecute{Version: 1, RequestID: "update-1", RoomKey: transcript.JoinRoomKey("203.0.113.10:7331"), TargetVersion: "v0.1.24"}),
		},
	})
	uiModel = updated.(model)

	if restarted {
		t.Fatal("expected fallback-written not to restart join")
	}
}
```

- [ ] **Step 2: Run the entire test suite**

Run: `go test ./... -count=1`

Expected: PASS

- [ ] **Step 3: Build the CLI to verify no missing wiring**

Run: `go build ./cmd/chatbox`

Expected: exit 0 with no output.

- [ ] **Step 4: Update the design doc only if implementation required a material change**

```markdown
If the implementation differs from the approved design in any user-visible way, update:
- docs/plans/2026-04-21-room-global-update-design.md
Otherwise leave it unchanged.
```

- [ ] **Step 5: Commit the regression pass**

```bash
git add internal/room/host_test.go internal/tui/model_test.go cmd/chatbox/main_test.go docs/plans/2026-04-21-room-global-update-design.md
git commit -m "test: cover room global update regressions"
```
