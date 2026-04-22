package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"chatbox/internal/historymeta"
	"chatbox/internal/identity"
	"chatbox/internal/room"
	"chatbox/internal/session"
	"chatbox/internal/transcript"
	"chatbox/internal/update"
	"chatbox/internal/version"
)

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiEscapePattern.ReplaceAllString(s, "")
}

func TestModelShowsConnectedStatusAndIncomingMessage(t *testing.T) {
	t.Parallel()

	fake := &fakeSession{peerName: "joiner"}
	uiModel := newModel(modelOptions{
		mode:          "host",
		listeningAddr: "127.0.0.1:7331",
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})

	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(sessionReadyMsg{session: fake})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			From: "joiner",
			Body: "hello",
			At:   time.Date(2026, 4, 14, 20, 30, 45, 0, time.Local),
		},
	})
	uiModel = updated.(model)

	view := stripANSI(uiModel.View())
	if !strings.Contains(view, "connected") {
		t.Fatalf("expected connected status in view, got %q", view)
	}
	if !strings.Contains(view, "hello") {
		t.Fatalf("expected incoming message in view, got %q", view)
	}
	if !strings.Contains(view, "[20:30]") {
		t.Fatalf("expected compact message timestamp in view, got %q", view)
	}
}

func TestModelRendersCompactStatusBar(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:          "host",
		listeningAddr: "127.0.0.1:7331",
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	uiModel = updated.(model)

	firstLine := strings.Split(stripANSI(uiModel.View()), "\n")[0]
	if !strings.Contains(firstLine, "chatbox host") {
		t.Fatalf("expected compact status bar to contain mode, got %q", firstLine)
	}
	if !strings.Contains(firstLine, "listening on 127.0.0.1:7331") {
		t.Fatalf("expected compact status bar to contain status, got %q", firstLine)
	}
	if !strings.Contains(firstLine, "/help") {
		t.Fatalf("expected compact status bar to include help hint, got %q", firstLine)
	}
	if !strings.Contains(stripANSI(uiModel.View()), "commands: /help /status /events /quit") {
		t.Fatalf("expected startup hints to include /events, got %q", stripANSI(uiModel.View()))
	}
}

func TestModelShowsUpdateNoticeInTUIView(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:          "join",
		uiMode:        "tui",
		listeningAddr: "127.0.0.1:7331",
		session:       &fakeSession{peerName: "host"},
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})

	uiModel.addSystemEntry("new version available: v0.1.18 (current: dev-91cd3e3)")
	uiModel.addSystemEntry("run: chatbox self-update")

	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	uiModel = updated.(model)

	view := stripANSI(uiModel.View())
	if !strings.Contains(view, "new version available: v0.1.18 (current: dev-91cd3e3)") {
		t.Fatalf("expected update notice in TUI view, got %q", view)
	}
	if !strings.Contains(view, "run: chatbox self-update") {
		t.Fatalf("expected self-update hint in TUI view, got %q", view)
	}
}

func TestTUIHelpCommandListsEvents(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:    "join",
		uiMode:  uiModeTUI,
		session: &fakeSession{peerName: "host"},
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})

	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	uiModel = updated.(model)
	uiModel.input.SetValue("/help")
	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	uiModel = updated.(model)

	view := stripANSI(uiModel.View())
	if !strings.Contains(view, "commands: /help /status /events /quit") {
		t.Fatalf("expected help output to include /events, got %q", view)
	}
}

func TestModelInitReceivesBackgroundUpdateNotice(t *testing.T) {
	t.Parallel()

	notices := make(chan string, 1)
	notices <- "new version available: v0.1.18 (current: dev-91cd3e3)\nrun: chatbox self-update"
	close(notices)

	uiModel := newModel(modelOptions{
		mode:          "join",
		uiMode:        "tui",
		listeningAddr: "127.0.0.1:7331",
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
		updateNotices: notices,
	})

	cmd := uiModel.Init()
	if cmd == nil {
		t.Fatal("expected init command to wait for background update notices")
	}

	msg := cmd()
	if msg == nil {
		t.Fatal("expected background update notice message")
	}
	updated, _ := uiModel.Update(msg)
	uiModel = updated.(model)

	view := stripANSI(uiModel.View())
	if !strings.Contains(view, "new version available: v0.1.18 (current: dev-91cd3e3)") {
		t.Fatalf("expected update notice in TUI view, got %q", view)
	}
	if !strings.Contains(view, "run: chatbox self-update") {
		t.Fatalf("expected self-update hint in TUI view, got %q", view)
	}
}

func TestModelSessionReadyCreatesLocalIdentity(t *testing.T) {
	configDir := t.TempDir()
	originalUserConfigDir := os.Getenv("XDG_CONFIG_HOME")
	if err := os.Setenv("XDG_CONFIG_HOME", configDir); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	t.Cleanup(func() {
		if originalUserConfigDir == "" {
			_ = os.Unsetenv("XDG_CONFIG_HOME")
		} else {
			_ = os.Setenv("XDG_CONFIG_HOME", originalUserConfigDir)
		}
	})

	uiModel := newModel(modelOptions{
		mode:    "join",
		session: &fakeSession{peerName: "host"},
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: &fakeSession{peerName: "host"}})
	uiModel = updated.(model)
	if uiModel.identityID == "" {
		t.Fatal("expected session ready to load a local identity")
	}
}

func TestModelSendsTypedMessageOnEnter(t *testing.T) {
	t.Parallel()

	fake := &fakeSession{peerName: "host", localName: "alice"}
	uiModel := newModel(modelOptions{
		mode:    "join",
		session: fake,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})

	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	uiModel = updated.(model)
	uiModel.input.SetValue("hello from cli")
	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	uiModel = updated.(model)

	if len(fake.sent) != 1 || fake.sent[0].Body != "hello from cli" {
		t.Fatalf("expected fake session to receive sent message, got %#v", fake.sent)
	}

	view := uiModel.View()
	if !strings.Contains(view, "alice: hello from cli") {
		t.Fatalf("expected local message in view, got %q", view)
	}
	if !strings.Contains(view, time.Now().Format("2006-01-02")) {
		t.Fatalf("expected local message date in view, got %q", view)
	}
	if !strings.Contains(view, "[sending]") {
		t.Fatalf("expected local message to start in sending state, got %q", view)
	}

	updated, _ = uiModel.Update(receiptMsg{
		receipt: session.Receipt{MessageID: fake.sent[0].ID},
	})
	uiModel = updated.(model)
	if got := stripANSI(uiModel.View()); strings.Contains(got, "[sent]") || strings.Contains(got, "[sending]") {
		t.Fatalf("expected sent messages to hide delivery status, got %q", got)
	}
}

func TestCopySelectionSkipsSystemAndErrorEntries(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:   "join",
		uiMode: uiModeTUI,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	uiModel.history = nil
	uiModel.copySelection = nil
	uiModel.copySelectionPos = -1
	uiModel.renderedViewport = renderedViewportState{}

	uiModel.addHistoryEntry(historyEntry{kind: historyKindSystem, body: "joined", at: time.Date(2026, 4, 22, 10, 0, 0, 0, time.Local)})
	uiModel.addHistoryEntry(historyEntry{kind: historyKindMessage, from: "alice", body: "one", at: time.Date(2026, 4, 22, 10, 0, 1, 0, time.Local)})
	uiModel.addHistoryEntry(historyEntry{kind: historyKindError, body: "broken", at: time.Date(2026, 4, 22, 10, 0, 2, 0, time.Local)})
	uiModel.addHistoryEntry(historyEntry{kind: historyKindMessage, from: "bob", body: "two", at: time.Date(2026, 4, 22, 10, 0, 3, 0, time.Local)})

	uiModel.moveCopySelection(-1)
	if got := uiModel.selectedCopyHistoryIndex(); got != 1 {
		t.Fatalf("expected first selectable message index 1, got %d", got)
	}

	uiModel.moveCopySelection(1)
	if got := uiModel.selectedCopyHistoryIndex(); got != 3 {
		t.Fatalf("expected second selectable message index 3, got %d", got)
	}
}

func TestRenderedCopyTextIncludesWrappedMessageLines(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:   "join",
		uiMode: uiModeTUI,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	uiModel.history = nil
	uiModel.copySelection = nil
	uiModel.copySelectionPos = -1
	uiModel.renderedViewport = renderedViewportState{}
	uiModel.width = 32
	uiModel.height = 10
	uiModel.resize()
	uiModel.addHistoryEntry(historyEntry{
		kind: historyKindMessage,
		from: "alice",
		body: "this message should wrap across multiple visual lines in the tui",
		at:   time.Date(2026, 4, 22, 10, 5, 0, 0, time.Local),
	})

	text, ok := uiModel.selectedCopyText()
	if !ok {
		t.Fatal("expected selected copy text")
	}
	if !strings.Contains(text, "[10:05]") || !strings.Contains(text, "alice:") {
		t.Fatalf("expected rendered metadata in copied text, got %q", text)
	}
	if !strings.Contains(text, "\n") {
		t.Fatalf("expected wrapped multi-line copy text, got %q", text)
	}
}

func TestCtrlYEntersCopyModeAndEscExits(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:   "join",
		uiMode: uiModeTUI,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	uiModel.history = nil
	uiModel.copySelection = nil
	uiModel.copySelectionPos = -1
	uiModel.renderedViewport = renderedViewportState{}
	uiModel.width = 80
	uiModel.height = 12
	uiModel.resize()
	uiModel.addHistoryEntry(historyEntry{
		kind: historyKindMessage,
		from: "alice",
		body: "copy me",
		at:   time.Date(2026, 4, 22, 11, 0, 0, 0, time.Local),
	})

	if strings.Contains(stripANSI(uiModel.View()), "> [11:00] alice: copy me") {
		t.Fatalf("expected no copy selection highlight before entering copy mode, got %q", stripANSI(uiModel.View()))
	}

	updated, _ := uiModel.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	uiModel = updated.(model)
	if !uiModel.copyMode {
		t.Fatal("expected Ctrl+Y to enter copy mode")
	}
	if !strings.Contains(stripANSI(uiModel.View()), "copy mode") {
		t.Fatalf("expected copy mode notice, got %q", stripANSI(uiModel.View()))
	}
	if !strings.Contains(stripANSI(uiModel.View()), "> [11:00] alice: copy me") {
		t.Fatalf("expected selected message highlight in copy mode, got %q", stripANSI(uiModel.View()))
	}

	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyEsc})
	uiModel = updated.(model)
	if uiModel.copyMode {
		t.Fatal("expected Esc to exit copy mode")
	}
	if strings.Contains(stripANSI(uiModel.View()), "> [11:00] alice: copy me") {
		t.Fatalf("expected selection highlight to clear after exiting copy mode, got %q", stripANSI(uiModel.View()))
	}
}

func TestCtrlYCopiesSelectedMessageInCopyMode(t *testing.T) {
	t.Parallel()

	var copied string

	uiModel := newModel(modelOptions{
		mode:   "join",
		uiMode: uiModeTUI,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	uiModel.history = nil
	uiModel.copySelection = nil
	uiModel.copySelectionPos = -1
	uiModel.renderedViewport = renderedViewportState{}
	uiModel.clipboardWriter = func(text string) error {
		copied = text
		return nil
	}
	uiModel.width = 80
	uiModel.height = 12
	uiModel.resize()
	uiModel.addHistoryEntry(historyEntry{
		kind: historyKindMessage,
		from: "alice",
		body: "copy me",
		at:   time.Date(2026, 4, 22, 11, 0, 0, 0, time.Local),
	})

	updated, _ := uiModel.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	uiModel = updated.(model)

	if !strings.Contains(copied, "copy me") {
		t.Fatalf("expected copied message body, got %q", copied)
	}
	if !strings.Contains(stripANSI(uiModel.View()), "copied message") {
		t.Fatalf("expected copied status notice, got %q", stripANSI(uiModel.View()))
	}
}

func TestCtrlYShowsCopyFailureInStatusBarInCopyMode(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:   "join",
		uiMode: uiModeTUI,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	uiModel.history = nil
	uiModel.copySelection = nil
	uiModel.copySelectionPos = -1
	uiModel.renderedViewport = renderedViewportState{}
	uiModel.clipboardWriter = func(string) error {
		return errClipboardUnsupported
	}
	uiModel.addHistoryEntry(historyEntry{
		kind: historyKindMessage,
		from: "alice",
		body: "copy me",
		at:   time.Date(2026, 4, 22, 11, 1, 0, 0, time.Local),
	})

	updated, _ := uiModel.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	uiModel = updated.(model)

	if !strings.Contains(stripANSI(uiModel.View()), "copy unsupported") {
		t.Fatalf("expected copy failure notice, got %q", stripANSI(uiModel.View()))
	}
}

func TestCopyModeSelectionFollowsBottomOnlyUntilUserMovesAway(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:   "join",
		uiMode: uiModeTUI,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	uiModel.history = nil
	uiModel.copySelection = nil
	uiModel.copySelectionPos = -1
	uiModel.renderedViewport = renderedViewportState{}
	uiModel.width = 72
	uiModel.height = 8
	uiModel.resize()

	for i := 0; i < 3; i++ {
		uiModel.addHistoryEntry(historyEntry{
			kind: historyKindMessage,
			from: "alice",
			body: fmt.Sprintf("msg-%d", i),
			at:   time.Date(2026, 4, 22, 11, 10, i, 0, time.Local),
		})
	}

	updated, _ := uiModel.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	uiModel = updated.(model)
	if got := uiModel.selectedCopyHistoryIndex(); got != 2 {
		t.Fatalf("expected copy mode selection to start on newest message, got %d", got)
	}

	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyUp})
	uiModel = updated.(model)
	if got := uiModel.selectedCopyHistoryIndex(); got != 1 {
		t.Fatalf("expected manual move off bottom, got %d", got)
	}

	uiModel.addHistoryEntry(historyEntry{
		kind: historyKindMessage,
		from: "bob",
		body: "new message",
		at:   time.Date(2026, 4, 22, 11, 10, 5, 0, time.Local),
	})
	if got := uiModel.selectedCopyHistoryIndex(); got != 1 {
		t.Fatalf("expected selection to stay on manual choice, got %d", got)
	}
}

func TestCopyModeEnterQuotesSelectedMessage(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:   "join",
		uiMode: uiModeTUI,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	uiModel.history = nil
	uiModel.copySelection = nil
	uiModel.copySelectionPos = -1
	uiModel.renderedViewport = renderedViewportState{}
	uiModel.width = 80
	uiModel.height = 12
	uiModel.resize()
	uiModel.addHistoryEntry(historyEntry{
		kind: historyKindMessage,
		from: "alice",
		body: "hello world",
		at:   time.Date(2026, 4, 22, 11, 20, 0, 0, time.Local),
	})

	updated, _ := uiModel.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	uiModel = updated.(model)

	want := "> alice [11:20]\n> hello world\n\n"
	if got := uiModel.input.Value(); got != want {
		t.Fatalf("expected quote text %q, got %q", want, got)
	}
	if uiModel.copyMode {
		t.Fatal("expected quote insertion to exit copy mode")
	}
}

