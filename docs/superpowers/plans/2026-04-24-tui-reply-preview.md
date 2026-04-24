# TUI Reply Preview Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace raw quote-block insertion with a lightweight TUI reply draft bar, then render sent replies as compact preview + body without changing the wire protocol.

**Architecture:** Keep reply state local to the TUI model. `quote` creates a local `replyDraft`, Enter formats that draft into the existing plain-text protocol body, and TUI rendering recognizes only the new one-line quote format to show a compact reply preview while older `>` blocks still render untouched.

**Tech Stack:** Go, Bubble Tea, Lip Gloss, existing `internal/tui` model/test suite

---

## File Map

- Modify: `internal/tui/model.go`
  - Add local reply-draft state and compact reply parsing helpers
  - Render a reply bar above the input box
  - Route `quote`, `Esc`, `Enter`, and mouse-click clear behavior through the new state
  - Keep network payloads as plain text and keep transcript persistence unchanged
- Modify: `internal/tui/model_test.go`
  - Add red/green coverage for reply draft creation, cancel, submit formatting, compact rendering, attachment/revoke summaries, and legacy fallback behavior
- Verify only: `internal/tui/attachment_commands.go`
  - Reuse attachment parsing helpers; do not change attachment transfer behavior unless tests prove a gap

### Task 1: Replace Raw Quote Insertion With Local Reply Draft State

**Files:**
- Modify: `internal/tui/model.go`
- Test: `internal/tui/model_test.go`

- [ ] **Step 1: Write the failing reply-draft tests**

```go
func TestQuoteCreatesReplyDraftInsteadOfEditingInput(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:   "join",
		uiMode: uiModeTUI,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	uiModel.width = 80
	uiModel.height = 12
	uiModel.resize()
	uiModel.input.SetValue("draft reply")
	uiModel.input.SetCursor(len("draft reply"))
	uiModel.addHistoryEntry(historyEntry{
		kind: historyKindMessage,
		from: "alice",
		body: "hello world",
		at:   time.Date(2026, 4, 24, 11, 22, 0, 0, time.Local),
	})

	updated, _ := uiModel.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	uiModel = updated.(model)

	if got := uiModel.input.Value(); got != "draft reply" {
		t.Fatalf("expected input to stay editable, got %q", got)
	}
	if uiModel.replyDraft == nil {
		t.Fatal("expected quote action to create a reply draft")
	}
	if uiModel.replyDraft.sender != "alice" || uiModel.replyDraft.preview != "hello world" {
		t.Fatalf("unexpected reply draft: %#v", uiModel.replyDraft)
	}
	if uiModel.copyMode {
		t.Fatal("expected quote action to exit copy mode")
	}
}

func TestReplyDraftRendersSingleLineBarAboveInput(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:   "join",
		uiMode: uiModeTUI,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	uiModel.width = 80
	uiModel.height = 12
	uiModel.replyDraft = &replyDraft{
		targetMessageID: "msg-1",
		sender:          "alice",
		sentAt:          time.Date(2026, 4, 24, 11, 22, 0, 0, time.Local),
		preview:         "hello world",
	}
	uiModel.resize()

	view := stripANSI(uiModel.View())
	if !strings.Contains(view, "reply alice [11:22] hello world [x]") {
		t.Fatalf("expected reply bar in view, got %q", view)
	}
}

func TestEscapeClearsReplyDraftInNormalMode(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:   "join",
		uiMode: uiModeTUI,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	uiModel.width = 80
	uiModel.height = 12
	uiModel.replyDraft = &replyDraft{
		targetMessageID: "msg-1",
		sender:          "alice",
		sentAt:          time.Date(2026, 4, 24, 11, 22, 0, 0, time.Local),
		preview:         "hello world",
	}
	uiModel.resize()

	updated, _ := uiModel.Update(tea.KeyMsg{Type: tea.KeyEsc})
	uiModel = updated.(model)

	if uiModel.replyDraft != nil {
		t.Fatalf("expected Esc to clear reply draft, got %#v", uiModel.replyDraft)
	}
	if strings.Contains(stripANSI(uiModel.View()), "reply alice [11:22]") {
		t.Fatalf("expected reply bar to disappear, got %q", stripANSI(uiModel.View()))
	}
}
```

