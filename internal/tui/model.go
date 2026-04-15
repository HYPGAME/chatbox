package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"chatbox/internal/room"
	"chatbox/internal/session"
	"chatbox/internal/transcript"
)

type sessionClient interface {
	Messages() <-chan session.Message
	Receipts() <-chan session.Receipt
	Done() <-chan struct{}
	Err() error
	Close() error
	PeerName() string
	Send(string) (session.Message, error)
	Resend(session.Message) error
}

type transcriptStore interface {
	Load() ([]transcript.Record, error)
	AppendMessage(transcript.Record) error
	UpdateStatus(messageID, status string) error
}

type connectFunc func(context.Context) (sessionClient, error)
type historyPrinterFunc func([]string) tea.Cmd
type alertNotifierFunc func()

type modelOptions struct {
	mode             string
	uiMode           string
	alertMode        string
	listeningAddr    string
	session          sessionClient
	roomEvents       <-chan room.Event
	peerCount        func() int
	sessionReady     <-chan sessionResult
	connect          connectFunc
	reconnectDelay   time.Duration
	transcriptKey    string
	transcriptOpener func(peerName string) (transcriptStore, error)
	historyPrinter   historyPrinterFunc
	alertNotifier    alertNotifierFunc
}

type sessionResult struct {
	session sessionClient
	err     error
}

type sessionReadyMsg struct {
	session sessionClient
	err     error
}

type incomingMessageMsg struct {
	message session.Message
}

type receiptMsg struct {
	receipt session.Receipt
}

type roomEventMsg struct {
	event room.Event
}

type sessionClosedMsg struct {
	err error
}

type historyKind string

const (
	historyKindMessage historyKind = "message"
	historyKindSystem  historyKind = "system"
	historyKindError   historyKind = "error"

	uiModeTUI        = "tui"
	uiModeScrollback = "scrollback"
	statusRetrying   = "retrying"
)

type historyEntry struct {
	kind      historyKind
	messageID string
	from      string
	body      string
	at        time.Time
	outgoing  bool
	status    string
}

type model struct {
	mode             string
	uiMode           string
	alertMode        string
	listeningAddr    string
	session          sessionClient
	roomEvents       <-chan room.Event
	peerCountValue   int
	sessionReady     <-chan sessionResult
	connect          connectFunc
	reconnectDelay   time.Duration
	transcriptKey    string
	transcriptOpener func(peerName string) (transcriptStore, error)
	historyPrinter   historyPrinterFunc
	alertNotifier    alertNotifierFunc

	transcript                 transcriptStore
	transcriptConversationKey  string
	currentConversationKey     string
	currentPeer                string

	history      []historyEntry
	printedCount int
	entryIndex   map[string]int
	seenMessages map[string]struct{}
	pending      map[string]session.Message

	status string

	viewport viewport.Model
	input    textarea.Model

	width  int
	height int

	draggingViewport bool
	lastMouseY       int
}

var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	inputStyle  = lipgloss.NewStyle().BorderTop(true).BorderForeground(lipgloss.Color("8"))

	bubbleTeaRunner  = runProgram
	scrollbackRunner = runScrollback
)

func RunHost(host *session.Host, localName string, psk []byte, uiMode string, alertMode string) error {
	hostRoom := room.NewHostRoom(localName)
	go hostRoom.Serve(context.Background(), host)

	return runUI(newModel(modelOptions{
		mode:             "host",
		uiMode:           uiMode,
		alertMode:        alertMode,
		listeningAddr:    host.Addr(),
		session:          hostRoom,
		roomEvents:       hostRoom.Events(),
		peerCount:        hostRoom.PeerCount,
		transcriptOpener: defaultTranscriptOpener(localName, psk),
	}))
}

func RunJoin(conn *session.Session, localName string, peerAddr string, cfg session.Config, uiMode string, alertMode string) error {
	return runUI(newModel(modelOptions{
		mode:          "join",
		uiMode:        uiMode,
		alertMode:     alertMode,
		listeningAddr: peerAddr,
		session:       conn,
		connect: func(ctx context.Context) (sessionClient, error) {
			return session.Dial(ctx, peerAddr, cfg)
		},
		transcriptOpener: defaultTranscriptOpener(localName, cfg.PSK),
	}))
}