func TestCopyModeEnterQuotesMultilineMessage(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:   "join",
		uiMode: uiModeTUI,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	uiModel.history = nil
	uiModel.copySelection = nil
	uiModel.copySelectionPos = -1
	uiModel.renderedViewport = renderedViewportState{}
	uiModel.addHistoryEntry(historyEntry{
		kind: historyKindMessage,
		from: "alice",
		body: "line one\nline two",
		at:   time.Date(2026, 4, 22, 11, 21, 0, 0, time.Local),
	})

	updated, _ := uiModel.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	uiModel = updated.(model)

	want := "> alice [11:21]\n> line one\n> line two\n\n"
	if got := uiModel.input.Value(); got != want {
		t.Fatalf("expected multiline quote text %q, got %q", want, got)
	}
}

func TestCopyModeEnterAppendsQuoteAfterExistingInput(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:   "join",
		uiMode: uiModeTUI,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	uiModel.history = nil
	uiModel.copySelection = nil
	uiModel.copySelectionPos = -1
	uiModel.renderedViewport = renderedViewportState{}
	uiModel.input.SetValue("draft reply")
	uiModel.input.SetCursor(len("draft reply"))
	uiModel.addHistoryEntry(historyEntry{
		kind: historyKindMessage,
		from: "alice",
		body: "hello world",
		at:   time.Date(2026, 4, 22, 11, 22, 0, 0, time.Local),
	})

	updated, _ := uiModel.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	uiModel = updated.(model)

	want := "draft reply\n> alice [11:22]\n> hello world\n\n"
	if got := uiModel.input.Value(); got != want {
		t.Fatalf("expected appended quote text %q, got %q", want, got)
	}
}

func TestRenderEntryWithStatusColorsOnlySenderLabel(t *testing.T) {
	oldProfile := lipgloss.ColorProfile()
	oldDarkBackground := lipgloss.HasDarkBackground()
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)
	defer lipgloss.SetColorProfile(oldProfile)
	defer lipgloss.SetHasDarkBackground(oldDarkBackground)

	entry := historyEntry{
		kind: historyKindMessage,
		from: "alice",
		body: "hello",
		at:   time.Date(2026, 4, 17, 11, 30, 0, 0, time.Local),
	}

	rendered := renderEntryWithStatus(entry, "")
	plain := "[2026-04-17 11:30:00] alice: hello"

	if rendered == plain {
		t.Fatalf("expected colored sender output, got plain rendering %q", rendered)
	}
	if !strings.Contains(rendered, "alice") {
		t.Fatalf("expected rendered output to contain sender label, got %q", rendered)
	}
	if !strings.Contains(rendered, "\x1b[0m: hello") {
		t.Fatalf("expected only sender label to be colored, got %q", rendered)
	}
}

func TestRenderEntryWithStatusUsesMutedTimestampAndSecondaryLines(t *testing.T) {
	oldProfile := lipgloss.ColorProfile()
	oldDarkBackground := lipgloss.HasDarkBackground()
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)
	defer lipgloss.SetColorProfile(oldProfile)
	defer lipgloss.SetHasDarkBackground(oldDarkBackground)

	message := historyEntry{
		kind: historyKindMessage,
		from: "alice",
		body: "hello",
		at:   time.Date(2026, 4, 17, 15, 10, 0, 0, time.Local),
	}
	system := historyEntry{
		kind: historyKindSystem,
		body: "alice joined",
		at:   time.Date(2026, 4, 17, 15, 11, 0, 0, time.Local),
	}
	errorEntry := historyEntry{
		kind: historyKindError,
		body: "network down",
		at:   time.Date(2026, 4, 17, 15, 12, 0, 0, time.Local),
	}

	renderedMessage := renderEntryWithStatus(message, "")
	if !strings.HasPrefix(renderedMessage, "\x1b[") {
		t.Fatalf("expected timestamp to be colorized at the start of the line, got %q", renderedMessage)
	}
	if got := stripANSI(renderedMessage); got != "[2026-04-17 15:10:00] alice: hello" {
		t.Fatalf("expected message text to remain stable after stripping ANSI, got %q", got)
	}

	renderedSystem := renderEntryWithStatus(system, "")
	if renderedSystem == "system [2026-04-17 15:11:00]: alice joined" {
		t.Fatalf("expected system line to use muted styling, got %q", renderedSystem)
	}
	if got := stripANSI(renderedSystem); got != "system [2026-04-17 15:11:00]: alice joined" {
		t.Fatalf("expected system text to remain stable after stripping ANSI, got %q", got)
	}

	renderedError := renderEntryWithStatus(errorEntry, "")
	if strings.Contains(renderedError, "\x1b[91m") {
		t.Fatalf("expected error line to avoid the old bright red style, got %q", renderedError)
	}
	if got := stripANSI(renderedError); got != "error [2026-04-17 15:12:00]: network down" {
		t.Fatalf("expected error text to remain stable after stripping ANSI, got %q", got)
	}
}

func TestRenderTUIEntryUsesCompactTime(t *testing.T) {
	t.Parallel()

	entry := historyEntry{
		kind: historyKindMessage,
		from: "alice",
		body: "hello",
		at:   time.Date(2026, 4, 17, 15, 10, 0, 0, time.Local),
	}

	if got := stripANSI(renderTUIEntry(entry, false)); got != "[15:10] alice: hello" {
		t.Fatalf("expected compact TUI message timestamp, got %q", got)
	}
}

func TestRefreshViewportAddsDateSeparators(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode: "join",
		session: &fakeSession{
			peerName: "host",
		},
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 80, Height: 16})
	uiModel = updated.(model)

	uiModel.addMessageEntry(session.Message{
		ID:   "m1",
		From: "host",
		Body: "first",
		At:   time.Date(2026, 4, 17, 23, 59, 0, 0, time.Local),
	}, false, transcript.StatusSent, false)
	uiModel.addMessageEntry(session.Message{
		ID:   "m2",
		From: "host",
		Body: "second",
		At:   time.Date(2026, 4, 18, 0, 1, 0, 0, time.Local),
	}, false, transcript.StatusSent, false)

	view := stripANSI(uiModel.View())
	if !strings.Contains(view, "--- 2026-04-17 ---") {
		t.Fatalf("expected first date separator, got %q", view)
	}
	if !strings.Contains(view, "--- 2026-04-18 ---") {
		t.Fatalf("expected second date separator, got %q", view)
	}
}

func TestModelShowsDisconnectedStatus(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:          "host",
		listeningAddr: "127.0.0.1:7331",
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(sessionClosedMsg{err: errors.New("network down")})
	uiModel = updated.(model)

	if !strings.Contains(uiModel.View(), "disconnected") {
		t.Fatalf("expected disconnected status in view, got %q", uiModel.View())
	}
}

func TestModelRetainsScrollableHistoryAcrossManyMessages(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode: "join",
		session: &fakeSession{
			peerName: "host",
		},
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})

	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 72, Height: 8})
	uiModel = updated.(model)

	for i := 0; i < 30; i++ {
		updated, _ = uiModel.Update(incomingMessageMsg{
			message: session.Message{
				ID:   fmt.Sprintf("msg-%02d", i),
				From: "host",
				Body: fmt.Sprintf("message-%02d", i),
				At:   time.Date(2026, 4, 14, 22, 0, i, 0, time.Local),
			},
		})
		uiModel = updated.(model)
	}

	if !strings.Contains(uiModel.View(), "message-29") {
		t.Fatalf("expected latest message in bottom view, got %q", uiModel.View())
	}

	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyHome})
	uiModel = updated.(model)
	if !strings.Contains(uiModel.View(), "message-00") {
		t.Fatalf("expected first message after Home scroll, got %q", uiModel.View())
	}
}

func TestHostModelShowsPeerCountInStatus(t *testing.T) {
	t.Parallel()

	hostRoom := &fakeHostRoom{fakeSession: fakeSession{peerName: "room"}, peerCount: 2}
	uiModel := newModel(modelOptions{
		mode:          "host",
		listeningAddr: "0.0.0.0:7331",
		session:       hostRoom,
		roomEvents:    hostRoom.Events(),
		peerCount:     hostRoom.PeerCount,
		peerNames:     hostRoom.PeerNames,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})

	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	uiModel = updated.(model)
	if !strings.Contains(uiModel.View(), "hosting on 0.0.0.0:7331 (2 peers)") {
		t.Fatalf("expected host status with peer count, got %q", uiModel.View())
	}
}

func TestHostStatusCommandShowsOnlineRoster(t *testing.T) {
	t.Parallel()

	hostRoom := &fakeHostRoom{
		fakeSession: fakeSession{peerName: "room"},
		peerCount:   2,
		peerNames:   []string{"alice", "bob", "carol"},
	}
	uiModel := newModel(modelOptions{
		mode:          "host",
		listeningAddr: "0.0.0.0:7331",
		session:       hostRoom,
		roomEvents:    hostRoom.Events(),
		peerCount:     hostRoom.PeerCount,
		peerNames:     hostRoom.PeerNames,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})

	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	uiModel = updated.(model)
	uiModel.input.SetValue("/status")
	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	uiModel = updated.(model)

	view := stripANSI(uiModel.View())
	if !strings.Contains(view, "hosting on 0.0.0.0:7331 (2 peers)") {
		t.Fatalf("expected host status line, got %q", view)
	}
	if !strings.Contains(view, "online (3): alice, bob, carol") {
		t.Fatalf("expected online roster line, got %q", view)
	}
}

func TestHostEventsCommandShowsJoinLeaveLog(t *testing.T) {
	oldLocal := time.Local
	time.Local = time.FixedZone("CST", 8*60*60)
	t.Cleanup(func() {
		time.Local = oldLocal
	})

	hostRoom := &fakeHostRoom{fakeSession: fakeSession{peerName: "room"}, peerCount: 0}
	uiModel := newModel(modelOptions{
		mode:          "host",
		listeningAddr: "0.0.0.0:7331",
		session:       hostRoom,
		roomEvents:    hostRoom.Events(),
		peerCount:     hostRoom.PeerCount,
		peerNames:     hostRoom.PeerNames,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})

	updated, _ := uiModel.Update(roomEventMsg{event: room.Event{
		Kind:      room.EventPeerJoined,
		PeerName:  "aaa",
		PeerCount: 1,
		At:        time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC),
	}})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(roomEventMsg{event: room.Event{
		Kind:      room.EventPeerLeft,
		PeerName:  "aaa",
		PeerCount: 0,
		At:        time.Date(2026, 4, 20, 18, 5, 0, 0, time.UTC),
	}})
	uiModel = updated.(model)
	uiModel.input.SetValue("/events")
	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	uiModel = updated.(model)

	view := stripANSI(uiModel.View())
	if !strings.Contains(view, "events: aaa joined at 2026-04-21 02:00:00") {
		t.Fatalf("expected joined event line, got %q", view)
	}
	if !strings.Contains(view, "events: aaa left at 2026-04-21 02:05:00") {
		t.Fatalf("expected left event line, got %q", view)
	}
}

func TestJoinStatusCommandSendsHiddenRequestAndRendersRosterResponse(t *testing.T) {
	t.Parallel()

	fake := &fakeSession{peerName: "host", localName: "bob"}
	uiModel := newModel(modelOptions{
		mode:    "join",
		session: fake,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})

	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	uiModel = updated.(model)
	uiModel.input.SetValue("/status")
	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	uiModel = updated.(model)

	if len(fake.sent) != 1 || fake.sent[0].Body != room.StatusRequestBody() {
		t.Fatalf("expected hidden status request to be sent, got %#v", fake.sent)
	}
	view := stripANSI(uiModel.View())
	if !strings.Contains(view, "connected to host") {
		t.Fatalf("expected local status line, got %q", view)
	}
	if strings.Contains(view, room.StatusRequestBody()) {
		t.Fatalf("expected hidden request body to stay out of view, got %q", view)
	}

	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "status-response-1",
			From: "host",
			Body: room.StatusResponseBody([]string{"alice", "bob", "carol"}),
			At:   time.Date(2026, 4, 17, 16, 10, 0, 0, time.Local),
		},
	})
	uiModel = updated.(model)

	view = stripANSI(uiModel.View())
	if !strings.Contains(view, "online (3): alice, bob, carol") {
		t.Fatalf("expected online roster response in view, got %q", view)
	}
	if strings.Contains(view, room.StatusControlPrefix()) {
		t.Fatalf("expected hidden response payload to stay out of view, got %q", view)
	}
}

func TestJoinStatusCommandWrapsLongRosterResponse(t *testing.T) {
	t.Parallel()

	fake := &fakeSession{peerName: "host", localName: "bob"}
	uiModel := newModel(modelOptions{
		mode:    "join",
		session: fake,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})

	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 42, Height: 10})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "status-response-wrap-1",
			From: "host",
			Body: room.StatusResponseBody([]string{
				"alice [v0.1.26]",
				"bob [v0.1.25]",
				"carol [unknown]",
				"host [v0.1.26]",
			}),
			At: time.Date(2026, 4, 22, 18, 10, 0, 0, time.Local),
		},
	})
	uiModel = updated.(model)

	view := stripANSI(uiModel.View())
	for _, token := range []string{"alice", "v0.1.26", "bob", "v0.1.25", "carol", "unknown", "host"} {
		if !strings.Contains(view, token) {
			t.Fatalf("expected wrapped status view to retain %q, got %q", token, view)
		}
	}
}

func TestJoinEventsCommandSendsHiddenRequestAndRendersResponse(t *testing.T) {
	oldLocal := time.Local
	time.Local = time.FixedZone("CST", 8*60*60)
	t.Cleanup(func() {
		time.Local = oldLocal
	})

	fake := &fakeSession{peerName: "host", localName: "bob"}
	uiModel := newModel(modelOptions{
		mode:    "join",
		session: fake,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})

	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	uiModel = updated.(model)
	uiModel.input.SetValue("/events")
	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	uiModel = updated.(model)

	if len(fake.sent) != 1 || fake.sent[0].Body != room.EventsRequestBody() {
		t.Fatalf("expected hidden events request to be sent, got %#v", fake.sent)
	}

	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "events-response-1",
			From: "host",
			Body: room.EventsResponseBody([]room.Event{
				{
					Kind:     room.EventPeerJoined,
					PeerName: "aaa",
					At:       time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC),
				},
			}),
			At: time.Date(2026, 4, 20, 18, 1, 0, 0, time.UTC),
		},
	})
	uiModel = updated.(model)

	view := stripANSI(uiModel.View())
	if !strings.Contains(view, "events: aaa joined at 2026-04-21 02:00:00") {
		t.Fatalf("expected events response in view, got %q", view)
	}
	if strings.Contains(view, "\x00chatbox:events:") {
		t.Fatalf("expected hidden events payload to stay out of view, got %q", view)
	}
}

