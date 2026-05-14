# TUI Visual Upgrade Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upgrade the existing `--ui tui` rendering so the chat view has a clearer top bar, cleaner message hierarchy, attachment cards, and a separated bottom status/input area.

**Architecture:** Keep the current Bubble Tea model and history entry data model. Make incremental rendering changes in `internal/tui/model.go`, with focused regression coverage in `internal/tui/model_test.go`; do not change protocol, transcript, scrollback, attachment storage, or command behavior.

**Tech Stack:** Go, Bubble Tea, Lip Gloss, existing `internal/tui` tests.

---

## File Structure

- Modify: `internal/tui/model.go`
  - Add a top bar renderer and compact top bar field selection.
  - Add TUI entry render context for compact consecutive messages.
  - Update TUI system/date/attachment rendering.
  - Add bottom status notice rendering and resize accounting.
- Modify: `internal/tui/model_test.go`
  - Add focused tests for top bar, narrow top bar, compact consecutive messages, system/date layout, attachment cards, and bottom status notice placement.
  - Update existing assertions that intentionally changed from old compact rendering.
- No docs update is required for this task because user-visible command behavior is unchanged.

## Task 1: Top Bar

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/model_test.go`

- [ ] **Step 1: Write failing top bar tests**

Add these tests near the existing status bar tests in `internal/tui/model_test.go`:

```go
func TestRenderTopBarShowsRoomConnectionAndOnlineCount(t *testing.T) {
	t.Parallel()

	hostRoom := &fakeHostRoom{
		fakeSession: fakeSession{peerName: "room"},
		peerCount:   2,
		peerNames:   []string{"alice", "bob", "carol"},
	}
	uiModel := newModel(modelOptions{
		mode:          "host",
		uiMode:        uiModeTUI,
		listeningAddr: "0.0.0.0:7331",
		session:       hostRoom,
		roomEvents:    hostRoom.Events(),
		peerCount:     hostRoom.PeerCount,
		peerNames:     hostRoom.PeerNames,
		transcriptKey: "group:帝影剑域:abcd1234",
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	uiModel.status = "connected"
	uiModel.width = 100

	got := stripANSI(uiModel.renderTopBar())
	if got != "chatbox · 帝影剑域 · connected · 3 online" {
		t.Fatalf("unexpected top bar: %q", got)
	}
}

func TestRenderTopBarTruncatesLowPriorityFields(t *testing.T) {
	t.Parallel()

	hostRoom := &fakeHostRoom{
		fakeSession: fakeSession{peerName: "room"},
		peerCount:   2,
		peerNames:   []string{"alice", "bob", "carol"},
	}
	uiModel := newModel(modelOptions{
		mode:          "host",
		uiMode:        uiModeTUI,
		listeningAddr: "0.0.0.0:7331",
		session:       hostRoom,
		roomEvents:    hostRoom.Events(),
		peerCount:     hostRoom.PeerCount,
		peerNames:     hostRoom.PeerNames,
		transcriptKey: "group:帝影剑域:abcd1234",
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	uiModel.status = "connected"
	uiModel.width = 24

	got := stripANSI(uiModel.renderTopBar())
	if got != "chatbox · connected" {
		t.Fatalf("expected compact top bar, got %q", got)
	}
}
```

- [ ] **Step 2: Run top bar tests and verify they fail**

Run:

```bash
go test ./internal/tui -run 'TestRenderTopBarShowsRoomConnectionAndOnlineCount|TestRenderTopBarTruncatesLowPriorityFields' -count=1
```

Expected: FAIL with `uiModel.renderTopBar undefined`.

- [ ] **Step 3: Add top bar rendering helpers**

In `internal/tui/model.go`, add this style beside the other style vars:

```go
	topBarStyle          = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8AB4D6"))
```

Replace the TUI branch in `View()` so it starts with the top bar:

```go
	lines := []string{m.renderTopBar()}
	lines = append(lines, m.viewport.View())
```

Add these helpers near the existing `renderStatusBar` function:

```go
func (m model) renderTopBar() string {
	text := compactTopBarParts(
		"chatbox",
		m.topBarRoomText(),
		m.topBarStatusText(),
		m.topBarOnlineText(),
		m.width,
	)
	return topBarStyle.Render(text)
}

func (m model) topBarRoomText() string {
	if roomName := m.displayRoomName(); roomName != "" {
		return roomName
	}
	switch strings.TrimSpace(m.mode) {
	case "host":
		if addr := strings.TrimSpace(m.listeningAddr); addr != "" {
			return "host " + addr
		}
		return "host"
	case "join":
		if peer := strings.TrimSpace(m.currentPeer); peer != "" {
			return "join " + peer
		}
		return "join"
	default:
		return strings.TrimSpace(m.mode)
	}
}

func (m model) topBarStatusText() string {
	status := strings.TrimSpace(m.status)
	if status == "" {
		return "connecting"
	}
	return status
}

func (m model) topBarOnlineText() string {
	if m.peerNames != nil {
		if names := m.peerNames(); len(names) > 0 {
			return fmt.Sprintf("%d online", len(names))
		}
	}
	if m.mode == "host" && m.peerCountValue > 0 {
		return fmt.Sprintf("%d online", m.peerCountValue+1)
	}
	return ""
}

func compactTopBarParts(brand, room, status, online string, width int) string {
	candidates := [][]string{
		{brand, room, status, online},
		{brand, status, online},
		{brand, status},
		{brand},
	}
	for _, candidate := range candidates {
		text := joinNonEmptyParts(candidate, " · ")
		if width <= 0 || lipgloss.Width(text) <= width {
			return text
		}
	}
	return brand
}

func joinNonEmptyParts(parts []string, sep string) string {
	nonEmpty := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			nonEmpty = append(nonEmpty, part)
		}
	}
	return strings.Join(nonEmpty, sep)
}
```

Keep `renderStatusBar` in place for existing direct tests, but make it delegate to `renderTopBar` temporarily:

```go
func (m model) renderStatusBar() string {
	return m.renderTopBar()
}
```

- [ ] **Step 4: Run top bar tests and verify they pass**

Run:

```bash
go test ./internal/tui -run 'TestRenderTopBarShowsRoomConnectionAndOnlineCount|TestRenderTopBarTruncatesLowPriorityFields|TestRenderStatusBarShowsGroupRoomName|TestRenderStatusBarOmitsRoomNameForNonGroupSession' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit top bar change**

Run:

```bash
git add internal/tui/model.go internal/tui/model_test.go
git commit -m "feat: add polished tui top bar"
```

## Task 2: Message Viewport Hierarchy

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/model_test.go`

- [ ] **Step 1: Write failing viewport hierarchy tests**

Add these tests near `TestRefreshViewportAddsDateSeparators` in `internal/tui/model_test.go`:

```go
func TestRefreshViewportUsesPolishedDateSeparators(t *testing.T) {
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

	view := stripANSI(uiModel.View())
	if !strings.Contains(view, "──────── 2026-04-17 ────────") {
		t.Fatalf("expected polished date separator, got %q", view)
	}
}

func TestRenderTUISystemEntryUsesQuietCenteredText(t *testing.T) {
	t.Parallel()

	entry := historyEntry{
		kind: historyKindSystem,
		body: "bob joined",
		at:   time.Date(2026, 5, 14, 15, 4, 0, 0, time.Local),
	}

	if got := stripANSI(renderTUIEntry(entry, false)); got != "        bob joined" {
		t.Fatalf("expected quiet system line, got %q", got)
	}
}

func TestRefreshViewportCompactsConsecutiveMessagesFromSameSender(t *testing.T) {
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
		From: "alice",
		Body: "first",
		At:   time.Date(2026, 5, 14, 10, 0, 0, 0, time.Local),
	}, false, transcript.StatusSent, false)
	uiModel.addMessageEntry(session.Message{
		ID:   "m2",
		From: "alice",
		Body: "second",
		At:   time.Date(2026, 5, 14, 10, 1, 0, 0, time.Local),
	}, false, transcript.StatusSent, false)

	view := stripANSI(uiModel.View())
	if !strings.Contains(view, "[10:00] alice: first") {
		t.Fatalf("expected first message header, got %q", view)
	}
	if strings.Contains(view, "[10:01] alice: second") {
		t.Fatalf("expected second message to suppress repeated sender header, got %q", view)
	}
	if !strings.Contains(view, "       second") {
		t.Fatalf("expected compact continuation body, got %q", view)
	}
}
```

Update the existing `TestRefreshViewportAddsDateSeparators` assertions from:

```go
if !strings.Contains(view, "--- 2026-04-17 ---") {
	t.Fatalf("expected first date separator, got %q", view)
}
if !strings.Contains(view, "--- 2026-04-18 ---") {
	t.Fatalf("expected second date separator, got %q", view)
}
```

to:

```go
if !strings.Contains(view, "──────── 2026-04-17 ────────") {
	t.Fatalf("expected first date separator, got %q", view)
}
if !strings.Contains(view, "──────── 2026-04-18 ────────") {
	t.Fatalf("expected second date separator, got %q", view)
}
```

- [ ] **Step 2: Run viewport hierarchy tests and verify they fail**

Run:

```bash
go test ./internal/tui -run 'TestRefreshViewportUsesPolishedDateSeparators|TestRenderTUISystemEntryUsesQuietCenteredText|TestRefreshViewportCompactsConsecutiveMessagesFromSameSender|TestRefreshViewportAddsDateSeparators' -count=1
```

Expected: FAIL because the date separator still uses `---`, system messages still include `system [HH:MM]:`, and repeated senders still render full headers.

- [ ] **Step 3: Add render context and polished system/date rendering**

In `internal/tui/model.go`, add this type near the other rendered state types:

```go
type tuiEntryRenderContext struct {
	compactSender bool
}
```

Replace `renderDateSeparator` with:

```go
func renderDateSeparator(date string) string {
	return separatorStyle.Render(fmt.Sprintf("──────── %s ────────", date))
}
```

Replace `renderTUIEntryWithFeedback` with a delegating wrapper and a context-aware implementation:

```go
func renderTUIEntryWithFeedback(entry historyEntry, selected bool, feedback attachmentFeedbackState) string {
	return renderTUIEntryWithFeedbackAndContext(entry, selected, feedback, tuiEntryRenderContext{})
}

