package room

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"chatbox/internal/session"
)

type EventKind string

const (
	EventPeerJoined EventKind = "peer_joined"
	EventPeerLeft   EventKind = "peer_left"
)

type Event struct {
	Kind      EventKind
	PeerName  string
	PeerCount int
	At        time.Time
}

type memberSession interface {
	Messages() <-chan session.Message
	Done() <-chan struct{}
	Err() error
	Close() error
	PeerName() string
	Resend(session.Message) error
}

type trackedMember struct {
	id      uint64
	session memberSession
}

type HostRoom struct {
	localName string

	messages chan session.Message
	receipts chan session.Receipt
	events   chan Event
	done     chan struct{}

	closeOnce sync.Once
	memberSeq atomic.Uint64

	mu      sync.Mutex
	members map[uint64]trackedMember
}

func NewHostRoom(localName string) *HostRoom {
	return &HostRoom{
		localName: localName,
		messages:  make(chan session.Message, 64),
		receipts:  make(chan session.Receipt, 64),
		events:    make(chan Event, 64),
		done:      make(chan struct{}),
		members:   make(map[uint64]trackedMember),
	}
}

func (r *HostRoom) Messages() <-chan session.Message {
	return r.messages
}

func (r *HostRoom) Receipts() <-chan session.Receipt {
	return r.receipts
}

func (r *HostRoom) Events() <-chan Event {
	return r.events
}

func (r *HostRoom) Done() <-chan struct{} {
	return r.done
}

func (r *HostRoom) Err() error {
	return nil
}

func (r *HostRoom) Close() error {
	r.closeOnce.Do(func() {
		close(r.done)

		r.mu.Lock()
		members := make([]trackedMember, 0, len(r.members))
		for _, member := range r.members {
			members = append(members, member)
		}
		r.members = make(map[uint64]trackedMember)
		r.mu.Unlock()

		for _, member := range members {
			_ = member.session.Close()
		}

		close(r.messages)
		close(r.receipts)
		close(r.events)
	})
	return nil
}

func (r *HostRoom) PeerName() string {
	return "room"
}

func (r *HostRoom) AddMember(member memberSession) {
	if member == nil {
		return
	}

	tracked := trackedMember{
		id:      r.memberSeq.Add(1),
		session: member,
	}

	r.mu.Lock()
	r.members[tracked.id] = tracked
	peerCount := len(r.members)
	r.mu.Unlock()

	r.publishEvent(Event{
		Kind:      EventPeerJoined,
		PeerName:  member.PeerName(),
		PeerCount: peerCount,
		At:        time.Now(),
	})

	go r.runMember(tracked)
}

func (r *HostRoom) Send(text string) (session.Message, error) {
	if !utf8.ValidString(text) {
		return session.Message{}, errors.New("message must be valid UTF-8")
	}

	messageID, err := generateMessageID()
	if err != nil {
		return session.Message{}, err
	}
	message := session.Message{
		ID:   messageID,
		From: r.localName,
		Body: text,
		At:   time.Now(),
	}

	if err := r.broadcast(message, 0); err != nil {
		return session.Message{}, err
	}
	r.publishMessage(message)
	r.publishReceipt(session.Receipt{
		MessageID: message.ID,
		At:        time.Now(),
	})
	return message, nil
}

func (r *HostRoom) Resend(message session.Message) error {
	if message.ID == "" {
		return errors.New("cannot resend message without ID")
	}
	if err := r.broadcast(message, 0); err != nil {
		return err
	}
	r.publishMessage(message)
	r.publishReceipt(session.Receipt{
		MessageID: message.ID,
		At:        time.Now(),
	})
	return nil
}

func (r *HostRoom) PeerCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.members)
}

func (r *HostRoom) runMember(member trackedMember) {
	for {
		select {
		case <-r.done:
			return
		case <-member.session.Done():
			r.removeMember(member)
			return
		case message, ok := <-member.session.Messages():
			if !ok {
				r.removeMember(member)
				return
			}
			if err := r.broadcast(message, member.id); err != nil {
				continue
			}
			r.publishMessage(message)
		}
	}
}

func (r *HostRoom) broadcast(message session.Message, excludeMemberID uint64) error {
	for _, member := range r.memberSnapshot() {
		if excludeMemberID != 0 && member.id == excludeMemberID {
			continue
		}
		if err := member.session.Resend(message); err != nil {
			return err
		}
	}
	return nil
}

func (r *HostRoom) memberSnapshot() []trackedMember {
	r.mu.Lock()
	defer r.mu.Unlock()

	members := make([]trackedMember, 0, len(r.members))
	for _, member := range r.members {
		members = append(members, member)
	}
	return members
}

func (r *HostRoom) removeMember(member trackedMember) {
	r.mu.Lock()
	if _, ok := r.members[member.id]; !ok {
		r.mu.Unlock()
		return
	}
	delete(r.members, member.id)
	peerCount := len(r.members)
	r.mu.Unlock()

	r.publishEvent(Event{
		Kind:      EventPeerLeft,
		PeerName:  member.session.PeerName(),
		PeerCount: peerCount,
		At:        time.Now(),
	})
}

func (r *HostRoom) publishMessage(message session.Message) {
	select {
	case r.messages <- message:
	case <-r.done:
	}
}

func (r *HostRoom) publishReceipt(receipt session.Receipt) {
	select {
	case r.receipts <- receipt:
	case <-r.done:
	}
}

func (r *HostRoom) publishEvent(event Event) {
	select {
	case r.events <- event:
	case <-r.done:
	}
}

func generateMessageID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