func TestModelJoinUpdateAllSendsHiddenRequest(t *testing.T) {
	t.Parallel()

	fake := &fakeSession{peerName: "host", localName: "alice"}
	uiModel := newModel(modelOptions{
		mode:          "join",
		uiMode:        uiModeTUI,
		localName:     "alice",
		listeningAddr: "203.0.113.10:7331",
		session:       fake,
		identityLoader: func() (identity.Store, error) {
			return identity.Store{IdentityID: "identity-a", Path: "/tmp/identity.json"}, nil
		},
		roomAuthLoader: func(roomKey, identityID string) (historymeta.Record, error) {
			return historymeta.Record{RoomKey: roomKey, IdentityID: identityID}, nil
		},
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: fake})
	uiModel = updated.(model)
	uiModel.input.SetValue("/update-all v0.1.24")
	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	uiModel = updated.(model)

	request, ok := room.ParseUpdateRequest(fake.sent[len(fake.sent)-1].Body)
	if !ok {
		t.Fatalf("expected hidden update request, got %#v", fake.sent)
	}
	if request.TargetVersion != "v0.1.24" || request.RequesterIdentity != "identity-a" {
		t.Fatalf("expected explicit update request, got %#v", request)
	}
}

func TestModelHostUpdateAllUsesRoomSubmitter(t *testing.T) {
	t.Parallel()

	hostRoom := &fakeHostRoom{fakeSession: fakeSession{peerName: "room"}, peerCount: 1}
	uiModel := newModel(modelOptions{
		mode:          "host",
		uiMode:        uiModeTUI,
		localName:     "host",
		listeningAddr: "0.0.0.0:7331",
		session:       hostRoom,
		roomEvents:    hostRoom.Events(),
		peerCount:     hostRoom.PeerCount,
		peerNames:     hostRoom.PeerNames,
		identityLoader: func() (identity.Store, error) {
			return identity.Store{IdentityID: "identity-host", Path: "/tmp/identity.json"}, nil
		},
		roomAuthLoader: func(roomKey, identityID string) (historymeta.Record, error) {
			return historymeta.Record{RoomKey: roomKey, IdentityID: identityID}, nil
		},
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: hostRoom})
	uiModel = updated.(model)
	uiModel.input.SetValue("/update-all")
	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	uiModel = updated.(model)

	if len(hostRoom.submittedUpdates) != 1 {
		t.Fatalf("expected host room submitter to be used, got %#v", hostRoom.submittedUpdates)
	}
	if hostRoom.submittedUpdates[0].RequesterName != "host" {
		t.Fatalf("expected host requester name, got %#v", hostRoom.submittedUpdates[0])
	}
}

func TestModelRendersPermissionDeniedUpdateResult(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:          "join",
		uiMode:        uiModeTUI,
		localName:     "alice",
		listeningAddr: "203.0.113.10:7331",
		session:       &fakeSession{peerName: "host"},
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
		identityLoader: func() (identity.Store, error) {
			return identity.Store{IdentityID: "identity-a", Path: "/tmp/identity.json"}, nil
		},
		roomAuthLoader: func(roomKey, identityID string) (historymeta.Record, error) {
			return historymeta.Record{RoomKey: roomKey, IdentityID: identityID}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: &fakeSession{peerName: "host"}})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "result-1",
			From: "host",
			Body: room.UpdateResultBody(room.UpdateResult{
				Version:       1,
				RequestID:     "update-1",
				RoomKey:       transcript.JoinRoomKey("203.0.113.10:7331"),
				ReporterName:  "host",
				TargetVersion: "v0.1.24",
				Status:        "permission-denied",
			}),
		},
	})
	uiModel = updated.(model)

	if !strings.Contains(stripANSI(uiModel.View()), "permission-denied") {
		t.Fatalf("expected readable denial in view, got %q", stripANSI(uiModel.View()))
	}
}

func TestModelJoinExecutesApprovedUpdateAndReportsSuccess(t *testing.T) {
	t.Parallel()

	fake := &fakeSession{peerName: "host", localName: "alice"}
	var restarted bool
	uiModel := newModel(modelOptions{
		mode:          "join",
		uiMode:        uiModeTUI,
		localName:     "alice",
		listeningAddr: "203.0.113.10:7331",
		session:       fake,
		startupArgs:   []string{"join", "--peer", "203.0.113.10:7331", "--psk-file", "/tmp/test.psk"},
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
		identityLoader: func() (identity.Store, error) {
			return identity.Store{IdentityID: "identity-a", Path: "/tmp/identity.json"}, nil
		},
		roomAuthLoader: func(roomKey, identityID string) (historymeta.Record, error) {
			return historymeta.Record{RoomKey: roomKey, IdentityID: identityID}, nil
		},
		updatePerformer: func(context.Context, string) (update.Outcome, error) {
			return update.Outcome{Status: "success", LatestVersion: "v0.1.24", Restartable: true}, nil
		},
		restartLauncher: func(update.RestartSpec) error {
			restarted = true
			return nil
		},
		executablePath: func() (string, error) {
			return "/tmp/chatbox", nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: fake})
	uiModel = updated.(model)
	cmd := uiModel.handleIncomingMessage(session.Message{
		ID:   "execute-1",
		From: "host",
		Body: room.UpdateExecuteBody(room.UpdateExecute{
			Version:           1,
			RequestID:         "update-1",
			RoomKey:           transcript.JoinRoomKey("203.0.113.10:7331"),
			InitiatorIdentity: "identity-host",
			InitiatorName:     "host",
			TargetVersion:     "v0.1.24",
		}),
	})
	if cmd == nil {
		t.Fatal("expected update execution command")
	}

	msg := cmd()
	if msg == nil {
		t.Fatal("expected room update performed message")
	}
	updated, _ = uiModel.Update(msg)
	uiModel = updated.(model)

	if restarted {
		t.Fatal("expected restart launcher to be deferred until after tui shutdown")
	}
	if uiModel.pendingRestart == nil {
		t.Fatal("expected pending restart spec to be recorded for post-shutdown launch")
	}
	result, ok := room.ParseUpdateResult(fake.sent[len(fake.sent)-1].Body)
	if !ok {
		t.Fatalf("expected hidden update result, got %#v", fake.sent)
	}
	if result.Status != "success" || result.ReporterID != "identity-a" {
		t.Fatalf("expected success update result, got %#v", result)
	}
}

func TestLaunchPendingRestartIfNeededUsesRestartLauncher(t *testing.T) {
	t.Parallel()

	var restarted bool
	err := launchPendingRestartIfNeeded(model{
		restartLauncher: func(spec update.RestartSpec) error {
			restarted = true
			if spec.Path != "/tmp/chatbox" || len(spec.Args) == 0 || spec.Args[0] != "join" {
				t.Fatalf("expected pending restart spec to be forwarded, got %#v", spec)
			}
			return nil
		},
		pendingRestart: &update.RestartSpec{
			Path: "/tmp/chatbox",
			Args: []string{"join", "--peer", "203.0.113.10:7331"},
		},
	})
	if err != nil {
		t.Fatalf("launchPendingRestartIfNeeded returned error: %v", err)
	}
	if !restarted {
		t.Fatal("expected pending restart to be launched after tui shutdown")
	}
}

func TestModelMarksPeerSyncCapableOnHistorySyncHello(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:          "join",
		listeningAddr: "203.0.113.10:7331",
		session:       &fakeSession{peerName: "host"},
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: &fakeSession{peerName: "host"}})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "sync-hello-1",
			From: "host",
			Body: room.HistorySyncHelloBody(room.HistorySyncHello{
				Version:    1,
				IdentityID: "identity-host",
				RoomKey:    transcript.JoinRoomKey("203.0.113.10:7331"),
			}),
			At: time.Date(2026, 4, 20, 21, 0, 0, 0, time.UTC),
		},
	})
	uiModel = updated.(model)

	if !uiModel.syncCapablePeers["host"] {
		t.Fatal("expected peer to be marked sync-capable after sync hello")
	}
}

func TestModelHidesHistorySyncControlMessagesFromViewAndTranscript(t *testing.T) {
	t.Parallel()

	store := &fakeTranscriptStore{}
	uiModel := newModel(modelOptions{
		mode:          "join",
		listeningAddr: "203.0.113.10:7331",
		session:       &fakeSession{peerName: "host"},
		transcriptOpener: func(string) (transcriptStore, error) {
			return store, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: &fakeSession{peerName: "host"}})
	uiModel = updated.(model)
	controlBody := room.HistorySyncOfferBody(room.HistorySyncOffer{
		Version:        1,
		SourceIdentity: "identity-host",
		TargetIdentity: "identity-local",
		RoomKey:        transcript.JoinRoomKey("203.0.113.10:7331"),
	})
	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "sync-offer-1",
			From: "host",
			Body: controlBody,
			At:   time.Date(2026, 4, 20, 21, 1, 0, 0, time.UTC),
		},
	})
	uiModel = updated.(model)

	view := stripANSI(uiModel.View())
	if strings.Contains(view, controlBody) {
		t.Fatalf("expected history sync control message to stay out of view, got %q", view)
	}
	if len(store.appends) != 0 {
		t.Fatalf("expected history sync control message not to persist to transcript, got %#v", store.appends)
	}
}

func TestModelSendsHistorySyncHelloAfterSessionReady(t *testing.T) {
	t.Parallel()

	fake := &fakeSession{peerName: "host", localName: "alice"}
	uiModel := newModel(modelOptions{
		mode:          "join",
		listeningAddr: "203.0.113.10:7331",
		session:       fake,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
		identityLoader: func() (identity.Store, error) {
			return identity.Store{IdentityID: "identity-local", Path: "/tmp/identity.json"}, nil
		},
		roomAuthLoader: func(roomKey, identityID string) (historymeta.Record, error) {
			return historymeta.Record{
				RoomKey:    roomKey,
				IdentityID: identityID,
				JoinedAt:   time.Date(2026, 4, 20, 20, 0, 0, 0, time.UTC),
			}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: fake})
	uiModel = updated.(model)

	if len(fake.sent) != 2 {
		t.Fatalf("expected version announce and sync hello after session ready, got %#v", fake.sent)
	}
	announce, ok := room.ParseVersionAnnounce(fake.sent[0].Body)
	if !ok {
		t.Fatalf("expected first payload to be version announce, got %#v", fake.sent[0])
	}
	if announce.ClientVersion != version.Version {
		t.Fatalf("expected version announce %q, got %#v", version.Version, announce)
	}
	hello, ok := room.ParseHistorySyncHello(fake.sent[1].Body)
	if !ok {
		t.Fatalf("expected second payload to be sync hello, got %#v", fake.sent[1])
	}
	if hello.IdentityID != "identity-local" {
		t.Fatalf("expected sync hello identity %q, got %#v", "identity-local", hello)
	}
	if hello.ClientVersion != version.Version {
		t.Fatalf("expected sync hello client version %q, got %#v", version.Version, hello)
	}
	if hello.RoomKey != transcript.JoinRoomKey("203.0.113.10:7331") {
		t.Fatalf("expected sync hello room key %q, got %#v", transcript.JoinRoomKey("203.0.113.10:7331"), hello)
	}
}

func TestModelSendsVersionAnnouncementAfterSessionReadyWithoutHistorySyncPrereqs(t *testing.T) {
	t.Parallel()

	fake := &fakeSession{peerName: "host", localName: "alice"}
	uiModel := newModel(modelOptions{
		mode:          "join",
		listeningAddr: "203.0.113.10:7331",
		session:       fake,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
		identityLoader: func() (identity.Store, error) {
			return identity.Store{IdentityID: "", Path: "/tmp/identity.json"}, nil
		},
		roomAuthLoader: func(string, string) (historymeta.Record, error) {
			t.Fatal("roomAuthLoader should not run when identity is empty")
			return historymeta.Record{}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: fake})
	uiModel = updated.(model)

	if len(fake.sent) == 0 {
		t.Fatal("expected version announce to be sent after session ready")
	}
	announce, ok := room.ParseVersionAnnounce(fake.sent[0].Body)
	if !ok {
		t.Fatalf("expected first payload to be version announce, got %#v", fake.sent[0])
	}
	if announce.ClientVersion != version.Version {
		t.Fatalf("expected advertised version %q, got %#v", version.Version, announce)
	}
}

func TestModelRespondsToHistorySyncHelloWithOfferWhenItHasMoreHistory(t *testing.T) {
	t.Parallel()

	fake := &fakeSession{peerName: "host", localName: "alice"}
	uiModel := newModel(modelOptions{
		mode:          "join",
		listeningAddr: "203.0.113.10:7331",
		session:       fake,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
		identityLoader: func() (identity.Store, error) {
			return identity.Store{IdentityID: "identity-local", Path: "/tmp/identity.json"}, nil
		},
		roomAuthLoader: func(roomKey, identityID string) (historymeta.Record, error) {
			return historymeta.Record{
				RoomKey:    roomKey,
				IdentityID: identityID,
				JoinedAt:   time.Date(2026, 4, 20, 20, 0, 0, 0, time.UTC),
			}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: fake})
	uiModel = updated.(model)
	uiModel.addHistoryEntry(historyEntry{
		kind: historyKindMessage,
		from: "alice",
		body: "local history",
		at:   time.Date(2026, 4, 20, 20, 30, 0, 0, time.UTC),
	})
	initialSent := len(fake.sent)
	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "sync-hello-2",
			From: "host",
			Body: room.HistorySyncHelloBody(room.HistorySyncHello{
				Version:    1,
				IdentityID: "identity-host",
				RoomKey:    transcript.JoinRoomKey("203.0.113.10:7331"),
				Summary:    room.HistorySyncSummary{Count: 0},
			}),
			At: time.Date(2026, 4, 20, 21, 0, 0, 0, time.UTC),
		},
	})
	uiModel = updated.(model)

	if len(fake.sent) <= initialSent {
		t.Fatal("expected sync offer to be sent")
	}
	offer, ok := room.ParseHistorySyncOffer(fake.sent[len(fake.sent)-1].Body)
	if !ok {
		t.Fatalf("expected last sent payload to be sync offer, got %#v", fake.sent[len(fake.sent)-1])
	}
	if offer.SourceIdentity != "identity-local" || offer.TargetIdentity != "identity-host" {
		t.Fatalf("expected offer identities to be populated, got %#v", offer)
	}
}