- [ ] **Step 2: Run the reply-draft tests to verify they fail**

Run:

```bash
go test ./internal/tui -run 'TestQuoteCreatesReplyDraftInsteadOfEditingInput|TestReplyDraftRendersSingleLineBarAboveInput|TestEscapeClearsReplyDraftInNormalMode' -count=1
```

Expected: FAIL because quote currently mutates `m.input` and no reply-draft UI/state exists.

- [ ] **Step 3: Write the minimal reply-draft implementation**

Add local reply-draft types and fields in `internal/tui/model.go`:

```go
type replyDraft struct {
	targetMessageID string
	sender          string
	sentAt          time.Time
	preview         string
}

type renderedReplyBarState struct {
	text       string
	clearStart int
	clearEnd   int
}
```

Extend the model state:

```go
	replyDraft       *replyDraft
	renderedReplyBar renderedReplyBarState
```

Add helpers to create and clear the draft:

```go
func buildReplyPreview(entry historyEntry) string {
	body := strings.ReplaceAll(renderedMessageBody(entry), "\r\n", "\n")
	line := strings.TrimSpace(strings.Split(body, "\n")[0])
	if line == "" {
		return "消息"
	}
	return truncateRunes(line, 48)
}

func truncateRunes(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "..."
}

func (m *model) setReplyDraft(entry historyEntry) {
	draft := replyDraft{
		targetMessageID: strings.TrimSpace(entry.messageID),
		sender:          strings.TrimSpace(entry.from),
		sentAt:          entry.at,
		preview:         buildReplyPreview(entry),
	}
	m.replyDraft = &draft
	m.resize()
}

func (m *model) clearReplyDraft() bool {
	if m.replyDraft == nil {
		return false
	}
	m.replyDraft = nil
	m.resize()
	return true
}

func (m model) buildRenderedReplyBarState() renderedReplyBarState {
	if m.replyDraft == nil {
		return renderedReplyBarState{}
	}
	label := fmt.Sprintf(
		"reply %s [%s] %s [x]",
		m.replyDraft.sender,
		m.replyDraft.sentAt.Local().Format("15:04"),
		m.replyDraft.preview,
	)
	clearStart := strings.LastIndex(label, "[x]")
	return renderedReplyBarState{
		text:       label,
		clearStart: clearStart,
		clearEnd:   clearStart + len("[x]"),
	}
}

func (m model) renderReplyBar() string {
	state := m.buildRenderedReplyBarState()
	if strings.TrimSpace(state.text) == "" {
		return ""
	}
	return inputHintStyle.Render(state.text)
}
```

Replace quote behavior and update the main key handler:

```go
func (m *model) quoteSelectedMessage() {
	index := m.selectedCopyHistoryIndex()
	if index < 0 || index >= len(m.history) {
		m.setStatusNotice("no message to copy", true)
		return
	}
	m.setReplyDraft(m.history[index])
	m.exitCopyMode()
}
```

```go
case tea.KeyEsc:
	if m.clearReplyDraft() {
		return m, nil
	}
```

Wire the reply bar into layout and resize:

```go
func (m model) View() string {
	lines := []string{m.renderStatusBar(), m.viewport.View()}
	if actionBar := m.renderActionBar(); actionBar != "" {
		lines = append(lines, actionBar)
	}
	if suggestions := m.renderSlashCommandSuggestions(); suggestions != "" {
		lines = append(lines, suggestions)
	}
	if replyBar := m.renderReplyBar(); replyBar != "" {
		lines = append(lines, replyBar)
	}
	lines = append(lines, m.renderInputBox())
	return strings.Join(lines, "\n")
}

func (m *model) resize() {
	inputHeight := 5
	suggestionHeight := 0
	if len(m.activeSlashCommandSuggestions()) > 0 {
		suggestionHeight = len(m.activeSlashCommandSuggestions()) + 2
	}
	actionBarHeight := 0
	if strings.TrimSpace(m.buildRenderedActionBarState().text) != "" {
		actionBarHeight = 1
	}
	replyBarHeight := 0
	if m.replyDraft != nil {
		replyBarHeight = 1
	}
	viewportHeight := m.height - inputHeight - 1 - suggestionHeight - actionBarHeight - replyBarHeight
	if viewportHeight < 5 {
		viewportHeight = 5
	}
	if m.width > 4 {
		m.viewport.Width = m.width - 2
		m.input.SetWidth(m.width - 8)
	}
	m.viewport.Height = viewportHeight
	m.refreshViewport(m.viewport.AtBottom())
}
```