func defaultTranscriptOpener(localName string, psk []byte) func(string) (transcriptStore, error) {
	return func(peerName string) (transcriptStore, error) {
		baseDir, err := transcript.DefaultBaseDir()
		if err != nil {
			return nil, err
		}
		return transcript.OpenStore(baseDir, localName, peerName, psk)
	}
}

func runProgram(m model) error {
	program := tea.NewProgram(m, programOptionsForMode(m.uiMode)...)
	_, err := program.Run()
	return err
}

func runUI(m model) error {
	if m.uiMode == uiModeScrollback {
		return scrollbackRunner(m)
	}
	return bubbleTeaRunner(m)
}

func newModel(opts modelOptions) model {
	input := textarea.New()
	input.SetWidth(80)
	input.SetHeight(1)
	input.ShowLineNumbers = false
	input.Placeholder = "Type a message or /help"
	input.Focus()

	m := model{
		mode:             opts.mode,
		uiMode:           opts.uiMode,
		alertMode:        opts.alertMode,
		listeningAddr:    opts.listeningAddr,
		session:          opts.session,
		roomEvents:       opts.roomEvents,
		sessionReady:     opts.sessionReady,
		connect:          opts.connect,
		reconnectDelay:   opts.reconnectDelay,
		transcriptKey:    opts.transcriptKey,
		transcriptOpener: opts.transcriptOpener,
		historyPrinter:   opts.historyPrinter,
		alertNotifier:    opts.alertNotifier,
		viewport:         viewport.New(80, 20),
		input:            input,
		entryIndex:       make(map[string]int),
		seenMessages:     make(map[string]struct{}),
		pending:          make(map[string]session.Message),
	}
	if m.uiMode == "" {
		m.uiMode = uiModeTUI
	}
	if m.historyPrinter == nil {
		m.historyPrinter = defaultHistoryPrinter
	}
	m.viewport.MouseWheelEnabled = true
	m.viewport.MouseWheelDelta = 3
	if m.reconnectDelay == 0 {
		m.reconnectDelay = time.Second
	}
	if opts.peerCount != nil {
		m.peerCountValue = opts.peerCount()
	}

	switch {
	case opts.mode == "host" && opts.roomEvents != nil:
		m.status = m.hostStatus()
	case opts.session != nil:
		m.status = fmt.Sprintf("connected to %s", opts.session.PeerName())
		m.currentPeer = opts.session.PeerName()
	case opts.mode == "host":
		m.status = fmt.Sprintf("listening on %s", opts.listeningAddr)
	default:
		m.status = "connecting"
	}

	m.addStartupHints()
	return m
}

func (m model) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, 2)
	switch {
	case m.session != nil:
		cmds = append(cmds, emitSessionReady(m.session))
	case m.connect != nil:
		cmds = append(cmds, attemptConnect(m.connect))
	case m.sessionReady != nil:
		cmds = append(cmds, waitForSessionReady(m.sessionReady))
	}
	if m.roomEvents != nil {
		cmds = append(cmds, waitForRoomEvent(m.roomEvents))
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, m.flushScrollbackCmd()
	case tea.MouseMsg:
		if handled := m.handleMouse(msg); handled {
			return m, nil
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	case sessionReadyMsg:
		return m.handleSessionReady(msg)
	case incomingMessageMsg:
		m.handleIncomingMessage(msg.message)
		return m, tea.Batch(waitForIncomingMessage(m.session), m.flushScrollbackCmd())
	case receiptMsg:
		receiptCmd := m.handleReceipt(msg.receipt)
		return m, tea.Batch(waitForReceipt(m.session), receiptCmd)
	case roomEventMsg:
		return m.handleRoomEvent(msg.event)
	case sessionClosedMsg:
		return m.handleSessionClosed(msg.err)
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.failPendingMessages()
			if m.session != nil {
				_ = m.session.Close()
			}
			return m, tea.Quit
		case tea.KeyEnter:
			text := strings.TrimSpace(m.input.Value())
			m.input.Reset()
			if text == "" {
				return m, nil
			}
			return m.handleSubmit(text)
		case tea.KeyPgUp:
			m.viewport.PageUp()
			return m, nil
		case tea.KeyPgDown:
			m.viewport.PageDown()
			return m, nil
		case tea.KeyHome:
			m.viewport.GotoTop()
			return m, nil
		case tea.KeyEnd:
			m.viewport.GotoBottom()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) View() string {
	header := headerStyle.Render(fmt.Sprintf("chatbox [%s]", m.mode))
	status := statusStyle.Render(m.status)
	if strings.Contains(strings.ToLower(m.status), "disconnected") {
		status = errorStyle.Render(m.status)
	}
	if m.uiMode == uiModeScrollback {
		scrollbackHint := "history: terminal scrollback (use terminal scroll/drag)"
		return strings.Join([]string{
			header,
			status,
			scrollbackHint,
			inputStyle.Render(m.input.View()),
		}, "\n")
	}

	return strings.Join([]string{
		header,
		status,
		m.viewport.View(),
		inputStyle.Render(m.input.View()),
	}, "\n")
}

func (m *model) handleSessionReady(msg sessionReadyMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m.handleReconnectError(msg.err)
	}

	if err := m.bindSession(msg.session); err != nil {
		m.addErrorEntry(err.Error())
	}

	cmds := []tea.Cmd{
		waitForIncomingMessage(m.session),
		waitForReceipt(m.session),
		waitForSessionClose(m.session),
		m.flushScrollbackCmd(),
	}
	return *m, tea.Batch(cmds...)
}

