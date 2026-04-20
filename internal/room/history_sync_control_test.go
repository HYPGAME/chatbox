package room

import (
	"testing"
	"time"

	"chatbox/internal/transcript"
)

func TestHistorySyncControlHelloRoundTrips(t *testing.T) {
	t.Parallel()

	hello := HistorySyncHello{
		Version:    1,
		IdentityID: "identity-1",
		RoomKey:    "join:203.0.113.10:7331",
		Summary: HistorySyncSummary{
			Count:  3,
			Oldest: time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC),
			Newest: time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC),
		},
	}

	parsed, ok := ParseHistorySyncHello(HistorySyncHelloBody(hello))
	if !ok {
		t.Fatal("expected hello payload to parse")
	}
	if parsed.IdentityID != hello.IdentityID || parsed.RoomKey != hello.RoomKey || parsed.Summary.Count != hello.Summary.Count {
		t.Fatalf("expected hello to round-trip, got %#v", parsed)
	}
}

func TestHistorySyncControlOfferAndRequestRoundTrip(t *testing.T) {
	t.Parallel()

	offer := HistorySyncOffer{
		Version:        1,
		SourceIdentity: "identity-source",
		TargetIdentity: "identity-target",
		RoomKey:        "room",
		Summary:        HistorySyncSummary{Count: 8},
	}
	parsedOffer, ok := ParseHistorySyncOffer(HistorySyncOfferBody(offer))
	if !ok {
		t.Fatal("expected offer payload to parse")
	}
	if parsedOffer.SourceIdentity != offer.SourceIdentity || parsedOffer.TargetIdentity != offer.TargetIdentity {
		t.Fatalf("expected offer to round-trip, got %#v", parsedOffer)
	}

	request := HistorySyncRequest{
		Version:        1,
		SourceIdentity: "identity-source",
		TargetIdentity: "identity-target",
		RoomKey:        "room",
		Since:          time.Date(2026, 4, 20, 9, 0, 0, 0, time.UTC),
	}
	parsedRequest, ok := ParseHistorySyncRequest(HistorySyncRequestBody(request))
	if !ok {
		t.Fatal("expected request payload to parse")
	}
	if parsedRequest.SourceIdentity != request.SourceIdentity || !parsedRequest.Since.Equal(request.Since) {
		t.Fatalf("expected request to round-trip, got %#v", parsedRequest)
	}
}

func TestHistorySyncControlChunkRoundTripsTranscriptRecords(t *testing.T) {
	t.Parallel()

	chunk := HistorySyncChunk{
		Version:        1,
		SourceIdentity: "identity-source",
		TargetIdentity: "identity-target",
		RoomKey:        "room",
		Records: []transcript.Record{
			{
				MessageID: "msg-1",
				Direction: transcript.DirectionIncoming,
				From:      "alice",
				Body:      "hello",
				At:        time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC),
				Status:    transcript.StatusSent,
			},
		},
	}

	parsed, ok := ParseHistorySyncChunk(HistorySyncChunkBody(chunk))
	if !ok {
		t.Fatal("expected chunk payload to parse")
	}
	if len(parsed.Records) != 1 || parsed.Records[0].MessageID != "msg-1" || parsed.Records[0].Body != "hello" {
		t.Fatalf("expected chunk records to round-trip, got %#v", parsed)
	}
}

func TestHistorySyncControlIgnoresNonSyncMessages(t *testing.T) {
	t.Parallel()

	if IsHistorySyncControl("hello") {
		t.Fatal("expected regular message not to be sync control")
	}
	if _, ok := ParseHistorySyncHello("hello"); ok {
		t.Fatal("expected regular message not to parse as sync hello")
	}
}