- [ ] **Step 4: Run the reply-draft tests to verify they pass**

Run:

```bash
go test ./internal/tui -run 'TestQuoteCreatesReplyDraftInsteadOfEditingInput|TestReplyDraftRendersSingleLineBarAboveInput|TestEscapeClearsReplyDraftInNormalMode' -count=1
```

Expected: PASS

- [ ] **Step 5: Commit the reply-draft state changes**

```bash
git add internal/tui/model.go internal/tui/model_test.go
git commit -m "Add TUI reply draft state"
```

### Task 2: Format Reply Drafts On Send And Render Compact Reply Preview

**Files:**
- Modify: `internal/tui/model.go`
- Test: `internal/tui/model_test.go`

- [ ] **Step 1: Write the failing send/render tests**

```go
func TestSubmitReplyDraftFormatsOutgoingBody(t *testing.T) {
	t.Parallel()

	fake := &fakeSession{peerName: "aaa", localName: "bob"}
	uiModel := newModel(modelOptions{
		mode:   "join",
		uiMode: uiModeTUI,
		session: fake,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	uiModel.replyDraft = &replyDraft{
		targetMessageID: "msg-1",
		sender:          "alice",
		sentAt:          time.Date(2026, 4, 24, 11, 22, 0, 0, time.Local),
		preview:         "hello world...",
	}
	uiModel.input.SetValue("收到，我晚点处理")
	uiModel.input.SetCursor(len([]rune(uiModel.input.Value())))

	updated, _ := uiModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	uiModel = updated.(model)

	if len(fake.sent) != 1 {
		t.Fatalf("expected one sent message, got %#v", fake.sent)
	}
	want := "> alice [11:22] hello world...\n收到，我晚点处理"
	if fake.sent[0].Body != want {
		t.Fatalf("expected reply body %q, got %q", want, fake.sent[0].Body)
	}
	if uiModel.replyDraft != nil {
		t.Fatalf("expected reply draft to clear after send, got %#v", uiModel.replyDraft)
	}
}

func TestSubmitReplyDraftRequiresNonEmptyBody(t *testing.T) {
	t.Parallel()

	fake := &fakeSession{peerName: "aaa", localName: "bob"}
	uiModel := newModel(modelOptions{
		mode:   "join",
		uiMode: uiModeTUI,
		session: fake,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	uiModel.replyDraft = &replyDraft{
		targetMessageID: "msg-1",
		sender:          "alice",
		sentAt:          time.Date(2026, 4, 24, 11, 22, 0, 0, time.Local),
		preview:         "hello world",
	}
	uiModel.input.SetValue("   ")
	uiModel.input.SetCursor(3)

	updated, _ := uiModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	uiModel = updated.(model)

	if len(fake.sent) != 0 {
		t.Fatalf("expected empty reply body not to send, got %#v", fake.sent)
	}
	if uiModel.replyDraft == nil {
		t.Fatal("expected reply draft to remain so the user can keep editing")
	}
	if !strings.Contains(stripANSI(uiModel.View()), "reply body required") {
		t.Fatalf("expected empty-body notice, got %q", stripANSI(uiModel.View()))
	}
}

func TestRenderTUIEntryShowsCompactReplyPreview(t *testing.T) {
	t.Parallel()

	entry := historyEntry{
		kind: historyKindMessage,
		from: "bob",
		body: "> alice [11:22] hello world...\n收到，我晚点处理",
		at:   time.Date(2026, 4, 24, 11, 23, 0, 0, time.Local),
	}

	got := stripANSI(renderTUIEntry(entry, false))
	if !strings.Contains(got, "reply alice [11:22] hello world...") {
		t.Fatalf("expected compact reply header, got %q", got)
	}
	if !strings.Contains(got, "收到，我晚点处理") {
		t.Fatalf("expected reply body in view, got %q", got)
	}
	if strings.Contains(got, "\n\n") {
		t.Fatalf("expected no extra blank line, got %q", got)
	}
}
```

