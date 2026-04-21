package room

import (
	"testing"
	"time"
)

func TestRevokeControlRoundTrips(t *testing.T) {
	t.Parallel()

	revoke := Revoke{
		Version:          1,
		RoomKey:          "join:203.0.113.10:7331",
		OperatorIdentity: "identity-1",
		TargetMessageID:  "msg-1",
		At:               time.Date(2026, 4, 21, 18, 0, 0, 0, time.UTC),
	}

	parsed, ok := ParseRevoke(RevokeBody(revoke))
	if !ok {
		t.Fatal("expected revoke payload to parse")
	}
	if parsed.RoomKey != revoke.RoomKey || parsed.OperatorIdentity != revoke.OperatorIdentity || parsed.TargetMessageID != revoke.TargetMessageID {
		t.Fatalf("expected revoke to round-trip, got %#v", parsed)
	}
}

func TestRevokeControlIgnoresRegularMessages(t *testing.T) {
	t.Parallel()

	if IsRevokeControl("hello") {
		t.Fatal("expected regular message not to be revoke control")
	}
	if _, ok := ParseRevoke("hello"); ok {
		t.Fatal("expected regular message not to parse as revoke control")
	}
}