func renderTUIEntryWithFeedbackAndContext(entry historyEntry, selected bool, feedback attachmentFeedbackState, ctx tuiEntryRenderContext) string {
	timestamp := entry.at.Local().Format("15:04")
	switch entry.kind {
	case historyKindSystem:
		return systemLineStyle().Render("        " + entry.body)
	case historyKindError:
		return historyErrorStyle().Render(fmt.Sprintf("error [%s]: %s", timestamp, entry.body))
	default:
		statusSuffix := ""
		if entry.outgoing && entry.status != "" && entry.status != transcript.StatusSent && !entry.revoked {
			statusSuffix = fmt.Sprintf(" [%s]", entry.status)
		}
		timestampSegmentStyle, senderSegmentStyle, textSegmentStyle := attachmentFeedbackStyles(feedback, senderMessageStyle(entry.from))
		coloredLabel := senderSegmentStyle.Render(entry.from)
		coloredTimestamp := timestampSegmentStyle.Render(fmt.Sprintf("[%s]", timestamp))
		header := coloredTimestamp + textSegmentStyle.Render(" ") + coloredLabel
		body := textSegmentStyle.Render(renderedMessageBody(entry) + statusSuffix)
		line := header + textSegmentStyle.Render(": ") + body
		if ctx.compactSender {
			line = textSegmentStyle.Render("       " + renderedMessageBody(entry) + statusSuffix)
		}
		if !entry.revoked {
			if compact, ok := renderCompactReplyBody(entry.body, senderSegmentStyle, textSegmentStyle, statusSuffix); ok {
				if ctx.compactSender {
					line = compact
				} else {
					line = header + textSegmentStyle.Render(":") + "\n" + compact
				}
			}
		}
		if selected {
			return inputHintStyle.Render("> ") + line
		}
		return line
	}
}
```

- [ ] **Step 4: Use render context in viewport state**

In `buildRenderedViewportState`, initialize sender tracking before the loop:

```go
	lastMessageSender := ""
	lastMessageDate := ""
	lastEntryWasMessage := false