func (m *model) bindSession(conn sessionClient) error {
	peerName := conn.PeerName()
	conversationKey := m.conversationKeyForPeer(peerName)
	conversationChanged := m.currentConversationKey != "" && m.currentConversationKey != conversationKey
	if conversationChanged {
		m.failPendingMessages()
		m.resetConversation()
	}

	if err := m.ensureTranscriptLoaded(conversationKey); err != nil {
		m.addErrorEntry(err.Error())
	}

	m.session = conn
	m.currentConversationKey = conversationKey
	m.currentPeer = peerName
	if m.mode == "host" && m.roomEvents != nil {
		m.status = m.hostStatus()
	} else {
		m.status = fmt.Sprintf("connected to %s", peerName)
		if m.uiMode != uiModeScrollback {
			m.addSystemEntry(m.status)
		}
	}
	m.resendPendingMessages()
	return nil
}

func (m *model) handleReconnectError(err error) (tea.Model, tea.Cmd) {
	if err == nil {
		return *m, nil
	}

	m.addErrorEntry(err.Error())
	switch {
	case m.mode == "host":
		m.status = m.hostStatus()
		return *m, tea.Batch(retryConnectAfter(m.reconnectDelay, m.connect), m.flushScrollbackCmd())
	case m.connect != nil:
		m.status = "reconnecting"
		return *m, tea.Batch(retryConnectAfter(m.reconnectDelay, m.connect), m.flushScrollbackCmd())
	default:
		m.status = "disconnected"
		return *m, m.flushScrollbackCmd()
	}
}

func (m *model) handleSessionClosed(err error) (tea.Model, tea.Cmd) {
	m.session = nil

	switch {
	case m.mode == "host" && m.roomEvents != nil:
		m.status = m.hostStatus()
		if err != nil && err.Error() != "session closed locally" {
			m.addErrorEntry(err.Error())
		}
		return *m, m.flushScrollbackCmd()
	case m.mode == "host" && m.connect != nil:
		m.status = fmt.Sprintf("listening on %s", m.listeningAddr)
		if err != nil && err != context.Canceled && err.Error() != "session closed locally" {
			m.addErrorEntry(err.Error())
		}
		if m.uiMode != uiModeScrollback {
			m.addSystemEntry("waiting for peer")
		}
		return *m, tea.Batch(attemptConnect(m.connect), m.flushScrollbackCmd())
	case m.connect != nil:
		m.status = "reconnecting"
		if err != nil && err.Error() != "session closed locally" {
			m.addErrorEntry(err.Error())
		}
		return *m, tea.Batch(retryConnectAfter(m.reconnectDelay, m.connect), m.flushScrollbackCmd())
	default:
		m.status = "disconnected"
		if err != nil && err.Error() != "session closed locally" {
			m.addErrorEntry(err.Error())
		}
		return *m, m.flushScrollbackCmd()
	}
}

func (m *model) handleRoomEvent(event room.Event) (tea.Model, tea.Cmd) {
	m.peerCountValue = event.PeerCount
	m.status = m.hostStatus()

	switch event.Kind {
	case room.EventPeerJoined:
		m.addSystemEntry(fmt.Sprintf("%s joined", event.PeerName))
	case room.EventPeerLeft:
		m.addSystemEntry(fmt.Sprintf("%s left", event.PeerName))
	}

	cmds := []tea.Cmd{m.flushScrollbackCmd()}
	if m.roomEvents != nil {
		cmds = append(cmds, waitForRoomEvent(m.roomEvents))
	}
	return *m, tea.Batch(cmds...)
}

