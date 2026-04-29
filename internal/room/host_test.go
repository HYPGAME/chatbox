package room

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"chatbox/internal/admins"
	"chatbox/internal/historymeta"
	"chatbox/internal/hosthistory"
	"chatbox/internal/session"
	"chatbox/internal/transcript"
	"chatbox/internal/version"
)

func TestHostRoomBroadcastsJoinerMessagesToOtherMembersAndHost(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	memberA := newFakeMember("aaa")
	memberB := newFakeMember("bbb")
	room.AddMember(memberA)
	room.AddMember(memberB)
	drainJoinEvents(t, room, 2)

	inbound := session.Message{
		ID:   "msg-1",
		From: "aaa",
		Body: "hello room",
		At:   time.Date(2026, 4, 15, 18, 0, 0, 0, time.UTC),
	}
	memberA.messages <- inbound

	gotHost := waitForRoomMessage(t, room.Messages())
	if gotHost != inbound {
		t.Fatalf("expected host stream to receive %#v, got %#v", inbound, gotHost)
	}

	gotForward := waitForResentMessage(t, memberB.resent)
	if gotForward != inbound {
		t.Fatalf("expected other member to receive %#v, got %#v", inbound, gotForward)
	}

	assertNoResentMessage(t, memberA.resent)
}

func TestHostRoomBroadcastsHostMessagesToAllMembers(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	memberA := newFakeMember("aaa")
	memberB := newFakeMember("bbb")
	room.AddMember(memberA)
	room.AddMember(memberB)
	drainJoinEvents(t, room, 2)

	message, err := room.Send("hello everyone")
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if message.From != "host" {
		t.Fatalf("expected host sender name, got %q", message.From)
	}

	gotHost := waitForRoomMessage(t, room.Messages())
	if gotHost != message {
		t.Fatalf("expected host stream to receive %#v, got %#v", message, gotHost)
	}

	if got := waitForResentMessage(t, memberA.resent); got != message {
		t.Fatalf("expected first member to receive %#v, got %#v", message, got)
	}
	if got := waitForResentMessage(t, memberB.resent); got != message {
		t.Fatalf("expected second member to receive %#v, got %#v", message, got)
	}
}

func TestHostRoomEmitsJoinAndLeaveEvents(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	member := newFakeMember("aaa")
	room.AddMember(member)

	joined := waitForRoomEvent(t, room.Events())
	if joined.Kind != EventPeerJoined || joined.PeerName != "aaa" || joined.PeerCount != 1 {
		t.Fatalf("expected joined event for aaa with count 1, got %#v", joined)
	}

	close(member.done)

	left := waitForRoomEvent(t, room.Events())
	if left.Kind != EventPeerLeft || left.PeerName != "aaa" || left.PeerCount != 0 {
		t.Fatalf("expected left event for aaa with count 0, got %#v", left)
	}
}

func TestHostRoomInterceptsStatusRequestsAndRepliesOnlyToRequester(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	memberA := newFakeMember("bbb")
	memberB := newFakeMember("aaa")
	room.AddMember(memberA)
	room.AddMember(memberB)
	drainJoinEvents(t, room, 2)

	memberA.messages <- session.Message{
		ID:   "status-1",
		From: "bbb",
		Body: StatusRequestBody(),
		At:   time.Date(2026, 4, 17, 16, 0, 0, 0, time.UTC),
	}

	response := waitForResentMessage(t, memberA.resent)
	line, ok := ParseStatusResponse(response.Body)
	if !ok {
		t.Fatalf("expected hidden status response, got %#v", response)
	}
	if line != "online (3): aaa [unknown], bbb [unknown], host ["+version.Version+"]" {
		t.Fatalf("expected sorted online roster, got %q", line)
	}

	assertNoResentMessage(t, memberB.resent)
	assertNoRoomMessage(t, room.Messages())
}

func TestHostRoomUsesVersionAnnouncementsInStatusResponses(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	memberA := newFakeMember("bbb")
	memberB := newFakeMember("aaa")
	room.AddMember(memberA)
	room.AddMember(memberB)
	drainJoinEvents(t, room, 2)

	memberA.messages <- session.Message{
		ID:   "version-1",
		From: "bbb",
		Body: VersionAnnounceBody(VersionAnnounce{
			Version:       1,
			ClientVersion: "v0.1.31",
		}),
		At: time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC),
	}

	memberA.messages <- session.Message{
		ID:   "status-1",
		From: "bbb",
		Body: StatusRequestBody(),
		At:   time.Date(2026, 4, 22, 12, 0, 1, 0, time.UTC),
	}

	response := waitForResentMessage(t, memberA.resent)
	line, ok := ParseStatusResponse(response.Body)
	if !ok {
		t.Fatalf("expected hidden status response, got %#v", response)
	}
	if !strings.Contains(line, "bbb [v0.1.31]") {
		t.Fatalf("expected advertised version in roster, got %q", line)
	}
	if !strings.Contains(line, "aaa [unknown]") {
		t.Fatalf("expected untouched legacy peer in roster, got %q", line)
	}
}