func TestModelOffersHistorySyncWhenItHasNewerHistoryDespiteSameCount(t *testing.T) {
	t.Parallel()

	fake := &fakeSession{peerName: "host", localName: "alice"}
	uiModel := newModel(modelOptions{
		mode:          "join",
		listeningAddr: "203.0.113.10:7331",
		session:       fake,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
		identityLoader: func() (identity.Store, error) {
			return identity.Store{IdentityID: "identity-local", Path: "/tmp/identity.json"}, nil
		},
		roomAuthLoader: func(roomKey, identityID string) (historymeta.Record, error) {
			return historymeta.Record{
				RoomKey:    roomKey,
				IdentityID: identityID,
				JoinedAt:   time.Date(2026, 4, 20, 20, 0, 0, 0, time.UTC),
			}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: fake})
	uiModel = updated.(model)
	uiModel.addHistoryEntry(historyEntry{
		kind:      historyKindMessage,
		messageID: "new-local",
		from:      "alice",
		body:      "newer local history",
		at:        time.Date(2026, 4, 20, 22, 0, 0, 0, time.UTC),
	})
	initialSent := len(fake.sent)
	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "sync-hello-newer",
			From: "host",
			Body: room.HistorySyncHelloBody(room.HistorySyncHello{
				Version:    1,
				IdentityID: "identity-host",
				RoomKey:    transcript.JoinRoomKey("203.0.113.10:7331"),
				Summary: room.HistorySyncSummary{
					Count:  1,
					Newest: time.Date(2026, 4, 20, 21, 0, 0, 0, time.UTC),
				},
			}),
			At: time.Date(2026, 4, 20, 22, 1, 0, 0, time.UTC),
		},
	})
	uiModel = updated.(model)

	if len(fake.sent) <= initialSent {
		t.Fatal("expected sync offer for newer local history even when counts match")
	}
	if _, ok := room.ParseHistorySyncOffer(fake.sent[len(fake.sent)-1].Body); !ok {
		t.Fatalf("expected last sent payload to be sync offer, got %#v", fake.sent[len(fake.sent)-1])
	}
}

func TestModelOffersHistorySyncWhenItHasAnyHistoryForPeer(t *testing.T) {
	t.Parallel()

	fake := &fakeSession{peerName: "host", localName: "alice"}
	uiModel := newModel(modelOptions{
		mode:          "join",
		listeningAddr: "203.0.113.10:7331",
		session:       fake,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
		identityLoader: func() (identity.Store, error) {
			return identity.Store{IdentityID: "identity-local", Path: "/tmp/identity.json"}, nil
		},
		roomAuthLoader: func(roomKey, identityID string) (historymeta.Record, error) {
			return historymeta.Record{
				RoomKey:    roomKey,
				IdentityID: identityID,
				JoinedAt:   time.Date(2026, 4, 20, 20, 0, 0, 0, time.UTC),
			}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: fake})
	uiModel = updated.(model)
	uiModel.addHistoryEntry(historyEntry{
		kind:      historyKindMessage,
		messageID: "older-local",
		from:      "alice",
		body:      "older local history",
		at:        time.Date(2026, 4, 20, 21, 0, 0, 0, time.UTC),
	})
	initialSent := len(fake.sent)
	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "sync-hello-peer-any-history",
			From: "host",
			Body: room.HistorySyncHelloBody(room.HistorySyncHello{
				Version:    1,
				IdentityID: "identity-host",
				RoomKey:    transcript.JoinRoomKey("203.0.113.10:7331"),
				Summary: room.HistorySyncSummary{
					Count:  5,
					Newest: time.Date(2026, 4, 20, 22, 0, 0, 0, time.UTC),
				},
			}),
			At: time.Date(2026, 4, 20, 22, 1, 0, 0, time.UTC),
		},
	})
	uiModel = updated.(model)

	if len(fake.sent) <= initialSent {
		t.Fatal("expected sync offer even when local summary is not newer")
	}
	if _, ok := room.ParseHistorySyncOffer(fake.sent[len(fake.sent)-1].Body); !ok {
		t.Fatalf("expected last sent payload to be sync offer, got %#v", fake.sent[len(fake.sent)-1])
	}
}

func TestModelRequestsHistoryFromMatchingOffer(t *testing.T) {
	t.Parallel()

	fake := &fakeSession{peerName: "host", localName: "alice"}
	joinedAt := time.Date(2026, 4, 20, 20, 0, 0, 0, time.UTC)
	uiModel := newModel(modelOptions{
		mode:          "join",
		listeningAddr: "203.0.113.10:7331",
		session:       fake,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
		identityLoader: func() (identity.Store, error) {
			return identity.Store{IdentityID: "identity-local", Path: "/tmp/identity.json"}, nil
		},
		roomAuthLoader: func(roomKey, identityID string) (historymeta.Record, error) {
			return historymeta.Record{
				RoomKey:    roomKey,
				IdentityID: identityID,
				JoinedAt:   joinedAt,
			}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: fake})
	uiModel = updated.(model)
	initialSent := len(fake.sent)
	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "sync-offer-2",
			From: "host",
			Body: room.HistorySyncOfferBody(room.HistorySyncOffer{
				Version:        1,
				SourceIdentity: "identity-host",
				TargetIdentity: "identity-local",
				RoomKey:        transcript.JoinRoomKey("203.0.113.10:7331"),
				Summary:        room.HistorySyncSummary{Count: 3},
			}),
			At: time.Date(2026, 4, 20, 21, 0, 0, 0, time.UTC),
		},
	})
	uiModel = updated.(model)

	if len(fake.sent) <= initialSent {
		t.Fatal("expected sync request to be sent")
	}
	request, ok := room.ParseHistorySyncRequest(fake.sent[len(fake.sent)-1].Body)
	if !ok {
		t.Fatalf("expected last sent payload to be sync request, got %#v", fake.sent[len(fake.sent)-1])
	}
	if request.SourceIdentity != "identity-host" || request.TargetIdentity != "identity-local" {
		t.Fatalf("expected request identities to be populated, got %#v", request)
	}
	if !request.Since.Equal(joinedAt) {
		t.Fatalf("expected request since %v, got %v", joinedAt, request.Since)
	}
}

func TestModelRequestsHistoryWhenOfferHasNewerHistoryDespiteSameCount(t *testing.T) {
	t.Parallel()

	fake := &fakeSession{peerName: "host", localName: "alice"}
	joinedAt := time.Date(2026, 4, 20, 20, 0, 0, 0, time.UTC)
	uiModel := newModel(modelOptions{
		mode:          "join",
		listeningAddr: "203.0.113.10:7331",
		session:       fake,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
		identityLoader: func() (identity.Store, error) {
			return identity.Store{IdentityID: "identity-local", Path: "/tmp/identity.json"}, nil
		},
		roomAuthLoader: func(roomKey, identityID string) (historymeta.Record, error) {
			return historymeta.Record{
				RoomKey:    roomKey,
				IdentityID: identityID,
				JoinedAt:   joinedAt,
			}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: fake})
	uiModel = updated.(model)
	uiModel.addHistoryEntry(historyEntry{
		kind:      historyKindMessage,
		messageID: "old-local",
		from:      "alice",
		body:      "older local history",
		at:        time.Date(2026, 4, 20, 21, 0, 0, 0, time.UTC),
	})
	initialSent := len(fake.sent)
	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "sync-offer-newer",
			From: "host",
			Body: room.HistorySyncOfferBody(room.HistorySyncOffer{
				Version:        1,
				SourceIdentity: "identity-host",
				TargetIdentity: "identity-local",
				RoomKey:        transcript.JoinRoomKey("203.0.113.10:7331"),
				Summary: room.HistorySyncSummary{
					Count:  1,
					Newest: time.Date(2026, 4, 20, 22, 0, 0, 0, time.UTC),
				},
			}),
			At: time.Date(2026, 4, 20, 22, 1, 0, 0, time.UTC),
		},
	})
	uiModel = updated.(model)

	if len(fake.sent) <= initialSent {
		t.Fatal("expected sync request for newer offered history even when counts match")
	}
	request, ok := room.ParseHistorySyncRequest(fake.sent[len(fake.sent)-1].Body)
	if !ok {
		t.Fatalf("expected last sent payload to be sync request, got %#v", fake.sent[len(fake.sent)-1])
	}
	if !request.Since.Equal(joinedAt) {
		t.Fatalf("expected request since %v, got %v", joinedAt, request.Since)
	}
}

func TestModelRequestsHistoryFromEachPeerOncePerConnectionEvenWithoutNewerSummary(t *testing.T) {
	t.Parallel()

	fake := &fakeSession{peerName: "host", localName: "alice"}
	joinedAt := time.Date(2026, 4, 20, 20, 0, 0, 0, time.UTC)
	uiModel := newModel(modelOptions{
		mode:          "join",
		listeningAddr: "203.0.113.10:7331",
		session:       fake,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
		identityLoader: func() (identity.Store, error) {
			return identity.Store{IdentityID: "identity-local", Path: "/tmp/identity.json"}, nil
		},
		roomAuthLoader: func(roomKey, identityID string) (historymeta.Record, error) {
			return historymeta.Record{
				RoomKey:    roomKey,
				IdentityID: identityID,
				JoinedAt:   joinedAt,
			}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: fake})
	uiModel = updated.(model)
	uiModel.addHistoryEntry(historyEntry{
		kind:      historyKindMessage,
		messageID: "local-1",
		from:      "alice",
		body:      "already have newer message",
		at:        time.Date(2026, 4, 20, 22, 0, 0, 0, time.UTC),
		status:    transcript.StatusSent,
	})

	initialSent := len(fake.sent)
	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "sync-offer-peer-a",
			From: "host",
			Body: room.HistorySyncOfferBody(room.HistorySyncOffer{
				Version:        1,
				SourceIdentity: "identity-peer-a",
				TargetIdentity: "identity-local",
				RoomKey:        transcript.JoinRoomKey("203.0.113.10:7331"),
				Summary: room.HistorySyncSummary{
					Count:  1,
					Newest: time.Date(2026, 4, 20, 21, 0, 0, 0, time.UTC),
				},
			}),
			At: time.Date(2026, 4, 20, 22, 1, 0, 0, time.UTC),
		},
	})
	uiModel = updated.(model)

	if len(fake.sent) <= initialSent {
		t.Fatal("expected first peer offer to trigger sync request even without newer summary")
	}
	requestA, ok := room.ParseHistorySyncRequest(fake.sent[len(fake.sent)-1].Body)
	if !ok {
		t.Fatalf("expected sync request for first peer, got %#v", fake.sent[len(fake.sent)-1])
	}
	if requestA.SourceIdentity != "identity-peer-a" {
		t.Fatalf("expected first request to target peer-a, got %#v", requestA)
	}

	sentAfterA := len(fake.sent)
	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "sync-offer-peer-b",
			From: "host",
			Body: room.HistorySyncOfferBody(room.HistorySyncOffer{
				Version:        1,
				SourceIdentity: "identity-peer-b",
				TargetIdentity: "identity-local",
				RoomKey:        transcript.JoinRoomKey("203.0.113.10:7331"),
				Summary: room.HistorySyncSummary{
					Count:  1,
					Newest: time.Date(2026, 4, 20, 21, 0, 0, 0, time.UTC),
				},
			}),
			At: time.Date(2026, 4, 20, 22, 2, 0, 0, time.UTC),
		},
	})
	uiModel = updated.(model)

	if len(fake.sent) <= sentAfterA {
		t.Fatal("expected second peer offer to also trigger one sync request")
	}
	requestB, ok := room.ParseHistorySyncRequest(fake.sent[len(fake.sent)-1].Body)
	if !ok {
		t.Fatalf("expected sync request for second peer, got %#v", fake.sent[len(fake.sent)-1])
	}
	if requestB.SourceIdentity != "identity-peer-b" {
		t.Fatalf("expected second request to target peer-b, got %#v", requestB)
	}

	sentAfterB := len(fake.sent)
	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "sync-offer-peer-b-repeat",
			From: "host",
			Body: room.HistorySyncOfferBody(room.HistorySyncOffer{
				Version:        1,
				SourceIdentity: "identity-peer-b",
				TargetIdentity: "identity-local",
				RoomKey:        transcript.JoinRoomKey("203.0.113.10:7331"),
				Summary: room.HistorySyncSummary{
					Count:  1,
					Newest: time.Date(2026, 4, 20, 21, 0, 0, 0, time.UTC),
				},
			}),
			At: time.Date(2026, 4, 20, 22, 3, 0, 0, time.UTC),
		},
	})
	uiModel = updated.(model)

	if len(fake.sent) != sentAfterB {
		t.Fatalf("expected repeated offer from same peer not to trigger duplicate request, sent=%#v", fake.sent[sentAfterB:])
	}
}

func TestModelResetsRequestedHistoryOnNewSessionReady(t *testing.T) {
	t.Parallel()

	joinedAt := time.Date(2026, 4, 20, 20, 0, 0, 0, time.UTC)
	first := &fakeSession{peerName: "host", localName: "alice"}
	second := &fakeSession{peerName: "host", localName: "alice"}
	uiModel := newModel(modelOptions{
		mode:          "join",
		listeningAddr: "203.0.113.10:7331",
		session:       first,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
		identityLoader: func() (identity.Store, error) {
			return identity.Store{IdentityID: "identity-local", Path: "/tmp/identity.json"}, nil
		},
		roomAuthLoader: func(roomKey, identityID string) (historymeta.Record, error) {
			return historymeta.Record{
				RoomKey:    roomKey,
				IdentityID: identityID,
				JoinedAt:   joinedAt,
			}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: first})
	uiModel = updated.(model)
	uiModel.addHistoryEntry(historyEntry{
		kind:      historyKindMessage,
		messageID: "local-1",
		from:      "alice",
		body:      "already have newer message",
		at:        time.Date(2026, 4, 20, 22, 0, 0, 0, time.UTC),
		status:    transcript.StatusSent,
	})
	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "sync-offer-first-session",
			From: "host",
			Body: room.HistorySyncOfferBody(room.HistorySyncOffer{
				Version:        1,
				SourceIdentity: "identity-peer-a",
				TargetIdentity: "identity-local",
				RoomKey:        transcript.JoinRoomKey("203.0.113.10:7331"),
				Summary: room.HistorySyncSummary{
					Count:  1,
					Newest: time.Date(2026, 4, 20, 21, 0, 0, 0, time.UTC),
				},
			}),
			At: time.Date(2026, 4, 20, 22, 1, 0, 0, time.UTC),
		},
	})
	uiModel = updated.(model)

	updated, _ = uiModel.Update(sessionReadyMsg{session: second})
	uiModel = updated.(model)
	sentAfterReconnect := len(second.sent)
	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "sync-offer-second-session",
			From: "host",
			Body: room.HistorySyncOfferBody(room.HistorySyncOffer{
				Version:        1,
				SourceIdentity: "identity-peer-a",
				TargetIdentity: "identity-local",
				RoomKey:        transcript.JoinRoomKey("203.0.113.10:7331"),
				Summary: room.HistorySyncSummary{
					Count:  1,
					Newest: time.Date(2026, 4, 20, 21, 0, 0, 0, time.UTC),
				},
			}),
			At: time.Date(2026, 4, 20, 22, 2, 0, 0, time.UTC),
		},
	})
	uiModel = updated.(model)

	if len(second.sent) <= sentAfterReconnect {
		t.Fatal("expected same peer to be requested again after reconnect")
	}
}

