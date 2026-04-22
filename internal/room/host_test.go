package room

import (
	"context"
	"strings"
	"testing"
	"time"

	"chatbox/internal/admins"
	"chatbox/internal/session"
	"chatbox/internal/version"
)

func TestHostRoomBroadcastsJoinerMessagesToOtherMembersAndHost(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	memberA := newFakeMember("aaa")
	memberB := newFakeMember("bbb")
	room.AddMember(memberA)
	room.AddMember(memberB)
	drainJoinEvents(t, room, 2)

	inbound := session.Message{
		ID:   "msg-1",
		From: "aaa",
		Body: "hello room",
		At:   time.Date(2026, 4, 15, 18, 0, 0, 0, time.UTC),
	}
	memberA.messages <- inbound

	gotHost := waitForRoomMessage(t, room.Messages())
	if gotHost != inbound {
		t.Fatalf("expected host stream to receive %#v, got %#v", inbound, gotHost)
	}

	gotForward := waitForResentMessage(t, memberB.resent)
	if gotForward != inbound {
		t.Fatalf("expected other member to receive %#v, got %#v", inbound, gotForward)
	}

	assertNoResentMessage(t, memberA.resent)
}

func TestHostRoomBroadcastsHostMessagesToAllMembers(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	memberA := newFakeMember("aaa")
	memberB := newFakeMember("bbb")
	room.AddMember(memberA)
	room.AddMember(memberB)
	drainJoinEvents(t, room, 2)

	message, err := room.Send("hello everyone")
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if message.From != "host" {
		t.Fatalf("expected host sender name, got %q", message.From)
	}

	gotHost := waitForRoomMessage(t, room.Messages())
	if gotHost != message {
		t.Fatalf("expected host stream to receive %#v, got %#v", message, gotHost)
	}

	if got := waitForResentMessage(t, memberA.resent); got != message {
		t.Fatalf("expected first member to receive %#v, got %#v", message, got)
	}
	if got := waitForResentMessage(t, memberB.resent); got != message {
		t.Fatalf("expected second member to receive %#v, got %#v", message, got)
	}
}

func TestHostRoomEmitsJoinAndLeaveEvents(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	member := newFakeMember("aaa")
	room.AddMember(member)

	joined := waitForRoomEvent(t, room.Events())
	if joined.Kind != EventPeerJoined || joined.PeerName != "aaa" || joined.PeerCount != 1 {
		t.Fatalf("expected joined event for aaa with count 1, got %#v", joined)
	}

	close(member.done)

	left := waitForRoomEvent(t, room.Events())
	if left.Kind != EventPeerLeft || left.PeerName != "aaa" || left.PeerCount != 0 {
		t.Fatalf("expected left event for aaa with count 0, got %#v", left)
	}
}

func TestHostRoomInterceptsStatusRequestsAndRepliesOnlyToRequester(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	memberA := newFakeMember("bbb")
	memberB := newFakeMember("aaa")
	room.AddMember(memberA)
	room.AddMember(memberB)
	drainJoinEvents(t, room, 2)

	memberA.messages <- session.Message{
		ID:   "status-1",
		From: "bbb",
		Body: StatusRequestBody(),
		At:   time.Date(2026, 4, 17, 16, 0, 0, 0, time.UTC),
	}

	response := waitForResentMessage(t, memberA.resent)
	line, ok := ParseStatusResponse(response.Body)
	if !ok {
		t.Fatalf("expected hidden status response, got %#v", response)
	}
	if line != "online (3): aaa [unknown], bbb [unknown], host ["+version.Version+"]" {
		t.Fatalf("expected sorted online roster, got %q", line)
	}

	assertNoResentMessage(t, memberB.resent)
	assertNoRoomMessage(t, room.Messages())
}

func TestHostRoomInterceptsEventsRequestsAndRepliesOnlyToRequester(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	memberA := newFakeMember("aaa")
	memberB := newFakeMember("bbb")
	room.AddMember(memberA)
	room.AddMember(memberB)
	drainJoinEvents(t, room, 2)
	close(memberB.done)
	left := waitForRoomEvent(t, room.Events())
	if left.Kind != EventPeerLeft || left.PeerName != "bbb" {
		t.Fatalf("expected bbb left event, got %#v", left)
	}

	memberA.messages <- session.Message{
		ID:   "events-1",
		From: "aaa",
		Body: EventsRequestBody(),
		At:   time.Date(2026, 4, 20, 18, 10, 0, 0, time.UTC),
	}

	response := waitForResentMessage(t, memberA.resent)
	events, ok := ParseEventsResponse(response.Body)
	if !ok {
		t.Fatalf("expected hidden events response, got %#v", response)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 join/leave events, got %#v", events)
	}
	assertNoResentMessage(t, memberB.resent)
	assertNoRoomMessage(t, room.Messages())
}