func TestHostRoomInterceptsEventsRequestsAndRepliesOnlyToRequester(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	memberA := newFakeMember("aaa")
	memberB := newFakeMember("bbb")
	room.AddMember(memberA)
	room.AddMember(memberB)
	drainJoinEvents(t, room, 2)
	close(memberB.done)
	left := waitForRoomEvent(t, room.Events())
	if left.Kind != EventPeerLeft || left.PeerName != "bbb" {
		t.Fatalf("expected bbb left event, got %#v", left)
	}

	memberA.messages <- session.Message{
		ID:   "events-1",
		From: "aaa",
		Body: EventsRequestBody(),
		At:   time.Date(2026, 4, 20, 18, 10, 0, 0, time.UTC),
	}

	response := waitForResentMessage(t, memberA.resent)
	events, ok := ParseEventsResponse(response.Body)
	if !ok {
		t.Fatalf("expected hidden events response, got %#v", response)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 join/leave events, got %#v", events)
	}
	assertNoResentMessage(t, memberB.resent)
	assertNoRoomMessage(t, room.Messages())
}

func TestHostRoomRoutesHistorySyncMessagesOnlyToSyncCapableMembers(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	memberA := newFakeMember("aaa")
	memberB := newFakeMember("bbb")
	room.AddMember(memberA)
	room.AddMember(memberB)
	drainJoinEvents(t, room, 2)

	memberA.messages <- session.Message{
		ID:   "sync-hello-1",
		From: "aaa",
		Body: HistorySyncHelloBody(HistorySyncHello{
			Version:    1,
			IdentityID: "identity-a",
			RoomKey:    "room",
		}),
		At: time.Date(2026, 4, 20, 21, 0, 0, 0, time.UTC),
	}

	assertNoResentMessage(t, memberB.resent)
	assertNoRoomMessage(t, room.Messages())

	memberA.messages <- session.Message{
		ID:   "sync-offer-1",
		From: "aaa",
		Body: HistorySyncOfferBody(HistorySyncOffer{
			Version:        1,
			SourceIdentity: "identity-a",
			TargetIdentity: "identity-b",
			RoomKey:        "room",
			Summary:        HistorySyncSummary{Count: 1},
		}),
		At: time.Date(2026, 4, 20, 21, 1, 0, 0, time.UTC),
	}

	assertNoResentMessage(t, memberB.resent)
	assertNoRoomMessage(t, room.Messages())
}

func TestHostRoomForwardsHistorySyncMessagesToMembersWhoAnnouncedHello(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	memberA := newFakeMember("aaa")
	memberB := newFakeMember("bbb")
	room.AddMember(memberA)
	room.AddMember(memberB)
	drainJoinEvents(t, room, 2)

	memberB.messages <- session.Message{
		ID:   "sync-hello-b",
		From: "bbb",
		Body: HistorySyncHelloBody(HistorySyncHello{
			Version:    1,
			IdentityID: "identity-b",
			RoomKey:    "room",
		}),
		At: time.Date(2026, 4, 20, 21, 0, 0, 0, time.UTC),
	}
	assertNoResentMessage(t, memberA.resent)

	memberA.messages <- session.Message{
		ID:   "sync-hello-a",
		From: "aaa",
		Body: HistorySyncHelloBody(HistorySyncHello{
			Version:    1,
			IdentityID: "identity-a",
			RoomKey:    "room",
		}),
		At: time.Date(2026, 4, 20, 21, 0, 10, 0, time.UTC),
	}
	if got := waitForResentMessage(t, memberB.resent); got.Body == "" || !IsHistorySyncControl(got.Body) {
		t.Fatalf("expected sync-capable member to receive sync control, got %#v", got)
	}
	assertNoRoomMessage(t, room.Messages())
}

func TestHostRoomStoresAuthoritativeJoinWhenSyncHelloArrives(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	joins := &fakeJoinStore{}
	room.ConfigureHistoryRetention(&fakeRetainedHistoryStore{}, "join:127.0.0.1:7331", joins.open)

	member := newFakeMember("alice")
	room.AddMember(member)
	drainJoinEvents(t, room, 1)

	member.messages <- session.Message{
		ID:   "sync-hello-authoritative",
		From: "alice",
		Body: HistorySyncHelloBody(HistorySyncHello{
			Version:    1,
			IdentityID: "identity-a",
			RoomKey:    "join:127.0.0.1:7331",
		}),
		At: time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC),
	}

	waitForJoinRecord(t, joins, "join:127.0.0.1:7331", "identity-a")
}

func TestHostRoomCanonicalizesJoinRecordWhenSyncHelloUsesAliasRoomKey(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	joins := &fakeJoinStore{}
	room.ConfigureHistoryRetention(&fakeRetainedHistoryStore{}, "join:0.0.0.0:7331", joins.open)

	member := newFakeMember("alice")
	room.AddMember(member)
	drainJoinEvents(t, room, 1)

	member.messages <- session.Message{
		ID:   "sync-hello-authoritative-alias",
		From: "alice",
		Body: HistorySyncHelloBody(HistorySyncHello{
			Version:    1,
			IdentityID: "identity-a",
			RoomKey:    "join:10.77.1.4:7331",
		}),
		At: time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC),
	}

	waitForJoinRecord(t, joins, "join:0.0.0.0:7331", "identity-a")
}

