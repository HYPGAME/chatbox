package session

import (
	"testing"
	"time"
)

func TestEncodeAndDecodeMessagePayloadPreservesSenderTimestamp(t *testing.T) {
	t.Parallel()

	original := Message{
		ID:   "msg-1",
		From: "joiner",
		Body: "hello",
		At:   time.Date(2026, 4, 14, 20, 30, 45, 123000000, time.UTC),
	}

	payload, err := encodeMessagePayload(original)
	if err != nil {
		t.Fatalf("encodeMessagePayload returned error: %v", err)
	}

	decoded, err := decodeMessagePayload("joiner", payload)
	if err != nil {
		t.Fatalf("decodeMessagePayload returned error: %v", err)
	}

	if !decoded.At.Equal(original.At) {
		t.Fatalf("expected timestamp %s, got %s", original.At, decoded.At)
	}
	if decoded.Body != original.Body {
		t.Fatalf("expected body %q, got %q", original.Body, decoded.Body)
	}
	if decoded.ID != original.ID {
		t.Fatalf("expected message ID %q, got %q", original.ID, decoded.ID)
	}
}
