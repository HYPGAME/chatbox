//go:build darwin

package tui

import "testing"

func TestDefaultClipboardWriterUsesInjectedPbcopyRunner(t *testing.T) {
	t.Parallel()

	previous := runPbcopy
	defer func() { runPbcopy = previous }()

	var got string
	runPbcopy = func(text string) error {
		got = text
		return nil
	}

	writer := defaultClipboardWriter()
	if writer == nil {
		t.Fatal("expected darwin clipboard writer")
	}
	if err := writer("copied text"); err != nil {
		t.Fatalf("clipboard writer returned error: %v", err)
	}
	if got != "copied text" {
		t.Fatalf("expected pbcopy payload %q, got %q", "copied text", got)
	}
}