func TestHostRoomAnswersAuthorizedHostHistoryRequest(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	store := &fakeRetainedHistoryStore{
		window: hosthistory.Window{
			Records: []transcript.Record{
				{
					MessageID:      "msg-1",
					Direction:      transcript.DirectionIncoming,
					From:           "bob",
					AuthorIdentity: "identity-b",
					Body:           "late message",
					At:             time.Date(2026, 4, 27, 11, 59, 0, 0, time.UTC),
					Status:         transcript.StatusSent,
				},
			},
		},
	}
	joinedAt := time.Date(2026, 4, 27, 11, 0, 0, 0, time.UTC)
	joins := &fakeJoinStore{
		record: historymeta.Record{
			RoomKey:    "join:127.0.0.1:7331",
			IdentityID: "identity-a",
			JoinedAt:   joinedAt,
		},
	}
	room.ConfigureHistoryRetention(store, "join:127.0.0.1:7331", joins.open)

	member := newFakeMember("alice")
	room.AddMember(member)
	drainJoinEvents(t, room, 1)

	newestLocal := time.Date(2026, 4, 27, 11, 58, 0, 0, time.UTC)
	member.messages <- session.Message{
		ID:   "hostsync-request-1",
		From: "alice",
		Body: HostHistoryRequestBody(HostHistoryRequest{
			Version:     1,
			RoomKey:     "join:127.0.0.1:7331",
			IdentityID:  "identity-a",
			JoinedAt:    time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC),
			NewestLocal: newestLocal,
		}),
		At: time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC),
	}

	response := waitForResentMessage(t, member.resent)
	chunk, ok := ParseHostHistoryChunk(response.Body)
	if !ok {
		t.Fatalf("expected host history chunk, got %#v", response)
	}
	if len(chunk.Records) != 1 || chunk.TargetIdentity != "identity-a" {
		t.Fatalf("expected authorized retained chunk, got %#v", chunk)
	}
	expectedSince := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	if !store.lastSince.Equal(expectedSince) {
		t.Fatalf("expected hostsync lower bound %v, got %v", expectedSince, store.lastSince)
	}
	assertNoRoomMessage(t, room.Messages())
}

func TestHostRoomAnswersHostHistoryRequestAcrossRoomKeyAlias(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	store := &fakeRetainedHistoryStore{
		window: hosthistory.Window{
			Records: []transcript.Record{
				{
					MessageID:      "msg-1",
					Direction:      transcript.DirectionIncoming,
					From:           "bob",
					AuthorIdentity: "identity-b",
					Body:           "retained across alias",
					At:             time.Date(2026, 4, 27, 11, 59, 0, 0, time.UTC),
					Status:         transcript.StatusSent,
				},
			},
		},
	}
	joinedAt := time.Date(2026, 4, 27, 11, 0, 0, 0, time.UTC)
	joins := &fakeJoinStore{
		record: historymeta.Record{
			RoomKey:    "join:0.0.0.0:7331",
			IdentityID: "identity-a",
			JoinedAt:   joinedAt,
		},
	}
	room.ConfigureHistoryRetention(store, "join:0.0.0.0:7331", joins.open)

	member := newFakeMember("alice")
	room.AddMember(member)
	drainJoinEvents(t, room, 1)

	member.messages <- session.Message{
		ID:   "hostsync-request-alias",
		From: "alice",
		Body: HostHistoryRequestBody(HostHistoryRequest{
			Version:     1,
			RoomKey:     "join:10.77.1.4:7331",
			IdentityID:  "identity-a",
			JoinedAt:    time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC),
			NewestLocal: time.Date(2026, 4, 27, 11, 58, 0, 0, time.UTC),
		}),
		At: time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC),
	}

	response := waitForResentMessage(t, member.resent)
	chunk, ok := ParseHostHistoryChunk(response.Body)
	if !ok {
		t.Fatalf("expected host history chunk, got %#v", response)
	}
	if chunk.RoomKey != "join:10.77.1.4:7331" {
		t.Fatalf("expected response to preserve requester room key, got %#v", chunk)
	}
	if len(chunk.Records) != 1 || chunk.Records[0].MessageID != "msg-1" {
		t.Fatalf("expected retained history across alias, got %#v", chunk)
	}
	if store.lastRoomKey != "join:0.0.0.0:7331" {
		t.Fatalf("expected retained history lookup to use canonical room key, got %q", store.lastRoomKey)
	}
	waitForJoinRecord(t, joins, "join:0.0.0.0:7331", "identity-a")
}