- [ ] **Step 2: Run the send/render tests to verify they fail**

Run:

```bash
go test ./internal/tui -run 'TestSubmitReplyDraftFormatsOutgoingBody|TestSubmitReplyDraftRequiresNonEmptyBody|TestRenderTUIEntryShowsCompactReplyPreview' -count=1
```

Expected: FAIL because Enter still sends raw input text and TUI rendering does not recognize compact reply bodies.

- [ ] **Step 3: Write the minimal submit/render implementation**

Add helpers for reply submission formatting:

```go
func formatReplySubmission(draft *replyDraft, body string) string {
	if draft == nil {
		return strings.TrimSpace(body)
	}
	return fmt.Sprintf(
		"> %s [%s] %s\n%s",
		draft.sender,
		draft.sentAt.Local().Format("15:04"),
		draft.preview,
		strings.TrimSpace(body),
	)
}

func parseCompactReply(body string) (header string, replyBody string, ok bool) {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.SplitN(body, "\n", 2)
	if len(lines) != 2 {
		return "", "", false
	}
	first := strings.TrimSpace(lines[0])
	second := strings.TrimSpace(lines[1])
	if second == "" || !compactReplyLinePattern.MatchString(first) {
		return "", "", false
	}
	return strings.TrimPrefix(first, "> "), second, true
}
```

Use a dedicated submit helper so Enter can validate before resetting input:

```go
func (m *model) submitInput() (tea.Model, tea.Cmd) {
	raw := m.input.Value()
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		if m.replyDraft != nil {
			m.setStatusNotice("reply body required", true)
			return *m, nil
		}
		m.input.Reset()
		m.resize()
		return *m, nil
	}
	text := formatReplySubmission(m.replyDraft, raw)
	if m.replyDraft != nil {
		m.replyDraft = nil
	}
	m.input.Reset()
	m.resize()
	return m.handleSubmit(text)
}
```

Replace Enter handling:

```go
case tea.KeyEnter:
	return m.submitInput()
```

Render the compact reply preview only in TUI:

```go
var compactReplyLinePattern = regexp.MustCompile(`^> .+ \[[0-9]{2}:[0-9]{2}\] .+$`)

func renderCompactReplyBody(entry historyEntry) (string, bool) {
	header, replyBody, ok := parseCompactReply(entry.body)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("reply %s\n%s", header, replyBody), true
}
```

Update `renderTUIEntryWithFeedback`:

```go
func renderTUIEntryWithFeedback(entry historyEntry, selected bool, feedback attachmentFeedbackState) string {
	timestamp := entry.at.Local().Format("15:04")
	switch entry.kind {
	case historyKindSystem:
		return systemLineStyle().Render(fmt.Sprintf("system [%s]: %s", timestamp, entry.body))
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
		separator := textSegmentStyle.Render(": ")
		bodyText := renderedMessageBody(entry) + statusSuffix
		if compact, ok := renderCompactReplyBody(entry); ok {
			bodyText = compact + statusSuffix
		}
		body := textSegmentStyle.Render(bodyText)
		line := coloredTimestamp + textSegmentStyle.Render(" ") + coloredLabel + separator + body
		if selected {
			return inputHintStyle.Render("> ") + line
		}
		return line
	}
}
```

- [ ] **Step 4: Run the send/render tests to verify they pass**

Run:

```bash
go test ./internal/tui -run 'TestSubmitReplyDraftFormatsOutgoingBody|TestSubmitReplyDraftRequiresNonEmptyBody|TestRenderTUIEntryShowsCompactReplyPreview' -count=1
```

Expected: PASS

- [ ] **Step 5: Commit the send/render changes**

```bash
git add internal/tui/model.go internal/tui/model_test.go
git commit -m "Render compact TUI reply previews"
```

### Task 3: Add Mouse Clear, Attachment/Revoke Summaries, And Legacy Fallback Coverage

**Files:**
- Modify: `internal/tui/model.go`
- Test: `internal/tui/model_test.go`

- [ ] **Step 1: Write the failing compatibility tests**

