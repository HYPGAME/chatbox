package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
	"github.com/mattn/go-runewidth"

	"chatbox/internal/session"
	"chatbox/internal/transcript"
)

type promptConsole struct {
	out    io.Writer
	prompt string

	mu          sync.Mutex
	buffer      []rune
	cursor      int
	escapeSeq   []rune
	history     []string
	historyPos  int
	historyLine []rune
}

func newPromptConsole(out io.Writer) *promptConsole {
	return &promptConsole{
		out:    out,
		prompt: "> ",
	}
}

func (c *promptConsole) printLine(line string) {
	c.printLines([]string{line})
}

func (c *promptConsole) printLines(lines []string) {
	if len(lines) == 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.clearLocked()
	for _, line := range lines {
		_, _ = fmt.Fprint(c.out, line, "\r\n")
	}
	c.renderPromptLocked()
}

func (c *promptConsole) handleRune(r rune) (string, bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.consumeEscapeLocked(r) {
		c.renderPromptLocked()
		return "", false, false
	}

	switch r {
	case 3:
		c.clearLocked()
		_, _ = fmt.Fprint(c.out, "\r\n")
		return "", false, true
	case '\r', '\n':
		line := string(c.buffer)
		if line != "" && (len(c.history) == 0 || c.history[len(c.history)-1] != line) {
			c.history = append(c.history, line)
		}
		c.historyPos = len(c.history)
		c.historyLine = nil
		c.buffer = nil
		c.cursor = 0
		c.clearLocked()
		_, _ = fmt.Fprint(c.out, "\r\n")
		c.renderPromptLocked()
		return line, true, false
	case 127, '\b':
		c.exitHistoryLocked()
		if c.cursor > 0 && len(c.buffer) > 0 {
			c.buffer = append(c.buffer[:c.cursor-1], c.buffer[c.cursor:]...)
			c.cursor--
		}
	default:
		if !isControlRune(r) {
			c.exitHistoryLocked()
			c.buffer = append(c.buffer[:c.cursor], append([]rune{r}, c.buffer[c.cursor:]...)...)
			c.cursor++
		}
	}

	c.renderPromptLocked()
	return "", false, false
}

func (c *promptConsole) clearLocked() {
	_, _ = fmt.Fprint(c.out, "\r\x1b[2K")
}

func (c *promptConsole) renderPromptLocked() {
	c.clearLocked()
	_, _ = fmt.Fprint(c.out, c.prompt, string(c.buffer))
	if remaining := runewidth.StringWidth(string(c.buffer[c.cursor:])); remaining > 0 {
		_, _ = fmt.Fprintf(c.out, "\x1b[%dD", remaining)
	}
}

func (c *promptConsole) bell() {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, _ = fmt.Fprint(c.out, "\a")
}

func isControlRune(r rune) bool {
	return r < 32 && r != '\t'
}

func (c *promptConsole) consumeEscapeLocked(r rune) bool {
	if len(c.escapeSeq) > 0 {
		c.escapeSeq = append(c.escapeSeq, r)
		if c.applyEscapeLocked() {
			c.escapeSeq = nil
		} else if !isEscapePrefix(c.escapeSeq) {
			c.escapeSeq = nil
		}
		return true
	}

	if r == 27 {
		c.escapeSeq = []rune{r}
		return true
	}
	return false
}

func (c *promptConsole) applyEscapeLocked() bool {
	switch string(c.escapeSeq) {
	case "\x1b[A":
		c.historyUpLocked()
		return true
	case "\x1b[B":
		c.historyDownLocked()
		return true
	case "\x1b[C":
		if c.cursor < len(c.buffer) {
			c.cursor++
		}
		return true
	case "\x1b[D":
		if c.cursor > 0 {
			c.cursor--
		}
		return true
	case "\x1b[H", "\x1bOH", "\x1b[1~", "\x1b[7~":
		c.cursor = 0
		return true
	case "\x1b[F", "\x1bOF", "\x1b[4~", "\x1b[8~":
		c.cursor = len(c.buffer)
		return true
	case "\x1b[3~":
		c.exitHistoryLocked()
		if c.cursor < len(c.buffer) {
			c.buffer = append(c.buffer[:c.cursor], c.buffer[c.cursor+1:]...)
		}
		return true
	default:
		return false
	}
}