func TestHostRoomAnswersHostHistoryRequestUsingLegacyAliasJoinRecordWhenCanonicalRecordIsNewer(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	store := &fakeRetainedHistoryStore{
		filterBySince: true,
		window: hosthistory.Window{
			Records: []transcript.Record{
				{
					MessageID:      "msg-legacy-alias",
					Direction:      transcript.DirectionIncoming,
					From:           "bob",
					AuthorIdentity: "identity-b",
					Body:           "offline while a was away",
					At:             time.Date(2026, 4, 27, 11, 59, 0, 0, time.UTC),
					Status:         transcript.StatusSent,
				},
			},
		},
	}
	joins := &fakeJoinStore{
		records: map[string]historymeta.Record{
			"join:0.0.0.0:7331": {
				RoomKey:    "join:0.0.0.0:7331",
				IdentityID: "identity-a",
				JoinedAt:   time.Date(2026, 4, 27, 12, 30, 0, 0, time.UTC),
			},
			"join:10.77.1.4:7331": {
				RoomKey:    "join:10.77.1.4:7331",
				IdentityID: "identity-a",
				JoinedAt:   time.Date(2026, 4, 27, 11, 0, 0, 0, time.UTC),
			},
		},
	}
	room.ConfigureHistoryRetention(store, "join:0.0.0.0:7331", joins.open)

	member := newFakeMember("alice")
	room.AddMember(member)
	drainJoinEvents(t, room, 1)

	member.messages <- session.Message{
		ID:   "hostsync-request-legacy-alias",
		From: "alice",
		Body: HostHistoryRequestBody(HostHistoryRequest{
			Version:     1,
			RoomKey:     "join:10.77.1.4:7331",
			IdentityID:  "identity-a",
			JoinedAt:    time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC),
			NewestLocal: time.Date(2026, 4, 27, 11, 58, 0, 0, time.UTC),
		}),
		At: time.Date(2026, 4, 27, 12, 31, 0, 0, time.UTC),
	}

	response := waitForResentMessage(t, member.resent)
	chunk, ok := ParseHostHistoryChunk(response.Body)
	if !ok {
		t.Fatalf("expected host history chunk, got %#v", response)
	}
	if len(chunk.Records) != 1 || chunk.Records[0].MessageID != "msg-legacy-alias" {
		t.Fatalf("expected host history lookup to fall back to legacy alias join record, got %#v", chunk)
	}
	expectedSince := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	if !store.lastSince.Equal(expectedSince) {
		t.Fatalf("expected host history lower bound %v, got %v", expectedSince, store.lastSince)
	}
}

func TestHostRoomAnswersHostHistoryRequestUsingEarlierRequesterJoinedAtForLegacyClient(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	store := &fakeRetainedHistoryStore{
		filterBySince: true,
		window: hosthistory.Window{
			Records: []transcript.Record{
				{
					MessageID:      "msg-legacy-client",
					Direction:      transcript.DirectionIncoming,
					From:           "bob",
					AuthorIdentity: "identity-b",
					Body:           "offline before historymeta existed",
					At:             time.Date(2026, 4, 27, 11, 59, 0, 0, time.UTC),
					Status:         transcript.StatusSent,
				},
			},
		},
	}
	joins := &fakeJoinStore{
		record: historymeta.Record{
			RoomKey:    "join:127.0.0.1:7331",
			IdentityID: "identity-a",
			JoinedAt:   time.Date(2026, 4, 27, 12, 30, 0, 0, time.UTC),
		},
	}
	room.ConfigureHistoryRetention(store, "join:127.0.0.1:7331", joins.open)

	member := newFakeMember("alice")
	room.AddMember(member)
	drainJoinEvents(t, room, 1)

	requestJoinedAt := time.Date(2026, 4, 27, 11, 0, 0, 0, time.UTC)
	member.messages <- session.Message{
		ID:   "hostsync-request-legacy-client",
		From: "alice",
		Body: HostHistoryRequestBody(HostHistoryRequest{
			Version:     1,
			RoomKey:     "join:127.0.0.1:7331",
			IdentityID:  "identity-a",
			JoinedAt:    requestJoinedAt,
			NewestLocal: time.Time{},
		}),
		At: time.Date(2026, 4, 27, 12, 31, 0, 0, time.UTC),
	}

	response := waitForResentMessage(t, member.resent)
	chunk, ok := ParseHostHistoryChunk(response.Body)
	if !ok {
		t.Fatalf("expected host history chunk, got %#v", response)
	}
	if len(chunk.Records) != 1 || chunk.Records[0].MessageID != "msg-legacy-client" {
		t.Fatalf("expected legacy requester window to be honored, got %#v", chunk)
	}
	if !store.lastSince.Equal(requestJoinedAt) {
		t.Fatalf("expected host history lower bound %v, got %v", requestJoinedAt, store.lastSince)
	}
}

func TestHostRoomAnswersHostHistoryRequestWithFullWindowDespiteNewerLocalTranscript(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	joinedAt := time.Date(2026, 4, 27, 11, 0, 0, 0, time.UTC)
	store := &fakeRetainedHistoryStore{
		filterBySince: true,
		window: hosthistory.Window{
			Records: []transcript.Record{
				{
					MessageID:      "msg-earlier-gap",
					Direction:      transcript.DirectionIncoming,
					From:           "bob",
					AuthorIdentity: "identity-b",
					Body:           "older retained gap",
					At:             joinedAt.Add(10 * time.Minute),
					Status:         transcript.StatusSent,
				},
				{
					MessageID:      "msg-late",
					Direction:      transcript.DirectionIncoming,
					From:           "bob",
					AuthorIdentity: "identity-b",
					Body:           "late retained message",
					At:             joinedAt.Add(59 * time.Minute),
					Status:         transcript.StatusSent,
				},
			},
		},
	}
	joins := &fakeJoinStore{
		record: historymeta.Record{
			RoomKey:    "join:127.0.0.1:7331",
			IdentityID: "identity-a",
			JoinedAt:   joinedAt,
		},
	}
	room.ConfigureHistoryRetention(store, "join:127.0.0.1:7331", joins.open)

	member := newFakeMember("alice")
	room.AddMember(member)
	drainJoinEvents(t, room, 1)

	member.messages <- session.Message{
		ID:   "hostsync-request-full-window",
		From: "alice",
		Body: HostHistoryRequestBody(HostHistoryRequest{
			Version:     1,
			RoomKey:     "join:127.0.0.1:7331",
			IdentityID:  "identity-a",
			JoinedAt:    joinedAt,
			NewestLocal: joinedAt.Add(58 * time.Minute),
		}),
		At: joinedAt.Add(time.Hour),
	}

	response := waitForResentMessage(t, member.resent)
	chunk, ok := ParseHostHistoryChunk(response.Body)
	if !ok {
		t.Fatalf("expected host history chunk, got %#v", response)
	}
	if len(chunk.Records) != 2 {
		t.Fatalf("expected full retained history window despite newer local transcript, got %#v", chunk)
	}
	if chunk.Records[0].MessageID != "msg-earlier-gap" || chunk.Records[1].MessageID != "msg-late" {
		t.Fatalf("expected both earlier and late retained messages, got %#v", chunk.Records)
	}
	if !store.lastSince.Equal(joinedAt) {
		t.Fatalf("expected host history lower bound to stay at joinedAt %v, got %v", joinedAt, store.lastSince)
	}
}

