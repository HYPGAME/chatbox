package session

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
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

func TestSessionDeliversDataBeforeAckWriteFailureCloses(t *testing.T) {
	t.Parallel()

	message := Message{
		ID:   "msg-ack-failure",
		From: "host",
		Body: "visible before ack failure",
		At:   time.Date(2026, 5, 14, 18, 0, 0, 0, time.UTC),
	}
	conn, recvCipher := newSingleInboundPacketConn(t, message, errors.New("ack write failed"))
	sendCipher, err := newCipherState(bytesForTest(0x66))
	if err != nil {
		t.Fatalf("newCipherState returned error: %v", err)
	}
	session := newManualSession(conn, sendCipher, recvCipher)

	go session.readLoop()

	select {
	case got, ok := <-session.Messages():
		if !ok {
			t.Fatal("expected message before session closed")
		}
		if got.ID != message.ID || got.Body != message.Body {
			t.Fatalf("expected delivered message %#v, got %#v", message, got)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("timed out waiting for message delivery")
	}

	select {
	case <-session.Done():
	case <-time.After(250 * time.Millisecond):
		t.Fatal("timed out waiting for session to close after ack failure")
	}
}

func TestSessionWriteFrameTimesOutBlockedWriter(t *testing.T) {
	t.Parallel()

	conn := newDeadlineBlockingConn()
	defer conn.Close()
	sendCipher, err := newCipherState(bytesForTest(0x77))
	if err != nil {
		t.Fatalf("newCipherState returned error: %v", err)
	}
	session := &Session{
		conn:       conn,
		cfg:        Config{WriteTimeout: 20 * time.Millisecond}.withDefaults(),
		sendCipher: sendCipher,
	}

	done := make(chan error, 1)
	go func() {
		done <- session.writeFrame(frameTypePing, nil)
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected write timeout error")
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("timed out waiting for blocked write to fail")
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

func newSingleInboundPacketConn(t *testing.T, message Message, writeErr error) (*singleInboundPacketConn, *cipherState) {
	t.Helper()

	key := bytesForTest(0x55)
	sealer, err := newCipherState(key)
	if err != nil {
		t.Fatalf("newCipherState returned error: %v", err)
	}
	recvCipher, err := newCipherState(key)
	if err != nil {
		t.Fatalf("newCipherState returned error: %v", err)
	}
	payload, err := encodeMessagePayload(message)
	if err != nil {
		t.Fatalf("encodeMessagePayload returned error: %v", err)
	}
	packet, err := sealer.seal(frameTypeData, payload)
	if err != nil {
		t.Fatalf("seal returned error: %v", err)
	}
	var framed bytes.Buffer
	if err := writePacket(&framed, packet); err != nil {
		t.Fatalf("writePacket returned error: %v", err)
	}
	return &singleInboundPacketConn{
		reader:   bytes.NewReader(framed.Bytes()),
		writeErr: writeErr,
	}, recvCipher
}

func newManualSession(conn net.Conn, sendCipher, recvCipher *cipherState) *Session {
	session := &Session{
		conn:              conn,
		cfg:               Config{WriteTimeout: 20 * time.Millisecond}.withDefaults(),
		peerName:          "host",
		negotiatedVersion: 3,
		sendCipher:        sendCipher,
		recvCipher:        recvCipher,
		messages:          make(chan Message, 1),
		receipts:          make(chan Receipt, 1),
		done:              make(chan struct{}),
		seenIDs:           make(map[string]struct{}),
	}
	session.touch()
	return session
}

type singleInboundPacketConn struct {
	reader   *bytes.Reader
	writeErr error
}

func (c *singleInboundPacketConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

func (c *singleInboundPacketConn) Write([]byte) (int, error) {
	return 0, c.writeErr
}

func (c *singleInboundPacketConn) Close() error                     { return nil }
func (c *singleInboundPacketConn) LocalAddr() net.Addr              { return testAddr("local") }
func (c *singleInboundPacketConn) RemoteAddr() net.Addr             { return testAddr("remote") }
func (c *singleInboundPacketConn) SetDeadline(time.Time) error      { return nil }
func (c *singleInboundPacketConn) SetReadDeadline(time.Time) error  { return nil }
func (c *singleInboundPacketConn) SetWriteDeadline(time.Time) error { return nil }

type deadlineBlockingConn struct {
	mu            sync.Mutex
	writeDeadline time.Time
	closed        chan struct{}
}

func newDeadlineBlockingConn() *deadlineBlockingConn {
	return &deadlineBlockingConn{closed: make(chan struct{})}
}

func (c *deadlineBlockingConn) Read([]byte) (int, error) {
	<-c.closed
	return 0, io.ErrClosedPipe
}

func (c *deadlineBlockingConn) Write([]byte) (int, error) {
	for {
		c.mu.Lock()
		deadline := c.writeDeadline
		c.mu.Unlock()

		if !deadline.IsZero() {
			timer := time.NewTimer(time.Until(deadline))
			select {
			case <-timer.C:
				return 0, osErrDeadlineExceeded{}
			case <-c.closed:
				if !timer.Stop() {
					<-timer.C
				}
				return 0, io.ErrClosedPipe
			}
		}

		select {
		case <-c.closed:
			return 0, io.ErrClosedPipe
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func (c *deadlineBlockingConn) Close() error {
	select {
	case <-c.closed:
	default:
		close(c.closed)
	}
	return nil
}

func (c *deadlineBlockingConn) LocalAddr() net.Addr  { return testAddr("local") }
func (c *deadlineBlockingConn) RemoteAddr() net.Addr { return testAddr("remote") }
func (c *deadlineBlockingConn) SetDeadline(t time.Time) error {
	return c.SetWriteDeadline(t)
}
func (c *deadlineBlockingConn) SetReadDeadline(time.Time) error { return nil }
func (c *deadlineBlockingConn) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	c.writeDeadline = t
	c.mu.Unlock()
	return nil
}

type testAddr string

func (a testAddr) Network() string { return "test" }
func (a testAddr) String() string  { return string(a) }

type osErrDeadlineExceeded struct{}

func (osErrDeadlineExceeded) Error() string   { return "i/o timeout" }
func (osErrDeadlineExceeded) Timeout() bool   { return true }
func (osErrDeadlineExceeded) Temporary() bool { return true }

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