```

Replace the line that renders each entry:

```go
line := renderTUIEntryWithFeedback(entry, i == selectedIndex, feedback)
```

with:

```go
		compactSender := entry.kind == historyKindMessage &&
			lastEntryWasMessage &&
			entry.from == lastMessageSender &&
			entryDate == lastMessageDate
		line := renderTUIEntryWithFeedbackAndContext(entry, i == selectedIndex, feedback, tuiEntryRenderContext{
			compactSender: compactSender,
		})
		if entry.kind == historyKindMessage {
			lastEntryWasMessage = true
			lastMessageSender = entry.from
			lastMessageDate = entryDate
		} else {
			lastEntryWasMessage = false
			lastMessageSender = ""
			lastMessageDate = ""
		}
```

Keep the existing wrapping and attachment click mapping after this block unchanged.

- [ ] **Step 5: Run viewport hierarchy tests and verify they pass**

Run:

```bash
go test ./internal/tui -run 'TestRefreshViewportUsesPolishedDateSeparators|TestRenderTUISystemEntryUsesQuietCenteredText|TestRefreshViewportCompactsConsecutiveMessagesFromSameSender|TestRefreshViewportAddsDateSeparators|TestRenderTUIEntryShowsReplyCard|TestRenderTUIEntryKeepsReplyCardAboveBodyWithoutBlankLine' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit viewport hierarchy change**

