package tui

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"chatbox/internal/admins"
	"chatbox/internal/attachment"
	"chatbox/internal/historymeta"
	"chatbox/internal/hosthistory"
	"chatbox/internal/identity"
	"chatbox/internal/room"
	"chatbox/internal/session"
	"chatbox/internal/transcript"
	"chatbox/internal/update"
	"chatbox/internal/version"
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
	AppendRevoke(transcript.RevokeRecord) error
	UpdateStatus(messageID, status string) error
}

type connectFunc func(context.Context) (sessionClient, error)
type historyPrinterFunc func([]string) tea.Cmd
type alertNotifierFunc func()
type updatePerformerFunc func(context.Context, string) (update.Outcome, error)
type restartLauncherFunc func(update.RestartSpec) error
type executablePathFunc func() (string, error)

var defaultAlertNotifierFactory = func() alertNotifierFunc {
	return newTerminalBellAlertNotifier(func() {
		_, _ = fmt.Fprint(os.Stdout, "\a")
	})
}

type updateRequestSubmitter interface {
	SubmitUpdateRequest(room.UpdateRequest) error
}

type modelOptions struct {
	mode             string
	uiMode           string
	alertMode        string
	localName        string
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
	startupArgs      []string
	updatePerformer  updatePerformerFunc
	restartLauncher  restartLauncherFunc
	executablePath   executablePathFunc
	attachmentClient attachmentClient
	clipboardReader  clipboardReaderFunc
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

type roomUpdatePerformedMsg struct {
	execute room.UpdateExecute
	outcome update.Outcome
	err     error
}

type historyKind string

const (
	historyKindMessage historyKind = "message"
	historyKindSystem  historyKind = "system"
	historyKindError   historyKind = "error"

	uiModeTUI        = "tui"
	uiModeScrollback = "scrollback"
	statusRetrying   = "retrying"
	viewportTopRow   = 1
)

type historyEntry struct {
	kind           historyKind
	messageID      string
	from           string
	body           string
	authorIdentity string
	at             time.Time
	outgoing       bool
	status         string
	revoked        bool
	revokedAt      time.Time
}

type renderedHistoryLine struct {
	text         string
	historyIndex int
	attachmentID string
	clickable    bool
}

type renderedViewportState struct {
	lines      []renderedHistoryLine
	lineRanges map[int][2]int
}

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

type mouseViewportPress struct {
	x          int
	y          int
	inViewport bool
}

type attachmentFeedbackState int

const (
	attachmentFeedbackNone attachmentFeedbackState = iota
	attachmentFeedbackHover
	attachmentFeedbackClick
)

type slashCommandSuggestion struct {
	command     string
	description string
}

type model struct {
	mode             string
	uiMode           string
	alertMode        string
	localName        string
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
	startupArgs      []string
	updatePerformer  updatePerformerFunc
	restartLauncher  restartLauncherFunc
	executablePath   executablePathFunc

	transcript                transcriptStore
	transcriptConversationKey string
	currentConversationKey    string
	currentPeer               string
	identityID                string
	roomAuthorization         historymeta.Record
	roomEventLog              []room.Event

	history                []historyEntry
	printedCount           int
	pendingScrollbackLines []string
	entryIndex             map[string]int
	seenMessages           map[string]struct{}
	pending                map[string]session.Message
	pendingRevokes         map[string][]transcript.RevokeRecord
	peerIdentities         map[string]string
	syncCapablePeers       map[string]bool
	requestedHistory       map[string]struct{}
	offeredHistory         map[string]struct{}
	executedRoomUpdates    map[string]struct{}
	updateStatuses         map[string]map[string]string

	status string

	viewport viewport.Model
	input    textarea.Model

	width  int
	height int

	draggingViewport        bool
	lastMouseY              int
	pendingViewportPress    *mouseViewportPress
	hoveredHistoryIndex     int
	activeClickHistoryIndex int

	copyMode            bool
	copySelection       []int
	copySelectionPos    int
	followCopySelection bool
	clipboardWriter     clipboardWriterFunc
	statusNotice        string
	statusNoticeIsError bool
	renderedViewport    renderedViewportState
	renderedActionBar   renderedActionBarState
	replyDraft          *replyDraft
	renderedReplyBar    renderedReplyBarState

	revokeMode       bool
	revokeCandidates []int
	revokeSelection  int

	pendingRestart *update.RestartSpec

	attachmentClient       attachmentClient
	clipboardReader        clipboardReaderFunc
	operationNotice        string
	operationNoticeIsError bool
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
	attachmentHoverStyle = lipgloss.NewStyle().Background(lipgloss.Color("#2B343C")).Underline(true)
	attachmentClickStyle = lipgloss.NewStyle().Background(lipgloss.Color("#364049")).Underline(true)
	compactReplyPattern  = regexp.MustCompile(`^> (.+) \[([0-9]{2}:[0-9]{2})\] (.+)$`)

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

	tuiCommandsHelp        = "commands: /help /status /events /quit /file /open /download /update-all | Ctrl+V attach / Ctrl+Y copy / Ctrl+R revoke"
	scrollbackCommandsHelp = "commands: /help /status /events /quit /file /open /download /update-all"

	slashCommandSuggestions = []slashCommandSuggestion{
		{command: "/help", description: "显示支持的命令"},
		{command: "/status", description: "查询在线成员信息"},
		{command: "/events", description: "查看成员进出记录"},
		{command: "/file", description: "上传图片或文件"},
		{command: "/open", description: "打开附件"},
		{command: "/download", description: "下载附件到本地"},
		{command: "/quit", description: "退出当前会话"},
		{command: "/update-all", description: "触发全房间更新，可选指定版本"},
	}
)

func RunHost(host *session.Host, localName string, psk []byte, transcriptKey string, uiMode string, alertMode string) error {
	return RunHostWithUpdateNotices(host, localName, psk, transcriptKey, uiMode, alertMode, nil)
}

func RunHostWithUpdateNotices(host *session.Host, localName string, psk []byte, transcriptKey string, uiMode string, alertMode string, updateNotices <-chan string) error {
	hostRoom := room.NewHostRoom(localName)
	hostRoom.ConfigureUpdates(loadHostAdminStore(), defaultRoomReleaseResolver)
	if baseDir, err := hosthistory.DefaultBaseDir(); err == nil {
		if retained, openErr := hosthistory.OpenStore(baseDir, psk); openErr == nil {
			if metaDir, metaErr := historymeta.DefaultBaseDir(); metaErr == nil {
				hostRoom.ConfigureHistoryRetention(retained, hostRetentionRoomKey(host.Addr(), transcriptKey), func(roomKey, identityID string) (historymeta.Record, error) {
					return historymeta.OpenOrCreateFirstSeenRecord(metaDir, roomKey, identityID, time.Now)
				})
				_, _ = retained.CleanupExpired(time.Now())
			}
		}
	}
	go hostRoom.Serve(context.Background(), host)

	return runUI(newModel(modelOptions{
		mode:             "host",
		uiMode:           uiMode,
		alertMode:        alertMode,
		localName:        localName,
		listeningAddr:    host.Addr(),
		session:          hostRoom,
		roomEvents:       hostRoom.Events(),
		peerCount:        hostRoom.PeerCount,
		peerNames:        hostRoom.ParticipantNames,
		transcriptKey:    transcriptKey,
		transcriptOpener: defaultTranscriptOpener(localName, psk),
		updateNotices:    updateNotices,
		attachmentClient: newAttachmentClientForHost(host.Addr(), psk),
	}))
}

func RunJoin(conn *session.Session, localName string, peerAddr string, cfg session.Config, transcriptKey string, uiMode string, alertMode string) error {
	return RunJoinWithUpdateNotices(conn, localName, peerAddr, cfg, transcriptKey, uiMode, alertMode, nil)
}

func RunJoinWithUpdateNotices(conn *session.Session, localName string, peerAddr string, cfg session.Config, transcriptKey string, uiMode string, alertMode string, updateNotices <-chan string) error {
	return runUI(newModel(modelOptions{
		mode:          "join",
		uiMode:        uiMode,
		alertMode:     alertMode,
		localName:     localName,
		listeningAddr: peerAddr,
		session:       conn,
		connect: func(ctx context.Context) (sessionClient, error) {
			return session.Dial(ctx, peerAddr, cfg)
		},
		transcriptKey:    transcriptKey,
		transcriptOpener: defaultTranscriptOpener(localName, cfg.PSK),
		updateNotices:    updateNotices,
		startupArgs:      append([]string(nil), os.Args[1:]...),
		attachmentClient: newAttachmentClientForPeer(peerAddr, cfg.PSK),
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

func hostRetentionRoomKey(listenAddr, transcriptKey string) string {
	if strings.TrimSpace(transcriptKey) != "" {
		return transcriptKey
	}
	return transcript.JoinRoomKey(listenAddr)
}

func defaultUpdatePerformer(ctx context.Context, targetVersion string) (update.Outcome, error) {
	return update.Client{
		Repository:     "HYPGAME/chatbox",
		CurrentVersion: version.Version,
	}.PerformUpdate(ctx, targetVersion)
}

func defaultRoomReleaseResolver(ctx context.Context, _ string) (string, error) {
	release, err := update.Client{
		Repository:     "HYPGAME/chatbox",
		CurrentVersion: version.Version,
	}.LatestRelease(ctx)
	if err != nil {
		return "", err
	}
	return release.TagName, nil
}

func loadHostAdminStore() admins.Store {
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return admins.Store{AllowedUpdateIdentities: map[string]struct{}{}}
	}
	store, err := admins.Load(filepath.Join(configRoot, "chatbox", "admins.json"))
	if err != nil {
		return admins.Store{AllowedUpdateIdentities: map[string]struct{}{}}
	}
	return store
}

func runProgram(m model) error {
	program := tea.NewProgram(m, programOptionsForMode(m.uiMode)...)
	finalModel, err := program.Run()
	if err != nil {
		return err
	}
	return launchPendingRestartIfNeeded(finalModel)
}

func runUI(m model) error {
	if m.uiMode == uiModeScrollback {
		return scrollbackRunner(m)
	}
	if m.alertNotifier == nil && m.alertMode == "bell" {
		m.alertNotifier = defaultAlertNotifierFactory()
	}
	return bubbleTeaRunner(m)
}

func launchPendingRestartIfNeeded(finalModel tea.Model) error {
	if finalModel == nil {
		return nil
	}
	m, ok := finalModel.(model)
	if !ok {
		return nil
	}
	if m.pendingRestart == nil {
		return nil
	}
	return m.restartLauncher(*m.pendingRestart)
}

func newModel(opts modelOptions) model {
	input := textarea.New()
	input.SetWidth(80)
	input.SetHeight(1)
	input.ShowLineNumbers = false
	input.Placeholder = "Type a message or /help"
	input.Focus()

	m := model{
		mode:                    opts.mode,
		uiMode:                  opts.uiMode,
		alertMode:               opts.alertMode,
		localName:               opts.localName,
		listeningAddr:           opts.listeningAddr,
		session:                 opts.session,
		roomEvents:              opts.roomEvents,
		sessionReady:            opts.sessionReady,
		connect:                 opts.connect,
		reconnectDelay:          opts.reconnectDelay,
		peerNames:               opts.peerNames,
		transcriptKey:           opts.transcriptKey,
		transcriptOpener:        opts.transcriptOpener,
		historyPrinter:          opts.historyPrinter,
		alertNotifier:           opts.alertNotifier,
		identityLoader:          opts.identityLoader,
		roomAuthLoader:          opts.roomAuthLoader,
		updateNotices:           opts.updateNotices,
		startupArgs:             append([]string(nil), opts.startupArgs...),
		updatePerformer:         opts.updatePerformer,
		restartLauncher:         opts.restartLauncher,
		executablePath:          opts.executablePath,
		attachmentClient:        opts.attachmentClient,
		clipboardReader:         opts.clipboardReader,
		viewport:                viewport.New(80, 20),
		input:                   input,
		copySelectionPos:        -1,
		hoveredHistoryIndex:     -1,
		activeClickHistoryIndex: -1,
		followCopySelection:     true,
		entryIndex:              make(map[string]int),
		seenMessages:            make(map[string]struct{}),
		pending:                 make(map[string]session.Message),
		pendingRevokes:          make(map[string][]transcript.RevokeRecord),
		peerIdentities:          make(map[string]string),
		syncCapablePeers:        make(map[string]bool),
		requestedHistory:        make(map[string]struct{}),
		offeredHistory:          make(map[string]struct{}),
		executedRoomUpdates:     make(map[string]struct{}),
		updateStatuses:          make(map[string]map[string]string),
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
	if len(m.startupArgs) == 0 {
		m.startupArgs = append([]string(nil), os.Args[1:]...)
	}
	if m.updatePerformer == nil {
		m.updatePerformer = defaultUpdatePerformer
	}
	if m.restartLauncher == nil {
		m.restartLauncher = update.LaunchRestart
	}
	if m.executablePath == nil {
		m.executablePath = os.Executable
	}
	if m.clipboardWriter == nil {
		m.clipboardWriter = defaultClipboardWriter()
	}
	if m.clipboardReader == nil {
		m.clipboardReader = defaultClipboardReader()
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
		if handled, cmd := m.handleMouse(msg); handled {
			return m, cmd
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	case sessionReadyMsg:
		return m.handleSessionReady(msg)
	case incomingMessageMsg:
		messageCmd := m.handleIncomingMessage(msg.message)
		return m, tea.Batch(waitForIncomingMessage(m.session), messageCmd, m.flushScrollbackCmd())
	case receiptMsg:
		receiptCmd := m.handleReceipt(msg.receipt)
		return m, tea.Batch(waitForReceipt(m.session), receiptCmd)
	case roomEventMsg:
		return m.handleRoomEvent(msg.event)
	case updateNoticeMsg:
		return m.handleUpdateNotice(msg.text)
	case roomUpdatePerformedMsg:
		return m.handleRoomUpdatePerformed(msg)
	case attachmentStreamMsg:
		return m.handleAttachmentStream(msg)
	case sessionClosedMsg:
		return m.handleSessionClosed(msg.err)
	case tea.KeyMsg:
		if m.revokeMode {
			return m.handleRevokeKey(msg)
		}
		if m.copyMode {
			return m.handleCopyModeKey(msg)
		}
		if msg.Type == tea.KeyEsc && m.clearReplyDraft() {
			return m, nil
		}
		if msg.Paste {
			if handledModel, handledCmd, handled := m.handleAttachmentPaste(msg); handled {
				return handledModel, handledCmd
			}
		}
		switch msg.Type {
		case tea.KeyCtrlC:
			m.failPendingMessages()
			if m.session != nil {
				_ = m.session.Close()
			}
			return m, tea.Quit
		case tea.KeyCtrlV:
			return m.startPasteCommand()
		case tea.KeyCtrlR:
			m.enterRevokeMode()
			return m, nil
		case tea.KeyCtrlY:
			if m.uiMode == uiModeTUI {
				if m.enterCopyMode() {
					m.refreshViewport(false)
				}
				return m, nil
			}
		case tea.KeyEnter:
			return m.submitInput()
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

func (m *model) handleSessionReady(msg sessionReadyMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m.handleReconnectError(msg.err)
	}

	if err := m.bindSession(msg.session); err != nil {
		m.addErrorEntry(err.Error())
	}
	m.announceClientVersion()
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
	m.requestedHistory = make(map[string]struct{})
	m.offeredHistory = make(map[string]struct{})
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

func (m *model) announceClientVersion() {
	if m.mode != "join" || m.session == nil {
		return
	}
	_, _ = m.session.Send(room.VersionAnnounceBody(room.VersionAnnounce{
		Version:       1,
		ClientVersion: version.Version,
	}))
}

func (m *model) announceHistorySyncCapability() {
	if m.session == nil || m.identityID == "" || m.roomAuthorization.RoomKey == "" {
		return
	}
	summary := HistorySyncSummaryForRecords(m.history)
	_, _ = m.session.Send(room.HistorySyncHelloBody(room.HistorySyncHello{
		Version:       1,
		IdentityID:    m.identityID,
		ClientVersion: version.Version,
		RoomKey:       m.roomAuthorization.RoomKey,
		Summary:       summary,
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
		command, remainder := splitCommandRemainder(text)
		if command == "" {
			return *m, nil
		}
		switch command {
		case "/help":
			m.addSystemEntry(tuiCommandsHelp)
			return *m, m.flushScrollbackCmd()
		case "/status":
			m.handleStatusCommand()
			return *m, m.flushScrollbackCmd()
		case "/events":
			m.handleEventsCommand()
			return *m, m.flushScrollbackCmd()
		case "/file":
			return m.startAttachCommand(remainder)
		case "/attach":
			return m.startAttachCommand(remainder)
		case "/paste":
			return m.startPasteCommand()
		case "/open":
			return m.startOpenCommand(remainder)
		case "/download":
			attachmentID, destPath := splitFirstToken(remainder)
			return m.startDownloadCommand(attachmentID, destPath)
		case "/quit":
			m.failPendingMessages()
			if m.session != nil {
				_ = m.session.Close()
			}
			return *m, tea.Quit
		case "/update-all":
			if strings.TrimSpace(remainder) == "" {
				return m.handleUpdateAllCommand(nil)
			}
			return m.handleUpdateAllCommand([]string{strings.TrimSpace(remainder)})
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

func (m *model) handleRevokeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlY:
		if m.enterCopyMode() {
			m.refreshViewport(false)
		}
		return *m, nil
	case tea.KeyEsc:
		m.exitRevokeMode()
		return *m, nil
	case tea.KeyUp:
		if m.revokeSelection > 0 {
			m.revokeSelection--
			m.refreshViewport(false)
		}
		return *m, nil
	case tea.KeyDown:
		if m.revokeSelection+1 < len(m.revokeCandidates) {
			m.revokeSelection++
			m.refreshViewport(false)
		}
		return *m, nil
	case tea.KeyEnter:
		return m.confirmSelectedRevoke()
	default:
		return *m, nil
	}
}

func (m *model) handleCopyModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.exitCopyMode()
		m.refreshViewport(false)
		return *m, nil
	case tea.KeyUp:
		m.moveCopySelection(-1)
		m.refreshViewport(false)
		return *m, nil
	case tea.KeyDown:
		m.moveCopySelection(1)
		m.refreshViewport(false)
		return *m, nil
	case tea.KeyCtrlY:
		m.copySelectedMessage()
		return *m, nil
	case tea.KeyCtrlR:
		m.enterRevokeMode()
		return *m, nil
	case tea.KeyRunes:
		if len(msg.Runes) == 1 {
			switch msg.Runes[0] {
			case 'o', 'O':
				attachmentMsg, ok := m.selectedAttachmentMessage()
				if !ok {
					m.setStatusNotice("selected message is not an attachment", true)
					return *m, nil
				}
				return m.startOpenCommand(attachmentMsg.ID)
			case 'd', 'D':
				attachmentMsg, ok := m.selectedAttachmentMessage()
				if !ok {
					m.setStatusNotice("selected message is not an attachment", true)
					return *m, nil
				}
				return m.startDownloadCommand(attachmentMsg.ID, "")
			}
		}
		return *m, nil
	case tea.KeyEnter:
		m.quoteSelectedMessage()
		m.resize()
		m.refreshViewport(false)
		return *m, nil
	default:
		return *m, nil
	}
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
	m.resize()
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
	m.resize()
	return true
}

func (m *model) exitCopyMode() {
	if !m.copyMode {
		return
	}
	m.copyMode = false
	m.setStatusNotice("", false)
	m.resize()
}

func (m *model) exitRevokeMode() {
	if !m.revokeMode {
		return
	}
	m.revokeMode = false
	m.revokeCandidates = nil
	m.revokeSelection = 0
	m.resize()
}

func (m *model) rebuildRevokeCandidates() {
	m.revokeCandidates = m.revokeCandidates[:0]
	for i, entry := range m.history {
		if entry.kind != historyKindMessage || entry.messageID == "" || !entry.outgoing || entry.revoked {
			continue
		}
		if entry.authorIdentity == "" || entry.status != transcript.StatusSent {
			continue
		}
		m.revokeCandidates = append(m.revokeCandidates, i)
	}
}

func (m *model) confirmSelectedRevoke() (tea.Model, tea.Cmd) {
	if !m.revokeMode {
		return *m, nil
	}
	index := m.selectedRevokeHistoryIndex()
	if index < 0 || index >= len(m.history) {
		m.exitRevokeMode()
		return *m, nil
	}
	entry := m.history[index]
	revoke := room.Revoke{
		Version:          1,
		RoomKey:          m.roomAuthorization.RoomKey,
		OperatorIdentity: m.identityID,
		TargetMessageID:  entry.messageID,
		At:               time.Now(),
	}
	if m.session == nil || revoke.RoomKey == "" || revoke.OperatorIdentity == "" {
		m.exitRevokeMode()
		m.addErrorEntry("not connected yet")
		return *m, nil
	}
	if _, err := m.session.Send(room.RevokeBody(revoke)); err != nil {
		m.exitRevokeMode()
		m.addErrorEntry(err.Error())
		return *m, m.flushScrollbackCmd()
	}
	m.exitRevokeMode()
	m.handleRevokeRecord(transcript.RevokeRecord{
		TargetMessageID:  revoke.TargetMessageID,
		OperatorIdentity: revoke.OperatorIdentity,
		At:               revoke.At,
	}, true)
	return *m, m.flushScrollbackCmd()
}

func (m *model) handleIncomingMessage(message session.Message) tea.Cmd {
	if message.ID != "" {
		if _, ok := m.seenMessages[message.ID]; ok {
			return nil
		}
	}
	if handled, cmd := m.handleControlMessage(message); handled {
		if message.ID != "" {
			m.seenMessages[message.ID] = struct{}{}
		}
		return cmd
	}
	if line, ok := room.ParseStatusResponse(message.Body); ok {
		if message.ID != "" {
			m.seenMessages[message.ID] = struct{}{}
		}
		m.addSystemEntry(line)
		return nil
	}
	if events, ok := room.ParseEventsResponse(message.Body); ok {
		if message.ID != "" {
			m.seenMessages[message.ID] = struct{}{}
		}
		m.addEventsEntries(events)
		return nil
	}
	m.addMessageEntry(message, false, transcript.StatusSent, true)
	m.notifyLiveIncomingAlert()
	return nil
}

func (m *model) handleControlMessage(message session.Message) (bool, tea.Cmd) {
	if m.handleHistorySyncControl(message) {
		return true, nil
	}
	if handled, cmd := m.handleUpdateControl(message); handled {
		return true, cmd
	}
	if revoke, ok := room.ParseRevoke(message.Body); ok {
		if m.roomAuthorization.RoomKey == "" || revoke.RoomKey != m.roomAuthorization.RoomKey {
			return true, nil
		}
		m.rememberPeerIdentity(message.From, revoke.OperatorIdentity)
		m.handleRevokeRecord(transcript.RevokeRecord{
			TargetMessageID:  revoke.TargetMessageID,
			OperatorIdentity: revoke.OperatorIdentity,
			At:               revoke.At,
		}, true)
		return true, nil
	}
	return room.IsRevokeControl(message.Body), nil
}

func (m *model) handleHistorySyncControl(message session.Message) bool {
	if hello, ok := room.ParseHistorySyncHello(message.Body); ok {
		if hello.IdentityID != "" && strings.TrimSpace(message.From) != "" {
			m.syncCapablePeers[message.From] = true
			m.rememberPeerIdentity(message.From, hello.IdentityID)
		}
		m.maybeOfferHistorySync(hello)
		return true
	}
	if offer, ok := room.ParseHistorySyncOffer(message.Body); ok {
		m.rememberPeerIdentity(message.From, offer.SourceIdentity)
		m.maybeRequestHistorySync(offer)
		return true
	}
	if request, ok := room.ParseHistorySyncRequest(message.Body); ok {
		m.rememberPeerIdentity(message.From, request.TargetIdentity)
		m.maybeSendHistorySyncChunk(request)
		return true
	}
	if chunk, ok := room.ParseHistorySyncChunk(message.Body); ok {
		m.rememberPeerIdentity(message.From, chunk.SourceIdentity)
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
	if summary.Count == 0 {
		return
	}
	targetIdentity := strings.TrimSpace(hello.IdentityID)
	if targetIdentity == "" {
		return
	}
	if _, ok := m.offeredHistory[targetIdentity]; ok {
		if !historySummaryHasMore(summary, hello.Summary) {
			return
		}
	}
	_, _ = m.session.Send(room.HistorySyncOfferBody(room.HistorySyncOffer{
		Version:        1,
		SourceIdentity: m.identityID,
		TargetIdentity: targetIdentity,
		RoomKey:        m.roomAuthorization.RoomKey,
		Summary:        summary,
	}))
	m.offeredHistory[targetIdentity] = struct{}{}
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
	revokes := make([]transcript.RevokeRecord, 0)
	for _, entry := range m.history {
		if entry.kind != historyKindMessage || entry.messageID == "" || entry.at.Before(request.Since) {
			continue
		}
		record := transcript.Record{
			MessageID:      entry.messageID,
			Direction:      transcript.DirectionIncoming,
			From:           entry.from,
			AuthorIdentity: entry.authorIdentity,
			Body:           entry.body,
			At:             entry.at,
			Status:         entry.status,
			Revoked:        entry.revoked,
			RevokedAt:      entry.revokedAt,
		}
		if entry.outgoing {
			record.Direction = transcript.DirectionOutgoing
		}
		records = append(records, record)
		if entry.revoked {
			revokes = append(revokes, transcript.RevokeRecord{
				TargetMessageID:  entry.messageID,
				OperatorIdentity: entry.authorIdentity,
				At:               entry.revokedAt,
			})
		}
	}
	if len(records) == 0 && len(revokes) == 0 {
		return
	}
	_, _ = m.session.Send(room.HistorySyncChunkBody(room.HistorySyncChunk{
		Version:        1,
		SourceIdentity: m.identityID,
		TargetIdentity: request.TargetIdentity,
		RoomKey:        m.roomAuthorization.RoomKey,
		Records:        records,
		Revokes:        revokes,
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
		if record.AuthorIdentity == "" && record.Direction == transcript.DirectionOutgoing {
			record.AuthorIdentity = chunk.SourceIdentity
		}
		m.addHistoricalRecordEntry(record, true)
		m.seenMessages[record.MessageID] = struct{}{}
		added++
	}
	for _, revoke := range chunk.Revokes {
		m.handleRevokeRecord(revoke, true)
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
		if entry.authorIdentity != "" && record.AuthorIdentity != "" && entry.authorIdentity != record.AuthorIdentity {
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
	m.announceClientVersion()
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

func (m *model) handleUpdateAllCommand(args []string) (tea.Model, tea.Cmd) {
	if m.session == nil {
		m.addErrorEntry("not connected yet")
		return *m, m.flushScrollbackCmd()
	}
	if err := m.ensureIdentityLoaded(); err != nil {
		m.addErrorEntry(err.Error())
		return *m, m.flushScrollbackCmd()
	}
	if err := m.ensureRoomAuthorization(m.conversationKeyForPeer(m.currentPeer)); err != nil {
		m.addErrorEntry(err.Error())
		return *m, m.flushScrollbackCmd()
	}

	targetVersion := ""
	if len(args) > 0 {
		targetVersion = strings.TrimSpace(args[0])
	}
	request := room.UpdateRequest{
		Version:           1,
		RequestID:         fmt.Sprintf("update-%d", time.Now().UnixNano()),
		RoomKey:           m.roomAuthorization.RoomKey,
		RequesterIdentity: m.identityID,
		RequesterName:     m.localRequesterName(),
		TargetVersion:     targetVersion,
		At:                time.Now(),
	}
	if requester, ok := m.session.(updateRequestSubmitter); ok {
		if err := requester.SubmitUpdateRequest(request); err != nil {
			m.addErrorEntry(err.Error())
			return *m, m.flushScrollbackCmd()
		}
	} else {
		m.announceClientVersion()
		if _, err := m.session.Send(room.UpdateRequestBody(request)); err != nil {
			m.addErrorEntry(err.Error())
			return *m, m.flushScrollbackCmd()
		}
	}

	if targetVersion == "" {
		m.addSystemEntry("update request submitted: latest")
	} else {
		m.addSystemEntry(fmt.Sprintf("update request submitted: %s", targetVersion))
	}
	return *m, m.flushScrollbackCmd()
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

func (m *model) handleUpdateControl(message session.Message) (bool, tea.Cmd) {
	if execute, ok := room.ParseUpdateExecute(message.Body); ok {
		return true, m.handleUpdateExecute(message.From, execute)
	}
	if result, ok := room.ParseUpdateResult(message.Body); ok {
		m.handleUpdateResultControl(result)
		return true, nil
	}
	return room.IsUpdateControl(message.Body), nil
}

func (m *model) handleUpdateExecute(sender string, execute room.UpdateExecute) tea.Cmd {
	if m.roomAuthorization.RoomKey == "" || execute.RoomKey != m.roomAuthorization.RoomKey {
		return nil
	}
	targetLabel := strings.TrimSpace(execute.TargetVersion)
	if targetLabel == "" {
		targetLabel = "latest"
	}
	m.addSystemEntry(fmt.Sprintf("update request accepted: %s", targetLabel))

	if m.mode != "join" {
		return nil
	}
	if strings.TrimSpace(sender) != strings.TrimSpace(m.currentPeer) {
		return nil
	}
	if _, ok := m.executedRoomUpdates[execute.RequestID]; ok {
		return nil
	}
	m.executedRoomUpdates[execute.RequestID] = struct{}{}
	return performRoomUpdateCmd(execute, m.updatePerformer)
}

func (m *model) handleUpdateResultControl(result room.UpdateResult) {
	if m.roomAuthorization.RoomKey != "" && result.RoomKey != "" && result.RoomKey != m.roomAuthorization.RoomKey {
		return
	}
	reporterName := strings.TrimSpace(result.ReporterName)
	if reporterName == "" {
		reporterName = "peer"
	}
	statuses := m.updateStatuses[result.RequestID]
	if statuses == nil {
		statuses = make(map[string]string)
		m.updateStatuses[result.RequestID] = statuses
	}
	statuses[reporterName] = result.Status

	m.addSystemEntry(renderUpdateResultLine(reporterName, result))
	if summary := renderUpdateSummaryLine(statuses); summary != "" {
		m.addSystemEntry(summary)
	}
}

func (m *model) handleRoomUpdatePerformed(msg roomUpdatePerformedMsg) (tea.Model, tea.Cmd) {
	outcome := msg.outcome
	if msg.err != nil && strings.TrimSpace(outcome.Status) == "" {
		outcome.Status = "download-failed"
		outcome.Detail = msg.err.Error()
	}

	status := strings.TrimSpace(outcome.Status)
	detail := strings.TrimSpace(outcome.Detail)
	if status == "" {
		status = "download-failed"
	}

	shouldQuit := false
	if status == "success" && outcome.Restartable {
		executablePath, err := m.executablePath()
		if err != nil {
			status = "restart-failed"
			detail = err.Error()
		} else {
			spec, err := update.BuildRestartSpec(executablePath, m.startupArgs)
			if err != nil {
				status = "restart-failed"
				detail = err.Error()
			} else {
				m.pendingRestart = &spec
				shouldQuit = true
			}
		}
	}

	if m.session != nil {
		_, err := m.session.Send(room.UpdateResultBody(room.UpdateResult{
			Version:        1,
			RequestID:      msg.execute.RequestID,
			RoomKey:        msg.execute.RoomKey,
			ReporterName:   m.localRequesterName(),
			ReporterID:     m.identityID,
			TargetVersion:  msg.execute.TargetVersion,
			Status:         status,
			Detail:         detail,
			CurrentVersion: firstNonEmpty(outcome.CurrentVersion, version.Version),
			At:             time.Now(),
		}))
		if err != nil {
			m.addErrorEntry(err.Error())
		}
	}

	if shouldQuit {
		if m.session != nil {
			_ = m.session.Close()
		}
		return *m, tea.Quit
	}
	return *m, m.flushScrollbackCmd()
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
	actionBarHeight := 0
	if strings.TrimSpace(m.buildRenderedActionBarState().text) != "" {
		actionBarHeight = 1
	}
	replyBarHeight := 0
	if strings.TrimSpace(m.buildRenderedReplyBarState().text) != "" {
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

func (m model) activeSlashCommandSuggestions() []slashCommandSuggestion {
	if m.uiMode != uiModeTUI || m.revokeMode {
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
	parts := make([]string, 0, len(actions))
	cursor := 0
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

func truncateRunes(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "..."
}

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
	if m.uiMode != uiModeTUI || m.replyDraft == nil {
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

func (m model) renderActionBar() string {
	state := m.buildRenderedActionBarState()
	if strings.TrimSpace(state.text) == "" {
		return ""
	}
	return inputHintStyle.Render(state.text)
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
	record := transcript.Record{
		MessageID:      message.ID,
		Direction:      transcript.DirectionIncoming,
		From:           message.From,
		AuthorIdentity: m.authorIdentityForMessage(message, outgoing),
		Body:           message.Body,
		At:             message.At,
		Status:         status,
	}
	if outgoing {
		record.Direction = transcript.DirectionOutgoing
	}
	m.addRecordEntry(record, persist, false)
}

func (m *model) addHistoricalMessageEntry(message session.Message, outgoing bool, status string, persist bool) {
	record := transcript.Record{
		MessageID:      message.ID,
		Direction:      transcript.DirectionIncoming,
		From:           message.From,
		AuthorIdentity: m.authorIdentityForMessage(message, outgoing),
		Body:           message.Body,
		At:             message.At,
		Status:         status,
	}
	if outgoing {
		record.Direction = transcript.DirectionOutgoing
	}
	m.addRecordEntry(record, persist, true)
}

func (m *model) addHistoricalRecordEntry(record transcript.Record, persist bool) {
	m.addRecordEntry(record, persist, true)
}

func (m *model) addRecordEntry(record transcript.Record, persist bool, chronological bool) {
	entry := historyEntry{
		kind:           historyKindMessage,
		messageID:      record.MessageID,
		from:           record.From,
		body:           record.Body,
		authorIdentity: record.AuthorIdentity,
		at:             record.At,
		outgoing:       record.Direction == transcript.DirectionOutgoing,
		status:         record.Status,
		revoked:        record.Revoked,
		revokedAt:      record.RevokedAt,
	}

	if chronological {
		m.insertHistoryEntryChronologically(entry)
	} else {
		m.addHistoryEntry(entry)
	}
	if record.MessageID != "" {
		m.seenMessages[record.MessageID] = struct{}{}
		m.reindexHistoryMessageIDs()
		m.applyPendingRevokes(record.MessageID)
	}
	if persist && m.transcript != nil {
		_ = m.transcript.AppendMessage(record)
		if record.Revoked {
			_ = m.transcript.AppendRevoke(transcript.RevokeRecord{
				TargetMessageID:  record.MessageID,
				OperatorIdentity: record.AuthorIdentity,
				At:               record.RevokedAt,
			})
		}
	}
}

func (m *model) authorIdentityForMessage(message session.Message, outgoing bool) string {
	if outgoing {
		return m.identityID
	}
	return m.peerIdentities[strings.TrimSpace(message.From)]
}

func (m *model) rememberPeerIdentity(peerName, identityID string) {
	peerName = strings.TrimSpace(peerName)
	identityID = strings.TrimSpace(identityID)
	if peerName == "" || identityID == "" {
		return
	}
	if current := m.peerIdentities[peerName]; current == identityID {
		return
	}
	m.peerIdentities[peerName] = identityID

	updated := false
	for i := range m.history {
		entry := m.history[i]
		if entry.kind != historyKindMessage || strings.TrimSpace(entry.from) != peerName || entry.authorIdentity != "" {
			continue
		}
		entry.authorIdentity = identityID
		m.history[i] = entry
		if entry.messageID != "" {
			m.applyPendingRevokes(entry.messageID)
		}
		updated = true
	}
	if updated {
		m.refreshViewport(false)
	}
}

func (m *model) handleRevokeRecord(revoke transcript.RevokeRecord, persist bool) {
	applied, pending := m.applyRevokeRecord(revoke, persist)
	if applied || !pending {
		return
	}
	m.pendingRevokes[revoke.TargetMessageID] = append(m.pendingRevokes[revoke.TargetMessageID], revoke)
}

func (m *model) applyPendingRevokes(messageID string) {
	pending := m.pendingRevokes[messageID]
	if len(pending) == 0 {
		return
	}
	remaining := pending[:0]
	for _, revoke := range pending {
		applied, retry := m.applyRevokeRecord(revoke, true)
		if !applied && retry {
			remaining = append(remaining, revoke)
		}
	}
	if len(remaining) == 0 {
		delete(m.pendingRevokes, messageID)
		return
	}
	m.pendingRevokes[messageID] = remaining
}

func (m *model) applyRevokeRecord(revoke transcript.RevokeRecord, persist bool) (bool, bool) {
	targetID := strings.TrimSpace(revoke.TargetMessageID)
	operatorID := strings.TrimSpace(revoke.OperatorIdentity)
	if targetID == "" || operatorID == "" {
		return false, false
	}

	index, ok := m.entryIndex[targetID]
	if !ok {
		return false, true
	}
	entry := m.history[index]
	if entry.kind != historyKindMessage {
		return false, false
	}
	if entry.revoked {
		return true, false
	}
	if entry.authorIdentity == "" {
		return false, true
	}
	if entry.authorIdentity != operatorID {
		return false, false
	}

	entry.revoked = true
	entry.revokedAt = revoke.At
	m.history[index] = entry
	if m.uiMode == uiModeScrollback {
		m.pendingScrollbackLines = append(m.pendingScrollbackLines, renderScrollbackEntry(entry))
	}
	m.syncRevokeCandidates()
	m.refreshViewport(false)

	if persist && m.transcript != nil {
		_ = m.transcript.AppendRevoke(revoke)
	}
	return true, false
}

func (m *model) addHistoryEntry(entry historyEntry) {
	stickToBottom := m.viewport.AtBottom() || len(m.history) == 0
	m.history = append(m.history, entry)
	m.rebuildCopySelection()
	m.syncRevokeCandidates()
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
	m.rebuildCopySelection()
	m.syncRevokeCandidates()
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

func (m *model) selectedRevokeHistoryIndex() int {
	if !m.revokeMode || m.revokeSelection < 0 || m.revokeSelection >= len(m.revokeCandidates) {
		return -1
	}
	return m.revokeCandidates[m.revokeSelection]
}

func (m *model) syncRevokeCandidates() {
	if !m.revokeMode {
		return
	}
	m.rebuildRevokeCandidates()
	if len(m.revokeCandidates) == 0 {
		m.revokeMode = false
		m.revokeSelection = 0
		return
	}
	if m.revokeSelection >= len(m.revokeCandidates) {
		m.revokeSelection = len(m.revokeCandidates) - 1
	}
}

func (m *model) rebuildCopySelection() {
	selectedHistoryIndex := m.selectedCopyHistoryIndex()
	m.copySelection = m.copySelection[:0]
	for i, entry := range m.history {
		if entry.kind == historyKindMessage {
			m.copySelection = append(m.copySelection, i)
		}
	}
	if len(m.copySelection) == 0 {
		m.copySelectionPos = -1
		m.followCopySelection = true
		return
	}
	if m.followCopySelection {
		m.copySelectionPos = len(m.copySelection) - 1
		return
	}
	for i, historyIndex := range m.copySelection {
		if historyIndex == selectedHistoryIndex {
			m.copySelectionPos = i
			return
		}
	}
	if m.copySelectionPos < 0 {
		m.copySelectionPos = len(m.copySelection) - 1
		return
	}
	if m.copySelectionPos >= len(m.copySelection) {
		m.copySelectionPos = len(m.copySelection) - 1
	}
}

func (m model) selectedCopyHistoryIndex() int {
	if m.copySelectionPos < 0 || m.copySelectionPos >= len(m.copySelection) {
		return -1
	}
	return m.copySelection[m.copySelectionPos]
}

func (m *model) moveCopySelection(delta int) {
	if len(m.copySelection) == 0 {
		m.copySelectionPos = -1
		m.followCopySelection = true
		return
	}
	if m.copySelectionPos < 0 || m.copySelectionPos >= len(m.copySelection) {
		m.copySelectionPos = len(m.copySelection) - 1
	}
	next := m.copySelectionPos + delta
	if next < 0 {
		next = 0
	}
	if next >= len(m.copySelection) {
		next = len(m.copySelection) - 1
	}
	m.copySelectionPos = next
	m.followCopySelection = next == len(m.copySelection)-1
	m.scrollSelectedMessageIntoView()
}

func (m *model) scrollSelectedMessageIntoView() {
	if m.viewport.Height <= 0 {
		return
	}
	historyIndex := m.selectedCopyHistoryIndex()
	if historyIndex < 0 {
		return
	}
	state := m.buildRenderedViewportState()
	lineRange, ok := state.lineRanges[historyIndex]
	if !ok || lineRange[0] >= lineRange[1] {
		return
	}
	top := lineRange[0]
	bottom := lineRange[1] - 1
	if top < m.viewport.YOffset {
		m.viewport.SetYOffset(top)
		return
	}
	lastVisible := m.viewport.YOffset + m.viewport.Height - 1
	if bottom > lastVisible {
		m.viewport.SetYOffset(bottom - m.viewport.Height + 1)
	}
}

func (m *model) setStatusNotice(text string, isError bool) {
	m.statusNotice = strings.TrimSpace(text)
	m.statusNoticeIsError = isError
}

func (m model) selectedCopyText() (string, bool) {
	historyIndex := m.selectedCopyHistoryIndex()
	if historyIndex < 0 {
		return "", false
	}
	state := m.buildRenderedViewportState()
	lineRange, ok := state.lineRanges[historyIndex]
	if !ok || lineRange[0] >= lineRange[1] {
		return "", false
	}
	lines := make([]string, 0, lineRange[1]-lineRange[0])
	for i, line := range state.lines[lineRange[0]:lineRange[1]] {
		text := ansi.Strip(line.text)
		if i == 0 {
			text = strings.TrimPrefix(text, "> ")
		}
		lines = append(lines, text)
	}
	return strings.Join(lines, "\n"), true
}

func (m *model) copySelectedMessage() {
	text, ok := m.selectedCopyText()
	if !ok {
		m.setStatusNotice("no message to copy", true)
		return
	}
	if m.clipboardWriter == nil {
		m.setStatusNotice("copy unsupported", true)
		return
	}
	if err := m.clipboardWriter(text); err != nil {
		if errors.Is(err, errClipboardUnsupported) {
			m.setStatusNotice("copy unsupported", true)
			return
		}
		m.setStatusNotice("copy failed", true)
		return
	}
	m.setStatusNotice("copied message", false)
}

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

type compactReply struct {
	sender  string
	sentAt  string
	summary string
	body    string
}

func parseCompactReply(body string) (compactReply, bool) {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.SplitN(body, "\n", 2)
	if len(lines) != 2 {
		return compactReply{}, false
	}
	first := strings.TrimSpace(lines[0])
	replyBody := strings.TrimSpace(lines[1])
	if replyBody == "" {
		return compactReply{}, false
	}
	matches := compactReplyPattern.FindStringSubmatch(first)
	if matches == nil {
		return compactReply{}, false
	}
	return compactReply{
		sender:  strings.TrimSpace(matches[1]),
		sentAt:  matches[2],
		summary: strings.TrimSpace(matches[3]),
		body:    replyBody,
	}, true
}

func (m *model) quoteSelectedMessage() {
	index := m.selectedCopyHistoryIndex()
	if index < 0 || index >= len(m.history) {
		m.setStatusNotice("no message to copy", true)
		return
	}
	m.setReplyDraft(m.history[index])
	m.exitCopyMode()
}

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

func (m model) buildRenderedViewportState() renderedViewportState {
	state := renderedViewportState{
		lines:      make([]renderedHistoryLine, 0, len(m.history)*2),
		lineRanges: make(map[int][2]int, len(m.history)),
	}
	lastDate := ""
	selectedIndex := -1
	switch {
	case m.revokeMode:
		selectedIndex = m.selectedRevokeHistoryIndex()
	case m.copyMode:
		selectedIndex = m.selectedCopyHistoryIndex()
	}
	for i, entry := range m.history {
		entryDate := entry.at.Local().Format("2006-01-02")
		if entryDate != lastDate {
			state.lines = append(state.lines, renderedHistoryLine{
				text:         renderDateSeparator(entryDate),
				historyIndex: -1,
			})
			lastDate = entryDate
		}
		feedback := attachmentFeedbackNone
		switch {
		case i == m.activeClickHistoryIndex:
			feedback = attachmentFeedbackClick
		case i == m.hoveredHistoryIndex:
			feedback = attachmentFeedbackHover
		}
		line := renderTUIEntryWithFeedback(entry, i == selectedIndex, feedback)
		if m.viewport.Width > 0 {
			line = ansi.Wrap(line, m.viewport.Width, "")
		}
		attachmentID := ""
		clickable := false
		if entry.kind == historyKindMessage && !entry.revoked {
			if attachmentMsg, ok := attachment.ParseChatMessage(entry.body); ok {
				attachmentID = strings.TrimSpace(attachmentMsg.ID)
				clickable = attachmentID != ""
			}
		}
		start := len(state.lines)
		for _, wrapped := range strings.Split(line, "\n") {
			state.lines = append(state.lines, renderedHistoryLine{
				text:         wrapped,
				historyIndex: i,
				attachmentID: attachmentID,
				clickable:    clickable,
			})
		}
		state.lineRanges[i] = [2]int{start, len(state.lines)}
	}
	return state
}

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

func (m *model) handleMouse(msg tea.MouseMsg) (bool, tea.Cmd) {
	switch msg.Action {
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
	case tea.MouseActionMotion:
		if m.pendingViewportPress != nil && m.pendingViewportPress.inViewport && (msg.Button == tea.MouseButtonLeft || msg.Button == tea.MouseButtonNone) {
			if !m.draggingViewport && absInt(msg.Y-m.pendingViewportPress.y) > 1 {
				m.draggingViewport = true
			}
			if m.draggingViewport {
				delta := msg.Y - m.lastMouseY
				if delta > 0 {
					m.viewport.ScrollUp(delta)
				} else if delta < 0 {
					m.viewport.ScrollDown(-delta)
				}
				m.lastMouseY = msg.Y
				return true, nil
			}
		}
		if msg.Button == tea.MouseButtonNone {
			if m.updateHoveredHistoryIndex(msg.Y) {
				return true, nil
			}
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
	}
	return false, nil
}

func (m model) actionBarRow() int {
	return viewportTopRow + m.viewport.Height
}

func (m model) replyBarRow() int {
	row := viewportTopRow + m.viewport.Height
	if strings.TrimSpace(m.renderedActionBar.text) != "" {
		row++
	}
	if suggestions := m.renderSlashCommandSuggestions(); suggestions != "" {
		row += strings.Count(suggestions, "\n") + 1
	}
	return row
}

func (m model) isWithinActionBar(mouseY int) bool {
	return strings.TrimSpace(m.renderedActionBar.text) != "" && mouseY == m.actionBarRow()
}

func (m model) isWithinReplyBar(mouseY int) bool {
	return strings.TrimSpace(m.renderedReplyBar.text) != "" && mouseY == m.replyBarRow()
}

func (m model) clickedActionBarAction(mouseX, mouseY int) (actionBarAction, bool) {
	if !m.isWithinActionBar(mouseY) {
		return "", false
	}
	for _, button := range m.renderedActionBar.buttons {
		if mouseX >= button.startX && mouseX < button.endX {
			return button.action, true
		}
	}
	return "", false
}

func (m model) clickedReplyBarClear(mouseX, mouseY int) bool {
	if !m.isWithinReplyBar(mouseY) {
		return false
	}
	return mouseX >= m.renderedReplyBar.clearStart && mouseX < m.renderedReplyBar.clearEnd
}

func (m model) isWithinViewport(mouseY int) bool {
	if m.viewport.Height <= 0 {
		return false
	}
	viewportBottom := viewportTopRow + m.viewport.Height - 1
	return mouseY >= viewportTopRow && mouseY <= viewportBottom
}

func (m model) viewportLineIndex(mouseY int) int {
	return m.viewport.YOffset + (mouseY - viewportTopRow)
}

func (m model) clickedAttachment(mouseY int) (string, int) {
	if !m.isWithinViewport(mouseY) {
		return "", -1
	}
	lineIndex := m.viewportLineIndex(mouseY)
	if lineIndex < 0 || lineIndex >= len(m.renderedViewport.lines) {
		return "", -1
	}
	line := m.renderedViewport.lines[lineIndex]
	if !line.clickable {
		return "", -1
	}
	return line.attachmentID, line.historyIndex
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

func (m model) clickedAttachmentID(mouseY int) string {
	attachmentID, _ := m.clickedAttachment(mouseY)
	return attachmentID
}

func (m *model) updateHoveredHistoryIndex(mouseY int) bool {
	hoveredIndex := -1
	if attachmentID, historyIndex := m.clickedAttachment(mouseY); attachmentID != "" {
		hoveredIndex = historyIndex
	}
	if hoveredIndex == m.hoveredHistoryIndex {
		return false
	}
	m.hoveredHistoryIndex = hoveredIndex
	m.refreshViewport(false)
	return true
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
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
		m.resize()
		m.refreshViewport(false)
		return *m, nil
	default:
		return *m, nil
	}
}

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
		if record.Direction == transcript.DirectionOutgoing && record.AuthorIdentity == "" {
			record.AuthorIdentity = m.identityID
		}
		m.addHistoricalRecordEntry(record, false)
	}
	return nil
}

func (m *model) resetConversation() {
	m.history = nil
	m.printedCount = 0
	m.entryIndex = make(map[string]int)
	m.seenMessages = make(map[string]struct{})
	m.pending = make(map[string]session.Message)
	m.pendingRevokes = make(map[string][]transcript.RevokeRecord)
	m.peerIdentities = make(map[string]string)
	m.transcript = nil
	m.transcriptConversationKey = ""
	m.currentConversationKey = ""
	m.currentPeer = ""
	m.requestedHistory = make(map[string]struct{})
	m.offeredHistory = make(map[string]struct{})
	m.executedRoomUpdates = make(map[string]struct{})
	m.updateStatuses = make(map[string]map[string]string)
	m.copySelection = nil
	m.copySelectionPos = -1
	m.followCopySelection = true
	m.statusNotice = ""
	m.statusNoticeIsError = false
	m.renderedViewport = renderedViewportState{}
	m.hoveredHistoryIndex = -1
	m.activeClickHistoryIndex = -1
	m.copyMode = false
	m.revokeMode = false
	m.revokeCandidates = nil
	m.revokeSelection = 0
	m.operationNotice = ""
	m.operationNoticeIsError = false
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

func (m model) displayRoomName() string {
	return groupRoomNameFromKey(firstNonEmpty(m.currentConversationKey, m.transcriptKey))
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
	m.addSystemEntry(tuiCommandsHelp)
}

func (m *model) localRequesterName() string {
	if name := strings.TrimSpace(m.localName); name != "" {
		return name
	}
	if m.mode == "host" {
		return "host"
	}
	return "join"
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

func performRoomUpdateCmd(execute room.UpdateExecute, performer updatePerformerFunc) tea.Cmd {
	return func() tea.Msg {
		if performer == nil {
			return roomUpdatePerformedMsg{
				execute: execute,
				outcome: update.Outcome{
					Status: "download-failed",
					Detail: "update performer is not configured",
				},
			}
		}
		outcome, err := performer(context.Background(), execute.TargetVersion)
		return roomUpdatePerformedMsg{
			execute: execute,
			outcome: outcome,
			err:     err,
		}
	}
}

func renderUpdateResultLine(reporterName string, result room.UpdateResult) string {
	switch result.Status {
	case "permission-denied":
		return fmt.Sprintf("update denied: %s", result.Status)
	case "resolve-latest-failed":
		if detail := strings.TrimSpace(result.Detail); detail != "" {
			return fmt.Sprintf("update failed: %s (%s)", result.Status, detail)
		}
		return fmt.Sprintf("update failed: %s", result.Status)
	default:
		line := fmt.Sprintf("update result: %s %s", reporterName, result.Status)
		if target := strings.TrimSpace(result.TargetVersion); target != "" {
			line += " -> " + target
		}
		if detail := strings.TrimSpace(result.Detail); detail != "" {
			line += " (" + detail + ")"
		}
		return line
	}
}

func renderUpdateSummaryLine(statuses map[string]string) string {
	if len(statuses) == 0 {
		return ""
	}
	order := []string{
		"success",
		"already-up-to-date",
		"fallback-written",
		"permission-denied",
		"resolve-latest-failed",
		"restart-failed",
		"android-manual-required",
		"download-failed",
		"checksum-failed",
		"extract-failed",
		"replace-failed",
	}
	counts := make(map[string]int, len(order))
	for _, status := range statuses {
		counts[status]++
	}

	parts := make([]string, 0, len(order))
	for _, status := range order {
		if counts[status] == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%d", status, counts[status]))
	}
	if len(parts) == 0 {
		return ""
	}
	return "update summary: " + strings.Join(parts, " ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func groupRoomNameFromKey(roomKey string) string {
	roomKey = strings.TrimSpace(roomKey)
	if !strings.HasPrefix(roomKey, "group:") {
		return ""
	}
	parts := strings.SplitN(roomKey, ":", 3)
	if len(parts) != 3 {
		return ""
	}
	return strings.TrimSpace(parts[1])
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
		if entry.outgoing && status != "" && !entry.revoked {
			statusSuffix = fmt.Sprintf(" [%s]", status)
		}
		coloredLabel := senderMessageStyle(label).Render(label)
		coloredTimestamp := timestampStyle().Render(fmt.Sprintf("[%s]", timestamp))
		return fmt.Sprintf("%s %s: %s%s", coloredTimestamp, coloredLabel, renderedMessageBody(entry), statusSuffix)
	}
}

func renderTUIEntry(entry historyEntry, selected bool) string {
	return renderTUIEntryWithFeedback(entry, selected, attachmentFeedbackNone)
}

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
		header := coloredTimestamp + textSegmentStyle.Render(" ") + coloredLabel
		line := header + textSegmentStyle.Render(": ") + textSegmentStyle.Render(renderedMessageBody(entry)+statusSuffix)
		if !entry.revoked {
			if compact, ok := renderCompactReplyBody(entry.body, senderSegmentStyle, textSegmentStyle, statusSuffix); ok {
				line = header + textSegmentStyle.Render(":") + "\n" + compact
			}
		}
		if selected {
			return inputHintStyle.Render("> ") + line
		}
		return line
	}
}

func renderCompactReplyBody(body string, senderStyle lipgloss.Style, textStyle lipgloss.Style, statusSuffix string) (string, bool) {
	reply, ok := parseCompactReply(body)
	if !ok {
		return "", false
	}
	bar := replyCardBarStyle().Render("  │ ")
	meta := bar + senderStyle.Render(reply.sender) + replyCardMetaStyle().Render(" · "+reply.sentAt)
	summary := bar + textStyle.Render(reply.summary)
	replyBody := textStyle.Render(appendStatusSuffixToLastLine(reply.body, statusSuffix))
	return strings.Join([]string{meta, summary, replyBody}, "\n"), true
}

func appendStatusSuffixToLastLine(body string, statusSuffix string) string {
	if statusSuffix == "" {
		return body
	}
	lines := strings.Split(body, "\n")
	lines[len(lines)-1] += statusSuffix
	return strings.Join(lines, "\n")
}

func attachmentFeedbackStyles(feedback attachmentFeedbackState, senderStyle lipgloss.Style) (lipgloss.Style, lipgloss.Style, lipgloss.Style) {
	timestampSegmentStyle := timestampStyle()
	textSegmentStyle := lipgloss.NewStyle()
	switch feedback {
	case attachmentFeedbackHover:
		timestampSegmentStyle = timestampSegmentStyle.Inherit(attachmentHoverStyle)
		senderStyle = senderStyle.Inherit(attachmentHoverStyle)
		textSegmentStyle = textSegmentStyle.Inherit(attachmentHoverStyle)
	case attachmentFeedbackClick:
		timestampSegmentStyle = timestampSegmentStyle.Inherit(attachmentClickStyle)
		senderStyle = senderStyle.Inherit(attachmentClickStyle)
		textSegmentStyle = textSegmentStyle.Inherit(attachmentClickStyle)
	}
	return timestampSegmentStyle, senderStyle, textSegmentStyle
}

func renderedMessageBody(entry historyEntry) string {
	if entry.revoked {
		return "已撤回一条消息"
	}
	if body, ok := formatAttachmentBody(entry.body); ok {
		return body
	}
	return entry.body
}

func renderDateSeparator(date string) string {
	return separatorStyle.Render(fmt.Sprintf("--- %s ---", date))
}

func (m model) renderStatusBar() string {
	headerText := "chatbox " + m.mode
	if roomName := m.displayRoomName(); roomName != "" {
		headerText += " | chat: " + roomName
	}
	status := m.status
	style := statusStyle
	if strings.TrimSpace(m.operationNotice) != "" {
		status = m.operationNotice
		if m.operationNoticeIsError {
			style = errorStyle
		}
	} else if m.copyMode && strings.TrimSpace(m.statusNotice) == "" {
		status = "copy mode"
	} else if strings.TrimSpace(m.statusNotice) != "" {
		status = m.statusNotice
		if m.statusNoticeIsError {
			style = errorStyle
		}
	} else if strings.Contains(strings.ToLower(status), "disconnected") {
		style = errorStyle
	}
	return fmt.Sprintf("%s %s %s %s", headerStyle.Render(headerText), timestampStyle().Render("|"), style.Render(status), timestampStyle().Render("| /help"))
}

func (m model) renderInputBox() string {
	hint := "Enter send / Ctrl+V attach clipboard / Ctrl+Y copy mode / Ctrl+R revoke"
	if m.copyMode {
		hint = "copy mode: Click message/actions or use Up/Down / Enter quote / Ctrl+Y copy / O open / D download / Esc cancel"
	} else if m.revokeMode {
		hint = "revoke mode: Click message/actions or use Up/Down / Enter confirm / Esc cancel"
	}
	content := strings.Join([]string{
		m.input.View(),
		inputHintStyle.Render(hint),
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

func replyCardBarStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#5F6871"))
}

func replyCardMetaStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#7A8088"))
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
		options = append(options, tea.WithAltScreen(), tea.WithMouseAllMotion())
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
	if m.uiMode != uiModeScrollback {
		return nil
	}
	if m.printedCount >= len(m.history) && len(m.pendingScrollbackLines) == 0 {
		return nil
	}

	lines := make([]string, 0, len(m.history)-m.printedCount+len(m.pendingScrollbackLines))
	for _, entry := range m.history[m.printedCount:] {
		lines = append(lines, renderScrollbackEntry(entry))
	}
	lines = append(lines, m.pendingScrollbackLines...)
	m.printedCount = len(m.history)
	m.pendingScrollbackLines = nil
	return m.printLines(lines)
}

func (m *model) printLines(lines []string) tea.Cmd {
	if len(lines) == 0 || m.historyPrinter == nil {
		return nil
	}
	return m.historyPrinter(lines)
}