func TestHostRoomRoutesHistorySyncMessagesOnlyToSyncCapableMembers(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	memberA := newFakeMember("aaa")
	memberB := newFakeMember("bbb")
	room.AddMember(memberA)
	room.AddMember(memberB)
	drainJoinEvents(t, room, 2)

	memberA.messages <- session.Message{
		ID:   "sync-hello-1",
		From: "aaa",
		Body: HistorySyncHelloBody(HistorySyncHello{
			Version:    1,
			IdentityID: "identity-a",
			RoomKey:    "room",
		}),
		At: time.Date(2026, 4, 20, 21, 0, 0, 0, time.UTC),
	}

	assertNoResentMessage(t, memberB.resent)
	assertNoRoomMessage(t, room.Messages())

	memberA.messages <- session.Message{
		ID:   "sync-offer-1",
		From: "aaa",
		Body: HistorySyncOfferBody(HistorySyncOffer{
			Version:        1,
			SourceIdentity: "identity-a",
			TargetIdentity: "identity-b",
			RoomKey:        "room",
			Summary:        HistorySyncSummary{Count: 1},
		}),
		At: time.Date(2026, 4, 20, 21, 1, 0, 0, time.UTC),
	}

	assertNoResentMessage(t, memberB.resent)
	assertNoRoomMessage(t, room.Messages())
}

func TestHostRoomForwardsHistorySyncMessagesToMembersWhoAnnouncedHello(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	memberA := newFakeMember("aaa")
	memberB := newFakeMember("bbb")
	room.AddMember(memberA)
	room.AddMember(memberB)
	drainJoinEvents(t, room, 2)

	memberB.messages <- session.Message{
		ID:   "sync-hello-b",
		From: "bbb",
		Body: HistorySyncHelloBody(HistorySyncHello{
			Version:    1,
			IdentityID: "identity-b",
			RoomKey:    "room",
		}),
		At: time.Date(2026, 4, 20, 21, 0, 0, 0, time.UTC),
	}
	assertNoResentMessage(t, memberA.resent)

	memberA.messages <- session.Message{
		ID:   "sync-hello-a",
		From: "aaa",
		Body: HistorySyncHelloBody(HistorySyncHello{
			Version:    1,
			IdentityID: "identity-a",
			RoomKey:    "room",
		}),
		At: time.Date(2026, 4, 20, 21, 0, 10, 0, time.UTC),
	}
	if got := waitForResentMessage(t, memberB.resent); got.Body == "" || !IsHistorySyncControl(got.Body) {
		t.Fatalf("expected sync-capable member to receive sync control, got %#v", got)
	}
	assertNoRoomMessage(t, room.Messages())
}

func TestHostRoomAcceptsAuthorizedUpdateRequestAndBroadcastsExecute(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	member := newFakeMember("alice")
	room.admins = admins.Store{
		AllowedUpdateIdentities: map[string]struct{}{"identity-a": {}},
	}
	room.identityByPeerName["alice"] = "identity-a"
	resolverCalled := false
	room.releaseResolver = func(context.Context, string) (string, error) {
		resolverCalled = true
		return "v0.1.24", nil
	}
	room.AddMember(member)
	drainJoinEvents(t, room, 1)

	member.messages <- session.Message{
		ID:   "req-1",
		From: "alice",
		Body: UpdateRequestBody(UpdateRequest{
			Version:           1,
			RequestID:         "update-1",
			RoomKey:           "join:203.0.113.10:7331",
			RequesterIdentity: "identity-a",
			RequesterName:     "alice",
		}),
		At: time.Date(2026, 4, 21, 14, 0, 0, 0, time.UTC),
	}

	message := waitForResentMessage(t, member.resent)
	execute, ok := ParseUpdateExecute(message.Body)
	if !ok {
		t.Fatalf("expected execute message, got %#v", message)
	}
	if execute.TargetVersion != "" {
		t.Fatalf("expected empty target version to be broadcast as-is, got %#v", execute)
	}
	if resolverCalled {
		t.Fatal("expected empty target version request not to resolve latest release on host")
	}

	hostMessage := waitForRoomMessage(t, room.Messages())
	if hostMessage.Body != message.Body {
		t.Fatalf("expected host stream to receive execute control, got %#v", hostMessage)
	}
}

