package room

import (
	"testing"
	"time"

	"chatbox/internal/session"
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

type fakeMember struct {
	peerName string
	messages chan session.Message
	resent   chan session.Message
	done     chan struct{}
}

func newFakeMember(peerName string) *fakeMember {
	return &fakeMember{
		peerName: peerName,
		messages: make(chan session.Message, 8),
		resent:   make(chan session.Message, 8),
		done:     make(chan struct{}),
	}
}

func (f *fakeMember) Messages() <-chan session.Message { return f.messages }
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