func TestModelSendsHistorySyncChunkForAuthorizedRequest(t *testing.T) {
	t.Parallel()

	fake := &fakeSession{peerName: "host", localName: "alice"}
	joinedAt := time.Date(2026, 4, 20, 20, 0, 0, 0, time.UTC)
	uiModel := newModel(modelOptions{
		mode:          "join",
		listeningAddr: "203.0.113.10:7331",
		session:       fake,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
		identityLoader: func() (identity.Store, error) {
			return identity.Store{IdentityID: "identity-local", Path: "/tmp/identity.json"}, nil
		},
		roomAuthLoader: func(roomKey, identityID string) (historymeta.Record, error) {
			return historymeta.Record{RoomKey: roomKey, IdentityID: identityID, JoinedAt: joinedAt}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: fake})
	uiModel = updated.(model)
	uiModel.addHistoryEntry(historyEntry{
		kind:      historyKindMessage,
		messageID: "old-hidden",
		from:      "alice",
		body:      "too old",
		at:        joinedAt.Add(-time.Minute),
		status:    transcript.StatusSent,
	})
	uiModel.addHistoryEntry(historyEntry{
		kind:      historyKindMessage,
		messageID: "new-visible",
		from:      "alice",
		body:      "sync me",
		at:        joinedAt.Add(time.Minute),
		status:    transcript.StatusSent,
	})
	initialSent := len(fake.sent)
	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "sync-request-1",
			From: "host",
			Body: room.HistorySyncRequestBody(room.HistorySyncRequest{
				Version:        1,
				SourceIdentity: "identity-local",
				TargetIdentity: "identity-host",
				RoomKey:        transcript.JoinRoomKey("203.0.113.10:7331"),
				Since:          joinedAt,
			}),
			At: time.Date(2026, 4, 20, 21, 0, 0, 0, time.UTC),
		},
	})
	uiModel = updated.(model)

	if len(fake.sent) <= initialSent {
		t.Fatal("expected sync chunk to be sent")
	}
	chunk, ok := room.ParseHistorySyncChunk(fake.sent[len(fake.sent)-1].Body)
	if !ok {
		t.Fatalf("expected last sent payload to be sync chunk, got %#v", fake.sent[len(fake.sent)-1])
	}
	if len(chunk.Records) != 1 || chunk.Records[0].MessageID != "new-visible" {
		t.Fatalf("expected only authorized record to be sent, got %#v", chunk.Records)
	}
}

func TestModelSendsHistorySyncChunkWithRevokes(t *testing.T) {
	t.Parallel()

	fake := &fakeSession{peerName: "host", localName: "alice"}
	joinedAt := time.Date(2026, 4, 20, 20, 0, 0, 0, time.UTC)
	uiModel := newModel(modelOptions{
		mode:          "join",
		listeningAddr: "203.0.113.10:7331",
		session:       fake,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
		identityLoader: func() (identity.Store, error) {
			return identity.Store{IdentityID: "identity-local", Path: "/tmp/identity.json"}, nil
		},
		roomAuthLoader: func(roomKey, identityID string) (historymeta.Record, error) {
			return historymeta.Record{RoomKey: roomKey, IdentityID: identityID, JoinedAt: joinedAt}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: fake})
	uiModel = updated.(model)
	uiModel.addHistoryEntry(historyEntry{
		kind:           historyKindMessage,
		messageID:      "revoked-visible",
		from:           "alice",
		body:           "sync revoked",
		at:             joinedAt.Add(time.Minute),
		outgoing:       true,
		status:         transcript.StatusSent,
		authorIdentity: "identity-local",
		revoked:        true,
		revokedAt:      joinedAt.Add(2 * time.Minute),
	})
	initialSent := len(fake.sent)
	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "sync-request-revoked",
			From: "host",
			Body: room.HistorySyncRequestBody(room.HistorySyncRequest{
				Version:        1,
				SourceIdentity: "identity-local",
				TargetIdentity: "identity-host",
				RoomKey:        transcript.JoinRoomKey("203.0.113.10:7331"),
				Since:          joinedAt,
			}),
			At: joinedAt.Add(3 * time.Minute),
		},
	})
	uiModel = updated.(model)

	if len(fake.sent) <= initialSent {
		t.Fatal("expected sync chunk with revoke to be sent")
	}
	chunk, ok := room.ParseHistorySyncChunk(fake.sent[len(fake.sent)-1].Body)
	if !ok {
		t.Fatalf("expected last sent payload to be sync chunk, got %#v", fake.sent[len(fake.sent)-1])
	}
	if len(chunk.Records) != 1 || chunk.Records[0].AuthorIdentity != "identity-local" {
		t.Fatalf("expected sync chunk record to include author identity, got %#v", chunk.Records)
	}
	if len(chunk.Revokes) != 1 || chunk.Revokes[0].TargetMessageID != "revoked-visible" {
		t.Fatalf("expected sync chunk revoke to be included, got %#v", chunk.Revokes)
	}
}

func TestModelReplaysHistorySyncChunkIntoTranscript(t *testing.T) {
	t.Parallel()

	store := &fakeTranscriptStore{}
	joinedAt := time.Date(2026, 4, 20, 20, 0, 0, 0, time.UTC)
	uiModel := newModel(modelOptions{
		mode:          "join",
		listeningAddr: "203.0.113.10:7331",
		session:       &fakeSession{peerName: "host"},
		transcriptOpener: func(string) (transcriptStore, error) {
			return store, nil
		},
		identityLoader: func() (identity.Store, error) {
			return identity.Store{IdentityID: "identity-local", Path: "/tmp/identity.json"}, nil
		},
		roomAuthLoader: func(roomKey, identityID string) (historymeta.Record, error) {
			return historymeta.Record{RoomKey: roomKey, IdentityID: identityID, JoinedAt: joinedAt}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: &fakeSession{peerName: "host"}})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "sync-chunk-1",
			From: "host",
			Body: room.HistorySyncChunkBody(room.HistorySyncChunk{
				Version:        1,
				SourceIdentity: "identity-host",
				TargetIdentity: "identity-local",
				RoomKey:        transcript.JoinRoomKey("203.0.113.10:7331"),
				Records: []transcript.Record{
					{
						MessageID: "too-old",
						Direction: transcript.DirectionIncoming,
						From:      "bob",
						Body:      "old",
						At:        joinedAt.Add(-time.Minute),
						Status:    transcript.StatusSent,
					},
					{
						MessageID: "replayed-1",
						Direction: transcript.DirectionIncoming,
						From:      "bob",
						Body:      "replayed history",
						At:        joinedAt.Add(time.Minute),
						Status:    transcript.StatusSent,
					},
				},
			}),
			At: time.Date(2026, 4, 20, 21, 0, 0, 0, time.UTC),
		},
	})
	uiModel = updated.(model)

	if len(store.appends) != 1 || store.appends[0].MessageID != "replayed-1" {
		t.Fatalf("expected authorized replay to persist once, got %#v", store.appends)
	}
	if !strings.Contains(stripANSI(uiModel.View()), "replayed history") {
		t.Fatalf("expected replayed history in view, got %q", stripANSI(uiModel.View()))
	}
}

func TestModelReplaysHistorySyncChunkRevokesIntoTranscript(t *testing.T) {
	t.Parallel()

	store := &fakeTranscriptStore{}
	joinedAt := time.Date(2026, 4, 20, 20, 0, 0, 0, time.UTC)
	uiModel := newModel(modelOptions{
		mode:          "join",
		listeningAddr: "203.0.113.10:7331",
		session:       &fakeSession{peerName: "host"},
		transcriptOpener: func(string) (transcriptStore, error) {
			return store, nil
		},
		identityLoader: func() (identity.Store, error) {
			return identity.Store{IdentityID: "identity-local", Path: "/tmp/identity.json"}, nil
		},
		roomAuthLoader: func(roomKey, identityID string) (historymeta.Record, error) {
			return historymeta.Record{RoomKey: roomKey, IdentityID: identityID, JoinedAt: joinedAt}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: &fakeSession{peerName: "host"}})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "sync-chunk-revoke-1",
			From: "host",
			Body: room.HistorySyncChunkBody(room.HistorySyncChunk{
				Version:        1,
				SourceIdentity: "identity-host",
				TargetIdentity: "identity-local",
				RoomKey:        transcript.JoinRoomKey("203.0.113.10:7331"),
				Records: []transcript.Record{
					{
						MessageID:      "replayed-revoked-1",
						Direction:      transcript.DirectionIncoming,
						From:           "bob",
						AuthorIdentity: "identity-bob",
						Body:           "replayed history",
						At:             joinedAt.Add(time.Minute),
						Status:         transcript.StatusSent,
					},
				},
				Revokes: []transcript.RevokeRecord{
					{
						TargetMessageID:  "replayed-revoked-1",
						OperatorIdentity: "identity-bob",
						At:               joinedAt.Add(2 * time.Minute),
					},
				},
			}),
			At: time.Date(2026, 4, 20, 21, 0, 0, 0, time.UTC),
		},
	})
	uiModel = updated.(model)

	if len(store.appends) != 1 || store.appends[0].AuthorIdentity != "identity-bob" {
		t.Fatalf("expected replayed message with author identity to persist, got %#v", store.appends)
	}
	if len(store.revokes) != 1 || store.revokes[0].TargetMessageID != "replayed-revoked-1" {
		t.Fatalf("expected replayed revoke to persist, got %#v", store.revokes)
	}
	if !strings.Contains(stripANSI(uiModel.View()), "已撤回一条消息") {
		t.Fatalf("expected revoked replay to render as recalled, got %q", stripANSI(uiModel.View()))
	}
}

func TestModelIgnoresIncomingRevokeWithMismatchedIdentity(t *testing.T) {
	t.Parallel()

	store := &fakeTranscriptStore{
		loaded: []transcript.Record{
			{
				MessageID:      "msg-1",
				Direction:      transcript.DirectionIncoming,
				From:           "bob",
				AuthorIdentity: "identity-bob",
				Body:           "still here",
				At:             time.Date(2026, 4, 20, 20, 1, 0, 0, time.UTC),
				Status:         transcript.StatusSent,
			},
		},
	}
	uiModel := newModel(modelOptions{
		mode:          "join",
		listeningAddr: "203.0.113.10:7331",
		session:       &fakeSession{peerName: "host"},
		transcriptOpener: func(string) (transcriptStore, error) {
			return store, nil
		},
		identityLoader: func() (identity.Store, error) {
			return identity.Store{IdentityID: "identity-local", Path: "/tmp/identity.json"}, nil
		},
		roomAuthLoader: func(roomKey, identityID string) (historymeta.Record, error) {
			return historymeta.Record{RoomKey: roomKey, IdentityID: identityID, JoinedAt: time.Date(2026, 4, 20, 20, 0, 0, 0, time.UTC)}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: &fakeSession{peerName: "host"}})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "revoke-mismatch",
			From: "bob",
			Body: room.RevokeBody(room.Revoke{
				Version:          1,
				RoomKey:          transcript.JoinRoomKey("203.0.113.10:7331"),
				OperatorIdentity: "identity-evil",
				TargetMessageID:  "msg-1",
				At:               time.Date(2026, 4, 20, 20, 2, 0, 0, time.UTC),
			}),
			At: time.Date(2026, 4, 20, 20, 2, 0, 0, time.UTC),
		},
	})
	uiModel = updated.(model)

	if len(store.revokes) != 0 {
		t.Fatalf("expected mismatched revoke not to persist, got %#v", store.revokes)
	}
	view := stripANSI(uiModel.View())
	if !strings.Contains(view, "still here") || strings.Contains(view, "已撤回一条消息") {
		t.Fatalf("expected mismatched revoke to be ignored, got %q", view)
	}
}

func TestModelCtrlRRevokesLatestOwnMessage(t *testing.T) {
	t.Parallel()

	store := &fakeTranscriptStore{}
	fake := &fakeSession{peerName: "host", localName: "alice"}
	joinedAt := time.Date(2026, 4, 20, 20, 0, 0, 0, time.UTC)
	uiModel := newModel(modelOptions{
		mode:          "join",
		uiMode:        uiModeTUI,
		listeningAddr: "203.0.113.10:7331",
		session:       fake,
		transcriptOpener: func(string) (transcriptStore, error) {
			return store, nil
		},
		identityLoader: func() (identity.Store, error) {
			return identity.Store{IdentityID: "identity-local", Path: "/tmp/identity.json"}, nil
		},
		roomAuthLoader: func(roomKey, identityID string) (historymeta.Record, error) {
			return historymeta.Record{RoomKey: roomKey, IdentityID: identityID, JoinedAt: joinedAt}, nil
		},
	})

	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(sessionReadyMsg{session: fake})
	uiModel = updated.(model)
	uiModel.addHistoryEntry(historyEntry{
		kind:           historyKindMessage,
		messageID:      "msg-1",
		from:           "alice",
		body:           "older own",
		at:             joinedAt.Add(time.Minute),
		outgoing:       true,
		status:         transcript.StatusSent,
		authorIdentity: "identity-local",
	})
	uiModel.addHistoryEntry(historyEntry{
		kind:           historyKindMessage,
		messageID:      "msg-2",
		from:           "alice",
		body:           "latest own",
		at:             joinedAt.Add(2 * time.Minute),
		outgoing:       true,
		status:         transcript.StatusSent,
		authorIdentity: "identity-local",
	})
	uiModel.reindexHistoryMessageIDs()

	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	uiModel = updated.(model)

	last := fake.sent[len(fake.sent)-1]
	revoke, ok := room.ParseRevoke(last.Body)
	if !ok {
		t.Fatalf("expected last sent payload to be revoke control, got %#v", last)
	}
	if revoke.TargetMessageID != "msg-2" || revoke.OperatorIdentity != "identity-local" {
		t.Fatalf("expected latest own message to be revoked, got %#v", revoke)
	}
	if len(store.revokes) != 1 || store.revokes[0].TargetMessageID != "msg-2" {
		t.Fatalf("expected local revoke to persist, got %#v", store.revokes)
	}
	if !strings.Contains(stripANSI(uiModel.View()), "已撤回一条消息") {
		t.Fatalf("expected revoked message in view, got %q", stripANSI(uiModel.View()))
	}
}

