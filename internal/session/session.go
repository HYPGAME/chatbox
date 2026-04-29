package session

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

type Host struct {
	listener net.Listener
	cfg      Config
}

type Message struct {
	ID   string
	From string
	Body string
	At   time.Time
}

type Receipt struct {
	MessageID string
	At        time.Time
}

type Session struct {
	conn       net.Conn
	cfg        Config
	peerName   string
	sendCipher *cipherState
	recvCipher *cipherState

	messages chan Message
	receipts chan Receipt
	done     chan struct{}

	writeMu   sync.Mutex
	closeOnce sync.Once
	errMu     sync.Mutex
	err       error

	lastReceived atomic.Int64
	seenMu       sync.Mutex
	seenIDs      map[string]struct{}
}

func Listen(addr string, cfg Config) (*Host, error) {
	cfg = cfg.withDefaults()

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}

	return &Host{
		listener: listener,
		cfg:      cfg,
	}, nil
}

func (h *Host) Addr() string {
	return h.listener.Addr().String()
}

func (h *Host) Accept(ctx context.Context) (*Session, error) {
	conn, err := h.listener.Accept()
	if err != nil {
		return nil, fmt.Errorf("accept: %w", err)
	}

	state, err := serverHandshake(ctx, conn, h.cfg)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	return newSession(conn, h.cfg, state)
}

func (h *Host) Close() error {
	return h.listener.Close()
}

func Dial(ctx context.Context, peer string, cfg Config) (*Session, error) {
	cfg = cfg.withDefaults()

	dialer := net.Dialer{Timeout: cfg.HandshakeTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", peer)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}

	state, err := clientHandshake(ctx, conn, cfg)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	return newSession(conn, cfg, state)
}

func newSession(conn net.Conn, cfg Config, state *handshakeState) (*Session, error) {
	sendCipher, err := newCipherState(state.sendKey)
	if err != nil {
		return nil, err
	}
	recvCipher, err := newCipherState(state.recvKey)
	if err != nil {
		return nil, err
	}

	session := &Session{
		conn:       conn,
		cfg:        cfg.withDefaults(),
		peerName:   state.peerName,
		sendCipher: sendCipher,
		recvCipher: recvCipher,
		messages:   make(chan Message, 32),
		receipts:   make(chan Receipt, 32),
		done:       make(chan struct{}),
		seenIDs:    make(map[string]struct{}),
	}
	session.touch()

	go session.readLoop()
	go session.heartbeatLoop()

	return session, nil
}

func (s *Session) Messages() <-chan Message {
	return s.messages
}

func (s *Session) Done() <-chan struct{} {
	return s.done
}

func (s *Session) Receipts() <-chan Receipt {
	return s.receipts
}

func (s *Session) Err() error {
	s.errMu.Lock()
	defer s.errMu.Unlock()
	return s.err
}

func (s *Session) PeerName() string {
	return s.peerName
}

func (s *Session) MaxMessageSize() int {
	return s.cfg.MaxMessageSize
}

func (s *Session) Send(text string) (Message, error) {
	if !utf8.ValidString(text) {
		return Message{}, errors.New("message must be valid UTF-8")
	}
	messageID, err := generateMessageID()
	if err != nil {
		return Message{}, err
	}
	message := Message{
		ID:   messageID,
		From: s.cfg.Name,
		Body: text,
		At:   time.Now(),
	}
	if err := s.sendMessage(message); err != nil {
		return Message{}, err
	}
	return message, nil
}

func (s *Session) Resend(message Message) error {
	if message.ID == "" {
		return errors.New("cannot resend message without ID")
	}
	return s.sendMessage(message)
}

func (s *Session) sendMessage(message Message) error {
	payload, err := encodeMessagePayload(message)
	if err != nil {
		return err
	}
	if len(payload) > s.cfg.MaxMessageSize {
		return fmt.Errorf("message exceeds %d bytes", s.cfg.MaxMessageSize)
	}

	select {
	case <-s.done:
		if err := s.Err(); err != nil {
			return err
		}
		return errors.New("session closed")
	default:
	}

	return s.writeFrame(frameTypeData, payload)
}

func (s *Session) Close() error {
	s.shutdown(errors.New("session closed locally"), true)
	return nil
}

func (s *Session) readLoop() {
	for {
		packet, err := readPacket(s.conn, s.cfg.MaxMessageSize)
		if err != nil {
			s.shutdown(fmt.Errorf("read packet: %w", err), false)
			return
		}

		frameType, payload, err := s.recvCipher.open(packet)
		if err != nil {
			s.shutdown(err, false)
			return
		}
		s.touch()

		switch frameType {
		case frameTypeData:
			if len(payload) > s.cfg.MaxMessageSize {
				s.shutdown(fmt.Errorf("received oversized payload: %d", len(payload)), false)
				return
			}
			message, err := decodeMessagePayload(s.peerName, payload)
			if err != nil {
				s.shutdown(err, false)
				return
			}
			if err := s.writeAck(message.ID); err != nil {
				s.shutdown(err, false)
				return
			}
			if s.markSeen(message.ID) {
				continue
			}
			select {
			case s.messages <- message:
			case <-s.done:
				return
			}
		case frameTypePing:
			if err := s.writeFrame(frameTypePong, nil); err != nil {
				s.shutdown(err, false)
				return
			}
		case frameTypePong:
		case frameTypeAck:
			receipt, err := decodeAckPayload(payload)
			if err != nil {
				s.shutdown(err, false)
				return
			}
			select {
			case s.receipts <- Receipt{MessageID: receipt, At: time.Now()}:
			case <-s.done:
				return
			}
		case frameTypeClose:
			s.shutdown(io.EOF, false)
			return
		default:
			s.shutdown(fmt.Errorf("unknown frame type: %d", frameType), false)
			return
		}
	}
}

func (s *Session) heartbeatLoop() {
	ticker := time.NewTicker(s.cfg.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			lastReceived := time.Unix(0, s.lastReceived.Load())
			if time.Since(lastReceived) > s.cfg.IdleTimeout {
				s.shutdown(errors.New("peer heartbeat timeout"), false)
				return
			}
			if err := s.writeFrame(frameTypePing, nil); err != nil {
				s.shutdown(err, false)
				return
			}
		}
	}
}

