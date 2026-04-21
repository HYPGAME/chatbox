package tui

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"chatbox/internal/historymeta"
	"chatbox/internal/identity"
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
	peerNames        func() []string
	sessionReady     <-chan sessionResult
	connect          connectFunc
	reconnectDelay   time.Duration
	transcriptKey    string
	transcriptOpener func(peerName string) (transcriptStore, error)
	historyPrinter   historyPrinterFunc
	alertNotifier    alertNotifierFunc
	identityLoader   func() (identity.Store, error)
	roomAuthLoader   func(roomKey, identityID string) (historymeta.Record, error)
	updateNotices    <-chan string
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

type updateNoticeMsg struct {
	text string
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

type slashCommandSuggestion struct {
	command     string
	description string
}

type model struct {
	mode             string
	uiMode           string
	alertMode        string
	listeningAddr    string
	session          sessionClient
	roomEvents       <-chan room.Event
	peerCountValue   int
	peerNames        func() []string
	sessionReady     <-chan sessionResult
	connect          connectFunc
	reconnectDelay   time.Duration
	transcriptKey    string
	transcriptOpener func(peerName string) (transcriptStore, error)
	historyPrinter   historyPrinterFunc
	alertNotifier    alertNotifierFunc
	identityLoader   func() (identity.Store, error)
	roomAuthLoader   func(roomKey, identityID string) (historymeta.Record, error)
	updateNotices    <-chan string

	transcript                transcriptStore
	transcriptConversationKey string
	currentConversationKey    string
	currentPeer               string
	identityID                string
	roomAuthorization         historymeta.Record
	roomEventLog              []room.Event

	history          []historyEntry
	printedCount     int
	entryIndex       map[string]int
	seenMessages     map[string]struct{}
	pending          map[string]session.Message
	syncCapablePeers map[string]bool
	requestedHistory map[string]struct{}

	status string

	viewport viewport.Model
	input    textarea.Model

	width  int
	height int

	draggingViewport bool
	lastMouseY       int
}

var (
	headerStyle          = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	statusStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	errorStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	inputStyle           = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8")).Padding(0, 1)
	inputHintStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#66707A"))
	slashSuggestionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	slashPanelStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8")).Padding(0, 1)
	separatorStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#59636E"))

	senderPalette = []lipgloss.Color{
		"#5C7993",
		"#6A7F5F",
		"#8A6C4A",
		"#7C658A",
		"#5F7F83",
		"#8B5E6D",
		"#6D6F8C",
		"#7D7A5B",
	}

	bubbleTeaRunner  = runProgram
	scrollbackRunner = runScrollback

	slashCommandSuggestions = []slashCommandSuggestion{
		{command: "/help", description: "显示支持的命令"},
		{command: "/status", description: "查询在线成员信息"},
		{command: "/events", description: "查看成员进出记录"},
		{command: "/quit", description: "退出当前会话"},
	}
)

func RunHost(host *session.Host, localName string, psk []byte, uiMode string, alertMode string) error {
	return RunHostWithUpdateNotices(host, localName, psk, uiMode, alertMode, nil)
}

func RunHostWithUpdateNotices(host *session.Host, localName string, psk []byte, uiMode string, alertMode string, updateNotices <-chan string) error {
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
		peerNames:        hostRoom.ParticipantNames,
		transcriptOpener: defaultTranscriptOpener(localName, psk),
		updateNotices:    updateNotices,
	}))
}

func RunJoin(conn *session.Session, localName string, peerAddr string, cfg session.Config, uiMode string, alertMode string) error {
	return RunJoinWithUpdateNotices(conn, localName, peerAddr, cfg, uiMode, alertMode, nil)
}

