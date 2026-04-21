package room

import (
	"testing"
	"time"
)

func TestUpdateControlRequestRoundTrip(t *testing.T) {
	t.Parallel()

	request := UpdateRequest{
		Version:           1,
		RequestID:         "req-1",
		RoomKey:           "join:203.0.113.10:7331",
		RequesterIdentity: "identity-a",
		RequesterName:     "alice",
		TargetVersion:     "v0.1.24",
		At:                time.Date(2026, 4, 21, 13, 0, 0, 0, time.UTC),
	}

	parsed, ok := ParseUpdateRequest(UpdateRequestBody(request))
	if !ok {
		t.Fatal("expected update request to parse")
	}
	if parsed.RequestID != request.RequestID || parsed.TargetVersion != request.TargetVersion {
		t.Fatalf("expected request to round-trip, got %#v", parsed)
	}
}

func TestUpdateControlExecuteRoundTrip(t *testing.T) {
	t.Parallel()

	execute := UpdateExecute{
		Version:           1,
		RequestID:         "req-1",
		RoomKey:           "join:203.0.113.10:7331",
		InitiatorIdentity: "identity-a",
		InitiatorName:     "alice",
		TargetVersion:     "v0.1.24",
		At:                time.Date(2026, 4, 21, 13, 1, 0, 0, time.UTC),
	}

	parsed, ok := ParseUpdateExecute(UpdateExecuteBody(execute))
	if !ok {
		t.Fatal("expected update execute to parse")
	}
	if parsed.RequestID != execute.RequestID || parsed.InitiatorIdentity != execute.InitiatorIdentity {
		t.Fatalf("expected execute to round-trip, got %#v", parsed)
	}
}

func TestUpdateControlResultRoundTrip(t *testing.T) {
	t.Parallel()

	result := UpdateResult{
		Version:        1,
		RequestID:      "req-1",
		RoomKey:        "join:203.0.113.10:7331",
		ReporterName:   "bob",
		ReporterID:     "identity-b",
		TargetVersion:  "v0.1.24",
		Status:         "success",
		CurrentVersion: "v0.1.23",
		At:             time.Date(2026, 4, 21, 13, 2, 0, 0, time.UTC),
	}

	parsed, ok := ParseUpdateResult(UpdateResultBody(result))
	if !ok {
		t.Fatal("expected update result to parse")
	}
	if parsed.Status != result.Status || parsed.ReporterID != result.ReporterID {
		t.Fatalf("expected result to round-trip, got %#v", parsed)
	}
}

func TestUpdateControlIgnoresRegularMessages(t *testing.T) {
	t.Parallel()

	if IsUpdateControl("hello") {
		t.Fatal("expected regular message not to be update control")
	}
	if _, ok := ParseUpdateRequest("hello"); ok {
		t.Fatal("expected regular message not to parse as update request")
	}
}
