package room

import (
	"context"
	"testing"
	"time"

	"chatbox/internal/session"
)

func TestGroupHostRelaysMessagesBetweenTwoJoiners(t *testing.T) {
	t.Parallel()

	room, joinerA, joinerB := newLiveHostRoom(t)
	drainRoomJoinEvents(t, room, 2)

	sent, err := joinerA.Send("hello room")
	if err != nil {
		t.Fatalf("joiner A Send returned error: %v", err)
	}

	receipt := waitForSessionReceipt(t, joinerA.Receipts())
	if receipt.MessageID != sent.ID {
		t.Fatalf("expected joiner A receipt for %q, got %q", sent.ID, receipt.MessageID)
	}

	gotHost := waitForRoomMessage(t, room.Messages())
	if gotHost.ID != sent.ID {
		t.Fatalf("expected host stream message ID %q, got %q", sent.ID, gotHost.ID)
	}
	if gotHost.From != "aaa" {
		t.Fatalf("expected host stream sender aaa, got %q", gotHost.From)
	}
	if gotHost.Body != sent.Body {
		t.Fatalf("expected host stream body %q, got %q", sent.Body, gotHost.Body)
	}

	gotJoinerB := waitForSessionMessage(t, joinerB.Messages())
	if gotJoinerB.ID != sent.ID {
		t.Fatalf("expected joiner B message ID %q, got %q", sent.ID, gotJoinerB.ID)
	}
	if gotJoinerB.From != "aaa" {
		t.Fatalf("expected joiner B sender aaa, got %q", gotJoinerB.From)
	}
	if gotJoinerB.Body != sent.Body {
		t.Fatalf("expected joiner B body %q, got %q", sent.Body, gotJoinerB.Body)
	}

	assertNoSessionMessage(t, joinerA.Messages())
	assertNoRoomReceipt(t, room.Receipts())
}

func TestGroupHostBroadcastsHostMessagesToAllJoiners(t *testing.T) {
	t.Parallel()

	room, joinerA, joinerB := newLiveHostRoom(t)
	drainRoomJoinEvents(t, room, 2)

	sent, err := room.Send("hello everyone")
	if err != nil {
		t.Fatalf("host room Send returned error: %v", err)
	}

	receipt := waitForRoomReceipt(t, room.Receipts())
	if receipt.MessageID != sent.ID {
		t.Fatalf("expected host room receipt for %q, got %q", sent.ID, receipt.MessageID)
	}

	gotHost := waitForRoomMessage(t, room.Messages())
	if gotHost != sent {
		t.Fatalf("expected host stream message %#v, got %#v", sent, gotHost)
	}

	gotJoinerA := waitForSessionMessage(t, joinerA.Messages())
	if gotJoinerA.ID != sent.ID {
		t.Fatalf("expected joiner A message ID %q, got %q", sent.ID, gotJoinerA.ID)
	}
	if gotJoinerA.From != "host" {
		t.Fatalf("expected joiner A sender host, got %q", gotJoinerA.From)
	}
	if gotJoinerA.Body != sent.Body {
		t.Fatalf("expected joiner A body %q, got %q", sent.Body, gotJoinerA.Body)
	}

	gotJoinerB := waitForSessionMessage(t, joinerB.Messages())
	if gotJoinerB.ID != sent.ID {
		t.Fatalf("expected joiner B message ID %q, got %q", sent.ID, gotJoinerB.ID)
	}
	if gotJoinerB.From != "host" {
		t.Fatalf("expected joiner B sender host, got %q", gotJoinerB.From)
	}
	if gotJoinerB.Body != sent.Body {
		t.Fatalf("expected joiner B body %q, got %q", sent.Body, gotJoinerB.Body)
	}
}

func newLiveHostRoom(t *testing.T) (*HostRoom, *session.Session, *session.Session) {
	t.Helper()

	cfg := session.Config{
		Name:              "host",
		PSK:               bytesForTest(0x66),
		Version:           session.ProtocolVersion,
		HandshakeTimeout:  2 * time.Second,
		HeartbeatInterval: 50 * time.Millisecond,
		IdleTimeout:       200 * time.Millisecond,
		MaxMessageSize:    4 * 1024,
	}

	host, err := session.Listen("127.0.0.1:0", cfg)
	if err != nil {
		t.Fatalf("Listen returned error: %v", err)
	}

	room := NewHostRoom("host")
	go room.Serve(context.Background(), host)

	joinerA, err := session.Dial(context.Background(), host.Addr(), withName(cfg, "aaa"))
	if err != nil {
		_ = room.Close()
		_ = host.Close()
		t.Fatalf("Dial for joiner A returned error: %v", err)
	}
	joinerB, err := session.Dial(context.Background(), host.Addr(), withName(cfg, "bbb"))
	if err != nil {
		_ = joinerA.Close()
		_ = room.Close()
		_ = host.Close()
		t.Fatalf("Dial for joiner B returned error: %v", err)
	}

	t.Cleanup(func() {
		_ = joinerA.Close()
		_ = joinerB.Close()
		_ = room.Close()
		_ = host.Close()
	})

	return room, joinerA, joinerB
}

func waitForSessionMessage(t *testing.T, messages <-chan session.Message) session.Message {
	t.Helper()

	select {
	case message := <-messages:
		return message
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for session message")
		return session.Message{}
	}
}

func waitForSessionReceipt(t *testing.T, receipts <-chan session.Receipt) session.Receipt {
	t.Helper()

	select {
	case receipt := <-receipts:
		return receipt
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for session receipt")
		return session.Receipt{}
	}
}

func waitForRoomReceipt(t *testing.T, receipts <-chan session.Receipt) session.Receipt {
	t.Helper()

	select {
	case receipt := <-receipts:
		return receipt
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for room receipt")
		return session.Receipt{}
	}
}

func drainRoomJoinEvents(t *testing.T, room *HostRoom, count int) {
	t.Helper()

	for range count {
		event := waitForRoomEvent(t, room.Events())
		if event.Kind != EventPeerJoined {
			t.Fatalf("expected joined event while draining, got %#v", event)
		}
	}
}

func assertNoSessionMessage(t *testing.T, messages <-chan session.Message) {
	t.Helper()

	select {
	case message := <-messages:
		t.Fatalf("expected no session message, got %#v", message)
	case <-time.After(150 * time.Millisecond):
	}
}

func assertNoRoomReceipt(t *testing.T, receipts <-chan session.Receipt) {
	t.Helper()

	select {
	case receipt := <-receipts:
		t.Fatalf("expected no room receipt, got %#v", receipt)
	case <-time.After(150 * time.Millisecond):
	}
}

func withName(cfg session.Config, name string) session.Config {
	cfg.Name = name
	return cfg
}

func bytesForTest(value byte) []byte {
	buf := make([]byte, 32)
	for i := range buf {
		buf[i] = value
	}
	return buf
}