func (s *Session) writeFrame(frameType byte, payload []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	packet, err := s.sendCipher.seal(frameType, payload)
	if err != nil {
		return err
	}

	return writePacket(s.conn, packet)
}

func (s *Session) shutdown(err error, sendClose bool) {
	s.closeOnce.Do(func() {
		if sendClose {
			_ = s.writeFrame(frameTypeClose, nil)
		}

		s.errMu.Lock()
		s.err = err
		s.errMu.Unlock()

		_ = s.conn.Close()
		close(s.done)
		close(s.messages)
		close(s.receipts)
	})
}

func (s *Session) touch() {
	s.lastReceived.Store(time.Now().UnixNano())
}

func writePacket(w io.Writer, packet []byte) error {
	lengthPrefix := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthPrefix, uint32(len(packet)))

	if _, err := w.Write(lengthPrefix); err != nil {
		return fmt.Errorf("write packet length: %w", err)
	}
	if _, err := w.Write(packet); err != nil {
		return fmt.Errorf("write packet body: %w", err)
	}
	return nil
}

func readPacket(r io.Reader, maxMessageSize int) ([]byte, error) {
	var lengthPrefix [4]byte
	if _, err := io.ReadFull(r, lengthPrefix[:]); err != nil {
		return nil, err
	}

	packetLength := binary.BigEndian.Uint32(lengthPrefix[:])
	maxPacketSize := uint32(maxMessageSize + 128)
	if packetLength == 0 || packetLength > maxPacketSize {
		return nil, fmt.Errorf("invalid packet length %d", packetLength)
	}

	packet := make([]byte, packetLength)
	if _, err := io.ReadFull(r, packet); err != nil {
		return nil, err
	}
	return packet, nil
}