func TestHostRoomSplitsOversizedHostHistoryWindow(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	joinedAt := time.Date(2026, 4, 20, 20, 0, 0, 0, time.UTC)
	store := &fakeRetainedHistoryStore{
		window: hosthistory.Window{
			Records: []transcript.Record{
				{
					MessageID:      "msg-1",
					Direction:      transcript.DirectionIncoming,
					From:           "bob",
					AuthorIdentity: "identity-b",
					Body:           strings.Repeat("a", 220),
					At:             joinedAt.Add(time.Minute),
					Status:         transcript.StatusSent,
				},
				{
					MessageID:      "msg-2",
					Direction:      transcript.DirectionIncoming,
					From:           "bob",
					AuthorIdentity: "identity-b",
					Body:           strings.Repeat("b", 220),
					At:             joinedAt.Add(2 * time.Minute),
					Status:         transcript.StatusSent,
				},
				{
					MessageID:      "msg-3",
					Direction:      transcript.DirectionIncoming,
					From:           "bob",
					AuthorIdentity: "identity-b",
					Body:           strings.Repeat("c", 220),
					At:             joinedAt.Add(3 * time.Minute),
					Status:         transcript.StatusSent,
				},
			},
		},
	}
	joins := &fakeJoinStore{
		record: historymeta.Record{
			RoomKey:    "join:127.0.0.1:7331",
			IdentityID: "identity-a",
			JoinedAt:   joinedAt,
		},
	}
	room.ConfigureHistoryRetention(store, "join:127.0.0.1:7331", joins.open)

	member := newFakeMember("alice")
	member.maxBytes = 700
	room.AddMember(member)
	drainJoinEvents(t, room, 1)

	member.messages <- session.Message{
		ID:   "hostsync-request-split",
		From: "alice",
		Body: HostHistoryRequestBody(HostHistoryRequest{
			Version:     1,
			RoomKey:     "join:127.0.0.1:7331",
			IdentityID:  "identity-a",
			JoinedAt:    joinedAt,
			NewestLocal: joinedAt,
		}),
		At: joinedAt.Add(4 * time.Minute),
	}

	responses := []session.Message{waitForResentMessage(t, member.resent)}
	for {
		select {
		case message := <-member.resent:
			responses = append(responses, message)
		case <-time.After(100 * time.Millisecond):
			goto collected
		}
	}

collected:
	if len(responses) < 2 {
		t.Fatalf("expected oversized host history to split into multiple chunks, got %#v", responses)
	}
	if responses[0].ID == responses[1].ID {
		t.Fatalf("expected split host history responses to use unique message ids, got %q", responses[0].ID)
	}

	totalRecords := 0
	for _, response := range responses {
		if len(response.Body) > member.maxBytes {
			t.Fatalf("expected split host history chunk to stay under %d bytes, got %d", member.maxBytes, len(response.Body))
		}
		chunk, ok := ParseHostHistoryChunk(response.Body)
		if !ok {
			t.Fatalf("expected host history chunk, got %#v", response)
		}
		totalRecords += len(chunk.Records)
	}
	if totalRecords != 3 {
		t.Fatalf("expected all retained records to be delivered across split chunks, got %d", totalRecords)
	}
}

func TestHostRoomRetainsVisibleChatMessagesAndRevokes(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	store := &fakeRetainedHistoryStore{}
	room.ConfigureHistoryRetention(store, "join:127.0.0.1:7331", nil)

	member := newFakeMember("alice")
	room.AddMember(member)
	drainJoinEvents(t, room, 1)

	member.messages <- session.Message{
		ID:   "sync-hello-retain",
		From: "alice",
		Body: HistorySyncHelloBody(HistorySyncHello{
			Version:    1,
			IdentityID: "identity-a",
			RoomKey:    "join:127.0.0.1:7331",
		}),
		At: time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC),
	}

	member.messages <- session.Message{
		ID:   "msg-retained-1",
		From: "alice",
		Body: "hello retained world",
		At:   time.Date(2026, 4, 27, 12, 0, 30, 0, time.UTC),
	}
	waitForRoomMessage(t, room.Messages())

	member.messages <- session.Message{
		ID:   "revoke-retained-1",
		From: "alice",
		Body: RevokeBody(Revoke{
			Version:          1,
			RoomKey:          "join:127.0.0.1:7331",
			TargetMessageID:  "msg-retained-1",
			OperatorIdentity: "identity-a",
			At:               time.Date(2026, 4, 27, 12, 1, 0, 0, time.UTC),
		}),
		At: time.Date(2026, 4, 27, 12, 1, 0, 0, time.UTC),
	}
	waitForRoomMessage(t, room.Messages())

	if len(store.appendedMessages) != 1 {
		t.Fatalf("expected one retained message, got %#v", store.appendedMessages)
	}
	if store.appendedMessages[0].MessageID != "msg-retained-1" || store.appendedMessages[0].AuthorIdentity != "identity-a" {
		t.Fatalf("expected retained message metadata, got %#v", store.appendedMessages[0])
	}
	if len(store.appendedRevokes) != 1 || store.appendedRevokes[0].TargetMessageID != "msg-retained-1" {
		t.Fatalf("expected one retained revoke, got %#v", store.appendedRevokes)
	}
}