func RunJoinWithUpdateNotices(conn *session.Session, localName string, peerAddr string, cfg session.Config, uiMode string, alertMode string, updateNotices <-chan string) error {
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
		updateNotices:    updateNotices,
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
		peerNames:        opts.peerNames,
		transcriptKey:    opts.transcriptKey,
		transcriptOpener: opts.transcriptOpener,
		historyPrinter:   opts.historyPrinter,
		alertNotifier:    opts.alertNotifier,
		identityLoader:   opts.identityLoader,
		roomAuthLoader:   opts.roomAuthLoader,
		updateNotices:    opts.updateNotices,
		viewport:         viewport.New(80, 20),
		input:            input,
		entryIndex:       make(map[string]int),
		seenMessages:     make(map[string]struct{}),
		pending:          make(map[string]session.Message),
		syncCapablePeers: make(map[string]bool),
		requestedHistory: make(map[string]struct{}),
	}
	if m.uiMode == "" {
		m.uiMode = uiModeTUI
	}
	if m.historyPrinter == nil {
		m.historyPrinter = defaultHistoryPrinter
	}
	if m.identityLoader == nil {
		m.identityLoader = defaultIdentityLoader
	}
	if m.roomAuthLoader == nil {
		m.roomAuthLoader = defaultRoomAuthorizationLoader
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
	if m.updateNotices != nil {
		cmds = append(cmds, waitForUpdateNotice(m.updateNotices))
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
	case updateNoticeMsg:
		return m.handleUpdateNotice(msg.text)
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
			m.resize()
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
	m.resize()
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

	lines := []string{m.renderStatusBar()}
	lines = append(lines, m.viewport.View())
	if suggestions := m.renderSlashCommandSuggestions(); suggestions != "" {
		lines = append(lines, suggestions)
	}
	lines = append(lines, m.renderInputBox())
	return strings.Join(lines, "\n")
}

func (m *model) handleSessionReady(msg sessionReadyMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m.handleReconnectError(msg.err)
	}

	if err := m.bindSession(msg.session); err != nil {
		m.addErrorEntry(err.Error())
	}
	m.announceHistorySyncCapability()

	cmds := []tea.Cmd{
		waitForIncomingMessage(m.session),
		waitForReceipt(m.session),
		waitForSessionClose(m.session),
		m.flushScrollbackCmd(),
	}
	return *m, tea.Batch(cmds...)
}

