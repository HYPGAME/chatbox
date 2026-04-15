package session

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

func newConnectedSessions(t *testing.T) (*Session, *Session) {
	t.Helper()

	cfg := Config{
		PSK:               bytesForTest(0x44),
		Version:           ProtocolVersion,
		HandshakeTimeout:  2 * time.Second,
		HeartbeatInterval: 50 * time.Millisecond,
		IdleTimeout:       200 * time.Millisecond,
		MaxMessageSize:    4 * 1024,
	}

	host, err := Listen("127.0.0.1:0", withName(cfg, "host"))
	if err != nil {
		t.Fatalf("Listen returned error: %v", err)
	}
	t.Cleanup(func() { _ = host.Close() })

	serverCh := make(chan *Session, 1)
	errCh := make(chan error, 1)

	go func() {
		session, err := host.Accept(context.Background())
		if err != nil {
			errCh <- err
			return
		}
		serverCh <- session
	}()

	clientSession, err := Dial(context.Background(), host.Addr(), withName(cfg, "joiner"))
	if err != nil {
		t.Fatalf("Dial returned error: %v", err)
	}

	select {
	case err := <-errCh:
		t.Fatalf("Accept returned error: %v", err)
	case serverSession := <-serverCh:
		return serverSession, clientSession
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for server session")
	}

	return nil, nil
}

func waitForMessage(t *testing.T, messages <-chan Message) Message {
	t.Helper()

	select {
	case message := <-messages:
		return message
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for message")
	}

	return Message{}
}

func waitForReceipt(t *testing.T, receipts <-chan Receipt) Receipt {
	t.Helper()

	select {
	case receipt := <-receipts:
		return receipt
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for receipt")
	}

	return Receipt{}
}

func withName(cfg Config, name string) Config {
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

func newTamperedServerProofPipe(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()

	serverConn, clientConn := net.Pipe()
	return &tamperConn{Conn: serverConn, tamperOffset: 35}, clientConn
}

type tamperConn struct {
	net.Conn
	mu           sync.Mutex
	tamperOffset int
	bytesWritten int
	tampered     bool
}

func (c *tamperConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	chunk := append([]byte(nil), p...)
	if !c.tampered {
		start := c.bytesWritten
		end := start + len(chunk)
		if c.tamperOffset >= start && c.tamperOffset < end {
			chunk[c.tamperOffset-start] ^= 0xff
			c.tampered = true
		}
	}

	n, err := c.Conn.Write(chunk)
	c.bytesWritten += n
	return n, err
}