func TestHostRoomRetainsHostAuthoredVisibleMessages(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	store := &fakeRetainedHistoryStore{}
	room.ConfigureHistoryRetention(store, "join:127.0.0.1:7331", nil)

	member := newFakeMember("alice")
	room.AddMember(member)
	drainJoinEvents(t, room, 1)

	sent, err := room.Send("host offline note")
	if err != nil {
		t.Fatalf("send host message: %v", err)
	}

	waitForRoomMessage(t, room.Messages())
	waitForResentMessage(t, member.resent)

	if len(store.appendedMessages) != 1 {
		t.Fatalf("expected one retained host-authored message, got %#v", store.appendedMessages)
	}
	got := store.appendedMessages[0]
	if got.MessageID != sent.ID {
		t.Fatalf("expected retained message id %q, got %#v", sent.ID, got)
	}
	if got.AuthorIdentity != "" {
		t.Fatalf("expected host-authored message author identity to stay empty, got %#v", got)
	}
	if got.Body != "host offline note" {
		t.Fatalf("expected retained host-authored body, got %#v", got)
	}
}

func TestHostRoomAcceptsAuthorizedUpdateRequestAndBroadcastsExecute(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	member := newFakeMember("alice")
	room.admins = admins.Store{
		AllowedUpdateIdentities: map[string]struct{}{"identity-a": {}},
	}
	room.identityByPeerName["alice"] = "identity-a"
	resolverCalled := false
	room.releaseResolver = func(context.Context, string) (string, error) {
		resolverCalled = true
		return "v0.1.24", nil
	}
	room.AddMember(member)
	drainJoinEvents(t, room, 1)

	member.messages <- session.Message{
		ID:   "req-1",
		From: "alice",
		Body: UpdateRequestBody(UpdateRequest{
			Version:           1,
			RequestID:         "update-1",
			RoomKey:           "join:203.0.113.10:7331",
			RequesterIdentity: "identity-a",
			RequesterName:     "alice",
		}),
		At: time.Date(2026, 4, 21, 14, 0, 0, 0, time.UTC),
	}

	message := waitForResentMessage(t, member.resent)
	execute, ok := ParseUpdateExecute(message.Body)
	if !ok {
		t.Fatalf("expected execute message, got %#v", message)
	}
	if execute.TargetVersion != "" {
		t.Fatalf("expected empty target version to be broadcast as-is, got %#v", execute)
	}
	if resolverCalled {
		t.Fatal("expected empty target version request not to resolve latest release on host")
	}

	hostMessage := waitForRoomMessage(t, room.Messages())
	if hostMessage.Body != message.Body {
		t.Fatalf("expected host stream to receive execute control, got %#v", hostMessage)
	}
}