func TestModelReplaysHistorySyncChunkInChronologicalOrder(t *testing.T) {
	t.Parallel()

	store := &fakeTranscriptStore{}
	joinedAt := time.Date(2026, 4, 20, 20, 0, 0, 0, time.UTC)
	uiModel := newModel(modelOptions{
		mode:          "join",
		listeningAddr: "203.0.113.10:7331",
		session:       &fakeSession{peerName: "host"},
		transcriptOpener: func(string) (transcriptStore, error) {
			return store, nil
		},
		identityLoader: func() (identity.Store, error) {
			return identity.Store{IdentityID: "identity-local", Path: "/tmp/identity.json"}, nil
		},
		roomAuthLoader: func(roomKey, identityID string) (historymeta.Record, error) {
			return historymeta.Record{RoomKey: roomKey, IdentityID: identityID, JoinedAt: joinedAt}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: &fakeSession{peerName: "host"}})
	uiModel = updated.(model)
	uiModel.addMessageEntry(session.Message{
		ID:   "live-2",
		From: "c",
		Body: "latest live",
		At:   joinedAt.Add(20 * time.Minute),
	}, false, transcript.StatusSent, true)

	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "sync-chunk-ordered",
			From: "host",
			Body: room.HistorySyncChunkBody(room.HistorySyncChunk{
				Version:        1,
				SourceIdentity: "identity-host",
				TargetIdentity: "identity-local",
				RoomKey:        transcript.JoinRoomKey("203.0.113.10:7331"),
				Records: []transcript.Record{
					{
						MessageID: "offline-1",
						Direction: transcript.DirectionIncoming,
						From:      "b",
						Body:      "offline earlier",
						At:        joinedAt.Add(10 * time.Minute),
						Status:    transcript.StatusSent,
					},
				},
			}),
			At: joinedAt.Add(21 * time.Minute),
		},
	})
	uiModel = updated.(model)

	view := stripANSI(uiModel.View())
	offlineIndex := strings.Index(view, "offline earlier")
	liveIndex := strings.Index(view, "latest live")
	if offlineIndex == -1 || liveIndex == -1 {
		t.Fatalf("expected both offline and live messages in view, got %q", view)
	}
	if offlineIndex > liveIndex {
		t.Fatalf("expected offline synced message to be inserted before newer live message, got %q", view)
	}
}

func TestModelDeduplicatesEquivalentHistorySyncMessageWithDifferentID(t *testing.T) {
	t.Parallel()

	joinedAt := time.Date(2026, 4, 20, 20, 0, 0, 0, time.UTC)
	messageAt := joinedAt.Add(5 * time.Minute)
	store := &fakeTranscriptStore{
		loaded: []transcript.Record{
			{
				MessageID: "local-1",
				Direction: transcript.DirectionIncoming,
				From:      "bob",
				Body:      "same logical message",
				At:        messageAt,
				Status:    transcript.StatusSent,
			},
		},
	}

	uiModel := newModel(modelOptions{
		mode:          "join",
		listeningAddr: "203.0.113.10:7331",
		session:       &fakeSession{peerName: "host"},
		transcriptOpener: func(string) (transcriptStore, error) {
			return store, nil
		},
		identityLoader: func() (identity.Store, error) {
			return identity.Store{IdentityID: "identity-local", Path: "/tmp/identity.json"}, nil
		},
		roomAuthLoader: func(roomKey, identityID string) (historymeta.Record, error) {
			return historymeta.Record{RoomKey: roomKey, IdentityID: identityID, JoinedAt: joinedAt}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: &fakeSession{peerName: "host"}})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "sync-chunk-dedupe",
			From: "host",
			Body: room.HistorySyncChunkBody(room.HistorySyncChunk{
				Version:        1,
				SourceIdentity: "identity-host",
				TargetIdentity: "identity-local",
				RoomKey:        transcript.JoinRoomKey("203.0.113.10:7331"),
				Records: []transcript.Record{
					{
						MessageID: "remote-copy-2",
						Direction: transcript.DirectionIncoming,
						From:      "bob",
						Body:      "same logical message",
						At:        messageAt,
						Status:    transcript.StatusSent,
					},
				},
			}),
			At: joinedAt.Add(10 * time.Minute),
		},
	})
	uiModel = updated.(model)

	view := stripANSI(uiModel.View())
	if strings.Count(view, "same logical message") != 1 {
		t.Fatalf("expected equivalent synced message to be deduplicated, got %q", view)
	}
	if len(store.appends) != 0 {
		t.Fatalf("expected duplicate synced message not to persist, got %#v", store.appends)
	}
	if strings.Contains(view, "history synced: 1 messages") {
		t.Fatalf("expected duplicate replay not to show synced count, got %q", view)
	}
	if _, ok := uiModel.seenMessages["remote-copy-2"]; !ok {
		t.Fatal("expected duplicate synced message ID to be remembered as seen")
	}
}

func TestModelDeduplicatesEquivalentHistorySyncMessageWithSmallTimestampDrift(t *testing.T) {
	t.Parallel()

	joinedAt := time.Date(2026, 4, 20, 20, 0, 0, 0, time.UTC)
	messageAt := joinedAt.Add(5 * time.Minute)
	store := &fakeTranscriptStore{
		loaded: []transcript.Record{
			{
				MessageID: "local-1",
				Direction: transcript.DirectionIncoming,
				From:      "bob",
				Body:      "same logical message",
				At:        messageAt,
				Status:    transcript.StatusSent,
			},
		},
	}

	uiModel := newModel(modelOptions{
		mode:          "join",
		listeningAddr: "203.0.113.10:7331",
		session:       &fakeSession{peerName: "host"},
		transcriptOpener: func(string) (transcriptStore, error) {
			return store, nil
		},
		identityLoader: func() (identity.Store, error) {
			return identity.Store{IdentityID: "identity-local", Path: "/tmp/identity.json"}, nil
		},
		roomAuthLoader: func(roomKey, identityID string) (historymeta.Record, error) {
			return historymeta.Record{RoomKey: roomKey, IdentityID: identityID, JoinedAt: joinedAt}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: &fakeSession{peerName: "host"}})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "sync-chunk-dedupe-drift",
			From: "host",
			Body: room.HistorySyncChunkBody(room.HistorySyncChunk{
				Version:        1,
				SourceIdentity: "identity-host",
				TargetIdentity: "identity-local",
				RoomKey:        transcript.JoinRoomKey("203.0.113.10:7331"),
				Records: []transcript.Record{
					{
						MessageID: "remote-copy-drift",
						Direction: transcript.DirectionIncoming,
						From:      "bob",
						Body:      "same logical message",
						At:        messageAt.Add(2 * time.Second),
						Status:    transcript.StatusSent,
					},
				},
			}),
			At: joinedAt.Add(10 * time.Minute),
		},
	})
	uiModel = updated.(model)

	view := stripANSI(uiModel.View())
	if strings.Count(view, "same logical message") != 1 {
		t.Fatalf("expected drifted synced message to be deduplicated, got %q", view)
	}
	if len(store.appends) != 0 {
		t.Fatalf("expected drifted duplicate synced message not to persist, got %#v", store.appends)
	}
	if _, ok := uiModel.seenMessages["remote-copy-drift"]; !ok {
		t.Fatal("expected drifted duplicate synced message ID to be remembered as seen")
	}
}

func TestModelDeduplicatesEquivalentHistorySyncMessageAcrossDirectionMismatch(t *testing.T) {
	t.Parallel()

	joinedAt := time.Date(2026, 4, 20, 20, 0, 0, 0, time.UTC)
	messageAt := joinedAt.Add(5 * time.Minute)
	store := &fakeTranscriptStore{
		loaded: []transcript.Record{
			{
				MessageID:      "local-outgoing-1",
				Direction:      transcript.DirectionOutgoing,
				From:           "alice",
				AuthorIdentity: "identity-local",
				Body:           "same logical message",
				At:             messageAt,
				Status:         transcript.StatusSent,
			},
		},
	}

	uiModel := newModel(modelOptions{
		mode:          "join",
		listeningAddr: "203.0.113.10:7331",
		session:       &fakeSession{peerName: "host"},
		transcriptOpener: func(string) (transcriptStore, error) {
			return store, nil
		},
		identityLoader: func() (identity.Store, error) {
			return identity.Store{IdentityID: "identity-local", Path: "/tmp/identity.json"}, nil
		},
		roomAuthLoader: func(roomKey, identityID string) (historymeta.Record, error) {
			return historymeta.Record{RoomKey: roomKey, IdentityID: identityID, JoinedAt: joinedAt}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: &fakeSession{peerName: "host"}})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "sync-chunk-dedupe-direction",
			From: "host",
			Body: room.HistorySyncChunkBody(room.HistorySyncChunk{
				Version:        1,
				SourceIdentity: "identity-host",
				TargetIdentity: "identity-local",
				RoomKey:        transcript.JoinRoomKey("203.0.113.10:7331"),
				Records: []transcript.Record{
					{
						MessageID:      "remote-copy-direction",
						Direction:      transcript.DirectionIncoming,
						From:           "alice",
						AuthorIdentity: "identity-local",
						Body:           "same logical message",
						At:             messageAt,
						Status:         transcript.StatusSent,
					},
				},
			}),
			At: joinedAt.Add(10 * time.Minute),
		},
	})
	uiModel = updated.(model)

	view := stripANSI(uiModel.View())
	if strings.Count(view, "same logical message") != 1 {
		t.Fatalf("expected direction-mismatched synced message to be deduplicated, got %q", view)
	}
	if len(store.appends) != 0 {
		t.Fatalf("expected direction-mismatched duplicate not to persist, got %#v", store.appends)
	}
	if _, ok := uiModel.seenMessages["remote-copy-direction"]; !ok {
		t.Fatal("expected direction-mismatched duplicate ID to be remembered as seen")
	}
}

func TestModelLoadsHistoryAcrossDisplayNameChangesForSameRoom(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	psk := bytes.Repeat([]byte{0x52}, 32)
	openStore := func(localName string) func(string) (transcriptStore, error) {
		return func(conversationKey string) (transcriptStore, error) {
			return transcript.OpenStore(baseDir, localName, conversationKey, psk)
		}
	}
	authLoader := func(roomKey, identityID string) (historymeta.Record, error) {
		return historymeta.Record{
			RoomKey:    roomKey,
			IdentityID: identityID,
			JoinedAt:   time.Date(2026, 4, 20, 20, 0, 0, 0, time.UTC),
		}, nil
	}
	identityLoader := func() (identity.Store, error) {
		return identity.Store{IdentityID: "identity-local", Path: "/tmp/identity.json"}, nil
	}

	firstSession := &fakeSession{peerName: "host", localName: "b"}
	firstModel := newModel(modelOptions{
		mode:             "join",
		listeningAddr:    "203.0.113.10:7331",
		session:          firstSession,
		transcriptOpener: openStore("b"),
		identityLoader:   identityLoader,
		roomAuthLoader:   authLoader,
	})
	updated, _ := firstModel.Update(sessionReadyMsg{session: firstSession})
	firstModel = updated.(model)
	firstModel.input.SetValue("message while named b")
	updated, _ = firstModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	firstModel = updated.(model)

	secondSession := &fakeSession{peerName: "host", localName: "a"}
	secondModel := newModel(modelOptions{
		mode:             "join",
		listeningAddr:    "203.0.113.10:7331",
		session:          secondSession,
		transcriptOpener: openStore("a"),
		identityLoader:   identityLoader,
		roomAuthLoader:   authLoader,
	})
	updated, _ = secondModel.Update(sessionReadyMsg{session: secondSession})
	secondModel = updated.(model)

	if !strings.Contains(stripANSI(secondModel.View()), "message while named b") {
		t.Fatalf("expected transcript history to survive display name change, got %q", stripANSI(secondModel.View()))
	}
}

func TestModelShowsSlashCommandSuggestions(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:    "join",
		session: &fakeSession{peerName: "host"},
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})

	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	uiModel = updated.(model)

	uiModel.input.SetValue("/")
	view := stripANSI(uiModel.View())
	if !strings.Contains(view, "commands") {
		t.Fatalf("expected suggestions panel title, got %q", view)
	}
	if !strings.Contains(view, "/help -- 显示支持的命令") {
		t.Fatalf("expected /help suggestion, got %q", view)
	}
	if !strings.Contains(view, "/status -- 查询在线成员信息") {
		t.Fatalf("expected /status suggestion, got %q", view)
	}
	if !strings.Contains(view, "/events -- 查看成员进出记录") {
		t.Fatalf("expected /events suggestion, got %q", view)
	}
	if !strings.Contains(view, "/quit -- 退出当前会话") {
		t.Fatalf("expected /quit suggestion, got %q", view)
	}

	uiModel.input.SetValue("/st")
	view = stripANSI(uiModel.View())
	if !strings.Contains(view, "/status -- 查询在线成员信息") {
		t.Fatalf("expected filtered /status suggestion, got %q", view)
	}
	if strings.Contains(view, "/help -- 显示支持的命令") {
		t.Fatalf("expected /help to be filtered out, got %q", view)
	}
	if strings.Contains(view, "/quit -- 退出当前会话") {
		t.Fatalf("expected /quit to be filtered out, got %q", view)
	}

	uiModel.input.SetValue("hello")
	view = stripANSI(uiModel.View())
	if strings.Contains(view, "/status -- 查询在线成员信息") {
		t.Fatalf("expected suggestions to hide for normal text, got %q", view)
	}
}

func TestInputAreaShowsSendHint(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:    "join",
		session: &fakeSession{peerName: "host"},
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 80, Height: 16})
	uiModel = updated.(model)

	if view := stripANSI(uiModel.View()); !strings.Contains(view, "Enter send") {
		t.Fatalf("expected input hint in view, got %q", view)
	}
}

func TestModelRendersSlashCommandSuggestionsAboveInput(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:    "join",
		session: &fakeSession{peerName: "host"},
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})

	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	uiModel = updated.(model)
	uiModel.addHistoryEntry(historyEntry{
		kind: historyKindMessage,
		from: "host",
		body: "older message",
		at:   time.Date(2026, 4, 20, 18, 0, 0, 0, time.Local),
	})
	uiModel.input.SetValue("/")

	view := stripANSI(uiModel.View())
	messageIndex := strings.Index(view, "host: older message")
	suggestionIndex := strings.Index(view, "/help -- 显示支持的命令")
	inputIndex := strings.LastIndex(view, "/")
	if messageIndex == -1 || suggestionIndex == -1 || inputIndex == -1 {
		t.Fatalf("expected message, suggestion, and input in view, got %q", view)
	}
	if !(messageIndex < suggestionIndex && suggestionIndex < inputIndex) {
		t.Fatalf("expected suggestions between history and input, got %q", view)
	}
}

func TestScrollbackModeHidesSlashCommandSuggestions(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:    "join",
		uiMode:  uiModeScrollback,
		session: &fakeSession{peerName: "host"},
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})

	uiModel.input.SetValue("/")
	view := stripANSI(uiModel.View())
	if strings.Contains(view, "/help -- 显示支持的命令") {
		t.Fatalf("expected scrollback mode to hide command suggestions, got %q", view)
	}
	if strings.Contains(view, "/status -- 查询在线成员信息") {
		t.Fatalf("expected scrollback mode to hide command suggestions, got %q", view)
	}
	if strings.Contains(view, "/quit -- 退出当前会话") {
		t.Fatalf("expected scrollback mode to hide command suggestions, got %q", view)
	}
}