func (m *model) bindSession(conn sessionClient) error {
	if err := m.ensureIdentityLoaded(); err != nil {
		return err
	}
	peerName := conn.PeerName()
	conversationKey := m.conversationKeyForPeer(peerName)
	if err := m.ensureRoomAuthorization(conversationKey); err != nil {
		return err
	}
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

func defaultIdentityLoader() (identity.Store, error) {
	baseDir, err := identity.DefaultBaseDir()
	if err != nil {
		return identity.Store{}, err
	}
	return identity.OpenOrCreate(baseDir)
}

func (m *model) ensureIdentityLoaded() error {
	if m.identityID != "" || m.identityLoader == nil {
		return nil
	}
	store, err := m.identityLoader()
	if err != nil {
		return fmt.Errorf("load identity: %w", err)
	}
	m.identityID = store.IdentityID
	return nil
}

func defaultRoomAuthorizationLoader(roomKey, identityID string) (historymeta.Record, error) {
	baseDir, err := historymeta.DefaultBaseDir()
	if err != nil {
		return historymeta.Record{}, err
	}
	return historymeta.OpenOrCreateRoomAuthorization(baseDir, roomKey, identityID, time.Now)
}

func (m *model) ensureRoomAuthorization(conversationKey string) error {
	if m.roomAuthLoader == nil || conversationKey == "" || m.identityID == "" {
		return nil
	}
	if m.roomAuthorization.RoomKey == conversationKey && m.roomAuthorization.IdentityID == m.identityID {
		return nil
	}
	record, err := m.roomAuthLoader(conversationKey, m.identityID)
	if err != nil {
		return fmt.Errorf("load room authorization: %w", err)
	}
	m.roomAuthorization = record
	return nil
}

func (m *model) announceHistorySyncCapability() {
	if m.session == nil || m.identityID == "" || m.roomAuthorization.RoomKey == "" {
		return
	}
	summary := HistorySyncSummaryForRecords(m.history)
	_, _ = m.session.Send(room.HistorySyncHelloBody(room.HistorySyncHello{
		Version:    1,
		IdentityID: m.identityID,
		RoomKey:    m.roomAuthorization.RoomKey,
		Summary:    summary,
	}))
}

func HistorySyncSummaryForRecords(history []historyEntry) room.HistorySyncSummary {
	summary := room.HistorySyncSummary{}
	for _, entry := range history {
		if entry.kind != historyKindMessage {
			continue
		}
		summary.Count++
		if summary.Oldest.IsZero() || entry.at.Before(summary.Oldest) {
			summary.Oldest = entry.at
		}
		if summary.Newest.IsZero() || entry.at.After(summary.Newest) {
			summary.Newest = entry.at
		}
	}
	return summary
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
	m.roomEventLog = append(m.roomEventLog, event)

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

func (m *model) handleUpdateNotice(text string) (tea.Model, tea.Cmd) {
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		m.addSystemEntry(line)
	}

	if m.updateNotices == nil {
		return *m, m.flushScrollbackCmd()
	}
	return *m, tea.Batch(waitForUpdateNotice(m.updateNotices), m.flushScrollbackCmd())
}

func (m *model) handleSubmit(text string) (tea.Model, tea.Cmd) {
	if strings.HasPrefix(text, "/") {
		switch text {
		case "/help":
			m.addSystemEntry("commands: /help /status /events /quit")
			return *m, m.flushScrollbackCmd()
		case "/status":
			m.handleStatusCommand()
			return *m, m.flushScrollbackCmd()
		case "/events":
			m.handleEventsCommand()
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
	if m.handleHistorySyncControl(message) {
		if message.ID != "" {
			m.seenMessages[message.ID] = struct{}{}
		}
		return
	}
	if line, ok := room.ParseStatusResponse(message.Body); ok {
		if message.ID != "" {
			m.seenMessages[message.ID] = struct{}{}
		}
		m.addSystemEntry(line)
		return
	}
	if events, ok := room.ParseEventsResponse(message.Body); ok {
		if message.ID != "" {
			m.seenMessages[message.ID] = struct{}{}
		}
		m.addEventsEntries(events)
		return
	}
	m.addMessageEntry(message, false, transcript.StatusSent, true)
	m.notifyLiveIncomingAlert()
}

func (m *model) handleHistorySyncControl(message session.Message) bool {
	if hello, ok := room.ParseHistorySyncHello(message.Body); ok {
		if hello.IdentityID != "" && strings.TrimSpace(message.From) != "" {
			m.syncCapablePeers[message.From] = true
		}
		m.maybeOfferHistorySync(hello)
		return true
	}
	if offer, ok := room.ParseHistorySyncOffer(message.Body); ok {
		m.maybeRequestHistorySync(offer)
		return true
	}
	if request, ok := room.ParseHistorySyncRequest(message.Body); ok {
		m.maybeSendHistorySyncChunk(request)
		return true
	}
	if chunk, ok := room.ParseHistorySyncChunk(message.Body); ok {
		m.replayHistorySyncChunk(chunk)
		return true
	}
	if room.IsHistorySyncControl(message.Body) {
		return true
	}
	return false
}

func (m *model) maybeOfferHistorySync(hello room.HistorySyncHello) {
	if m.session == nil || m.identityID == "" || m.roomAuthorization.RoomKey == "" {
		return
	}
	if hello.IdentityID == "" || hello.RoomKey != m.roomAuthorization.RoomKey {
		return
	}
	summary := HistorySyncSummaryForRecords(m.history)
	if !historySummaryHasMore(summary, hello.Summary) {
		return
	}
	_, _ = m.session.Send(room.HistorySyncOfferBody(room.HistorySyncOffer{
		Version:        1,
		SourceIdentity: m.identityID,
		TargetIdentity: hello.IdentityID,
		RoomKey:        m.roomAuthorization.RoomKey,
		Summary:        summary,
	}))
}

func (m *model) maybeRequestHistorySync(offer room.HistorySyncOffer) {
	if m.session == nil || m.identityID == "" || m.roomAuthorization.RoomKey == "" {
		return
	}
	if offer.TargetIdentity != m.identityID || offer.RoomKey != m.roomAuthorization.RoomKey {
		return
	}
	sourceIdentity := strings.TrimSpace(offer.SourceIdentity)
	if sourceIdentity == "" {
		return
	}
	if _, ok := m.requestedHistory[sourceIdentity]; ok {
		if !historySummaryHasMore(offer.Summary, HistorySyncSummaryForRecords(m.history)) {
			return
		}
	}
	_, _ = m.session.Send(room.HistorySyncRequestBody(room.HistorySyncRequest{
		Version:        1,
		SourceIdentity: sourceIdentity,
		TargetIdentity: m.identityID,
		RoomKey:        m.roomAuthorization.RoomKey,
		Since:          m.roomAuthorization.JoinedAt,
	}))
	m.requestedHistory[sourceIdentity] = struct{}{}
}

func historySummaryHasMore(candidate room.HistorySyncSummary, current room.HistorySyncSummary) bool {
	if candidate.Count > current.Count {
		return true
	}
	if current.Newest.IsZero() {
		return candidate.Count > 0 || !candidate.Newest.IsZero()
	}
	if !candidate.Newest.IsZero() && candidate.Newest.After(current.Newest) {
		return true
	}
	return false
}

func (m *model) maybeSendHistorySyncChunk(request room.HistorySyncRequest) {
	if m.session == nil || m.identityID == "" || m.roomAuthorization.RoomKey == "" {
		return
	}
	if request.SourceIdentity != m.identityID || request.RoomKey != m.roomAuthorization.RoomKey {
		return
	}

	records := make([]transcript.Record, 0)
	for _, entry := range m.history {
		if entry.kind != historyKindMessage || entry.messageID == "" || entry.at.Before(request.Since) {
			continue
		}
		records = append(records, transcript.Record{
			MessageID: entry.messageID,
			Direction: transcript.DirectionIncoming,
			From:      entry.from,
			Body:      entry.body,
			At:        entry.at,
			Status:    transcript.StatusSent,
		})
	}
	if len(records) == 0 {
		return
	}
	_, _ = m.session.Send(room.HistorySyncChunkBody(room.HistorySyncChunk{
		Version:        1,
		SourceIdentity: m.identityID,
		TargetIdentity: request.TargetIdentity,
		RoomKey:        m.roomAuthorization.RoomKey,
		Records:        records,
	}))
}

func (m *model) replayHistorySyncChunk(chunk room.HistorySyncChunk) {
	if m.identityID == "" || m.roomAuthorization.RoomKey == "" {
		return
	}
	if chunk.TargetIdentity != m.identityID || chunk.RoomKey != m.roomAuthorization.RoomKey {
		return
	}

	added := 0
	for _, record := range chunk.Records {
		if record.MessageID == "" || record.At.Before(m.roomAuthorization.JoinedAt) {
			continue
		}
		if _, ok := m.seenMessages[record.MessageID]; ok {
			continue
		}
		if _, ok := m.entryIndex[record.MessageID]; ok {
			continue
		}
		if m.hasEquivalentHistoryMessage(record) {
			m.seenMessages[record.MessageID] = struct{}{}
			continue
		}
		message := session.Message{
			ID:   record.MessageID,
			From: record.From,
			Body: record.Body,
			At:   record.At,
		}
		m.addHistoricalMessageEntry(message, record.Direction == transcript.DirectionOutgoing, transcript.StatusSent, true)
		m.seenMessages[record.MessageID] = struct{}{}
		added++
	}
	if added > 0 {
		m.addSystemEntry(fmt.Sprintf("history synced: %d messages", added))
	}
}

func (m model) hasEquivalentHistoryMessage(record transcript.Record) bool {
	for _, entry := range m.history {
		if entry.kind != historyKindMessage {
			continue
		}
		if entry.from != record.From {
			continue
		}
		if entry.body != record.Body {
			continue
		}
		if !timestampsEquivalent(entry.at, record.At) {
			continue
		}
		outgoing := record.Direction == transcript.DirectionOutgoing
		if entry.outgoing != outgoing {
			continue
		}
		return true
	}
	return false
}

func timestampsEquivalent(left, right time.Time) bool {
	diff := left.Sub(right)
	if diff < 0 {
		diff = -diff
	}
	return diff <= 3*time.Second
}

func (m *model) handleStatusCommand() {
	m.addSystemEntry(m.status)
	if m.peerNames != nil {
		m.addSystemEntry(room.FormatOnlineStatus(m.peerNames()))
		return
	}
	if m.session == nil {
		return
	}
	if _, err := m.session.Send(room.StatusRequestBody()); err != nil {
		m.addErrorEntry(err.Error())
	}
}

func (m *model) handleEventsCommand() {
	if m.mode == "host" && m.roomEvents != nil {
		m.addEventsEntries(m.roomEventLog)
		return
	}
	if m.session == nil {
		m.addErrorEntry("not connected yet")
		return
	}
	if _, err := m.session.Send(room.EventsRequestBody()); err != nil {
		m.addErrorEntry(err.Error())
	}
}

func (m *model) addEventsEntries(events []room.Event) {
	if len(events) == 0 {
		m.addSystemEntry("events: none")
		return
	}
	for _, event := range events {
		action := ""
		switch event.Kind {
		case room.EventPeerJoined:
			action = "joined"
		case room.EventPeerLeft:
			action = "left"
		default:
			continue
		}
		m.addSystemEntry(fmt.Sprintf("events: %s %s at %s", event.PeerName, action, event.At.Local().Format("2006-01-02 15:04:05")))
	}
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

	inputHeight := 5
	suggestionHeight := 0
	if len(m.activeSlashCommandSuggestions()) > 0 {
		suggestionHeight = len(m.activeSlashCommandSuggestions()) + 2
	}
	viewportHeight := m.height - inputHeight - 1 - suggestionHeight
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

func (m model) activeSlashCommandSuggestions() []slashCommandSuggestion {
	if m.uiMode != uiModeTUI {
		return nil
	}

	value := strings.TrimSpace(m.input.Value())
	if !strings.HasPrefix(value, "/") {
		return nil
	}

	matches := make([]slashCommandSuggestion, 0, len(slashCommandSuggestions))
	for _, suggestion := range slashCommandSuggestions {
		if value == "/" || strings.HasPrefix(suggestion.command, value) {
			matches = append(matches, suggestion)
		}
	}
	return matches
}

func (m model) renderSlashCommandSuggestions() string {
	suggestions := m.activeSlashCommandSuggestions()
	if len(suggestions) == 0 {
		return ""
	}

	lines := make([]string, 0, len(suggestions))
	lines = append(lines, headerStyle.Render("commands"))
	for _, suggestion := range suggestions {
		lines = append(lines, slashSuggestionStyle.Render(fmt.Sprintf("%s -- %s", suggestion.command, suggestion.description)))
	}
	return slashPanelStyle.Width(max(20, m.viewport.Width-4)).Render(strings.Join(lines, "\n"))
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
		m.reindexHistoryMessageIDs()
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

func (m *model) addHistoricalMessageEntry(message session.Message, outgoing bool, status string, persist bool) {
	entry := historyEntry{
		kind:      historyKindMessage,
		messageID: message.ID,
		from:      message.From,
		body:      message.Body,
		at:        message.At,
		outgoing:  outgoing,
		status:    status,
	}

	m.insertHistoryEntryChronologically(entry)
	if message.ID != "" {
		m.seenMessages[message.ID] = struct{}{}
		m.reindexHistoryMessageIDs()
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

func (m *model) insertHistoryEntryChronologically(entry historyEntry) {
	stickToBottom := m.viewport.AtBottom() || len(m.history) == 0
	index := len(m.history)
	for i, existing := range m.history {
		if existing.at.After(entry.at) {
			index = i
			break
		}
	}
	m.history = append(m.history, historyEntry{})
	copy(m.history[index+1:], m.history[index:])
	m.history[index] = entry
	m.refreshViewport(stickToBottom)
}

func (m *model) reindexHistoryMessageIDs() {
	m.entryIndex = make(map[string]int, len(m.history))
	for i, entry := range m.history {
		if entry.messageID != "" {
			m.entryIndex[entry.messageID] = i
		}
	}
}

func (m *model) refreshViewport(stickToBottom bool) {
	offset := m.viewport.YOffset
	lines := make([]string, 0, len(m.history)*2)
	lastDate := ""
	for _, entry := range m.history {
		entryDate := entry.at.Local().Format("2006-01-02")
		if entryDate != lastDate {
			lines = append(lines, renderDateSeparator(entryDate))
			lastDate = entryDate
		}
		lines = append(lines, renderTUIEntry(entry))
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
		m.addHistoricalMessageEntry(session.Message{
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
	m.requestedHistory = make(map[string]struct{})
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
	m.addSystemEntry("commands: /help /status /events /quit")
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

func waitForUpdateNotice(notices <-chan string) tea.Cmd {
	return func() tea.Msg {
		text, ok := <-notices
		if !ok {
			return nil
		}
		return updateNoticeMsg{text: text}
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
	timestamp := entry.at.Local().Format("2006-01-02 15:04:05")
	switch entry.kind {
	case historyKindSystem:
		return systemLineStyle().Render(fmt.Sprintf("system [%s]: %s", timestamp, entry.body))
	case historyKindError:
		return historyErrorStyle().Render(fmt.Sprintf("error [%s]: %s", timestamp, entry.body))
	default:
		label := entry.from
		statusSuffix := ""
		if entry.outgoing && status != "" {
			statusSuffix = fmt.Sprintf(" [%s]", status)
		}
		coloredLabel := senderMessageStyle(label).Render(label)
		coloredTimestamp := timestampStyle().Render(fmt.Sprintf("[%s]", timestamp))
		return fmt.Sprintf("%s %s: %s%s", coloredTimestamp, coloredLabel, entry.body, statusSuffix)
	}
}

func renderTUIEntry(entry historyEntry) string {
	timestamp := entry.at.Local().Format("15:04")
	switch entry.kind {
	case historyKindSystem:
		return systemLineStyle().Render(fmt.Sprintf("system [%s]: %s", timestamp, entry.body))
	case historyKindError:
		return historyErrorStyle().Render(fmt.Sprintf("error [%s]: %s", timestamp, entry.body))
	default:
		statusSuffix := ""
		if entry.outgoing && entry.status != "" && entry.status != transcript.StatusSent {
			statusSuffix = fmt.Sprintf(" [%s]", entry.status)
		}
		coloredLabel := senderMessageStyle(entry.from).Render(entry.from)
		coloredTimestamp := timestampStyle().Render(fmt.Sprintf("[%s]", timestamp))
		return fmt.Sprintf("%s %s: %s%s", coloredTimestamp, coloredLabel, entry.body, statusSuffix)
	}
}

func renderDateSeparator(date string) string {
	return separatorStyle.Render(fmt.Sprintf("--- %s ---", date))
}

func (m model) renderStatusBar() string {
	status := m.status
	style := statusStyle
	if strings.Contains(strings.ToLower(status), "disconnected") {
		style = errorStyle
	}
	return fmt.Sprintf("%s %s %s %s", headerStyle.Render("chatbox "+m.mode), timestampStyle().Render("|"), style.Render(status), timestampStyle().Render("| /help"))
}

func (m model) renderInputBox() string {
	content := strings.Join([]string{
		m.input.View(),
		inputHintStyle.Render("Enter send / Esc quit"),
	}, "\n")
	return inputStyle.Render(content)
}

func timestampStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
}

func systemLineStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#66707A"))
}

func historyErrorStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#8A666A"))
}

func senderMessageStyle(sender string) lipgloss.Style {
	normalized := strings.TrimSpace(strings.ToLower(sender))
	if normalized == "" || len(senderPalette) == 0 {
		return lipgloss.NewStyle()
	}

	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(normalized))
	return lipgloss.NewStyle().Foreground(senderPalette[int(hasher.Sum32()%uint32(len(senderPalette)))])
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