func TestHostRoomParticipantNamesIncludeKnownVersionsAndUnknownLegacyPeers(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	memberA := newFakeMember("alice")
	memberB := newFakeMember("bob")
	room.AddMember(memberA)
	room.AddMember(memberB)
	drainJoinEvents(t, room, 2)

	memberA.messages <- session.Message{
		ID:   "sync-hello-versioned",
		From: "alice",
		Body: HistorySyncHelloBody(HistorySyncHello{
			Version:       1,
			IdentityID:    "identity-a",
			ClientVersion: "v0.1.24",
			RoomKey:       "room",
		}),
		At: time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC),
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		names := room.ParticipantNames()
		joined := strings.Join(names, ", ")
		if strings.Contains(joined, "alice [v0.1.24]") {
			if !strings.Contains(joined, "bob [unknown]") {
				t.Fatalf("expected legacy peer to report unknown version, got %q", joined)
			}
			if !strings.Contains(joined, "host [") {
				t.Fatalf("expected host version to be included, got %q", joined)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("expected versioned participant roster, got %#v", room.ParticipantNames())
}

func TestHostRoomAcceptsUnlistedConnectedJoinUpdateRequest(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	member := newFakeMember("eve")
	room.identityByPeerName["eve"] = "identity-eve"
	room.AddMember(member)
	drainJoinEvents(t, room, 1)

	member.messages <- session.Message{
		ID:   "req-1",
		From: "eve",
		Body: UpdateRequestBody(UpdateRequest{
			Version:           1,
			RequestID:         "update-1",
			RoomKey:           "join:203.0.113.10:7331",
			RequesterIdentity: "identity-eve",
			RequesterName:     "eve",
			TargetVersion:     "v0.1.24",
		}),
		At: time.Date(2026, 4, 21, 14, 1, 0, 0, time.UTC),
	}

	message := waitForResentMessage(t, member.resent)
	execute, ok := ParseUpdateExecute(message.Body)
	if !ok {
		t.Fatalf("expected execute message, got %#v", message)
	}
	if execute.TargetVersion != "v0.1.24" || execute.InitiatorName != "eve" {
		t.Fatalf("expected connected join request to be accepted, got %#v", execute)
	}

	hostMessage := waitForRoomMessage(t, room.Messages())
	if hostMessage.Body != message.Body {
		t.Fatalf("expected host stream to receive execute control, got %#v", hostMessage)
	}
}

func TestHostRoomRejectsNonMemberUpdateRequestWithoutWhitelist(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	room.handleUpdateRequest(trackedMember{}, UpdateRequest{
		Version:           1,
		RequestID:         "update-external-1",
		RoomKey:           "join:203.0.113.10:7331",
		RequesterIdentity: "identity-external",
		RequesterName:     "external",
		TargetVersion:     "v0.1.24",
		At:                time.Date(2026, 4, 21, 14, 1, 30, 0, time.UTC),
	})

	message := waitForRoomMessage(t, room.Messages())
	result, ok := ParseUpdateResult(message.Body)
	if !ok {
		t.Fatalf("expected update result, got %#v", message)
	}
	if result.Status != "permission-denied" {
		t.Fatalf("expected non-member request to stay denied, got %#v", result)
	}
}

func TestHostRoomBroadcastsUpdateResultsToRoom(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	memberA := newFakeMember("alice")
	memberB := newFakeMember("bob")
	room.identityByPeerName["alice"] = "identity-a"
	room.identityByPeerName["bob"] = "identity-b"
	room.AddMember(memberA)
	room.AddMember(memberB)
	drainJoinEvents(t, room, 2)

	memberA.messages <- session.Message{
		ID:   "result-1",
		From: "alice",
		Body: UpdateResultBody(UpdateResult{
			Version:        1,
			RequestID:      "update-1",
			RoomKey:        "join:203.0.113.10:7331",
			ReporterName:   "alice",
			ReporterID:     "identity-a",
			TargetVersion:  "v0.1.24",
			Status:         "success",
			CurrentVersion: "v0.1.23",
		}),
		At: time.Date(2026, 4, 21, 14, 2, 0, 0, time.UTC),
	}

	rebroadcast := waitForResentMessage(t, memberA.resent)
	if _, ok := ParseUpdateResult(rebroadcast.Body); !ok {
		t.Fatalf("expected sender to receive broadcast update result, got %#v", rebroadcast)
	}
	other := waitForResentMessage(t, memberB.resent)
	result, ok := ParseUpdateResult(other.Body)
	if !ok {
		t.Fatalf("expected other member to receive update result, got %#v", other)
	}
	if result.ReporterName != "alice" || result.ReporterID != "identity-a" || result.Status != "success" {
		t.Fatalf("expected broadcast result details to round-trip, got %#v", result)
	}

	hostMessage := waitForRoomMessage(t, room.Messages())
	if hostMessage.Body != other.Body {
		t.Fatalf("expected host stream to receive broadcast result, got %#v", hostMessage)
	}
}

func TestHostRoomSubmitUpdateRequestBroadcastsExecute(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	member := newFakeMember("alice")
	room.AddMember(member)
	drainJoinEvents(t, room, 1)

	if err := room.SubmitUpdateRequest(UpdateRequest{
		Version:           1,
		RequestID:         "update-host-1",
		RoomKey:           "join:203.0.113.10:7331",
		RequesterIdentity: "identity-host",
		RequesterName:     "host",
	}); err != nil {
		t.Fatalf("SubmitUpdateRequest returned error: %v", err)
	}

	message := waitForResentMessage(t, member.resent)
	execute, ok := ParseUpdateExecute(message.Body)
	if !ok {
		t.Fatalf("expected execute message, got %#v", message)
	}
	if execute.TargetVersion != "" || execute.InitiatorName != "host" {
		t.Fatalf("expected host submit to broadcast execute, got %#v", execute)
	}
}

func TestHostRoomDeletesAttachmentOnRevoke(t *testing.T) {
	t.Parallel()

	room := NewHostRoom("host")
	defer room.Close()

	cleaner := &fakeAttachmentCleaner{}
	room.ConfigureAttachments(cleaner)

	member := newFakeMember("alice")
	room.AddMember(member)
	drainJoinEvents(t, room, 1)

	revokeMessage := session.Message{
		ID:   "revoke-1",
		From: "alice",
		Body: RevokeBody(Revoke{
			Version:          1,
			RoomKey:          "join:10.77.1.4:7331",
			OperatorIdentity: "id-alice",
			TargetMessageID:  "msg-1",
			At:               time.Now(),
		}),
		At: time.Now(),
	}
	member.messages <- revokeMessage

	waitForCondition(t, func() bool {
		return cleaner.lastDeleted() == "msg-1"
	})
	gotHost := waitForRoomMessage(t, room.Messages())
	if gotHost.Body != revokeMessage.Body {
		t.Fatalf("expected host stream to receive revoke control, got %#v", gotHost)
	}
}

type fakeMember struct {
	peerName string
	maxBytes int
	messages chan session.Message
	resent   chan session.Message
	receipts chan session.Receipt
	done     chan struct{}
}

type fakeAttachmentCleaner struct {
	mu      sync.Mutex
	deleted string
}

type fakeRetainedHistoryStore struct {
	window           hosthistory.Window
	lastRoomKey      string
	lastSince        time.Time
	filterBySince    bool
	appendedMessages []transcript.Record
	appendedRevokes  []transcript.RevokeRecord
}

type fakeJoinStore struct {
	record  historymeta.Record
	records map[string]historymeta.Record
	calls   []historymeta.Record
	callsMu sync.Mutex
}

func newFakeMember(peerName string) *fakeMember {
	return &fakeMember{
		peerName: peerName,
		messages: make(chan session.Message, 8),
		resent:   make(chan session.Message, 8),
		receipts: make(chan session.Receipt, 8),
		done:     make(chan struct{}),
	}
}

func (f *fakeAttachmentCleaner) DeleteByMessageID(messageID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = messageID
	return nil
}

func (f *fakeAttachmentCleaner) lastDeleted() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deleted
}

func (f *fakeRetainedHistoryStore) AppendMessage(_ string, record transcript.Record, _ time.Time) error {
	f.appendedMessages = append(f.appendedMessages, record)
	return nil
}

func (f *fakeRetainedHistoryStore) AppendRevoke(_ string, revoke transcript.RevokeRecord, _ time.Time) error {
	f.appendedRevokes = append(f.appendedRevokes, revoke)
	return nil
}

func (f *fakeRetainedHistoryStore) LoadWindow(roomKey string, since, _ time.Time) (hosthistory.Window, error) {
	f.lastRoomKey = roomKey
	f.lastSince = since
	if !f.filterBySince {
		return f.window, nil
	}
	filtered := hosthistory.Window{
		Records: make([]transcript.Record, 0, len(f.window.Records)),
		Revokes: make([]transcript.RevokeRecord, 0, len(f.window.Revokes)),
	}
	for _, record := range f.window.Records {
		if !record.At.Before(since) {
			filtered.Records = append(filtered.Records, record)
		}
	}
	for _, revoke := range f.window.Revokes {
		if !revoke.At.Before(since) {
			filtered.Revokes = append(filtered.Revokes, revoke)
		}
	}
	return filtered, nil
}

func (f *fakeJoinStore) open(roomKey, identityID string) (historymeta.Record, error) {
	f.callsMu.Lock()
	defer f.callsMu.Unlock()

	if f.records != nil {
		if record, ok := f.records[roomKey]; ok {
			f.calls = append(f.calls, historymeta.Record{RoomKey: roomKey, IdentityID: identityID})
			return record, nil
		}
	}
	if f.record.RoomKey == "" {
		f.record = historymeta.Record{
			RoomKey:    roomKey,
			IdentityID: identityID,
			JoinedAt:   time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC),
		}
	}
	f.calls = append(f.calls, historymeta.Record{RoomKey: roomKey, IdentityID: identityID})
	return f.record, nil
}

