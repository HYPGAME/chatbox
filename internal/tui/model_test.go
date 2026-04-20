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
	if !strings.Contains(view, "2026-04-14 20:30:45") {
		t.Fatalf("expected formatted timestamp in view, got %q", view)
	}
}

func TestModelSessionReadyCreatesLocalIdentity(t *testing.T) {
	t.Parallel()

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
	if got := stripANSI(uiModel.View()); !strings.Contains(got, "[sent]") {
		t.Fatalf("expected local message to transition to sent state, got %q", got)
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

	if len(fake.sent) == 0 {
		t.Fatal("expected sync hello to be sent after session ready")
	}
	hello, ok := room.ParseHistorySyncHello(fake.sent[len(fake.sent)-1].Body)
	if !ok {
		t.Fatalf("expected last sent payload to be sync hello, got %#v", fake.sent[len(fake.sent)-1])
	}
	if hello.IdentityID != "identity-local" {
		t.Fatalf("expected sync hello identity %q, got %#v", "identity-local", hello)
	}
	if hello.RoomKey != transcript.JoinRoomKey("203.0.113.10:7331") {
		t.Fatalf("expected sync hello room key %q, got %#v", transcript.JoinRoomKey("203.0.113.10:7331"), hello)
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
	if !strings.Contains(view, "/help -- 显示支持的命令") {
		t.Fatalf("expected /help suggestion, got %q", view)
	}
	if !strings.Contains(view, "/status -- 查询在线成员信息") {
		t.Fatalf("expected /status suggestion, got %q", view)
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
	peerName  string
	localName string
	sent      []session.Message
	resent    []session.Message
}

type fakeHostRoom struct {
	fakeSession
	events    chan room.Event
	peerCount int
	peerNames []string
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
