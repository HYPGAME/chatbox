package attachment

import "testing"

func TestFormatAndParseChatMessage(t *testing.T) {
	msg := ChatMessage{
		Version: 1,
		ID:      "att_1234567890",
		Kind:    KindImage,
		Name:    "cat.gif",
		Size:    12345678,
	}

	body := FormatChatMessage(msg)
	parsed, ok := ParseChatMessage(body)
	if !ok {
		t.Fatalf("expected attachment body to parse, got %q", body)
	}
	if parsed.ID != msg.ID || parsed.Kind != msg.Kind || parsed.Name != msg.Name || parsed.Size != msg.Size {
		t.Fatalf("expected parsed message %#v, got %#v", msg, parsed)
	}
}
