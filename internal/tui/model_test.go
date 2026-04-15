package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"chatbox/internal/session"
	"chatbox/internal/transcript"
)

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

	view := uiModel.View()
	if !strings.Contains(view, "connected") {
		t.Fatalf("expected connected status in view, got %q", view)
	}
	if !strings.Contains(view, "hello") {
		t.Fatalf("expected incoming message in view, got %q", view)
	}
	if !strings.Contains(view, "2026-04-14 20:30:45") {
		t.Fatalf("expected formatted timestamp in view, got %q", view)
	}
}

func TestModelSendsTypedMessageOnEnter(t *testing.T) {
	t.Parallel()

	fake := &fakeSession{peerName: "host"}
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
	if !strings.Contains(view, "you: hello from cli") {
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
	if !strings.Contains(uiModel.View(), "[sent]") {
		t.Fatalf("expected local message to transition to sent state, got %q", uiModel.View())
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
		Button: tea.MouseButtonLeft,
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
	t.Parallel()

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

func TestPromptConsoleEnterUsesCRLF(t *testing.T) {
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
	if !strings.Contains(output.String(), "\r\n") {
		t.Fatalf("expected CRLF output, got %q", output.String())
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

	joined := strings.Join(printed, "\n")
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

	fake := &fakeSession{peerName: "host"}
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

	joined := strings.Join(printed, "\n")
	if !strings.Contains(joined, "you: oi") {
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
	if strings.Contains(strings.Join(printed, "\n"), "[sent]") {
		t.Fatalf("expected scrollback to hide sent status, got %q", printed)
	}
}

func TestScrollbackReconnectPrintsRetryMarkerForPendingMessage(t *testing.T) {
	t.Parallel()

	first := &fakeSession{peerName: "host"}
	second := &fakeSession{peerName: "host"}
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

	joined := strings.Join(printed, "\n")
	if !strings.Contains(joined, "reliable hello [retrying]") {
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

type fakeSession struct {
	peerName string
	sent     []session.Message
	resent   []session.Message
}

func (f *fakeSession) Messages() <-chan session.Message { return nil }
func (f *fakeSession) Receipts() <-chan session.Receipt { return nil }
func (f *fakeSession) Done() <-chan struct{}            { return nil }
func (f *fakeSession) Err() error                       { return nil }
func (f *fakeSession) Close() error                     { return nil }
func (f *fakeSession) PeerName() string                 { return f.peerName }

func (f *fakeSession) Send(text string) (session.Message, error) {
	message := session.Message{
		ID:   fmt.Sprintf("fake-%d", len(f.sent)+1),
		From: f.peerName,
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

type fakeTranscriptStore struct {
	loaded  []transcript.Record
	appends []transcript.Record
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