func TestHostRoomParticipantNamesIncludeKnownVersionsAndUnknownLegacyPeers(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	memberA := newFakeMember("alice")
	memberB := newFakeMember("bob")
	room.AddMember(memberA)
	room.AddMember(memberB)
	drainJoinEvents(t, room, 2)

	memberA.messages <- session.Message{
		ID:   "sync-hello-versioned",
		From: "alice",
		Body: HistorySyncHelloBody(HistorySyncHello{
			Version:       1,
			IdentityID:    "identity-a",
			ClientVersion: "v0.1.24",
			RoomKey:       "room",
		}),
		At: time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC),
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		names := room.ParticipantNames()
		joined := strings.Join(names, ", ")
		if strings.Contains(joined, "alice [v0.1.24]") {
			if !strings.Contains(joined, "bob [unknown]") {
				t.Fatalf("expected legacy peer to report unknown version, got %q", joined)
			}
			if !strings.Contains(joined, "host [") {
				t.Fatalf("expected host version to be included, got %q", joined)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("expected versioned participant roster, got %#v", room.ParticipantNames())
}

func TestHostRoomAcceptsUnlistedConnectedJoinUpdateRequest(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	member := newFakeMember("eve")
	room.identityByPeerName["eve"] = "identity-eve"
	room.AddMember(member)
	drainJoinEvents(t, room, 1)

	member.messages <- session.Message{
		ID:   "req-1",
		From: "eve",
		Body: UpdateRequestBody(UpdateRequest{
			Version:           1,
			RequestID:         "update-1",
			RoomKey:           "join:203.0.113.10:7331",
			RequesterIdentity: "identity-eve",
			RequesterName:     "eve",
			TargetVersion:     "v0.1.24",
		}),
		At: time.Date(2026, 4, 21, 14, 1, 0, 0, time.UTC),
	}

	message := waitForResentMessage(t, member.resent)
	execute, ok := ParseUpdateExecute(message.Body)
	if !ok {
		t.Fatalf("expected execute message, got %#v", message)
	}
	if execute.TargetVersion != "v0.1.24" || execute.InitiatorName != "eve" {
		t.Fatalf("expected connected join request to be accepted, got %#v", execute)
	}

	hostMessage := waitForRoomMessage(t, room.Messages())
	if hostMessage.Body != message.Body {
		t.Fatalf("expected host stream to receive execute control, got %#v", hostMessage)
	}
}

func TestHostRoomRejectsNonMemberUpdateRequestWithoutWhitelist(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	room.handleUpdateRequest(trackedMember{}, UpdateRequest{
		Version:           1,
		RequestID:         "update-external-1",
		RoomKey:           "join:203.0.113.10:7331",
		RequesterIdentity: "identity-external",
		RequesterName:     "external",
		TargetVersion:     "v0.1.24",
		At:                time.Date(2026, 4, 21, 14, 1, 30, 0, time.UTC),
	})

	message := waitForRoomMessage(t, room.Messages())
	result, ok := ParseUpdateResult(message.Body)
	if !ok {
		t.Fatalf("expected update result, got %#v", message)
	}
	if result.Status != "permission-denied" {
		t.Fatalf("expected non-member request to stay denied, got %#v", result)
	}
}

func TestHostRoomBroadcastsUpdateResultsToRoom(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	memberA := newFakeMember("alice")
	memberB := newFakeMember("bob")
	room.identityByPeerName["alice"] = "identity-a"
	room.identityByPeerName["bob"] = "identity-b"
	room.AddMember(memberA)
	room.AddMember(memberB)
	drainJoinEvents(t, room, 2)

	memberA.messages <- session.Message{
		ID:   "result-1",
		From: "alice",
		Body: UpdateResultBody(UpdateResult{
			Version:        1,
			RequestID:      "update-1",
			RoomKey:        "join:203.0.113.10:7331",
			ReporterName:   "alice",
			ReporterID:     "identity-a",
			TargetVersion:  "v0.1.24",
			Status:         "success",
			CurrentVersion: "v0.1.23",
		}),
		At: time.Date(2026, 4, 21, 14, 2, 0, 0, time.UTC),
	}

	rebroadcast := waitForResentMessage(t, memberA.resent)
	if _, ok := ParseUpdateResult(rebroadcast.Body); !ok {
		t.Fatalf("expected sender to receive broadcast update result, got %#v", rebroadcast)
	}
	other := waitForResentMessage(t, memberB.resent)
	result, ok := ParseUpdateResult(other.Body)
	if !ok {
		t.Fatalf("expected other member to receive update result, got %#v", other)
	}
	if result.ReporterName != "alice" || result.ReporterID != "identity-a" || result.Status != "success" {
		t.Fatalf("expected broadcast result details to round-trip, got %#v", result)
	}

	hostMessage := waitForRoomMessage(t, room.Messages())
	if hostMessage.Body != other.Body {
		t.Fatalf("expected host stream to receive broadcast result, got %#v", hostMessage)
	}
}

func TestHostRoomSubmitUpdateRequestBroadcastsExecute(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	member := newFakeMember("alice")
	room.AddMember(member)
	drainJoinEvents(t, room, 1)

	if err := room.SubmitUpdateRequest(UpdateRequest{
		Version:           1,
		RequestID:         "update-host-1",
		RoomKey:           "join:203.0.113.10:7331",
		RequesterIdentity: "identity-host",
		RequesterName:     "host",
	}); err != nil {
		t.Fatalf("SubmitUpdateRequest returned error: %v", err)
	}

	message := waitForResentMessage(t, member.resent)
	execute, ok := ParseUpdateExecute(message.Body)
	if !ok {
		t.Fatalf("expected execute message, got %#v", message)
	}
	if execute.TargetVersion != "" || execute.InitiatorName != "host" {
		t.Fatalf("expected host submit to broadcast execute, got %#v", execute)
	}
}

type fakeMember struct {
	peerName string
	messages chan session.Message
	resent   chan session.Message
	receipts chan session.Receipt
	done     chan struct{}
}

func newFakeMember(peerName string) *fakeMember {
	return &fakeMember{
		peerName: peerName,
		messages: make(chan session.Message, 8),
		resent:   make(chan session.Message, 8),
		receipts: make(chan session.Receipt, 8),
		done:     make(chan struct{}),
	}
}

func (f *fakeMember) Messages() <-chan session.Message { return f.messages }
func (f *fakeMember) Receipts() <-chan session.Receipt { return f.receipts }
func (f *fakeMember) Done() <-chan struct{}            { return f.done }
func (f *fakeMember) Err() error                       { return nil }
func (f *fakeMember) Close() error {
	select {
	case <-f.done:
	default:
		close(f.done)
	}
	return nil
}
func (f *fakeMember) PeerName() string { return f.peerName }
func (f *fakeMember) Resend(message session.Message) error {
	f.resent <- message
	return nil
}

func waitForRoomMessage(t *testing.T, messages <-chan session.Message) session.Message {
	t.Helper()

	select {
	case message := <-messages:
		return message
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for room message")
		return session.Message{}
	}
}

func assertNoRoomMessage(t *testing.T, messages <-chan session.Message) {
	t.Helper()

	select {
	case message := <-messages:
		t.Fatalf("expected no room message, got %#v", message)
	case <-time.After(100 * time.Millisecond):
	}
}

func waitForRoomEvent(t *testing.T, events <-chan Event) Event {
	t.Helper()

	select {
	case event := <-events:
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for room event")
		return Event{}
	}
}

func waitForResentMessage(t *testing.T, messages <-chan session.Message) session.Message {
	t.Helper()

	select {
	case message := <-messages:
		return message
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for resent message")
		return session.Message{}
	}
}

func assertNoResentMessage(t *testing.T, messages <-chan session.Message) {
	t.Helper()

	select {
	case message := <-messages:
		t.Fatalf("expected no resent message, got %#v", message)
	case <-time.After(100 * time.Millisecond):
	}
}

func drainJoinEvents(t *testing.T, room *HostRoom, count int) {
	t.Helper()

	for range count {
		event := waitForRoomEvent(t, room.Events())
		if event.Kind != EventPeerJoined {
			t.Fatalf("expected joined event while draining, got %#v", event)
		}
	}
}
