package session

import (
	"context"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

const (
	messageTypeClientHello byte = 1
	messageTypeServerHello byte = 2
	messageTypeClientAuth  byte = 3

	nonceSize   = 32
	proofSize   = 32
	maxNameSize = 128
)

type handshakeState struct {
	peerName string
	sendKey  []byte
	recvKey  []byte
}

func clientHandshake(ctx context.Context, conn net.Conn, cfg Config) (*handshakeState, error) {
	cfg = cfg.withDefaults()
	if err := applyHandshakeDeadline(ctx, conn, cfg); err != nil {
		return nil, err
	}
	defer conn.SetDeadline(time.Time{})

	clientNonce, err := randomNonce()
	if err != nil {
		return nil, err
	}

	if err := writeClientHello(conn, cfg.Version, cfg.Name, clientNonce); err != nil {
		return nil, err
	}

	serverHello, err := readServerHello(conn)
	if err != nil {
		return nil, err
	}
	if serverHello.version != cfg.Version {
		return nil, fmt.Errorf("protocol version mismatch: local=%d remote=%d", cfg.Version, serverHello.version)
	}

	expectedServerProof := computeProof(cfg.PSK, "server-proof", clientNonce, serverHello.nonce)
	if !hmac.Equal(expectedServerProof[:], serverHello.proof[:]) {
		return nil, errors.New("server authentication failed")
	}

	clientProof := computeProof(cfg.PSK, "client-proof", clientNonce, serverHello.nonce)
	if err := writeClientAuth(conn, clientProof); err != nil {
		return nil, err
	}

	sendKey, recvKey, err := deriveDirectionalKeys(cfg.PSK, clientNonce, serverHello.nonce, true)
	if err != nil {
		return nil, err
	}

	return &handshakeState{
		peerName: serverHello.name,
		sendKey:  sendKey,
		recvKey:  recvKey,
	}, nil
}

func serverHandshake(ctx context.Context, conn net.Conn, cfg Config) (*handshakeState, error) {
	cfg = cfg.withDefaults()
	if err := applyHandshakeDeadline(ctx, conn, cfg); err != nil {
		return nil, err
	}
	defer conn.SetDeadline(time.Time{})

	clientHello, err := readClientHello(conn)
	if err != nil {
		return nil, err
	}
	if clientHello.version != cfg.Version {
		return nil, fmt.Errorf("protocol version mismatch: local=%d remote=%d", cfg.Version, clientHello.version)
	}

	serverNonce, err := randomNonce()
	if err != nil {
		return nil, err
	}

	serverProof := computeProof(cfg.PSK, "server-proof", clientHello.nonce, serverNonce)
	if err := writeServerHello(conn, cfg.Version, cfg.Name, serverNonce, serverProof); err != nil {
		return nil, err
	}

	clientProof, err := readClientAuth(conn)
	if err != nil {
		return nil, err
	}
	expectedClientProof := computeProof(cfg.PSK, "client-proof", clientHello.nonce, serverNonce)
	if !hmac.Equal(expectedClientProof[:], clientProof[:]) {
		return nil, errors.New("client authentication failed")
	}

	sendKey, recvKey, err := deriveDirectionalKeys(cfg.PSK, clientHello.nonce, serverNonce, false)
	if err != nil {
		return nil, err
	}

	return &handshakeState{
		peerName: clientHello.name,
		sendKey:  sendKey,
		recvKey:  recvKey,
	}, nil
}

type clientHello struct {
	version uint16
	name    string
	nonce   [nonceSize]byte
}

type serverHello struct {
	version uint16
	name    string
	nonce   [nonceSize]byte
	proof   [proofSize]byte
}

func applyHandshakeDeadline(ctx context.Context, conn net.Conn, cfg Config) error {
	deadline := time.Time{}
	if cfg.HandshakeTimeout > 0 {
		deadline = time.Now().Add(cfg.HandshakeTimeout)
	}
	if ctx != nil {
		if ctxDeadline, ok := ctx.Deadline(); ok && (deadline.IsZero() || ctxDeadline.Before(deadline)) {
			deadline = ctxDeadline
		}
	}
	if deadline.IsZero() {
		return nil
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("set handshake deadline: %w", err)
	}

	return nil
}

func writeClientHello(w io.Writer, version uint16, name string, nonce [nonceSize]byte) error {
	if err := writeMessageHeader(w, messageTypeClientHello); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, version); err != nil {
		return fmt.Errorf("write version: %w", err)
	}
	if _, err := w.Write(nonce[:]); err != nil {
		return fmt.Errorf("write nonce: %w", err)
	}
	return writeName(w, name)
}

func readClientHello(r io.Reader) (clientHello, error) {
	messageType, err := readMessageHeader(r)
	if err != nil {
		return clientHello{}, err
	}
	if messageType != messageTypeClientHello {
		return clientHello{}, fmt.Errorf("unexpected message type %d", messageType)
	}

	var hello clientHello
	if err := binary.Read(r, binary.BigEndian, &hello.version); err != nil {
		return clientHello{}, fmt.Errorf("read version: %w", err)
	}
	if _, err := io.ReadFull(r, hello.nonce[:]); err != nil {
		return clientHello{}, fmt.Errorf("read nonce: %w", err)
	}
	hello.name, err = readName(r)
	if err != nil {
		return clientHello{}, err
	}
	return hello, nil
}