func encodeMessagePayload(message Message) ([]byte, error) {
	if !utf8.ValidString(message.Body) {
		return nil, errors.New("message must be valid UTF-8")
	}
	if message.ID == "" {
		return nil, errors.New("message must have an ID")
	}
	if !utf8.ValidString(message.From) {
		return nil, errors.New("message sender must be valid UTF-8")
	}
	if message.From == "" {
		return nil, errors.New("message must have a sender")
	}

	idBytes := []byte(message.ID)
	fromBytes := []byte(message.From)
	body := []byte(message.Body)
	payload := make([]byte, 2+len(idBytes)+2+len(fromBytes)+8+len(body))
	binary.BigEndian.PutUint16(payload[:2], uint16(len(idBytes)))
	copy(payload[2:2+len(idBytes)], idBytes)
	fromStart := 2 + len(idBytes)
	binary.BigEndian.PutUint16(payload[fromStart:fromStart+2], uint16(len(fromBytes)))
	copy(payload[fromStart+2:fromStart+2+len(fromBytes)], fromBytes)
	timestampStart := fromStart + 2 + len(fromBytes)
	binary.BigEndian.PutUint64(payload[timestampStart:timestampStart+8], uint64(message.At.UnixNano()))
	copy(payload[timestampStart+8:], body)
	return payload, nil
}

func PayloadSize(message Message) (int, error) {
	payload, err := encodeMessagePayload(message)
	if err != nil {
		return 0, err
	}
	return len(payload), nil
}

func decodeMessagePayload(fallbackFrom string, payload []byte) (Message, error) {
	if len(payload) < 12 {
		return Message{}, errors.New("message payload too short")
	}

	idLength := int(binary.BigEndian.Uint16(payload[:2]))
	fromLengthOffset := 2 + idLength
	if idLength == 0 || len(payload) < fromLengthOffset+2+8 {
		return Message{}, errors.New("message payload has invalid ID")
	}

	fromLength := int(binary.BigEndian.Uint16(payload[fromLengthOffset : fromLengthOffset+2]))
	fromStart := fromLengthOffset + 2
	timestampStart := fromStart + fromLength
	if fromLength == 0 || len(payload) < timestampStart+8 {
		return Message{}, errors.New("message payload has invalid sender")
	}

	fromBytes := payload[fromStart:timestampStart]
	if !utf8.Valid(fromBytes) {
		return Message{}, errors.New("received invalid UTF-8 sender")
	}

	body := payload[timestampStart+8:]
	if !utf8.Valid(body) {
		return Message{}, errors.New("received invalid UTF-8 message")
	}

	from := string(fromBytes)
	if from == "" {
		from = fallbackFrom
	}

	return Message{
		ID:   string(payload[2 : 2+idLength]),
		From: from,
		Body: string(body),
		At:   time.Unix(0, int64(binary.BigEndian.Uint64(payload[timestampStart:timestampStart+8]))),
	}, nil
}

func encodeAckPayload(messageID string) ([]byte, error) {
	if messageID == "" {
		return nil, errors.New("ack requires message ID")
	}
	idBytes := []byte(messageID)
	payload := make([]byte, 2+len(idBytes))
	binary.BigEndian.PutUint16(payload[:2], uint16(len(idBytes)))
	copy(payload[2:], idBytes)
	return payload, nil
}

func decodeAckPayload(payload []byte) (string, error) {
	if len(payload) < 3 {
		return "", errors.New("ack payload too short")
	}
	idLength := int(binary.BigEndian.Uint16(payload[:2]))
	if idLength == 0 || len(payload) != 2+idLength {
		return "", errors.New("ack payload has invalid ID")
	}
	return string(payload[2:]), nil
}

func (s *Session) writeAck(messageID string) error {
	payload, err := encodeAckPayload(messageID)
	if err != nil {
		return err
	}
	return s.writeFrame(frameTypeAck, payload)
}

func (s *Session) markSeen(messageID string) bool {
	s.seenMu.Lock()
	defer s.seenMu.Unlock()

	if _, ok := s.seenIDs[messageID]; ok {
		return true
	}
	s.seenIDs[messageID] = struct{}{}
	return false
}

func generateMessageID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate message ID: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