func (f *fakeMember) Messages() <-chan session.Message { return f.messages }
func (f *fakeMember) Receipts() <-chan session.Receipt { return f.receipts }
func (f *fakeMember) Done() <-chan struct{}            { return f.done }
func (f *fakeMember) Err() error                       { return nil }
func (f *fakeMember) Close() error {
	select {
	case <-f.done:
	default:
		close(f.done)
	}
	return nil
}
func (f *fakeMember) PeerName() string { return f.peerName }
func (f *fakeMember) MaxMessageSize() int {
	if f.maxBytes > 0 {
		return f.maxBytes
	}
	return session.DefaultMaxMessageSize()
}
func (f *fakeMember) Resend(message session.Message) error {
	if f.maxBytes > 0 && len(message.Body) > f.maxBytes {
		return fmt.Errorf("message exceeds %d bytes", f.maxBytes)
	}
	f.resent <- message
	return nil
}

func waitForRoomMessage(t *testing.T, messages <-chan session.Message) session.Message {
	t.Helper()

	select {
	case message := <-messages:
		return message
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for room message")
		return session.Message{}
	}
}

func assertNoRoomMessage(t *testing.T, messages <-chan session.Message) {
	t.Helper()

	select {
	case message := <-messages:
		t.Fatalf("expected no room message, got %#v", message)
	case <-time.After(100 * time.Millisecond):
	}
}

func waitForRoomEvent(t *testing.T, events <-chan Event) Event {
	t.Helper()

	select {
	case event := <-events:
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for room event")
		return Event{}
	}
}

func waitForResentMessage(t *testing.T, messages <-chan session.Message) session.Message {
	t.Helper()

	select {
	case message := <-messages:
		return message
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for resent message")
		return session.Message{}
	}
}

func assertNoResentMessage(t *testing.T, messages <-chan session.Message) {
	t.Helper()

	select {
	case message := <-messages:
		t.Fatalf("expected no resent message, got %#v", message)
	case <-time.After(100 * time.Millisecond):
	}
}

func drainJoinEvents(t *testing.T, room *HostRoom, count int) {
	t.Helper()

	for range count {
		event := waitForRoomEvent(t, room.Events())
		if event.Kind != EventPeerJoined {
			t.Fatalf("expected joined event while draining, got %#v", event)
		}
	}
}

func waitForCondition(t *testing.T, fn func() bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}

func waitForJoinRecord(t *testing.T, store *fakeJoinStore, roomKey, identityID string) {
	t.Helper()

	waitForCondition(t, func() bool {
		store.callsMu.Lock()
		defer store.callsMu.Unlock()
		for _, call := range store.calls {
			if call.RoomKey == roomKey && call.IdentityID == identityID {
				return true
			}
		}
		return false
	})
}
