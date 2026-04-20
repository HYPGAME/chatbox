package room

import (
	"testing"
	"time"
)

func TestEventsControlResponseRoundTrips(t *testing.T) {
	t.Parallel()

	events := []Event{
		{
			Kind:      EventPeerJoined,
			PeerName:  "aaa",
			PeerCount: 1,
			At:        time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC),
		},
		{
			Kind:      EventPeerLeft,
			PeerName:  "bbb",
			PeerCount: 0,
			At:        time.Date(2026, 4, 20, 18, 5, 0, 0, time.UTC),
		},
	}

	body := EventsResponseBody(events)
	parsed, ok := ParseEventsResponse(body)
	if !ok {
		t.Fatal("expected events response to parse")
	}
	if len(parsed) != 2 {
		t.Fatalf("expected 2 parsed events, got %#v", parsed)
	}
	if parsed[0].Kind != EventPeerJoined || parsed[0].PeerName != "aaa" {
		t.Fatalf("expected joined event to round-trip, got %#v", parsed[0])
	}
	if parsed[1].Kind != EventPeerLeft || parsed[1].PeerName != "bbb" {
		t.Fatalf("expected left event to round-trip, got %#v", parsed[1])
	}
}

func TestEventsControlIgnoresRegularMessages(t *testing.T) {
	t.Parallel()

	if IsEventsRequest("hello") {
		t.Fatal("expected regular message not to be events request")
	}
	if _, ok := ParseEventsResponse("hello"); ok {
		t.Fatal("expected regular message not to parse as events response")
	}
}