func (m *model) handleSubmit(text string) (tea.Model, tea.Cmd) {
	if strings.HasPrefix(text, "/") {
		switch text {
		case "/help":
			m.addSystemEntry("commands: /help /status /quit")
			return *m, m.flushScrollbackCmd()
		case "/status":
			m.addSystemEntry(m.status)
			return *m, m.flushScrollbackCmd()
		case "/quit":
			m.failPendingMessages()
			if m.session != nil {
				_ = m.session.Close()
			}
			return *m, tea.Quit
		default:
			m.addErrorEntry("unknown command")
			return *m, m.flushScrollbackCmd()
		}
	}

	if m.session == nil {
		m.addErrorEntry("not connected yet")
		return *m, m.flushScrollbackCmd()
	}

	message, err := m.session.Send(text)
	if err != nil {
		m.addErrorEntry(err.Error())
		return *m, m.flushScrollbackCmd()
	}

	m.pending[message.ID] = message
	m.addMessageEntry(message, true, transcript.StatusSending, true)
	return *m, m.flushScrollbackCmd()
}

func (m *model) handleIncomingMessage(message session.Message) {
	if message.ID != "" {
		if _, ok := m.seenMessages[message.ID]; ok {
			return
		}
	}
	m.addMessageEntry(message, false, transcript.StatusSent, true)
	m.notifyLiveIncomingAlert()
}

func (m *model) handleReceipt(receipt session.Receipt) tea.Cmd {
	index, ok := m.entryIndex[receipt.MessageID]
	if !ok {
		return nil
	}

	entry := m.history[index]
	entry.status = transcript.StatusSent
	m.history[index] = entry
	delete(m.pending, receipt.MessageID)
	if m.transcript != nil {
		_ = m.transcript.UpdateStatus(receipt.MessageID, transcript.StatusSent)
	}
	m.refreshViewport(false)
	return nil
}

func (m *model) resize() {
	if m.width == 0 || m.height == 0 {
		return
	}

	inputHeight := 3
	viewportHeight := m.height - inputHeight - 3
	if viewportHeight < 5 {
		viewportHeight = 5
	}
	if m.width > 4 {
		m.viewport.Width = m.width - 2
		m.input.SetWidth(m.width - 4)
	}
	m.viewport.Height = viewportHeight
	m.refreshViewport(m.viewport.AtBottom())
}

func (m *model) addSystemEntry(text string) {
	m.addHistoryEntry(historyEntry{
		kind: historyKindSystem,
		body: text,
		at:   time.Now(),
	})
}

func (m *model) addErrorEntry(text string) {
	m.addHistoryEntry(historyEntry{
		kind: historyKindError,
		body: text,
		at:   time.Now(),
	})
}

func (m *model) addMessageEntry(message session.Message, outgoing bool, status string, persist bool) {
	entry := historyEntry{
		kind:      historyKindMessage,
		messageID: message.ID,
		from:      message.From,
		body:      message.Body,
		at:        message.At,
		outgoing:  outgoing,
		status:    status,
	}

	m.addHistoryEntry(entry)
	if message.ID != "" {
		m.seenMessages[message.ID] = struct{}{}
		m.entryIndex[message.ID] = len(m.history) - 1
	}

	if persist && m.transcript != nil {
		record := transcript.Record{
			MessageID: message.ID,
			Direction: transcript.DirectionIncoming,
			From:      message.From,
			Body:      message.Body,
			At:        message.At,
			Status:    status,
		}
		if outgoing {
			record.Direction = transcript.DirectionOutgoing
		}
		_ = m.transcript.AppendMessage(record)
	}
}

func (m *model) addHistoryEntry(entry historyEntry) {
	stickToBottom := m.viewport.AtBottom() || len(m.history) == 0
	m.history = append(m.history, entry)
	m.refreshViewport(stickToBottom)
}

func (m *model) refreshViewport(stickToBottom bool) {
	offset := m.viewport.YOffset
	lines := make([]string, 0, len(m.history))
	for _, entry := range m.history {
		lines = append(lines, renderEntry(entry))
	}

	m.viewport.SetContent(strings.Join(lines, "\n"))
	if stickToBottom {
		m.viewport.GotoBottom()
		return
	}
	m.viewport.SetYOffset(offset)
}

