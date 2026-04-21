package room

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"chatbox/internal/admins"
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
	Receipts() <-chan session.Receipt
	Done() <-chan struct{}
	Err() error
	Close() error
	PeerName() string
	Resend(session.Message) error
}

type trackedMember struct {
	id          uint64
	session     memberSession
	syncCapable bool
}

type HostRoom struct {
	localName string

	messages chan session.Message
	receipts chan session.Receipt
	events   chan Event
	done     chan struct{}
	eventLog []Event

	closeOnce sync.Once
	memberSeq atomic.Uint64
	memberWG  sync.WaitGroup

	mu                 sync.Mutex
	closed             bool
	members            map[uint64]trackedMember
	admins             admins.Store
	releaseResolver    func(context.Context, string) (string, error)
	identityByPeerName map[string]string
	identityByMemberID map[uint64]string
	processedUpdates   map[string]struct{}
	activeUpdateStatus map[string]map[string]string
}

func NewHostRoom(localName string) *HostRoom {
	return &HostRoom{
		localName: localName,
		messages:  make(chan session.Message, 64),
		receipts:  make(chan session.Receipt, 64),
		events:    make(chan Event, 64),
		done:      make(chan struct{}),
		members:   make(map[uint64]trackedMember),
		admins: admins.Store{
			AllowedUpdateIdentities: make(map[string]struct{}),
		},
		identityByPeerName: make(map[string]string),
		identityByMemberID: make(map[uint64]string),
		processedUpdates:   make(map[string]struct{}),
		activeUpdateStatus: make(map[string]map[string]string),
	}
}

func (r *HostRoom) ConfigureUpdates(store admins.Store, resolver func(context.Context, string) (string, error)) {
	if store.AllowedUpdateIdentities == nil {
		store.AllowedUpdateIdentities = make(map[string]struct{})
	}
	r.mu.Lock()
	r.admins = store
	r.releaseResolver = resolver
	r.mu.Unlock()
}

func (r *HostRoom) SubmitUpdateRequest(request UpdateRequest) error {
	r.handleUpdateRequest(trackedMember{}, request)
	return nil
}

func (r *HostRoom) Serve(ctx context.Context, host *session.Host) {
	if host == nil {
		return
	}

	for {
		conn, err := host.Accept(ctx)
		if err != nil {
			select {
			case <-r.done:
			default:
			}
			return
		}
		r.AddMember(conn)
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
		r.closed = true
		members := make([]trackedMember, 0, len(r.members))
		for _, member := range r.members {
			members = append(members, member)
		}
		r.members = make(map[uint64]trackedMember)
		r.mu.Unlock()

		for _, member := range members {
			_ = member.session.Close()
		}

		r.memberWG.Wait()

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

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		_ = member.Close()
		return
	}
	tracked := trackedMember{
		id:      r.memberSeq.Add(1),
		session: member,
	}
	r.memberWG.Add(1)

	r.members[tracked.id] = tracked
	peerCount := len(r.members)
	r.mu.Unlock()

	r.publishEvent(Event{
		Kind:      EventPeerJoined,
		PeerName:  member.PeerName(),
		PeerCount: peerCount,
		At:        time.Now(),
	})

	go func() {
		defer r.memberWG.Done()
		r.runMember(tracked)
	}()
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

func (r *HostRoom) ParticipantNames() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	names := make([]string, 0, len(r.members)+1)
	if strings.TrimSpace(r.localName) != "" {
		names = append(names, r.localName)
	}
	for _, member := range r.members {
		names = append(names, member.session.PeerName())
	}
	sort.Strings(names)
	return names
}

func (r *HostRoom) runMember(member trackedMember) {
	for {
		select {
		case <-r.done:
			return
		case <-member.session.Done():
			r.removeMember(member)
			return
		case _, ok := <-member.session.Receipts():
			if !ok {
				continue
			}
		case message, ok := <-member.session.Messages():
			if !ok {
				r.removeMember(member)
				return
			}
			if r.handleStatusRequest(member, message) {
				continue
			}
			if r.handleEventsRequest(member, message) {
				continue
			}
			if r.handleHistorySyncControl(member, message) {
				continue
			}
			if r.handleUpdateControl(member, message) {
				continue
			}
			if err := r.broadcast(message, member.id); err != nil {
				continue
			}
			r.publishMessage(message)
		}
	}
}