```go
func TestQuoteAttachmentUsesCompactAttachmentPreview(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:   "join",
		uiMode: uiModeTUI,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	uiModel.width = 80
	uiModel.height = 12
	uiModel.resize()
	uiModel.addHistoryEntry(historyEntry{
		kind: historyKindMessage,
		from: "alice",
		body: attachment.FormatChatMessage(attachment.ChatMessage{
			Version: 1,
			ID:      "att-plan-1",
			Kind:    attachment.KindImage,
			Name:    "cat.gif",
			Size:    6,
		}),
		at: time.Date(2026, 4, 24, 11, 24, 0, 0, time.Local),
	})

	updated, _ := uiModel.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	uiModel = updated.(model)

	if uiModel.replyDraft == nil || uiModel.replyDraft.preview != "[图片] cat.gif" {
		t.Fatalf("expected compact attachment preview, got %#v", uiModel.replyDraft)
	}
}

func TestQuoteRevokedMessageUsesRevokedPreview(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:   "join",
		uiMode: uiModeTUI,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	uiModel.width = 80
	uiModel.height = 12
	uiModel.resize()
	uiModel.addHistoryEntry(historyEntry{
		kind:    historyKindMessage,
		from:    "alice",
		body:    "hello world",
		at:      time.Date(2026, 4, 24, 11, 25, 0, 0, time.Local),
		revoked: true,
	})

	updated, _ := uiModel.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	uiModel = updated.(model)

	if uiModel.replyDraft == nil || uiModel.replyDraft.preview != "已撤回一条消息" {
		t.Fatalf("expected revoked preview, got %#v", uiModel.replyDraft)
	}
}

func TestLegacyMultiLineQuoteStillRendersAsPlainText(t *testing.T) {
	t.Parallel()

	entry := historyEntry{
		kind: historyKindMessage,
		from: "bob",
		body: "> alice [11:22]\n> hello world\n收到，我晚点处理",
		at:   time.Date(2026, 4, 24, 11, 26, 0, 0, time.Local),
	}

	got := stripANSI(renderTUIEntry(entry, false))
	if !strings.Contains(got, "> alice [11:22]") {
		t.Fatalf("expected legacy quote text to remain visible, got %q", got)
	}
	if strings.Contains(got, "reply alice [11:22]") {
		t.Fatalf("expected legacy quote not to be compacted, got %q", got)
	}
}

func TestClickReplyBarClearButtonRemovesDraft(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:   "join",
		uiMode: uiModeTUI,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	uiModel.width = 80
	uiModel.height = 12
	uiModel.replyDraft = &replyDraft{
		targetMessageID: "msg-1",
		sender:          "alice",
		sentAt:          time.Date(2026, 4, 24, 11, 22, 0, 0, time.Local),
		preview:         "hello world",
	}
	uiModel.resize()
	uiModel.refreshViewport(false)

	replyY := uiModel.replyBarRow()
	clickX := uiModel.renderedReplyBar.clearStart + 1

	updated, _ := uiModel.Update(tea.MouseMsg{X: clickX, Y: replyY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(tea.MouseMsg{X: clickX, Y: replyY, Button: tea.MouseButtonNone, Action: tea.MouseActionRelease})
	uiModel = updated.(model)

	if uiModel.replyDraft != nil {
		t.Fatalf("expected click to clear reply draft, got %#v", uiModel.replyDraft)
	}
}
```

- [ ] **Step 2: Run the compatibility tests to verify they fail**

Run:

```bash
go test ./internal/tui -run 'TestQuoteAttachmentUsesCompactAttachmentPreview|TestQuoteRevokedMessageUsesRevokedPreview|TestLegacyMultiLineQuoteStillRendersAsPlainText|TestClickReplyBarClearButtonRemovesDraft' -count=1
```

Expected: FAIL because reply previews still use generic first-line text, revoked summaries are not specialized, and the reply bar has no mouse hit target.

- [ ] **Step 3: Write the minimal compatibility implementation**

Upgrade preview building and mouse hit-testing:

```go
func buildReplyPreview(entry historyEntry) string {
	if entry.revoked {
		return "已撤回一条消息"
	}
	if attachmentMsg, ok := attachment.ParseChatMessage(entry.body); ok {
		switch attachmentMsg.Kind {
		case attachment.KindImage:
			return fmt.Sprintf("[图片] %s", attachmentMsg.Name)
		default:
			return fmt.Sprintf("[文件] %s", attachmentMsg.Name)
		}
	}
	body := strings.ReplaceAll(renderedMessageBody(entry), "\r\n", "\n")
	line := strings.TrimSpace(strings.Split(body, "\n")[0])
	if line == "" {
		return "消息"
	}
	return truncateRunes(line, 48)
}
```