func (m *model) handleMouse(msg tea.MouseMsg) bool {
	switch msg.Action {
	case tea.MouseActionPress:
		if msg.Button == tea.MouseButtonLeft && m.isWithinViewport(msg.Y) {
			m.draggingViewport = true
			m.lastMouseY = msg.Y
			return true
		}
	case tea.MouseActionMotion:
		if m.draggingViewport && msg.Button == tea.MouseButtonLeft {
			delta := msg.Y - m.lastMouseY
			if delta > 0 {
				m.viewport.ScrollUp(delta)
			} else if delta < 0 {
				m.viewport.ScrollDown(-delta)
			}
			m.lastMouseY = msg.Y
			return true
		}
	case tea.MouseActionRelease:
		if m.draggingViewport {
			m.draggingViewport = false
			return true
		}
	}
	return false
}

func (m model) isWithinViewport(mouseY int) bool {
	if m.viewport.Height <= 0 {
		return false
	}
	viewportTop := 2
	viewportBottom := viewportTop + m.viewport.Height - 1
	return mouseY >= viewportTop && mouseY <= viewportBottom
}

func (m *model) ensureTranscriptLoaded(conversationKey string) error {
	if m.transcriptOpener == nil {
		return nil
	}
	if m.transcript != nil && m.transcriptConversationKey == conversationKey {
		return nil
	}

	store, err := m.transcriptOpener(conversationKey)
	if err != nil {
		return fmt.Errorf("open transcript: %w", err)
	}
	records, err := store.Load()
	if err != nil {
		return fmt.Errorf("load transcript: %w", err)
	}

	m.transcript = store
	m.transcriptConversationKey = conversationKey
	for _, record := range records {
		if record.Direction == transcript.DirectionOutgoing && record.Status == transcript.StatusSending {
			record.Status = transcript.StatusFailed
			_ = m.transcript.UpdateStatus(record.MessageID, transcript.StatusFailed)
		}
		m.addMessageEntry(session.Message{
			ID:   record.MessageID,
			From: record.From,
			Body: record.Body,
			At:   record.At,
		}, record.Direction == transcript.DirectionOutgoing, record.Status, false)
	}
	return nil
}

func (m *model) resetConversation() {
	m.history = nil
	m.printedCount = 0
	m.entryIndex = make(map[string]int)
	m.seenMessages = make(map[string]struct{})
	m.pending = make(map[string]session.Message)
	m.transcript = nil
	m.transcriptConversationKey = ""
	m.currentConversationKey = ""
	m.currentPeer = ""
	m.addStartupHints()
}

func (m model) conversationKeyForPeer(peerName string) string {
	if key := strings.TrimSpace(m.transcriptKey); key != "" {
		return key
	}
	switch {
	case m.mode == "host" && m.roomEvents != nil && strings.TrimSpace(m.listeningAddr) != "":
		return transcript.HostRoomKey(m.listeningAddr)
	case m.mode == "join" && strings.TrimSpace(m.listeningAddr) != "":
		return transcript.JoinRoomKey(m.listeningAddr)
	default:
		return peerName
	}
}

func (m *model) resendPendingMessages() {
	if m.session == nil {
		return
	}
	for _, message := range m.pending {
		if err := m.session.Resend(message); err != nil {
			m.addErrorEntry(err.Error())
			continue
		}
		m.addRetryEntry(message)
	}
}

func (m *model) addStartupHints() {
	if m.uiMode == uiModeScrollback {
		return
	}
	m.addSystemEntry("commands: /help /status /quit")
}

func (m *model) addRetryEntry(message session.Message) {
	m.addHistoryEntry(historyEntry{
		kind:     historyKindMessage,
		from:     message.From,
		body:     message.Body,
		at:       time.Now(),
		outgoing: true,
		status:   statusRetrying,
	})
}

func (m *model) notifyLiveIncomingAlert() {
	if m.uiMode != uiModeScrollback {
		return
	}
	if m.alertMode != "bell" {
		return
	}
	if m.alertNotifier == nil {
		return
	}
	m.alertNotifier()
}

func (m *model) failPendingMessages() {
	for messageID := range m.pending {
		index, ok := m.entryIndex[messageID]
		if ok {
			entry := m.history[index]
			entry.status = transcript.StatusFailed
			m.history[index] = entry
			if m.transcript != nil {
				_ = m.transcript.UpdateStatus(messageID, transcript.StatusFailed)
			}
		}
	}
	if len(m.pending) > 0 {
		m.refreshViewport(false)
	}
	m.pending = make(map[string]session.Message)
}