func (r *HostRoom) handleStatusRequest(member trackedMember, message session.Message) bool {
	if !IsStatusRequest(message.Body) {
		return false
	}

	messageID, err := generateMessageID()
	if err != nil {
		return true
	}
	response := session.Message{
		ID:   messageID,
		From: r.localName,
		Body: StatusResponseBody(r.ParticipantNames()),
		At:   time.Now(),
	}
	_ = member.session.Resend(response)
	return true
}

func (r *HostRoom) handleEventsRequest(member trackedMember, message session.Message) bool {
	if !IsEventsRequest(message.Body) {
		return false
	}

	messageID, err := generateMessageID()
	if err != nil {
		return true
	}
	response := session.Message{
		ID:   messageID,
		From: r.localName,
		Body: EventsResponseBody(r.EventLog()),
		At:   time.Now(),
	}
	_ = member.session.Resend(response)
	return true
}

func (r *HostRoom) handleHistorySyncControl(member trackedMember, message session.Message) bool {
	if hello, ok := ParseHistorySyncHello(message.Body); ok {
		if hello.IdentityID == "" {
			return true
		}
		r.markMemberSyncCapable(member.id, true)
		r.rememberMemberIdentity(member.id, member.session.PeerName(), hello.IdentityID)
		_ = r.broadcastHistorySync(message, member.id)
		return true
	}
	if !IsHistorySyncControl(message.Body) {
		return false
	}
	_ = r.broadcastHistorySync(message, member.id)
	return true
}

func (r *HostRoom) handleUpdateControl(member trackedMember, message session.Message) bool {
	if request, ok := ParseUpdateRequest(message.Body); ok {
		r.handleUpdateRequest(member, request)
		return true
	}
	if result, ok := ParseUpdateResult(message.Body); ok {
		if member.session != nil {
			result.ReporterName = member.session.PeerName()
			if identityID := r.memberIdentity(member.id); identityID != "" {
				result.ReporterID = identityID
			}
		}
		r.handleUpdateResult(result)
		return true
	}
	if _, ok := ParseUpdateExecute(message.Body); ok {
		return true
	}
	return IsUpdateControl(message.Body)
}

func (r *HostRoom) handleUpdateRequest(member trackedMember, request UpdateRequest) {
	requesterName := strings.TrimSpace(request.RequesterName)
	requesterIdentity := strings.TrimSpace(request.RequesterIdentity)
	if member.session != nil {
		requesterName = strings.TrimSpace(member.session.PeerName())
		if mapped := r.memberIdentity(member.id); mapped != "" {
			requesterIdentity = mapped
		} else if requesterIdentity != "" {
			r.rememberMemberIdentity(member.id, requesterName, requesterIdentity)
		}
	}

	if requesterName != r.localName && !r.admins.Allows(requesterIdentity) {
		r.sendUpdateResult(member.session, UpdateResult{
			Version:       1,
			RequestID:     request.RequestID,
			RoomKey:       request.RoomKey,
			ReporterName:  r.localName,
			ReporterID:    "",
			TargetVersion: strings.TrimSpace(request.TargetVersion),
			Status:        "permission-denied",
			At:            time.Now(),
		}, true)
		return
	}

	targetVersion, err := r.resolveTargetVersion(strings.TrimSpace(request.TargetVersion))
	if err != nil {
		r.sendUpdateResult(member.session, UpdateResult{
			Version:        1,
			RequestID:      request.RequestID,
			RoomKey:        request.RoomKey,
			ReporterName:   r.localName,
			ReporterID:     "",
			TargetVersion:  strings.TrimSpace(request.TargetVersion),
			Status:         "resolve-latest-failed",
			Detail:         err.Error(),
			CurrentVersion: "",
			At:             time.Now(),
		}, true)
		return
	}

	if !r.markUpdateRequestStarted(request.RequestID) {
		return
	}

	execute := UpdateExecute{
		Version:           1,
		RequestID:         request.RequestID,
		RoomKey:           request.RoomKey,
		InitiatorIdentity: requesterIdentity,
		InitiatorName:     requesterName,
		TargetVersion:     targetVersion,
		At:                time.Now(),
	}
	message := session.Message{
		ID:   controlMessageID(request.RequestID + "-execute"),
		From: r.localName,
		Body: UpdateExecuteBody(execute),
		At:   execute.At,
	}
	if err := r.broadcast(message, 0); err != nil {
		r.sendUpdateResult(member.session, UpdateResult{
			Version:       1,
			RequestID:     request.RequestID,
			RoomKey:       request.RoomKey,
			ReporterName:  r.localName,
			TargetVersion: targetVersion,
			Status:        "dispatch-failed",
			Detail:        err.Error(),
			At:            time.Now(),
		}, true)
		return
	}
	r.publishMessage(message)
}