Persist rendered reply-bar hit ranges during viewport refresh:

```go
func (m *model) refreshViewport(stickToBottom bool) {
	offset := m.viewport.YOffset
	state := m.buildRenderedViewportState()
	m.renderedViewport = state
	m.renderedActionBar = m.buildRenderedActionBarState()
	m.renderedReplyBar = m.buildRenderedReplyBarState()
	lines := make([]string, 0, len(state.lines))
	for _, line := range state.lines {
		lines = append(lines, line.text)
	}
	m.viewport.SetContent(strings.Join(lines, "\n"))
	if stickToBottom {
		m.viewport.GotoBottom()
		return
	}
	m.viewport.SetYOffset(offset)
}
```

Add row helpers and mouse handling:

```go
func (m model) replyBarRow() int {
	row := m.actionBarRow()
	if strings.TrimSpace(m.renderedActionBar.text) != "" {
		row++
	}
	if suggestions := m.renderSlashCommandSuggestions(); suggestions != "" {
		row += strings.Count(suggestions, "\n") + 1
	}
	return row
}

func (m model) isWithinReplyBar(mouseY int) bool {
	return strings.TrimSpace(m.renderedReplyBar.text) != "" && mouseY == m.replyBarRow()
}

func (m model) clickedReplyBarClear(mouseX, mouseY int) bool {
	if !m.isWithinReplyBar(mouseY) {
		return false
	}
	return mouseX >= m.renderedReplyBar.clearStart && mouseX < m.renderedReplyBar.clearEnd
}
```

Handle reply-bar clicks before viewport clicks:

```go
case tea.MouseActionPress:
	if msg.Button == tea.MouseButtonLeft && (m.isWithinViewport(msg.Y) || m.isWithinActionBar(msg.Y) || m.isWithinReplyBar(msg.Y)) {
		if m.isWithinViewport(msg.Y) {
			m.updateHoveredHistoryIndex(msg.Y)
		}
		m.pendingViewportPress = &mouseViewportPress{x: msg.X, y: msg.Y, inViewport: m.isWithinViewport(msg.Y)}
		m.draggingViewport = false
		m.lastMouseY = msg.Y
		return true, nil
	}

case tea.MouseActionRelease:
	if (msg.Button == tea.MouseButtonLeft || msg.Button == tea.MouseButtonNone) && m.pendingViewportPress != nil {
		wasDragging := m.draggingViewport
		m.pendingViewportPress = nil
		m.draggingViewport = false
		if wasDragging {
			return true, nil
		}
		if m.clickedReplyBarClear(msg.X, msg.Y) {
			m.clearReplyDraft()
			return true, nil
		}
		if action, ok := m.clickedActionBarAction(msg.X, msg.Y); ok {
			nextModel, cmd := m.handleActionBarAction(action)
			*m = nextModel.(model)
			return true, cmd
		}
		// existing viewport handling continues here
	}
```

- [ ] **Step 4: Run the compatibility tests to verify they pass**

Run:

```bash
go test ./internal/tui -run 'TestQuoteAttachmentUsesCompactAttachmentPreview|TestQuoteRevokedMessageUsesRevokedPreview|TestLegacyMultiLineQuoteStillRendersAsPlainText|TestClickReplyBarClearButtonRemovesDraft' -count=1
```

Expected: PASS

- [ ] **Step 5: Commit the compatibility and mouse-clear changes**

```bash
git add internal/tui/model.go internal/tui/model_test.go
git commit -m "Polish TUI reply preview behavior"
```

## Final Verification

- [ ] Run the focused TUI suite:

```bash
go test ./internal/tui -count=1
```

Expected: PASS

- [ ] Run the full project suite:

```bash
go test ./... -count=1
```

Expected: PASS

- [ ] Build the CLI binary:

```bash
go build -o ./chatbox ./cmd/chatbox
```

Expected: build succeeds with no errors

- [ ] Sanity-check the version command:

```bash
./chatbox version
```

Expected: prints the local dev or release version string