func TestHostModelRendersJoinAndLeaveSystemEvents(t *testing.T) {
	t.Parallel()

	hostRoom := &fakeHostRoom{fakeSession: fakeSession{peerName: "room"}, peerCount: 0}
	uiModel := newModel(modelOptions{
		mode:          "host",
		listeningAddr: "0.0.0.0:7331",
		session:       hostRoom,
		roomEvents:    hostRoom.Events(),
		peerCount:     hostRoom.PeerCount,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})

	updated, _ := uiModel.Update(roomEventMsg{event: room.Event{
		Kind:      room.EventPeerJoined,
		PeerName:  "aaa",
		PeerCount: 1,
	}})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(roomEventMsg{event: room.Event{
		Kind:      room.EventPeerLeft,
		PeerName:  "aaa",
		PeerCount: 0,
	}})
	uiModel = updated.(model)

	view := uiModel.View()
	if !strings.Contains(view, "aaa joined") {
		t.Fatalf("expected joined system message, got %q", view)
	}
	if !strings.Contains(view, "aaa left") {
		t.Fatalf("expected left system message, got %q", view)
	}
}

func TestHostModelShowsRelayedMessagesFromOriginalSender(t *testing.T) {
	t.Parallel()

	hostRoom := &fakeHostRoom{fakeSession: fakeSession{peerName: "room"}, peerCount: 1}
	uiModel := newModel(modelOptions{
		mode:          "host",
		listeningAddr: "0.0.0.0:7331",
		session:       hostRoom,
		roomEvents:    hostRoom.Events(),
		peerCount:     hostRoom.PeerCount,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})

	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "room-1",
			From: "aaa",
			Body: "hello group",
			At:   time.Date(2026, 4, 15, 20, 0, 0, 0, time.Local),
		},
	})
	uiModel = updated.(model)

	view := uiModel.View()
	if !strings.Contains(view, "aaa: hello group") {
		t.Fatalf("expected relayed message to preserve sender label, got %q", view)
	}
	if strings.Contains(view, "you: hello group") {
		t.Fatalf("expected relayed message not to be rendered as local send, got %q", view)
	}
}

func TestModelMouseWheelScrollsHistory(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode: "join",
		session: &fakeSession{
			peerName: "host",
		},
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})

	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 72, Height: 8})
	uiModel = updated.(model)

	for i := 0; i < 30; i++ {
		updated, _ = uiModel.Update(incomingMessageMsg{
			message: session.Message{
				ID:   fmt.Sprintf("msg-wheel-%02d", i),
				From: "host",
				Body: fmt.Sprintf("wheel-%02d", i),
				At:   time.Date(2026, 4, 14, 22, 10, i, 0, time.Local),
			},
		})
		uiModel = updated.(model)
	}

	if uiModel.viewport.YOffset == 0 {
		t.Fatal("expected viewport to start at bottom offset for long history")
	}

	updated, _ = uiModel.Update(tea.MouseMsg{
		Type:   tea.MouseWheelUp,
		Button: tea.MouseButtonWheelUp,
		Action: tea.MouseActionPress,
	})
	uiModel = updated.(model)

	if strings.Contains(uiModel.View(), "wheel-29") {
		t.Fatalf("expected wheel up to move away from bottom, got %q", uiModel.View())
	}

	updated, _ = uiModel.Update(tea.MouseMsg{
		Type:   tea.MouseWheelDown,
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyEnd})
	uiModel = updated.(model)

	if !strings.Contains(uiModel.View(), "wheel-29") {
		t.Fatalf("expected wheel down/end flow to allow returning to latest history, got %q", uiModel.View())
	}
}

func TestModelMouseDragScrollsHistory(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode: "join",
		session: &fakeSession{
			peerName: "host",
		},
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})

	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 72, Height: 8})
	uiModel = updated.(model)

	for i := 0; i < 30; i++ {
		updated, _ = uiModel.Update(incomingMessageMsg{
			message: session.Message{
				ID:   fmt.Sprintf("msg-drag-%02d", i),
				From: "host",
				Body: fmt.Sprintf("drag-%02d", i),
				At:   time.Date(2026, 4, 14, 22, 20, i, 0, time.Local),
			},
		})
		uiModel = updated.(model)
	}

	viewportY := 3
	updated, _ = uiModel.Update(tea.MouseMsg{
		X:      2,
		Y:      viewportY,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(tea.MouseMsg{
		X:      2,
		Y:      viewportY + 2,
		Button: tea.MouseButtonNone,
		Action: tea.MouseActionMotion,
	})
	uiModel = updated.(model)

	if strings.Contains(uiModel.View(), "drag-29") {
		t.Fatalf("expected drag scroll to move away from latest history, got %q", uiModel.View())
	}

	updated, _ = uiModel.Update(tea.MouseMsg{
		X:      2,
		Y:      viewportY + 2,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionRelease,
	})
	uiModel = updated.(model)
}

func TestModelLoadsTranscriptEntriesOnConnect(t *testing.T) {
	t.Parallel()

	store := &fakeTranscriptStore{
		loaded: []transcript.Record{
			{
				MessageID: "old-1",
				Direction: transcript.DirectionIncoming,
				From:      "joiner",
				Body:      "from disk",
				At:        time.Date(2026, 4, 13, 10, 0, 0, 0, time.Local),
				Status:    transcript.StatusSent,
			},
		},
	}

	uiModel := newModel(modelOptions{
		mode:          "host",
		listeningAddr: "127.0.0.1:7331",
		transcriptOpener: func(string) (transcriptStore, error) {
			return store, nil
		},
	})
	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	uiModel = updated.(model)

	updated, _ = uiModel.Update(sessionReadyMsg{session: &fakeSession{peerName: "joiner"}})
	uiModel = updated.(model)

	if !strings.Contains(uiModel.View(), "from disk") {
		t.Fatalf("expected transcript history to load into view, got %q", uiModel.View())
	}
}

func TestHostTranscriptUsesRoomScopedKey(t *testing.T) {
	t.Parallel()

	var opened []string
	hostRoom := &fakeHostRoom{fakeSession: fakeSession{peerName: "room"}, peerCount: 0}
	uiModel := newModel(modelOptions{
		mode:          "host",
		listeningAddr: "0.0.0.0:7331",
		session:       hostRoom,
		roomEvents:    hostRoom.Events(),
		peerCount:     hostRoom.PeerCount,
		transcriptOpener: func(key string) (transcriptStore, error) {
			opened = append(opened, key)
			return &fakeTranscriptStore{}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: hostRoom})
	uiModel = updated.(model)

	want := transcript.HostRoomKey("0.0.0.0:7331")
	if len(opened) != 1 || opened[0] != want {
		t.Fatalf("expected host transcript opener to use %q, got %#v", want, opened)
	}
}

func TestJoinTranscriptUsesTargetScopedKey(t *testing.T) {
	t.Parallel()

	var opened []string
	uiModel := newModel(modelOptions{
		mode:          "join",
		listeningAddr: "203.0.113.10:7331",
		transcriptOpener: func(key string) (transcriptStore, error) {
			opened = append(opened, key)
			return &fakeTranscriptStore{}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: &fakeSession{peerName: "host"}})
	uiModel = updated.(model)

	want := transcript.JoinRoomKey("203.0.113.10:7331")
	if len(opened) != 1 || opened[0] != want {
		t.Fatalf("expected join transcript opener to use %q, got %#v", want, opened)
	}
}

func TestModelSessionReadyCreatesRoomAuthorization(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:          "join",
		listeningAddr: "203.0.113.10:7331",
		session:       &fakeSession{peerName: "host"},
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
		identityLoader: func() (identity.Store, error) {
			return identity.Store{IdentityID: "identity-1", Path: "/tmp/identity.json"}, nil
		},
		roomAuthLoader: func(roomKey, identityID string) (historymeta.Record, error) {
			return historymeta.Record{RoomKey: roomKey, IdentityID: identityID}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: &fakeSession{peerName: "host"}})
	uiModel = updated.(model)

	if uiModel.roomAuthorization.RoomKey != transcript.JoinRoomKey("203.0.113.10:7331") {
		t.Fatalf("expected room authorization to be loaded for join room, got %#v", uiModel.roomAuthorization)
	}
	if uiModel.roomAuthorization.IdentityID != "identity-1" {
		t.Fatalf("expected room authorization identity %q, got %#v", "identity-1", uiModel.roomAuthorization)
	}
}

func TestModelResendsPendingMessagesWhenSessionReconnects(t *testing.T) {
	t.Parallel()

	first := &fakeSession{peerName: "host"}
	second := &fakeSession{peerName: "host"}
	uiModel := newModel(modelOptions{
		mode:    "join",
		session: first,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})

	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	uiModel = updated.(model)
	uiModel.input.SetValue("reliable hello")
	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	uiModel = updated.(model)

	updated, _ = uiModel.Update(sessionClosedMsg{err: errors.New("network down")})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(sessionReadyMsg{session: second})
	uiModel = updated.(model)

	if len(second.resent) != 1 {
		t.Fatalf("expected one pending message to be resent, got %#v", second.resent)
	}
	if second.resent[0].Body != "reliable hello" {
		t.Fatalf("expected resent body %q, got %#v", "reliable hello", second.resent[0])
	}
}

func TestProgramUsesAltScreenOnlyForTUI(t *testing.T) {
	t.Parallel()

	if !uiModeUsesAltScreen(uiModeTUI) {
		t.Fatal("expected tui mode to use alt screen")
	}
	if uiModeUsesAltScreen(uiModeScrollback) {
		t.Fatal("expected scrollback mode to avoid alt screen")
	}
}

func TestRunUIUsesDedicatedScrollbackRunner(t *testing.T) {
	originalBubbleTeaRunner := bubbleTeaRunner
	originalScrollbackRunner := scrollbackRunner
	t.Cleanup(func() {
		bubbleTeaRunner = originalBubbleTeaRunner
		scrollbackRunner = originalScrollbackRunner
	})

	calledBubbleTea := false
	calledScrollback := false
	bubbleTeaRunner = func(model) error {
		calledBubbleTea = true
		return nil
	}
	scrollbackRunner = func(model) error {
		calledScrollback = true
		return nil
	}

	if err := runUI(model{uiMode: uiModeScrollback}); err != nil {
		t.Fatalf("runUI returned error: %v", err)
	}
	if calledBubbleTea {
		t.Fatal("expected scrollback mode to bypass bubble tea renderer")
	}
	if !calledScrollback {
		t.Fatal("expected scrollback mode to use dedicated scrollback runner")
	}
}

func TestPromptConsolePrintLineRedrawsTypedInput(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	console := newPromptConsole(&output)
	console.buffer = []rune("typing")
	console.cursor = len(console.buffer)
	console.printLine("system [2026-04-14 16:00:00]: connected")

	rendered := output.String()
	if !strings.Contains(rendered, "connected\r\n") {
		t.Fatalf("expected printed line in output, got %q", rendered)
	}
	if !strings.HasSuffix(rendered, "\r\x1b[2K> typing") {
		t.Fatalf("expected prompt redraw at end, got %q", rendered)
	}
}

func TestPromptConsoleEnterWithTextDoesNotEmitBlankLine(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	console := newPromptConsole(&output)
	console.buffer = []rune("draft")
	console.cursor = len(console.buffer)

	line, submitted, quit := console.handleRune('\r')
	if quit {
		t.Fatal("expected enter not to quit")
	}
	if !submitted {
		t.Fatal("expected enter to submit")
	}
	if line != "draft" {
		t.Fatalf("expected submitted line %q, got %q", "draft", line)
	}
	if strings.Contains(output.String(), "\r\n") {
		t.Fatalf("expected submitted text not to emit a blank line before printing the message, got %q", output.String())
	}
}

func TestPromptConsoleArrowKeysEditBuffer(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	console := newPromptConsole(&output)
	for _, r := range "helo" {
		_, _, _ = console.handleRune(r)
	}
	for _, r := range []rune{27, '[', 'D'} {
		_, _, _ = console.handleRune(r)
	}

	line, submitted, quit := console.handleRune('l')
	if submitted || quit {
		t.Fatalf("expected in-progress edit, got submitted=%v quit=%v", submitted, quit)
	}

	line, submitted, quit = console.handleRune('\r')
	if quit {
		t.Fatal("expected enter not to quit")
	}
	if !submitted {
		t.Fatal("expected enter to submit")
	}
	if line != "hello" {
		t.Fatalf("expected edited line %q, got %q", "hello", line)
	}
}

func TestPromptConsoleArrowKeysRecallSubmittedHistory(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	console := newPromptConsole(&output)
	for _, r := range "first" {
		_, _, _ = console.handleRune(r)
	}
	_, _, _ = console.handleRune('\r')
	for _, r := range "second" {
		_, _, _ = console.handleRune(r)
	}
	_, _, _ = console.handleRune('\r')

	for _, r := range []rune{27, '[', 'A'} {
		_, _, _ = console.handleRune(r)
	}
	if got := string(console.buffer); got != "second" {
		t.Fatalf("expected first up arrow to recall most recent line, got %q", got)
	}

	for _, r := range []rune{27, '[', 'A'} {
		_, _, _ = console.handleRune(r)
	}
	if got := string(console.buffer); got != "first" {
		t.Fatalf("expected second up arrow to recall earlier line, got %q", got)
	}

	for _, r := range []rune{27, '[', 'B'} {
		_, _, _ = console.handleRune(r)
	}
	if got := string(console.buffer); got != "second" {
		t.Fatalf("expected down arrow to move forward in history, got %q", got)
	}
}

func TestPromptConsoleUsesDisplayWidthForWideCharacters(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	console := newPromptConsole(&output)
	console.buffer = []rune("你好")
	console.cursor = 1
	console.printLine("system")

	if !strings.HasSuffix(output.String(), "\r\x1b[2K> 你好\x1b[2D") {
		t.Fatalf("expected prompt redraw to move back by display width, got %q", output.String())
	}
}

func TestScrollbackSessionReadyPrintsTranscriptAndNewMessages(t *testing.T) {
	t.Parallel()

	store := &fakeTranscriptStore{
		loaded: []transcript.Record{
			{
				MessageID: "old-1",
				Direction: transcript.DirectionIncoming,
				From:      "joiner",
				Body:      "from disk",
				At:        time.Date(2026, 4, 13, 10, 0, 0, 0, time.Local),
				Status:    transcript.StatusSent,
			},
		},
	}

	var printed []string
	uiModel := newModel(modelOptions{
		mode:          "host",
		uiMode:        uiModeScrollback,
		listeningAddr: "127.0.0.1:7331",
		transcriptOpener: func(string) (transcriptStore, error) {
			return store, nil
		},
		historyPrinter: func(lines []string) tea.Cmd {
			printed = append(printed, lines...)
			return nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: &fakeSession{peerName: "joiner"}})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "live-1",
			From: "joiner",
			Body: "live hello",
			At:   time.Date(2026, 4, 14, 22, 40, 0, 0, time.Local),
		},
	})
	uiModel = updated.(model)

	joined := stripANSI(strings.Join(printed, "\n"))
	if !strings.Contains(joined, "from disk") {
		t.Fatalf("expected transcript line to be printed, got %q", joined)
	}
	if !strings.Contains(joined, "live hello") {
		t.Fatalf("expected live line to be printed, got %q", joined)
	}
	if strings.Contains(joined, "connected to joiner") {
		t.Fatalf("expected auto connected status to stay out of scrollback history, got %q", joined)
	}
	if strings.Contains(joined, "commands: /help /status /quit") {
		t.Fatalf("expected auto command banner to stay out of scrollback history, got %q", joined)
	}
	if !strings.Contains(uiModel.View(), "terminal scrollback") {
		t.Fatalf("expected scrollback hint in view, got %q", uiModel.View())
	}
	if strings.Contains(uiModel.View(), "from disk") {
		t.Fatalf("expected scrollback view to avoid re-rendering history, got %q", uiModel.View())
	}
}