func isEscapePrefix(seq []rune) bool {
	raw := string(seq)
	for _, candidate := range []string{
		"\x1b[A",
		"\x1b[B",
		"\x1b[C",
		"\x1b[D",
		"\x1b[H",
		"\x1b[F",
		"\x1bOH",
		"\x1bOF",
		"\x1b[1~",
		"\x1b[3~",
		"\x1b[4~",
		"\x1b[7~",
		"\x1b[8~",
	} {
		if strings.HasPrefix(candidate, raw) {
			return true
		}
	}
	return false
}

func (c *promptConsole) historyUpLocked() {
	if len(c.history) == 0 {
		return
	}
	if c.historyPos >= len(c.history) {
		c.historyLine = append([]rune(nil), c.buffer...)
		c.historyPos = len(c.history) - 1
	} else if c.historyPos > 0 {
		c.historyPos--
	}
	c.setBufferLocked([]rune(c.history[c.historyPos]))
}

func (c *promptConsole) historyDownLocked() {
	if len(c.history) == 0 || c.historyPos >= len(c.history) {
		return
	}
	if c.historyPos < len(c.history)-1 {
		c.historyPos++
		c.setBufferLocked([]rune(c.history[c.historyPos]))
		return
	}
	c.historyPos = len(c.history)
	c.setBufferLocked(c.historyLine)
	c.historyLine = nil
}

func (c *promptConsole) exitHistoryLocked() {
	if c.historyPos < len(c.history) {
		c.historyPos = len(c.history)
		c.historyLine = nil
	}
}

func (c *promptConsole) setBufferLocked(buffer []rune) {
	c.buffer = append([]rune(nil), buffer...)
	c.cursor = len(c.buffer)
}

func runScrollback(m model) error {
	console := newPromptConsole(os.Stdout)
	m.historyPrinter = func(lines []string) tea.Cmd {
		console.printLines(lines)
		return nil
	}
	if m.alertNotifier == nil && m.alertMode == "bell" {
		m.alertNotifier = newTerminalBellAlertNotifier(console)
	}

	if m.reconnectDelay == 0 {
		m.reconnectDelay = time.Second
	}

	if err := runScrollbackLoop(&m, console, os.Stdin); err != nil {
		return err
	}
	return nil
}

func runScrollbackLoop(m *model, console *promptConsole, input io.Reader) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	restore, err := prepareScrollbackInput(input)
	if err != nil {
		return err
	}
	defer restore()

	inputLines := make(chan scrollbackInputEvent)
	inputErrs := make(chan error, 1)
	go readScrollbackInput(ctx, input, console, inputLines, inputErrs)

	connectResults := make(chan sessionResult, 1)
	var retry <-chan time.Time
	connecting := false

	startConnect := func() {
		if connecting || m.connect == nil {
			return
		}
		connecting = true
		go func() {
			conn, err := m.connect(ctx)
			select {
			case connectResults <- sessionResult{session: conn, err: err}:
			case <-ctx.Done():
				if conn != nil {
					_ = conn.Close()
				}
			}
		}()
	}

	handleConnectError := func(err error) {
		if err != nil {
			m.addErrorEntry(err.Error())
			m.flushScrollbackCmd()
		}
		if m.mode == "host" {
			m.status = fmt.Sprintf("listening on %s", m.listeningAddr)
		} else if m.connect != nil {
			m.status = "reconnecting"
		} else {
			m.status = "disconnected"
		}
		if m.connect != nil {
			retry = time.After(m.reconnectDelay)
		}
	}

	handleSessionClosed := func(err error) {
		m.session = nil
		switch {
		case m.mode == "host" && m.connect != nil:
			m.status = fmt.Sprintf("listening on %s", m.listeningAddr)
			if err != nil && err != context.Canceled && err.Error() != "session closed locally" {
				m.addErrorEntry(err.Error())
				m.flushScrollbackCmd()
			}
			startConnect()
		case m.connect != nil:
			m.status = "reconnecting"
			if err != nil && err.Error() != "session closed locally" {
				m.addErrorEntry(err.Error())
				m.flushScrollbackCmd()
			}
			retry = time.After(m.reconnectDelay)
		default:
			m.status = "disconnected"
			if err != nil && err.Error() != "session closed locally" {
				m.addErrorEntry(err.Error())
				m.flushScrollbackCmd()
			}
		}
	}

	m.flushScrollbackCmd()
	if m.session != nil {
		if err := m.bindSession(m.session); err != nil {
			m.addErrorEntry(err.Error())
		}
		m.flushScrollbackCmd()
	} else {
		startConnect()
	}

	for {
		var messages <-chan session.Message
		var receipts <-chan session.Receipt
		var done <-chan struct{}
		roomEvents := m.roomEvents
		if m.session != nil {
			messages = m.session.Messages()
			receipts = m.session.Receipts()
			done = m.session.Done()
		}

		select {
		case evt, ok := <-inputLines:
			if !ok {
				m.failPendingMessages()
				if m.session != nil {
					_ = m.session.Close()
				}
				return nil
			}
			if evt.quit {
				m.failPendingMessages()
				if m.session != nil {
					_ = m.session.Close()
				}
				return nil
			}
			if !evt.submitted {
				continue
			}
			if quit := handleScrollbackLine(m, strings.TrimSpace(evt.line)); quit {
				if m.session != nil {
					_ = m.session.Close()
				}
				return nil
			}
		case err := <-inputErrs:
			if err != nil {
				return err
			}
			return nil
		case result := <-connectResults:
			connecting = false
			if result.err != nil {
				handleConnectError(result.err)
				continue
			}
			if err := m.bindSession(result.session); err != nil {
				m.addErrorEntry(err.Error())
			}
			m.flushScrollbackCmd()
		case <-retry:
			retry = nil
			startConnect()
		case message, ok := <-messages:
			if !ok {
				continue
			}
			m.handleIncomingMessage(message)
			m.flushScrollbackCmd()
		case receipt, ok := <-receipts:
			if !ok {
				continue
			}
			_ = m.handleReceipt(receipt)
		case event, ok := <-roomEvents:
			if !ok {
				continue
			}
			_, _ = m.handleRoomEvent(event)
		case <-done:
			if m.session != nil {
				handleSessionClosed(m.session.Err())
			}
		}
	}
}