Run:

```bash
git add internal/tui/model.go internal/tui/model_test.go
git commit -m "feat: polish tui message hierarchy"
```

## Task 3: Attachment Cards

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/model_test.go`

- [ ] **Step 1: Write failing attachment card tests**

Add this test near `TestRenderedMessageBodyFormatsAttachmentMessagesCompactly` in `internal/tui/model_test.go`:

```go
func TestRenderTUIEntryShowsAttachmentCard(t *testing.T) {
	t.Parallel()

	entry := historyEntry{
		kind: historyKindMessage,
		from: "alice",
		body: attachment.FormatChatMessage(attachment.ChatMessage{
			Version: 1,
			ID:      "att_123456",
			Kind:    attachment.KindImage,
			Name:    "cat.gif",
			Size:    1536,
		}),
		at: time.Date(2026, 5, 14, 15, 4, 0, 0, time.Local),
	}

	lines := strings.Split(stripANSI(renderTUIEntry(entry, false)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected header plus two attachment card lines, got %#v", lines)
	}
	if lines[0] != "[15:04] alice:" {
		t.Fatalf("expected attachment header, got %#v", lines)
	}
	if lines[1] != "  [image] cat.gif · 1.5 KB" {
		t.Fatalf("expected attachment summary line, got %#v", lines)
	}
	if lines[2] != "  #att_123456 · O open · D download" {
		t.Fatalf("expected attachment action line, got %#v", lines)
	}
}
```

Update existing TUI attachment view assertions:

```go
if !strings.Contains(stripANSI(uiModel.View()), "[image] cat.gif (6 B) #att_a1") {
	t.Fatalf("expected compact attachment rendering in view, got %q", stripANSI(uiModel.View()))
}
```

to:

```go
view := stripANSI(uiModel.View())
if !strings.Contains(view, "[image] cat.gif · 6 B") || !strings.Contains(view, "#att_a1 · O open · D download") {
	t.Fatalf("expected attachment card rendering in view, got %q", view)
}
```

Make the same assertion shape for the pasted attachment test that currently checks `[image] pasted.png (3 B) #att_p1`.

- [ ] **Step 2: Run attachment card tests and verify they fail**

Run:

```bash
go test ./internal/tui -run 'TestRenderTUIEntryShowsAttachmentCard|TestModelFileCommandUploadsAndSendsVisibleAttachmentMessage|TestModelDirectPasteUploadsClipboardAttachment' -count=1
```

Expected: FAIL because TUI attachment messages still render as a single compact line.

- [ ] **Step 3: Add TUI attachment card renderer**

In `internal/tui/model.go`, add this helper near `renderCompactReplyBody`:

```go
func renderTUIAttachmentCard(body string, textStyle lipgloss.Style, statusSuffix string) (string, bool) {
	msg, ok := attachment.ParseChatMessage(body)
	if !ok {
		return "", false
	}
	summary := textStyle.Render(fmt.Sprintf("  [%s] %s · %s", msg.Kind, msg.Name, humanBytes(msg.Size)))
	actions := textStyle.Render(fmt.Sprintf("  #%s · O open · D download%s", msg.ID, statusSuffix))
	return strings.Join([]string{summary, actions}, "\n"), true
}
```

In `renderTUIEntryWithFeedbackAndContext`, after `header` is built and before the default `line := ...` path, add:

```go
		if !entry.revoked {
			if card, ok := renderTUIAttachmentCard(entry.body, textSegmentStyle, statusSuffix); ok {
				line := header + textSegmentStyle.Render(":") + "\n" + card
				if ctx.compactSender {
					line = card
				}
				if selected {
					return inputHintStyle.Render("> ") + line
				}
				return line
			}
		}
```

Leave `renderedMessageBody` unchanged so scrollback and plain text copy behavior keep the existing compact attachment summary.

- [ ] **Step 4: Run attachment card tests and verify they pass**

Run:

```bash
go test ./internal/tui -run 'TestRenderTUIEntryShowsAttachmentCard|TestModelFileCommandUploadsAndSendsVisibleAttachmentMessage|TestModelDirectPasteUploadsClipboardAttachment|TestModelMouseClickOpensAttachment|TestModelMouseHoverHighlightsAttachment|TestCopyModeAttachmentShortcutsOpenAndDownloadSelectedAttachment' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit attachment card change**

Run:

```bash
git add internal/tui/model.go internal/tui/model_test.go
git commit -m "feat: render tui attachment cards"
```

## Task 4: Bottom Status And Input Area

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/model_test.go`

- [ ] **Step 1: Write failing bottom status tests**

Add these tests near the input/reply bar tests in `internal/tui/model_test.go`:

```go
func TestStatusNoticeRendersAboveInputInsteadOfTopBar(t *testing.T) {
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
	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 80, Height: 18})
	uiModel = updated.(model)
	uiModel.setStatusNotice("copied message", false)
	uiModel.resize()

	view := stripANSI(uiModel.View())
	lines := strings.Split(view, "\n")
	if strings.Contains(lines[0], "copied message") {
		t.Fatalf("expected top bar to keep connection status, got %q", lines[0])
	}
	if !strings.Contains(view, "copied message") {
		t.Fatalf("expected bottom status notice, got %q", view)
	}
}

func TestResizeAccountsForBottomStatusNotice(t *testing.T) {
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
	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 80, Height: 18})
	uiModel = updated.(model)
	withoutNotice := uiModel.viewport.Height

	uiModel.setStatusNotice("copied message", false)
	uiModel.resize()
	withNotice := uiModel.viewport.Height

	if withNotice != withoutNotice-1 {
		t.Fatalf("expected status notice to reserve one row, got without=%d with=%d", withoutNotice, withNotice)
	}
}
```

- [ ] **Step 2: Run bottom status tests and verify they fail**

Run:

```bash
go test ./internal/tui -run 'TestStatusNoticeRendersAboveInputInsteadOfTopBar|TestResizeAccountsForBottomStatusNotice' -count=1
```

Expected: FAIL because status notices still render through the top status bar and resize does not reserve a separate bottom status row.

- [ ] **Step 3: Render bottom status notice in the TUI view**

In the TUI branch of `View()`, insert the status notice between reply bar and input box:

```go
	if replyBar := m.renderReplyBar(); replyBar != "" {
		lines = append(lines, replyBar)
	}
	if statusNotice := m.renderStatusNotice(); statusNotice != "" {
		lines = append(lines, statusNotice)
	}
	lines = append(lines, m.renderInputBox())
```

Add this helper near `renderInputBox`:

```go
func (m model) renderStatusNotice() string {
	text := ""
	isError := false
	switch {
	case strings.TrimSpace(m.operationNotice) != "":
		text = strings.TrimSpace(m.operationNotice)
		isError = m.operationNoticeIsError
	case strings.TrimSpace(m.statusNotice) != "":
		text = strings.TrimSpace(m.statusNotice)
		isError = m.statusNoticeIsError
	case m.copyMode:
		text = "copy mode"
	case m.revokeMode:
		text = "revoke mode"
	}
	if text == "" {
		return ""
	}
	style := inputHintStyle
	if isError {
		style = errorStyle
	}
	return style.Render(text)
}
```

Keep `renderTopBar` focused on room/connection/online state. Do not put `operationNotice` or `statusNotice` in the top bar.

- [ ] **Step 4: Account for bottom status height during resize**

Replace the hardcoded input/status height section at the top of `resize()`:

```go
	inputHeight := 5
```

with:

```go
	inputHeight := lipgloss.Height(m.renderInputBox())
	statusNoticeHeight := 0
	if strings.TrimSpace(m.renderStatusNotice()) != "" {
		statusNoticeHeight = lipgloss.Height(m.renderStatusNotice())
	}
```

Replace the viewport height calculation:

```go
viewportHeight := m.height - inputHeight - 1 - suggestionHeight - actionBarHeight - replyBarHeight
```

with:

```go
viewportHeight := m.height - inputHeight - 1 - suggestionHeight - actionBarHeight - replyBarHeight - statusNoticeHeight
```

- [ ] **Step 5: Run bottom status and existing mode tests**

Run:

```bash
go test ./internal/tui -run 'TestStatusNoticeRendersAboveInputInsteadOfTopBar|TestResizeAccountsForBottomStatusNotice|TestCopyModeRendersMouseActionBarForPlainMessage|TestCopyModeInputHintMentionsMouseActions|TestRevokeModeInputHintMentionsMouseActions|TestReplyDraftRendersSingleLineBarAboveInput' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit bottom status change**

Run:

```bash
git add internal/tui/model.go internal/tui/model_test.go
git commit -m "feat: separate tui status and input areas"
```

## Task 5: Regression Sweep

**Files:**
- Modify: `internal/tui/model_test.go` only if assertions need to be updated for the intentional visual changes.

- [ ] **Step 1: Run the full TUI package tests**

Run:

```bash
go test ./internal/tui -count=1
```

Expected: PASS. If a test fails because it asserts the old visual text, update only that assertion to the new top bar, date separator, quiet system line, attachment card, or bottom status placement. Do not change behavior-oriented assertions such as sent messages, transcript writes, update messages, mouse clicks, or attachment command effects.

- [ ] **Step 2: Run all project tests**

Run:

```bash
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 3: Build a local binary**

Run:

```bash
go build -trimpath -o ./chatbox ./cmd/chatbox
```

Expected: command exits 0 and writes `./chatbox`.

- [ ] **Step 4: Smoke-check version command**

Run:

```bash
./chatbox version
```

Expected: prints a version string and exits 0.

- [ ] **Step 5: Commit any test assertion cleanup**

If Step 1 required assertion cleanup, run:

```bash
git add internal/tui/model_test.go
git commit -m "test: align tui visual assertions"
```

If Step 1 did not require cleanup, do not create a commit for this step.

## Self-Review

Spec coverage:

- Top bar: Task 1 covers fixed top bar, room/mode, connection status, online count, and narrow-width degradation.
- Message viewport: Task 2 covers date separators, muted system lines, compact consecutive sender rendering, and keeps existing reply card tests in the validation command.
- Attachment cards: Task 3 covers card rendering, id preservation, open/download hints, and mouse/shortcut regressions.
- Input area: Task 4 covers reply/status/input layering and resize behavior.
- Compatibility: Each task keeps scrollback untouched and runs targeted behavior regressions; Task 5 runs `go test ./internal/tui`, `go test ./...`, and a local build.

Placeholder scan:

- The plan contains exact paths, concrete test code, concrete implementation snippets, commands, and expected results.

Type consistency:

- New helper names are consistent across tasks: `renderTopBar`, `compactTopBarParts`, `renderStatusNotice`, `tuiEntryRenderContext`, `renderTUIEntryWithFeedbackAndContext`, and `renderTUIAttachmentCard`.