func TestScrollbackReconnectErrorsPrintToTerminalHistory(t *testing.T) {
	t.Parallel()

	var printed []string
	uiModel := newModel(modelOptions{
		mode:   "join",
		uiMode: uiModeScrollback,
		connect: func(context.Context) (sessionClient, error) {
			return nil, errors.New("still offline")
		},
		historyPrinter: func(lines []string) tea.Cmd {
			printed = append(printed, lines...)
			return nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{err: errors.New("dial tcp timeout")})
	uiModel = updated.(model)

	if !strings.Contains(strings.Join(printed, "\n"), "dial tcp timeout") {
		t.Fatalf("expected reconnect error to be printed, got %q", printed)
	}
	if !strings.Contains(uiModel.View(), "reconnecting") {
		t.Fatalf("expected reconnecting status in view, got %q", uiModel.View())
	}
}

func TestScrollbackOutgoingReceiptDoesNotPrintDeliveryStatuses(t *testing.T) {
	t.Parallel()

	fake := &fakeSession{peerName: "host", localName: "alice"}
	var printed []string
	uiModel := newModel(modelOptions{
		mode:    "join",
		uiMode:  uiModeScrollback,
		session: fake,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
		historyPrinter: func(lines []string) tea.Cmd {
			printed = append(printed, lines...)
			return nil
		},
	})

	uiModel.input.SetValue("oi")
	updated, _ := uiModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	uiModel = updated.(model)

	joined := stripANSI(strings.Join(printed, "\n"))
	if !strings.Contains(joined, "alice: oi") {
		t.Fatalf("expected outgoing message to be printed, got %q", joined)
	}
	if strings.Contains(joined, "commands: /help /status /quit") {
		t.Fatalf("expected auto command banner to stay out of scrollback history, got %q", joined)
	}
	if strings.Contains(joined, "[sending]") {
		t.Fatalf("expected scrollback to hide sending status, got %q", joined)
	}

	beforeReceipt := len(printed)
	updated, _ = uiModel.Update(receiptMsg{
		receipt: session.Receipt{MessageID: fake.sent[0].ID},
	})
	uiModel = updated.(model)

	if len(printed) != beforeReceipt {
		t.Fatalf("expected receipt to avoid printing duplicate delivery line, got %q", printed)
	}
	if strings.Contains(stripANSI(strings.Join(printed, "\n")), "[sent]") {
		t.Fatalf("expected scrollback to hide sent status, got %q", printed)
	}
}

func TestScrollbackIncomingRevokePrintsRecalledPlaceholder(t *testing.T) {
	t.Parallel()

	store := &fakeTranscriptStore{
		loaded: []transcript.Record{
			{
				MessageID:      "old-1",
				Direction:      transcript.DirectionIncoming,
				From:           "bob",
				AuthorIdentity: "identity-bob",
				Body:           "from disk",
				At:             time.Date(2026, 4, 13, 10, 0, 0, 0, time.Local),
				Status:         transcript.StatusSent,
			},
		},
	}

	var printed []string
	uiModel := newModel(modelOptions{
		mode:          "join",
		uiMode:        uiModeScrollback,
		listeningAddr: "127.0.0.1:7331",
		session:       &fakeSession{peerName: "host"},
		transcriptOpener: func(string) (transcriptStore, error) {
			return store, nil
		},
		historyPrinter: func(lines []string) tea.Cmd {
			printed = append(printed, lines...)
			return nil
		},
		identityLoader: func() (identity.Store, error) {
			return identity.Store{IdentityID: "identity-local", Path: "/tmp/identity.json"}, nil
		},
		roomAuthLoader: func(roomKey, identityID string) (historymeta.Record, error) {
			return historymeta.Record{
				RoomKey:    roomKey,
				IdentityID: identityID,
				JoinedAt:   time.Date(2026, 4, 13, 9, 0, 0, 0, time.Local),
			}, nil
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: &fakeSession{peerName: "host"}})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "revoke-1",
			From: "bob",
			Body: room.RevokeBody(room.Revoke{
				Version:          1,
				RoomKey:          transcript.JoinRoomKey("127.0.0.1:7331"),
				OperatorIdentity: "identity-bob",
				TargetMessageID:  "old-1",
				At:               time.Date(2026, 4, 13, 10, 5, 0, 0, time.Local),
			}),
			At: time.Date(2026, 4, 13, 10, 5, 0, 0, time.Local),
		},
	})
	uiModel = updated.(model)

	joined := stripANSI(strings.Join(printed, "\n"))
	if !strings.Contains(joined, "bob: from disk") {
		t.Fatalf("expected original printed message in scrollback, got %q", joined)
	}
	if !strings.Contains(joined, "bob: 已撤回一条消息") {
		t.Fatalf("expected revoke placeholder to be printed in scrollback, got %q", joined)
	}
}

func TestScrollbackEventsCommandSendsHiddenRequestAndRendersResponse(t *testing.T) {
	oldLocal := time.Local
	time.Local = time.FixedZone("CST", 8*60*60)
	t.Cleanup(func() {
		time.Local = oldLocal
	})

	fake := &fakeSession{peerName: "host", localName: "alice"}
	var printed []string
	uiModel := newModel(modelOptions{
		mode:    "join",
		uiMode:  uiModeScrollback,
		session: fake,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
		historyPrinter: func(lines []string) tea.Cmd {
			printed = append(printed, lines...)
			return nil
		},
	})

	if quit := handleScrollbackLine(&uiModel, "/events"); quit {
		t.Fatal("expected /events not to quit")
	}
	if len(fake.sent) != 1 || fake.sent[0].Body != room.EventsRequestBody() {
		t.Fatalf("expected hidden events request to be sent, got %#v", fake.sent)
	}

	updated, _ := uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "events-response-1",
			From: "host",
			Body: room.EventsResponseBody([]room.Event{
				{
					Kind:     room.EventPeerJoined,
					PeerName: "aaa",
					At:       time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC),
				},
			}),
			At: time.Date(2026, 4, 20, 18, 1, 0, 0, time.UTC),
		},
	})
	uiModel = updated.(model)

	joined := stripANSI(strings.Join(printed, "\n"))
	if !strings.Contains(joined, "events: aaa joined at 2026-04-21 02:00:00") {
		t.Fatalf("expected events response to print in scrollback, got %q", joined)
	}
	if strings.Contains(joined, "\x00chatbox:events:") {
		t.Fatalf("expected hidden events payload to stay out of scrollback, got %q", joined)
	}
	if strings.Contains(joined, "unknown command") {
		t.Fatalf("expected /events not to be treated as unknown command, got %q", joined)
	}
}

func TestScrollbackReconnectPrintsRetryMarkerForPendingMessage(t *testing.T) {
	t.Parallel()

	first := &fakeSession{peerName: "host", localName: "alice"}
	second := &fakeSession{peerName: "host", localName: "alice"}
	var printed []string
	uiModel := newModel(modelOptions{
		mode:    "join",
		uiMode:  uiModeScrollback,
		session: first,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
		historyPrinter: func(lines []string) tea.Cmd {
			printed = append(printed, lines...)
			return nil
		},
	})

	uiModel.input.SetValue("reliable hello")
	updated, _ := uiModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(sessionClosedMsg{err: errors.New("network down")})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(sessionReadyMsg{session: second})
	uiModel = updated.(model)

	joined := stripANSI(strings.Join(printed, "\n"))
	if !strings.Contains(joined, "alice: reliable hello [retrying]") {
		t.Fatalf("expected retry marker in scrollback output, got %q", joined)
	}
	if strings.Contains(joined, "[sending]") || strings.Contains(joined, "[sent]") {
		t.Fatalf("expected scrollback output to hide normal delivery states, got %q", joined)
	}
}

func TestScrollbackAlertsOnlyForLiveInboundMessages(t *testing.T) {
	t.Parallel()

	alerts := 0
	uiModel := newModel(modelOptions{
		mode:      "join",
		uiMode:    uiModeScrollback,
		alertMode: "bell",
		session:   &fakeSession{peerName: "host"},
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
		alertNotifier: func() {
			alerts++
		},
	})

	updated, _ := uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "live-alert-1",
			From: "host",
			Body: "ping",
			At:   time.Date(2026, 4, 15, 10, 0, 0, 0, time.Local),
		},
	})
	uiModel = updated.(model)

	if alerts != 1 {
		t.Fatalf("expected one alert for live inbound message, got %d", alerts)
	}
}

func TestTUIAlertsOnlyForLiveInboundMessages(t *testing.T) {
	t.Parallel()

	alerts := 0
	uiModel := newModel(modelOptions{
		mode:      "join",
		uiMode:    uiModeTUI,
		alertMode: "bell",
		session:   &fakeSession{peerName: "host"},
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
		alertNotifier: func() {
			alerts++
		},
	})

	updated, _ := uiModel.Update(incomingMessageMsg{
		message: session.Message{
			ID:   "live-alert-tui-1",
			From: "host",
			Body: "ping",
			At:   time.Date(2026, 4, 15, 10, 0, 0, 0, time.Local),
		},
	})
	uiModel = updated.(model)

	if alerts != 1 {
		t.Fatalf("expected one alert for live inbound TUI message, got %d", alerts)
	}
}

func TestScrollbackDoesNotAlertForTranscriptReplay(t *testing.T) {
	t.Parallel()

	alerts := 0
	store := &fakeTranscriptStore{
		loaded: []transcript.Record{
			{
				MessageID: "old-1",
				Direction: transcript.DirectionIncoming,
				From:      "joiner",
				Body:      "from disk",
				At:        time.Date(2026, 4, 13, 10, 0, 0, 0, time.Local),
				Status:    transcript.StatusSent,
			},
		},
	}

	uiModel := newModel(modelOptions{
		mode:      "host",
		uiMode:    uiModeScrollback,
		alertMode: "bell",
		transcriptOpener: func(string) (transcriptStore, error) {
			return store, nil
		},
		alertNotifier: func() {
			alerts++
		},
	})

	updated, _ := uiModel.Update(sessionReadyMsg{session: &fakeSession{peerName: "joiner"}})
	uiModel = updated.(model)

	if alerts != 0 {
		t.Fatalf("expected transcript replay not to alert, got %d", alerts)
	}
}

func TestScrollbackDoesNotAlertForOutgoingReceiptOrRetry(t *testing.T) {
	t.Parallel()

	first := &fakeSession{peerName: "host"}
	second := &fakeSession{peerName: "host"}
	alerts := 0
	uiModel := newModel(modelOptions{
		mode:      "join",
		uiMode:    uiModeScrollback,
		alertMode: "bell",
		session:   first,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
		alertNotifier: func() {
			alerts++
		},
	})

	uiModel.input.SetValue("hello")
	updated, _ := uiModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(receiptMsg{
		receipt: session.Receipt{MessageID: first.sent[0].ID},
	})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(sessionClosedMsg{err: errors.New("network down")})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(sessionReadyMsg{session: second})
	uiModel = updated.(model)

	if alerts != 0 {
		t.Fatalf("expected outgoing/receipt/retry flows not to alert, got %d", alerts)
	}
}

func TestRunUITUIInitializesBellAlertNotifier(t *testing.T) {
	originalBubbleTeaRunner := bubbleTeaRunner
	originalAlertFactory := defaultAlertNotifierFactory
	defer func() {
		bubbleTeaRunner = originalBubbleTeaRunner
		defaultAlertNotifierFactory = originalAlertFactory
	}()

	defaultAlertNotifierFactory = func() alertNotifierFunc {
		return func() {}
	}
	bubbleTeaRunner = func(m model) error {
		if m.alertNotifier == nil {
			t.Fatal("expected TUI bell mode to initialize an alert notifier")
		}
		return nil
	}

	err := runUI(newModel(modelOptions{
		mode:      "join",
		uiMode:    uiModeTUI,
		alertMode: "bell",
		session:   &fakeSession{peerName: "host"},
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	}))
	if err != nil {
		t.Fatalf("runUI returned error: %v", err)
	}
}

type fakeSession struct {
	peerName  string
	localName string
	sent      []session.Message
	resent    []session.Message
}

type fakeHostRoom struct {
	fakeSession
	events           chan room.Event
	peerCount        int
	peerNames        []string
	submittedUpdates []room.UpdateRequest
}

func (f *fakeSession) Messages() <-chan session.Message { return nil }
func (f *fakeSession) Receipts() <-chan session.Receipt { return nil }
func (f *fakeSession) Done() <-chan struct{}            { return nil }
func (f *fakeSession) Err() error                       { return nil }
func (f *fakeSession) Close() error                     { return nil }
func (f *fakeSession) PeerName() string                 { return f.peerName }

func (f *fakeSession) Send(text string) (session.Message, error) {
	from := f.localName
	if from == "" {
		from = f.peerName
	}
	message := session.Message{
		ID:   fmt.Sprintf("fake-%d", len(f.sent)+1),
		From: from,
		Body: text,
		At:   time.Date(2026, 4, 14, 20, 31, len(f.sent), 0, time.Local),
	}
	f.sent = append(f.sent, message)
	return message, nil
}

func (f *fakeSession) Resend(message session.Message) error {
	f.resent = append(f.resent, message)
	return nil
}

func (f *fakeHostRoom) Events() <-chan room.Event {
	if f.events == nil {
		f.events = make(chan room.Event, 8)
	}
	return f.events
}

func (f *fakeHostRoom) PeerCount() int {
	return f.peerCount
}

func (f *fakeHostRoom) PeerNames() []string {
	return append([]string(nil), f.peerNames...)
}

func (f *fakeHostRoom) SubmitUpdateRequest(request room.UpdateRequest) error {
	f.submittedUpdates = append(f.submittedUpdates, request)
	return nil
}

type fakeTranscriptStore struct {
	loaded  []transcript.Record
	appends []transcript.Record
	revokes []transcript.RevokeRecord
	updates []string
}

func (f *fakeTranscriptStore) Load() ([]transcript.Record, error) {
	return f.loaded, nil
}

func (f *fakeTranscriptStore) AppendMessage(record transcript.Record) error {
	f.appends = append(f.appends, record)
	return nil
}

func (f *fakeTranscriptStore) UpdateStatus(messageID, status string) error {
	f.updates = append(f.updates, messageID+":"+status)
	return nil
}

func (f *fakeTranscriptStore) AppendRevoke(revoke transcript.RevokeRecord) error {
	f.revokes = append(f.revokes, revoke)
	return nil
}
