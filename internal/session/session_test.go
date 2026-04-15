package session

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestSessionsExchangeMessagesBidirectionally(t *testing.T) {
	t.Parallel()

	serverSession, clientSession := newConnectedSessions(t)
	defer serverSession.Close()
	defer clientSession.Close()

	beforeSend := time.Now()
	sentByClient, err := clientSession.Send("hello from joiner")
	if err != nil {
		t.Fatalf("client Send returned error: %v", err)
	}
	afterSend := time.Now()
	clientReceipt := waitForReceipt(t, clientSession.Receipts())
	if clientReceipt.MessageID != sentByClient.ID {
		t.Fatalf("expected receipt message ID %q, got %q", sentByClient.ID, clientReceipt.MessageID)
	}
	serverMessage := waitForMessage(t, serverSession.Messages())
	if serverMessage.ID != sentByClient.ID {
		t.Fatalf("expected server message ID %q, got %q", sentByClient.ID, serverMessage.ID)
	}
	if got := serverMessage.Body; got != "hello from joiner" {
		t.Fatalf("expected server message body %q, got %q", "hello from joiner", got)
	}
	if got := serverMessage.From; got != "joiner" {
		t.Fatalf("expected server message sender joiner, got %q", got)
	}
	if serverMessage.At.Before(beforeSend.Add(-time.Second)) || serverMessage.At.After(afterSend.Add(time.Second)) {
		t.Fatalf("expected server message timestamp between send bounds, got %s", serverMessage.At)
	}

	beforeReply := time.Now()
	sentByServer, err := serverSession.Send("hello from host")
	if err != nil {
		t.Fatalf("server Send returned error: %v", err)
	}
	afterReply := time.Now()
	serverReceipt := waitForReceipt(t, serverSession.Receipts())
	if serverReceipt.MessageID != sentByServer.ID {
		t.Fatalf("expected receipt message ID %q, got %q", sentByServer.ID, serverReceipt.MessageID)
	}
	clientMessage := waitForMessage(t, clientSession.Messages())
	if clientMessage.ID != sentByServer.ID {
		t.Fatalf("expected client message ID %q, got %q", sentByServer.ID, clientMessage.ID)
	}
	if got := clientMessage.Body; got != "hello from host" {
		t.Fatalf("expected client message body %q, got %q", "hello from host", got)
	}
	if got := clientMessage.From; got != "host" {
		t.Fatalf("expected client message sender host, got %q", got)
	}
	if clientMessage.At.Before(beforeReply.Add(-time.Second)) || clientMessage.At.After(afterReply.Add(time.Second)) {
		t.Fatalf("expected client message timestamp between send bounds, got %s", clientMessage.At)
	}
}

func TestSessionSendRejectsOversizedMessage(t *testing.T) {
	t.Parallel()

	serverSession, clientSession := newConnectedSessions(t)
	defer serverSession.Close()
	defer clientSession.Close()

	message := strings.Repeat("a", clientSession.cfg.MaxMessageSize+1)
	if _, err := clientSession.Send(message); err == nil {
		t.Fatal("expected Send to reject oversized message")
	}
}

func TestSessionResendDoesNotDuplicateDeliveredMessage(t *testing.T) {
	t.Parallel()

	serverSession, clientSession := newConnectedSessions(t)
	defer serverSession.Close()
	defer clientSession.Close()

	sent, err := clientSession.Send("hello once")
	if err != nil {
		t.Fatalf("client Send returned error: %v", err)
	}
	_ = waitForReceipt(t, clientSession.Receipts())
	_ = waitForMessage(t, serverSession.Messages())

	if err := clientSession.Resend(sent); err != nil {
		t.Fatalf("Resend returned error: %v", err)
	}

	receipt := waitForReceipt(t, clientSession.Receipts())
	if receipt.MessageID != sent.ID {
		t.Fatalf("expected resend receipt for %q, got %q", sent.ID, receipt.MessageID)
	}

	select {
	case duplicate := <-serverSession.Messages():
		t.Fatalf("expected receiver to deduplicate resend, got duplicate %#v", duplicate)
	case <-time.After(250 * time.Millisecond):
	}
}

func TestSessionDetectsAbruptDisconnect(t *testing.T) {
	t.Parallel()

	serverSession, clientSession := newConnectedSessions(t)
	defer clientSession.Close()

	if err := serverSession.conn.Close(); err != nil {
		t.Fatalf("closing server transport returned error: %v", err)
	}

	select {
	case <-clientSession.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for client session to detect disconnect")
	}

	if err := clientSession.Err(); err == nil {
		t.Fatal("expected client session to record disconnect error")
	}
}

func TestHandshakeRejectsTamperedServerProof(t *testing.T) {
	t.Parallel()

	serverConn, clientConn := newTamperedServerProofPipe(t)
	defer serverConn.Close()
	defer clientConn.Close()

	serverCfg := Config{
		Name:             "host",
		PSK:              bytes.Repeat([]byte{0x11}, 32),
		Version:          ProtocolVersion,
		HandshakeTimeout: 2 * time.Second,
	}
	clientCfg := Config{
		Name:             "joiner",
		PSK:              bytes.Repeat([]byte{0x11}, 32),
		Version:          ProtocolVersion,
		HandshakeTimeout: 2 * time.Second,
	}

	serverCh := make(chan error, 1)
	clientCh := make(chan error, 1)

	go func() {
		_, err := serverHandshake(context.Background(), serverConn, serverCfg)
		serverCh <- err
	}()
	go func() {
		_, err := clientHandshake(context.Background(), clientConn, clientCfg)
		clientCh <- err
	}()

	if err := <-clientCh; err == nil {
		t.Fatal("expected clientHandshake to reject tampered server proof")
	}
	if err := <-serverCh; err == nil {
		t.Fatal("expected serverHandshake to fail after client rejects tampered proof")
	}
}