func emitSessionReady(conn sessionClient) tea.Cmd {
	return func() tea.Msg {
		return sessionReadyMsg{session: conn}
	}
}

func waitForSessionReady(ready <-chan sessionResult) tea.Cmd {
	return func() tea.Msg {
		result, ok := <-ready
		if !ok {
			return sessionReadyMsg{err: fmt.Errorf("session setup channel closed")}
		}
		return sessionReadyMsg{session: result.session, err: result.err}
	}
}

func attemptConnect(connect connectFunc) tea.Cmd {
	return func() tea.Msg {
		conn, err := connect(context.Background())
		return sessionReadyMsg{session: conn, err: err}
	}
}

func retryConnectAfter(delay time.Duration, connect connectFunc) tea.Cmd {
	return tea.Tick(delay, func(time.Time) tea.Msg {
		conn, err := connect(context.Background())
		return sessionReadyMsg{session: conn, err: err}
	})
}

func waitForIncomingMessage(conn sessionClient) tea.Cmd {
	return func() tea.Msg {
		message, ok := <-conn.Messages()
		if !ok {
			return nil
		}
		return incomingMessageMsg{message: message}
	}
}

func waitForReceipt(conn sessionClient) tea.Cmd {
	return func() tea.Msg {
		receipt, ok := <-conn.Receipts()
		if !ok {
			return nil
		}
		return receiptMsg{receipt: receipt}
	}
}

func waitForRoomEvent(events <-chan room.Event) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			return nil
		}
		return roomEventMsg{event: event}
	}
}

func waitForSessionClose(conn sessionClient) tea.Cmd {
	return func() tea.Msg {
		<-conn.Done()
		return sessionClosedMsg{err: conn.Err()}
	}
}

func renderEntry(entry historyEntry) string {
	return renderEntryWithStatus(entry, entry.status)
}

func renderScrollbackEntry(entry historyEntry) string {
	status := ""
	if entry.outgoing && (entry.status == statusRetrying || entry.status == transcript.StatusFailed) {
		status = entry.status
	}
	return renderEntryWithStatus(entry, status)
}

func renderEntryWithStatus(entry historyEntry, status string) string {
	switch entry.kind {
	case historyKindSystem:
		return fmt.Sprintf("system [%s]: %s", entry.at.Local().Format("2006-01-02 15:04:05"), entry.body)
	case historyKindError:
		return errorStyle.Render(fmt.Sprintf("error [%s]: %s", entry.at.Local().Format("2006-01-02 15:04:05"), entry.body))
	default:
		label := entry.from
		if entry.outgoing {
			label = "you"
		}
		statusSuffix := ""
		if entry.outgoing && status != "" {
			statusSuffix = fmt.Sprintf(" [%s]", status)
		}
		return fmt.Sprintf("[%s] %s: %s%s", entry.at.Local().Format("2006-01-02 15:04:05"), label, entry.body, statusSuffix)
	}
}

func programOptionsForMode(uiMode string) []tea.ProgramOption {
	options := []tea.ProgramOption{}
	if uiModeUsesAltScreen(uiMode) {
		options = append(options, tea.WithAltScreen(), tea.WithMouseCellMotion())
	}
	return options
}

func uiModeUsesAltScreen(uiMode string) bool {
	return uiMode == uiModeTUI
}

func defaultHistoryPrinter(lines []string) tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(lines))
	for _, line := range lines {
		line := line
		cmds = append(cmds, tea.Println(line))
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Sequence(cmds...)
}

func (m model) hostStatus() string {
	return fmt.Sprintf("hosting on %s (%d peers)", m.listeningAddr, m.peerCountValue)
}

func (m *model) flushScrollbackCmd() tea.Cmd {
	if m.uiMode != uiModeScrollback || m.printedCount >= len(m.history) {
		return nil
	}

	lines := make([]string, 0, len(m.history)-m.printedCount)
	for _, entry := range m.history[m.printedCount:] {
		lines = append(lines, renderScrollbackEntry(entry))
	}
	m.printedCount = len(m.history)
	return m.printLines(lines)
}

func (m *model) printLines(lines []string) tea.Cmd {
	if len(lines) == 0 || m.historyPrinter == nil {
		return nil
	}
	return m.historyPrinter(lines)
}