type scrollbackInputEvent struct {
	line      string
	submitted bool
	quit      bool
}

func readScrollbackInput(ctx context.Context, input io.Reader, console *promptConsole, out chan<- scrollbackInputEvent, errs chan<- error) {
	defer close(out)

	reader := bufio.NewReader(input)
	for {
		r, _, err := reader.ReadRune()
		if err != nil {
			if err == io.EOF {
				return
			}
			select {
			case errs <- err:
			default:
			}
			return
		}

		line, submitted, quit := console.handleRune(r)
		evt := scrollbackInputEvent{
			line:      line,
			submitted: submitted,
			quit:      quit,
		}
		if submitted || quit {
			select {
			case out <- evt:
			case <-ctx.Done():
				return
			}
		}
		if quit {
			return
		}
	}
}

func prepareScrollbackInput(input io.Reader) (func(), error) {
	file, ok := input.(*os.File)
	if !ok {
		return func() {}, nil
	}
	if !term.IsTerminal(file.Fd()) {
		return func() {}, nil
	}

	state, err := term.MakeRaw(file.Fd())
	if err != nil {
		return nil, fmt.Errorf("enable raw terminal input: %w", err)
	}
	return func() {
		_ = term.Restore(file.Fd(), state)
	}, nil
}

func handleScrollbackLine(m *model, text string) bool {
	if strings.HasPrefix(text, "/") {
		switch text {
		case "/help":
			m.addSystemEntry("commands: /help /status /quit")
			m.flushScrollbackCmd()
		case "/status":
			m.addSystemEntry(m.status)
			m.flushScrollbackCmd()
		case "/quit":
			m.failPendingMessages()
			return true
		case "":
			return false
		default:
			m.addErrorEntry("unknown command")
			m.flushScrollbackCmd()
		}
		return false
	}

	if text == "" {
		return false
	}

	if m.session == nil {
		m.addErrorEntry("not connected yet")
		m.flushScrollbackCmd()
		return false
	}

	message, err := m.session.Send(text)
	if err != nil {
		m.addErrorEntry(err.Error())
		m.flushScrollbackCmd()
		return false
	}

	m.pending[message.ID] = message
	m.addMessageEntry(message, true, transcript.StatusSending, true)
	m.flushScrollbackCmd()
	return false
}