func writeServerHello(w io.Writer, version uint16, name string, nonce [nonceSize]byte, proof [proofSize]byte) error {
	if err := writeMessageHeader(w, messageTypeServerHello); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, version); err != nil {
		return fmt.Errorf("write version: %w", err)
	}
	if _, err := w.Write(nonce[:]); err != nil {
		return fmt.Errorf("write nonce: %w", err)
	}
	if _, err := w.Write(proof[:]); err != nil {
		return fmt.Errorf("write proof: %w", err)
	}
	return writeName(w, name)
}

func readServerHello(r io.Reader) (serverHello, error) {
	messageType, err := readMessageHeader(r)
	if err != nil {
		return serverHello{}, err
	}
	if messageType != messageTypeServerHello {
		return serverHello{}, fmt.Errorf("unexpected message type %d", messageType)
	}

	var hello serverHello
	if err := binary.Read(r, binary.BigEndian, &hello.version); err != nil {
		return serverHello{}, fmt.Errorf("read version: %w", err)
	}
	if _, err := io.ReadFull(r, hello.nonce[:]); err != nil {
		return serverHello{}, fmt.Errorf("read nonce: %w", err)
	}
	if _, err := io.ReadFull(r, hello.proof[:]); err != nil {
		return serverHello{}, fmt.Errorf("read proof: %w", err)
	}
	hello.name, err = readName(r)
	if err != nil {
		return serverHello{}, err
	}
	return hello, nil
}

func writeClientAuth(w io.Writer, proof [proofSize]byte) error {
	if err := writeMessageHeader(w, messageTypeClientAuth); err != nil {
		return err
	}
	if _, err := w.Write(proof[:]); err != nil {
		return fmt.Errorf("write client proof: %w", err)
	}
	return nil
}

func readClientAuth(r io.Reader) ([proofSize]byte, error) {
	messageType, err := readMessageHeader(r)
	if err != nil {
		return [proofSize]byte{}, err
	}
	if messageType != messageTypeClientAuth {
		return [proofSize]byte{}, fmt.Errorf("unexpected message type %d", messageType)
	}

	var proof [proofSize]byte
	if _, err := io.ReadFull(r, proof[:]); err != nil {
		return [proofSize]byte{}, fmt.Errorf("read client proof: %w", err)
	}
	return proof, nil
}

func computeProof(psk []byte, label string, clientNonce, serverNonce [nonceSize]byte) [proofSize]byte {
	mac := hmac.New(sha256.New, psk)
	mac.Write([]byte(label))
	mac.Write(clientNonce[:])
	mac.Write(serverNonce[:])

	var proof [proofSize]byte
	copy(proof[:], mac.Sum(nil))
	return proof
}

func deriveDirectionalKeys(psk []byte, clientNonce, serverNonce [nonceSize]byte, clientSide bool) ([]byte, []byte, error) {
	sendKey, err := deriveKey(psk, clientNonce, serverNonce, roleLabel(clientSide, true))
	if err != nil {
		return nil, nil, err
	}
	recvKey, err := deriveKey(psk, clientNonce, serverNonce, roleLabel(clientSide, false))
	if err != nil {
		return nil, nil, err
	}
	return sendKey, recvKey, nil
}

func deriveKey(psk []byte, clientNonce, serverNonce [nonceSize]byte, label string) ([]byte, error) {
	salt := append(clientNonce[:], serverNonce[:]...)
	key, err := hkdf.Key(sha256.New, psk, salt, label, 32)
	if err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}
	return key, nil
}

func roleLabel(clientSide, outbound bool) string {
	switch {
	case clientSide && outbound:
		return "client->server"
	case clientSide && !outbound:
		return "server->client"
	case !clientSide && outbound:
		return "server->client"
	default:
		return "client->server"
	}
}

func writeMessageHeader(w io.Writer, messageType byte) error {
	_, err := w.Write([]byte{messageType})
	if err != nil {
		return fmt.Errorf("write message header: %w", err)
	}
	return nil
}

func readMessageHeader(r io.Reader) (byte, error) {
	var header [1]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, fmt.Errorf("read message header: %w", err)
	}
	return header[0], nil
}

func writeName(w io.Writer, name string) error {
	if len(name) > maxNameSize {
		return fmt.Errorf("name too long: %d", len(name))
	}
	if err := binary.Write(w, binary.BigEndian, uint16(len(name))); err != nil {
		return fmt.Errorf("write name length: %w", err)
	}
	if _, err := w.Write([]byte(name)); err != nil {
		return fmt.Errorf("write name: %w", err)
	}
	return nil
}

func readName(r io.Reader) (string, error) {
	var length uint16
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return "", fmt.Errorf("read name length: %w", err)
	}
	if length > maxNameSize {
		return "", fmt.Errorf("name too long: %d", length)
	}
	name := make([]byte, length)
	if _, err := io.ReadFull(r, name); err != nil {
		return "", fmt.Errorf("read name: %w", err)
	}
	return string(name), nil
}

func randomNonce() ([nonceSize]byte, error) {
	var nonce [nonceSize]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return [nonceSize]byte{}, fmt.Errorf("generate nonce: %w", err)
	}
	return nonce, nil
}
