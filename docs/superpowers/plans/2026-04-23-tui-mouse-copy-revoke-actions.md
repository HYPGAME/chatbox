# TUI Mouse Copy/Revoke Actions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add mouse-driven message selection and clickable action buttons for TUI `copy mode` and `revoke mode` without changing normal-mode attachment click behavior.

**Architecture:** Extend the existing TUI model with a rendered action-bar state that sits outside the scrollable viewport, then route mouse clicks through action-bar hit-testing, mode-aware message selection, and the existing attachment/scroll handlers. Reuse the current keyboard action functions so mouse and keyboard stay behaviorally identical.

**Tech Stack:** Go, Bubble Tea, Lip Gloss, existing `internal/tui` model/test suite

---

## File Map

- Modify: `internal/tui/model.go`
  - Add rendered action-bar data structures and mode-aware rendering
  - Add helpers to map clicked viewport lines back to selectable messages
  - Extend mouse dispatch to support action-bar buttons and mode-aware selection
  - Reuse existing copy/revoke/open/download functions from keyboard flow
- Modify: `internal/tui/model_test.go`
  - Add red/green tests for action-bar rendering, mode selection, button clicks, and drag regressions
- Verify only: `internal/tui/attachment_commands.go`
  - Keep current attachment open/download result handling unchanged unless tests prove a gap

### Task 1: Add Action-Bar Rendering State

**Files:**
- Modify: `internal/tui/model.go`
- Test: `internal/tui/model_test.go`

- [ ] **Step 1: Write the failing rendering tests**

```go
func TestCopyModeRendersMouseActionBarForPlainMessage(t *testing.T) {
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
		body: "copy me",
		at:   time.Date(2026, 4, 23, 21, 0, 0, 0, time.Local),
	})

	updated, _ := uiModel.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	uiModel = updated.(model)

	view := stripANSI(uiModel.View())
	if !strings.Contains(view, "[copy] [quote] [cancel]") {
		t.Fatalf("expected copy-mode action bar, got %q", view)
	}
}

func TestCopyModeRendersMouseActionBarForAttachmentMessage(t *testing.T) {
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
		at: time.Date(2026, 4, 23, 21, 1, 0, 0, time.Local),
	})

	updated, _ := uiModel.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	uiModel = updated.(model)

	view := stripANSI(uiModel.View())
	if !strings.Contains(view, "[copy] [quote] [open] [download] [cancel]") {
		t.Fatalf("expected attachment action bar, got %q", view)
	}
}

func TestRevokeModeRendersMouseActionBar(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:   "join",
		uiMode: uiModeTUI,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	uiModel.identityID = "identity-a"
	uiModel.width = 80
	uiModel.height = 12
	uiModel.resize()
	uiModel.addHistoryEntry(historyEntry{
		kind:           historyKindMessage,
		messageID:      "m-plan-1",
		from:           "alice",
		authorIdentity: "identity-a",
		body:           "revoke me",
		at:             time.Date(2026, 4, 23, 21, 2, 0, 0, time.Local),
		outgoing:       true,
		status:         transcript.StatusSent,
	})

	uiModel.enterRevokeMode()

	view := stripANSI(uiModel.View())
	if !strings.Contains(view, "[revoke] [cancel]") {
		t.Fatalf("expected revoke-mode action bar, got %q", view)
	}
}
```

- [ ] **Step 2: Run the rendering tests to verify they fail**

Run:

```bash
go test ./internal/tui -run 'TestCopyModeRendersMouseActionBarForPlainMessage|TestCopyModeRendersMouseActionBarForAttachmentMessage|TestRevokeModeRendersMouseActionBar' -count=1
```

Expected: FAIL because the view does not render an action bar yet.

- [ ] **Step 3: Write the minimal rendering implementation**

Add action-bar state and rendering helpers in `internal/tui/model.go`:

```go
type actionBarAction string

const (
	actionBarCopy     actionBarAction = "copy"
	actionBarQuote    actionBarAction = "quote"
	actionBarOpen     actionBarAction = "open"
	actionBarDownload actionBarAction = "download"
	actionBarRevoke   actionBarAction = "revoke"
	actionBarCancel   actionBarAction = "cancel"
)

type renderedActionButton struct {
	action actionBarAction
	startX int
	endX   int
}

type renderedActionBarState struct {
	text    string
	buttons []renderedActionButton
}

func (m model) buildRenderedActionBarState() renderedActionBarState {
	actions := make([]actionBarAction, 0, 5)
	switch {
	case m.copyMode:
		actions = append(actions, actionBarCopy, actionBarQuote)
		if _, ok := m.selectedAttachmentMessage(); ok {
			actions = append(actions, actionBarOpen, actionBarDownload)
		}
		actions = append(actions, actionBarCancel)
	case m.revokeMode:
		actions = append(actions, actionBarRevoke, actionBarCancel)
	default:
		return renderedActionBarState{}
	}

	state := renderedActionBarState{}
	cursor := 0
	parts := make([]string, 0, len(actions))
	for _, action := range actions {
		label := fmt.Sprintf("[%s]", string(action))
		if len(parts) > 0 {
			cursor++
		}
		start := cursor
		cursor += lipgloss.Width(label)
		state.buttons = append(state.buttons, renderedActionButton{
			action: action,
			startX: start,
			endX:   cursor,
		})
		parts = append(parts, label)
	}
	state.text = strings.Join(parts, " ")
	return state
}

func (m model) renderActionBar() string {
	state := m.buildRenderedActionBarState()
	if strings.TrimSpace(state.text) == "" {
		return ""
	}
	return inputHintStyle.Render(state.text)
}
```

Wire the action bar into `View()` and `resize()`:

```go
func (m model) View() string {
	lines := []string{m.renderStatusBar(), m.viewport.View()}
	if actionBar := m.renderActionBar(); actionBar != "" {
		lines = append(lines, actionBar)
	}
	if suggestions := m.renderSlashCommandSuggestions(); suggestions != "" {
		lines = append(lines, suggestions)
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
	viewportHeight := m.height - inputHeight - 1 - suggestionHeight - actionBarHeight
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

- [ ] **Step 4: Run the rendering tests to verify they pass**

Run:

```bash
go test ./internal/tui -run 'TestCopyModeRendersMouseActionBarForPlainMessage|TestCopyModeRendersMouseActionBarForAttachmentMessage|TestRevokeModeRendersMouseActionBar' -count=1
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/model_test.go
git commit -m "Render TUI action bar for mouse mode actions"
```

### Task 2: Add Mode-Aware Mouse Message Selection

**Files:**
- Modify: `internal/tui/model.go`
- Test: `internal/tui/model_test.go`

- [ ] **Step 1: Write the failing selection tests**

```go
func TestModelMouseSelectsMessageInCopyMode(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:   "join",
		uiMode: uiModeTUI,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 72, Height: 12})
	uiModel = updated.(model)
	uiModel.addHistoryEntry(historyEntry{
		kind: historyKindMessage,
		from: "alice",
		body: "first",
		at:   time.Date(2026, 4, 23, 21, 10, 0, 0, time.Local),
	})
	uiModel.addHistoryEntry(historyEntry{
		kind: historyKindMessage,
		from: "bob",
		body: "second",
		at:   time.Date(2026, 4, 23, 21, 10, 1, 0, time.Local),
	})

	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	uiModel = updated.(model)
	lineRange := uiModel.renderedViewport.lineRanges[0]
	clickY := viewportTopRow + lineRange[0] - uiModel.viewport.YOffset

	updated, _ = uiModel.Update(tea.MouseMsg{X: 2, Y: clickY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(tea.MouseMsg{X: 2, Y: clickY, Button: tea.MouseButtonNone, Action: tea.MouseActionRelease})
	uiModel = updated.(model)

	if got := uiModel.selectedCopyHistoryIndex(); got != 0 {
		t.Fatalf("expected copy selection to move to history index 0, got %d", got)
	}
}

func TestModelMouseSelectsEligibleMessageInRevokeMode(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:   "join",
		uiMode: uiModeTUI,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 72, Height: 12})
	uiModel = updated.(model)
	uiModel.identityID = "identity-a"
	uiModel.addHistoryEntry(historyEntry{
		kind:           historyKindMessage,
		messageID:      "m-revoke-1",
		from:           "alice",
		authorIdentity: "identity-a",
		body:           "older",
		at:             time.Date(2026, 4, 23, 21, 11, 0, 0, time.Local),
		outgoing:       true,
		status:         transcript.StatusSent,
	})
	uiModel.addHistoryEntry(historyEntry{
		kind:           historyKindMessage,
		messageID:      "m-revoke-2",
		from:           "alice",
		authorIdentity: "identity-a",
		body:           "newer",
		at:             time.Date(2026, 4, 23, 21, 11, 1, 0, time.Local),
		outgoing:       true,
		status:         transcript.StatusSent,
	})

	uiModel.enterRevokeMode()
	lineRange := uiModel.renderedViewport.lineRanges[0]
	clickY := viewportTopRow + lineRange[0] - uiModel.viewport.YOffset

	updated, _ = uiModel.Update(tea.MouseMsg{X: 2, Y: clickY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(tea.MouseMsg{X: 2, Y: clickY, Button: tea.MouseButtonNone, Action: tea.MouseActionRelease})
	uiModel = updated.(model)

	if got := uiModel.selectedRevokeHistoryIndex(); got != 0 {
		t.Fatalf("expected revoke selection to move to history index 0, got %d", got)
	}
}
```

- [ ] **Step 2: Run the selection tests to verify they fail**

Run:

```bash
go test ./internal/tui -run 'TestModelMouseSelectsMessageInCopyMode|TestModelMouseSelectsEligibleMessageInRevokeMode' -count=1
```

Expected: FAIL because mouse release is still ignored in copy/revoke modes.

- [ ] **Step 3: Write the minimal selection implementation**

Add selection helpers in `internal/tui/model.go`:

```go
func (m *model) setCopySelectionByHistoryIndex(historyIndex int) bool {
	for i, candidate := range m.copySelection {
		if candidate == historyIndex {
			m.copySelectionPos = i
			m.followCopySelection = i == len(m.copySelection)-1
			m.scrollSelectedMessageIntoView()
			return true
		}
	}
	return false
}

func (m *model) setRevokeSelectionByHistoryIndex(historyIndex int) bool {
	for i, candidate := range m.revokeCandidates {
		if candidate == historyIndex {
			m.revokeSelection = i
			return true
		}
	}
	return false
}

func (m model) clickedHistoryIndex(mouseY int) int {
	if !m.isWithinViewport(mouseY) {
		return -1
	}
	lineIndex := m.viewportLineIndex(mouseY)
	if lineIndex < 0 || lineIndex >= len(m.renderedViewport.lines) {
		return -1
	}
	return m.renderedViewport.lines[lineIndex].historyIndex
}
```

Update `handleMouse` release handling:

```go
if (msg.Button == tea.MouseButtonLeft || msg.Button == tea.MouseButtonNone) && m.pendingViewportPress != nil {
	wasDragging := m.draggingViewport
	m.pendingViewportPress = nil
	m.draggingViewport = false
	if wasDragging {
		return true, nil
	}

	historyIndex := m.clickedHistoryIndex(msg.Y)
	if m.copyMode {
		if historyIndex >= 0 && m.setCopySelectionByHistoryIndex(historyIndex) {
			m.refreshViewport(false)
		}
		return true, nil
	}
	if m.revokeMode {
		if historyIndex >= 0 && m.setRevokeSelectionByHistoryIndex(historyIndex) {
			m.refreshViewport(false)
		}
		return true, nil
	}
	attachmentID, historyIndex := m.clickedAttachment(msg.Y)
	if attachmentID == "" {
		return true, nil
	}
	m.activeClickHistoryIndex = historyIndex
	m.refreshViewport(false)
	nextModel, cmd := m.startOpenCommand(attachmentID)
	*m = nextModel.(model)
	return true, cmd
}
```

- [ ] **Step 4: Run the selection tests to verify they pass**

Run:

```bash
go test ./internal/tui -run 'TestModelMouseSelectsMessageInCopyMode|TestModelMouseSelectsEligibleMessageInRevokeMode' -count=1
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/model_test.go
git commit -m "Select copy and revoke targets with the mouse"
```

### Task 3: Add Clickable Action-Bar Buttons

**Files:**
- Modify: `internal/tui/model.go`
- Test: `internal/tui/model_test.go`

- [ ] **Step 1: Write the failing action tests**

```go
func TestModelMouseCopyActionCopiesAndStaysInCopyMode(t *testing.T) {
	t.Parallel()

	var copied string
	uiModel := newModel(modelOptions{
		mode:   "join",
		uiMode: uiModeTUI,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	uiModel.clipboardWriter = func(text string) error {
		copied = text
		return nil
	}
	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 72, Height: 12})
	uiModel = updated.(model)
	uiModel.addHistoryEntry(historyEntry{
		kind: historyKindMessage,
		from: "alice",
		body: "copy me",
		at:   time.Date(2026, 4, 23, 21, 20, 0, 0, time.Local),
	})
	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	uiModel = updated.(model)

	actionBar := uiModel.buildRenderedActionBarState()
	copyX := actionBar.buttons[0].startX
	clickY := viewportTopRow + uiModel.viewport.Height

	updated, _ = uiModel.Update(tea.MouseMsg{X: copyX, Y: clickY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(tea.MouseMsg{X: copyX, Y: clickY, Button: tea.MouseButtonNone, Action: tea.MouseActionRelease})
	uiModel = updated.(model)

	if !strings.Contains(copied, "copy me") {
		t.Fatalf("expected copied text, got %q", copied)
	}
	if !uiModel.copyMode {
		t.Fatal("expected copy mode to remain active")
	}
}

func TestModelMouseQuoteActionQuotesAndExitsCopyMode(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:   "join",
		uiMode: uiModeTUI,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 72, Height: 12})
	uiModel = updated.(model)
	uiModel.addHistoryEntry(historyEntry{
		kind: historyKindMessage,
		from: "alice",
		body: "quote me",
		at:   time.Date(2026, 4, 23, 21, 21, 0, 0, time.Local),
	})
	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	uiModel = updated.(model)

	actionBar := uiModel.buildRenderedActionBarState()
	quoteX := actionBar.buttons[1].startX
	clickY := viewportTopRow + uiModel.viewport.Height

	updated, _ = uiModel.Update(tea.MouseMsg{X: quoteX, Y: clickY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(tea.MouseMsg{X: quoteX, Y: clickY, Button: tea.MouseButtonNone, Action: tea.MouseActionRelease})
	uiModel = updated.(model)

	if uiModel.copyMode {
		t.Fatal("expected quote action to exit copy mode")
	}
	if !strings.Contains(uiModel.input.Value(), "quote me") {
		t.Fatalf("expected quoted input, got %q", uiModel.input.Value())
	}
}

func TestModelMouseRevokeActionConfirmsAndExits(t *testing.T) {
	t.Parallel()

	fake := &fakeSession{peerName: "host", localName: "alice"}
	uiModel := newModel(modelOptions{
		mode:    "join",
		uiMode:  uiModeTUI,
		session: fake,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	uiModel.identityID = "identity-a"
	uiModel.roomAuthorization = historymeta.Record{RoomKey: "room-key", IdentityID: "identity-a"}
	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 72, Height: 12})
	uiModel = updated.(model)
	uiModel.addHistoryEntry(historyEntry{
		kind:           historyKindMessage,
		messageID:      "m-revoke-action",
		from:           "alice",
		authorIdentity: "identity-a",
		body:           "revoke me",
		at:             time.Date(2026, 4, 23, 21, 22, 0, 0, time.Local),
		outgoing:       true,
		status:         transcript.StatusSent,
	})
	uiModel.enterRevokeMode()

	actionBar := uiModel.buildRenderedActionBarState()
	revokeX := actionBar.buttons[0].startX
	clickY := viewportTopRow + uiModel.viewport.Height

	updated, _ = uiModel.Update(tea.MouseMsg{X: revokeX, Y: clickY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(tea.MouseMsg{X: revokeX, Y: clickY, Button: tea.MouseButtonNone, Action: tea.MouseActionRelease})
	uiModel = updated.(model)

	if uiModel.revokeMode {
		t.Fatal("expected revoke action to exit revoke mode")
	}
	if len(fake.sent) == 0 || !room.IsRevokeControl(fake.sent[len(fake.sent)-1].Body) {
		t.Fatalf("expected revoke control message, got %#v", fake.sent)
	}
}
```

- [ ] **Step 2: Run the action tests to verify they fail**

Run:

```bash
go test ./internal/tui -run 'TestModelMouseCopyActionCopiesAndStaysInCopyMode|TestModelMouseQuoteActionQuotesAndExitsCopyMode|TestModelMouseRevokeActionConfirmsAndExits' -count=1
```

Expected: FAIL because the action bar is rendered text only and has no hit-testing.

- [ ] **Step 3: Write the minimal action-click implementation**

Extend `model` state and mouse handling in `internal/tui/model.go`:

```go
type model struct {
	renderedViewport  renderedViewportState
	renderedActionBar renderedActionBarState
}

func (m *model) refreshActionBar() {
	m.renderedActionBar = m.buildRenderedActionBarState()
}

func (m model) actionBarRow() int {
	return viewportTopRow + m.viewport.Height
}

func (m model) clickedActionBarAction(mouseX, mouseY int) (actionBarAction, bool) {
	if mouseY != m.actionBarRow() {
		return "", false
	}
	for _, button := range m.renderedActionBar.buttons {
		if mouseX >= button.startX && mouseX < button.endX {
			return button.action, true
		}
	}
	return "", false
}

func (m *model) handleActionBarAction(action actionBarAction) (tea.Model, tea.Cmd) {
	switch action {
	case actionBarCopy:
		m.copySelectedMessage()
		return *m, nil
	case actionBarQuote:
		m.quoteSelectedMessage()
		m.resize()
		m.refreshViewport(false)
		return *m, nil
	case actionBarOpen:
		msg, ok := m.selectedAttachmentMessage()
		if !ok {
			m.setStatusNotice("selected message is not an attachment", true)
			return *m, nil
		}
		return m.startOpenCommand(msg.ID)
	case actionBarDownload:
		msg, ok := m.selectedAttachmentMessage()
		if !ok {
			m.setStatusNotice("selected message is not an attachment", true)
			return *m, nil
		}
		return m.startDownloadCommand(msg.ID, "")
	case actionBarRevoke:
		return m.confirmSelectedRevoke()
	case actionBarCancel:
		if m.copyMode {
			m.exitCopyMode()
		} else if m.revokeMode {
			m.exitRevokeMode()
		}
		m.refreshViewport(false)
		return *m, nil
	default:
		return *m, nil
	}
}
```

Dispatch action-bar clicks before viewport selection:

```go
case tea.MouseActionRelease:
	if (msg.Button == tea.MouseButtonLeft || msg.Button == tea.MouseButtonNone) && m.pendingViewportPress != nil {
		wasDragging := m.draggingViewport
		m.pendingViewportPress = nil
		m.draggingViewport = false
		if wasDragging {
			return true, nil
		}
		if action, ok := m.clickedActionBarAction(msg.X, msg.Y); ok {
			return m.handleActionBarAction(action)
		}
		// then copy/revoke selection, then normal-mode attachment open
	}
}
```

Refresh the action-bar cache in the exact functions that change mode or redraw selection:

```go
func (m *model) refreshViewport(stickToBottom bool) {
	offset := m.viewport.YOffset
	state := m.buildRenderedViewportState()
	m.renderedViewport = state
	m.renderedActionBar = m.buildRenderedActionBarState()
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

func (m *model) enterCopyMode() bool {
	if m.revokeMode {
		m.exitRevokeMode()
	}
	if len(m.copySelection) == 0 {
		m.setStatusNotice("no message to copy", true)
		return false
	}
	m.copyMode = true
	m.setStatusNotice("copy mode", false)
	if m.copySelectionPos < 0 || m.copySelectionPos >= len(m.copySelection) {
		m.copySelectionPos = len(m.copySelection) - 1
	}
	m.followCopySelection = m.copySelectionPos == len(m.copySelection)-1
	m.scrollSelectedMessageIntoView()
	m.renderedActionBar = m.buildRenderedActionBarState()
	return true
}

func (m *model) enterRevokeMode() {
	if m.copyMode {
		m.exitCopyMode()
	}
	m.rebuildRevokeCandidates()
	if len(m.revokeCandidates) == 0 {
		m.addSystemEntry("revoke: no eligible messages")
		return
	}
	m.revokeMode = true
	m.revokeSelection = len(m.revokeCandidates) - 1
	m.renderedActionBar = m.buildRenderedActionBarState()
}
```

- [ ] **Step 4: Run the action tests to verify they pass**

Run:

```bash
go test ./internal/tui -run 'TestModelMouseCopyActionCopiesAndStaysInCopyMode|TestModelMouseQuoteActionQuotesAndExitsCopyMode|TestModelMouseRevokeActionConfirmsAndExits' -count=1
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/model_test.go
git commit -m "Handle TUI mouse clicks on copy and revoke actions"
```

### Task 4: Add Attachment Buttons, Cancel Path, and Regressions

**Files:**
- Modify: `internal/tui/model.go`
- Test: `internal/tui/model_test.go`

- [ ] **Step 1: Write the failing regression tests**

```go
func TestModelMouseAttachmentActionsUseSelectedAttachment(t *testing.T) {
	t.Parallel()

	attachments := &fakeAttachmentClient{
		openPath:     "/tmp/opened/selected.gif",
		downloadPath: "/tmp/downloaded/selected.gif",
	}
	uiModel := newModel(modelOptions{
		mode:             "join",
		uiMode:           uiModeTUI,
		session:          &fakeSession{peerName: "host"},
		attachmentClient: attachments,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 72, Height: 12})
	uiModel = updated.(model)
	uiModel.addHistoryEntry(historyEntry{
		kind: historyKindMessage,
		from: "alice",
		body: attachment.FormatChatMessage(attachment.ChatMessage{
			Version: 1,
			ID:      "att-reg-1",
			Kind:    attachment.KindImage,
			Name:    "selected.gif",
			Size:    6,
		}),
		at: time.Date(2026, 4, 23, 21, 30, 0, 0, time.Local),
	})
	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	uiModel = updated.(model)

	actionBar := uiModel.buildRenderedActionBarState()
	openX := actionBar.buttons[2].startX
	downloadX := actionBar.buttons[3].startX
	clickY := viewportTopRow + uiModel.viewport.Height

	updated, cmd := uiModel.Update(tea.MouseMsg{X: openX, Y: clickY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	uiModel = updated.(model)
	updated, cmd = uiModel.Update(tea.MouseMsg{X: openX, Y: clickY, Button: tea.MouseButtonNone, Action: tea.MouseActionRelease})
	uiModel = updated.(model)
	if cmd == nil {
		t.Fatal("expected open command from action bar")
	}

	updated, cmd = uiModel.Update(tea.MouseMsg{X: downloadX, Y: clickY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	uiModel = updated.(model)
	updated, cmd = uiModel.Update(tea.MouseMsg{X: downloadX, Y: clickY, Button: tea.MouseButtonNone, Action: tea.MouseActionRelease})
	uiModel = updated.(model)
	if cmd == nil {
		t.Fatal("expected download command from action bar")
	}
}

func TestModelMouseCancelActionExitsCopyMode(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:   "join",
		uiMode: uiModeTUI,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 72, Height: 12})
	uiModel = updated.(model)
	uiModel.addHistoryEntry(historyEntry{
		kind: historyKindMessage,
		from: "alice",
		body: "cancel me",
		at:   time.Date(2026, 4, 23, 21, 31, 0, 0, time.Local),
	})
	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	uiModel = updated.(model)

	actionBar := uiModel.buildRenderedActionBarState()
	cancelX := actionBar.buttons[len(actionBar.buttons)-1].startX
	clickY := viewportTopRow + uiModel.viewport.Height

	updated, _ = uiModel.Update(tea.MouseMsg{X: cancelX, Y: clickY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(tea.MouseMsg{X: cancelX, Y: clickY, Button: tea.MouseButtonNone, Action: tea.MouseActionRelease})
	uiModel = updated.(model)

	if uiModel.copyMode {
		t.Fatal("expected cancel action to exit copy mode")
	}
}

func TestModelMouseDragDoesNotTriggerActionBarOrSelection(t *testing.T) {
	t.Parallel()

	uiModel := newModel(modelOptions{
		mode:   "join",
		uiMode: uiModeTUI,
		transcriptOpener: func(string) (transcriptStore, error) {
			return &fakeTranscriptStore{}, nil
		},
	})
	updated, _ := uiModel.Update(tea.WindowSizeMsg{Width: 72, Height: 8})
	uiModel = updated.(model)
	for i := 0; i < 20; i++ {
		uiModel.addHistoryEntry(historyEntry{
			kind: historyKindMessage,
			from: "alice",
			body: fmt.Sprintf("drag-%02d", i),
			at:   time.Date(2026, 4, 23, 21, 32, i, 0, time.Local),
		})
	}
	updated, _ = uiModel.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	uiModel = updated.(model)
	initialSelection := uiModel.selectedCopyHistoryIndex()

	updated, _ = uiModel.Update(tea.MouseMsg{X: 2, Y: 3, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(tea.MouseMsg{X: 2, Y: 5, Button: tea.MouseButtonNone, Action: tea.MouseActionMotion})
	uiModel = updated.(model)
	updated, _ = uiModel.Update(tea.MouseMsg{X: 2, Y: 5, Button: tea.MouseButtonNone, Action: tea.MouseActionRelease})
	uiModel = updated.(model)

	if got := uiModel.selectedCopyHistoryIndex(); got != initialSelection {
		t.Fatalf("expected drag not to change selection, got %d want %d", got, initialSelection)
	}
}
```

- [ ] **Step 2: Run the regression tests to verify they fail**

Run:

```bash
go test ./internal/tui -run 'TestModelMouseAttachmentActionsUseSelectedAttachment|TestModelMouseCancelActionExitsCopyMode|TestModelMouseDragDoesNotTriggerActionBarOrSelection|TestModelMouseClickOpensAttachment' -count=1
```

Expected: FAIL for the new mouse action-bar tests while `TestModelMouseClickOpensAttachment` continues to pass.

- [ ] **Step 3: Write the minimal regression fixes**

Finish the integration in `internal/tui/model.go`:

```go
func (m *model) refreshViewport(stickToBottom bool) {
	offset := m.viewport.YOffset
	state := m.buildRenderedViewportState()
	m.renderedViewport = state
	m.renderedActionBar = m.buildRenderedActionBarState()
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

func (m *model) exitCopyMode() {
	if !m.copyMode {
		return
	}
	m.copyMode = false
	m.setStatusNotice("", false)
	m.renderedActionBar = renderedActionBarState{}
}

func (m *model) exitRevokeMode() {
	if !m.revokeMode {
		return
	}
	m.revokeMode = false
	m.revokeCandidates = nil
	m.revokeSelection = 0
	m.renderedActionBar = renderedActionBarState{}
	m.refreshViewport(false)
}
```

Update `renderInputBox()` hints so the mode text matches the new feature:

```go
if m.copyMode {
	hint = "copy mode: Click message/actions or use Up/Down / Enter quote / Ctrl+Y copy / O open / D download / Esc cancel"
} else if m.revokeMode {
	hint = "revoke mode: Click message/actions or use Up/Down / Enter confirm / Esc cancel"
}
```

Keep the existing normal-mode attachment click path unchanged after copy/revoke branches in `handleMouse`.
That means `clickedAttachment`, `updateHoveredHistoryIndex`, and `handleAttachmentTransferResult` should not be modified for this task unless one of the regression tests proves a concrete break.

- [ ] **Step 4: Run the regression suite**

Run:

```bash
go test ./internal/tui -run 'TestModelMouseAttachmentActionsUseSelectedAttachment|TestModelMouseCancelActionExitsCopyMode|TestModelMouseDragDoesNotTriggerActionBarOrSelection|TestModelMouseClickOpensAttachment|TestModelMouseClickWrappedAttachmentLineOpensAttachment|TestModelMouseClickIgnoredInCopyAndRevokeModes' -count=1
go test ./internal/tui -count=1
go test ./... -count=1
```

Expected:

- First command: PASS
- Second command: PASS
- Third command: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/model_test.go
git commit -m "Finish TUI mouse actions for copy and revoke modes"
```