func (r *HostRoom) handleUpdateResult(result UpdateResult) {
	r.mu.Lock()
	statuses := r.activeUpdateStatus[result.RequestID]
	if statuses == nil {
		statuses = make(map[string]string)
		r.activeUpdateStatus[result.RequestID] = statuses
	}
	statuses[result.ReporterName] = result.Status
	r.mu.Unlock()

	message := session.Message{
		ID:   controlMessageID(result.RequestID + "-result"),
		From: r.localName,
		Body: UpdateResultBody(result),
		At:   result.At,
	}
	if err := r.broadcast(message, 0); err == nil {
		r.publishMessage(message)
	}
}

func (r *HostRoom) resolveTargetVersion(targetVersion string) (string, error) {
	targetVersion = strings.TrimSpace(targetVersion)
	if targetVersion != "" {
		return targetVersion, nil
	}
	if r.releaseResolver == nil {
		return "", errors.New("latest release resolver is not configured")
	}
	return r.releaseResolver(context.Background(), "")
}

func (r *HostRoom) sendUpdateResult(member memberSession, result UpdateResult, publish bool) {
	if result.At.IsZero() {
		result.At = time.Now()
	}
	message := session.Message{
		ID:   controlMessageID(result.RequestID + "-result"),
		From: r.localName,
		Body: UpdateResultBody(result),
		At:   result.At,
	}
	if member != nil {
		_ = member.Resend(message)
	}
	if publish {
		r.publishMessage(message)
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

func (r *HostRoom) broadcastHistorySync(message session.Message, excludeMemberID uint64) error {
	for _, member := range r.memberSnapshot() {
		if excludeMemberID != 0 && member.id == excludeMemberID {
			continue
		}
		if !member.syncCapable {
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

func (r *HostRoom) markMemberSyncCapable(memberID uint64, syncCapable bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	member, ok := r.members[memberID]
	if !ok {
		return
	}
	member.syncCapable = syncCapable
	r.members[memberID] = member
}

func (r *HostRoom) removeMember(member trackedMember) {
	r.mu.Lock()
	if _, ok := r.members[member.id]; !ok {
		r.mu.Unlock()
		return
	}
	delete(r.members, member.id)
	delete(r.identityByMemberID, member.id)
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
	r.appendEventLog(event)
	select {
	case r.events <- event:
	case <-r.done:
	}
}

func (r *HostRoom) EventLog() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Event(nil), r.eventLog...)
}

func (r *HostRoom) appendEventLog(event Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.eventLog = append(r.eventLog, event)
}

func (r *HostRoom) rememberMemberIdentity(memberID uint64, peerName, identityID string) {
	identityID = strings.TrimSpace(identityID)
	if identityID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.identityByMemberID[memberID] = identityID
	if peerName = strings.TrimSpace(peerName); peerName != "" {
		r.identityByPeerName[peerName] = identityID
	}
}

func (r *HostRoom) memberIdentity(memberID uint64) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.identityByMemberID[memberID]
}

func (r *HostRoom) markUpdateRequestStarted(requestID string) bool {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.processedUpdates[requestID]; ok {
		return false
	}
	r.processedUpdates[requestID] = struct{}{}
	if _, ok := r.activeUpdateStatus[requestID]; !ok {
		r.activeUpdateStatus[requestID] = make(map[string]string)
	}
	return true
}

func controlMessageID(fallback string) string {
	messageID, err := generateMessageID()
	if err == nil {
		return messageID
	}
	return fallback
}

func generateMessageID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
