package session

import (
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	frameTypeData  byte = 1
	frameTypeClose byte = 2
	frameTypePing  byte = 3
	frameTypePong  byte = 4
	frameTypeAck   byte = 5
)

type cipherState struct {
	aead    cipher.AEAD
	sendSeq uint64
	recvSeq uint64
}

func newCipherState(key []byte) (*cipherState, error) {
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	return &cipherState{aead: aead}, nil
}

func (c *cipherState) seal(frameType byte, payload []byte) ([]byte, error) {
	header := make([]byte, 9)
	header[0] = frameType
	binary.BigEndian.PutUint64(header[1:], c.sendSeq)

	nonce := nonceForSequence(c.sendSeq)
	ciphertext := c.aead.Seal(nil, nonce, payload, header)
	c.sendSeq++

	packet := make([]byte, len(header)+len(ciphertext))
	copy(packet, header)
	copy(packet[len(header):], ciphertext)
	return packet, nil
}

func (c *cipherState) open(packet []byte) (byte, []byte, error) {
	if len(packet) < 9+c.aead.Overhead() {
		return 0, nil, errors.New("packet too short")
	}

	frameType := packet[0]
	seq := binary.BigEndian.Uint64(packet[1:9])
	if seq != c.recvSeq {
		return 0, nil, fmt.Errorf("unexpected sequence %d", seq)
	}

	header := packet[:9]
	nonce := nonceForSequence(seq)
	plaintext, err := c.aead.Open(nil, nonce, packet[9:], header)
	if err != nil {
		return 0, nil, fmt.Errorf("decrypt frame: %w", err)
	}

	c.recvSeq++
	return frameType, plaintext, nil
}

func nonceForSequence(seq uint64) []byte {
	nonce := make([]byte, chacha20poly1305.NonceSize)
	binary.BigEndian.PutUint64(nonce[4:], seq)
	return nonce
}
