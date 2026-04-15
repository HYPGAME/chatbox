package session

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"
)

func TestHandshakeDerivesMirroredSessionKeys(t *testing.T) {
	t.Parallel()

	serverConn, clientConn := net.Pipe()
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

	type result struct {
		state *handshakeState
		err   error
	}

	serverCh := make(chan result, 1)
	clientCh := make(chan result, 1)

	go func() {
		state, err := serverHandshake(context.Background(), serverConn, serverCfg)
		serverCh <- result{state: state, err: err}
	}()
	go func() {
		state, err := clientHandshake(context.Background(), clientConn, clientCfg)
		clientCh <- result{state: state, err: err}
	}()

	serverResult := <-serverCh
	clientResult := <-clientCh

	if serverResult.err != nil {
		t.Fatalf("serverHandshake returned error: %v", serverResult.err)
	}
	if clientResult.err != nil {
		t.Fatalf("clientHandshake returned error: %v", clientResult.err)
	}
	if got := serverResult.state.peerName; got != "joiner" {
		t.Fatalf("expected server to see peer name joiner, got %q", got)
	}
	if got := clientResult.state.peerName; got != "host" {
		t.Fatalf("expected client to see peer name host, got %q", got)
	}
	if !bytes.Equal(clientResult.state.sendKey, serverResult.state.recvKey) {
		t.Fatal("expected client send key to match server receive key")
	}
	if !bytes.Equal(clientResult.state.recvKey, serverResult.state.sendKey) {
		t.Fatal("expected client receive key to match server send key")
	}
}

func TestHandshakeRejectsWrongPSK(t *testing.T) {
	t.Parallel()

	serverConn, clientConn := net.Pipe()
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
		PSK:              bytes.Repeat([]byte{0x22}, 32),
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

	if err := <-serverCh; err == nil {
		t.Fatal("expected serverHandshake to fail for mismatched PSK")
	}
	if err := <-clientCh; err == nil {
		t.Fatal("expected clientHandshake to fail for mismatched PSK")
	}
}

func TestHandshakeRejectsVersionMismatch(t *testing.T) {
	t.Parallel()

	serverConn, clientConn := net.Pipe()
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
		Version:          ProtocolVersion + 1,
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

	if err := <-serverCh; err == nil {
		t.Fatal("expected serverHandshake to fail for version mismatch")
	}
	if err := <-clientCh; err == nil {
		t.Fatal("expected clientHandshake to fail for version mismatch")
	}
}

func TestCipherStateRejectsReplaySequence(t *testing.T) {
	t.Parallel()

	state, err := newCipherState(bytes.Repeat([]byte{0x33}, 32))
	if err != nil {
		t.Fatalf("newCipherState returned error: %v", err)
	}

	ciphertext, err := state.seal(frameTypeData, []byte("hello"))
	if err != nil {
		t.Fatalf("seal returned error: %v", err)
	}

	inbound, err := newCipherState(bytes.Repeat([]byte{0x33}, 32))
	if err != nil {
		t.Fatalf("newCipherState returned error: %v", err)
	}

	if _, _, err := inbound.open(ciphertext); err != nil {
		t.Fatalf("first open returned error: %v", err)
	}
	if _, _, err := inbound.open(ciphertext); err == nil {
		t.Fatal("expected second open with same sequence to fail")
	}
}
