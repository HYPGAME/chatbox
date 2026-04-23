package tui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"chatbox/internal/attachment"
	"chatbox/internal/transcript"
)

type attachmentClient interface {
	UploadPath(context.Context, attachment.UploadPathRequest, attachment.ProgressFunc) (attachment.Record, error)
	BindMessage(context.Context, string, string) error
	FetchMeta(context.Context, string) (attachment.Record, error)
	Open(context.Context, string, attachment.ProgressFunc) (string, error)
	DownloadToPath(context.Context, string, string, attachment.ProgressFunc) (string, error)
	Delete(context.Context, string) error
}

type attachmentProgressUpdate struct {
	action string
	label  string
	value  attachment.Progress
}

type attachmentProgressMsg struct {
	update attachmentProgressUpdate
}

type attachmentStreamMsg struct {
	ch  <-chan tea.Msg
	msg tea.Msg
}

type attachmentUploadResultMsg struct {
	record  attachment.Record
	err     error
	cleanup func()
}

type attachmentTransferResultMsg struct {
	action string
	path   string
	err    error
}

func newAttachmentClientForHost(listenAddr string, psk []byte) attachmentClient {
	baseURL, err := attachment.BaseURLFromListenAddr(listenAddr)
	if err != nil {
		return nil
	}
	return newAttachmentClient(baseURL, psk)
}

func newAttachmentClientForPeer(peerAddr string, psk []byte) attachmentClient {
	baseURL, err := attachment.BaseURLFromPeer(peerAddr)
	if err != nil {
		return nil
	}
	return newAttachmentClient(baseURL, psk)
}

func newAttachmentClient(baseURL string, psk []byte) attachmentClient {
	cacheDir, err := attachment.DefaultCacheDir()
	if err != nil {
		cacheDir = ""
	}
	return attachment.Client{
		BaseURL:  baseURL,
		PSK:      append([]byte(nil), psk...),
		CacheDir: cacheDir,
	}
}

func waitForAttachmentStream(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return attachmentStreamMsg{ch: ch, msg: msg}
	}
}

func startAttachmentUploadCmd(client attachmentClient, req attachment.UploadPathRequest, cleanup func()) tea.Cmd {
	streamCh := make(chan tea.Msg, 8)

	go func() {
		defer close(streamCh)
		record, err := client.UploadPath(context.Background(), req, func(progress attachment.Progress) {
			streamCh <- attachmentProgressMsg{
				update: attachmentProgressUpdate{
					action: "uploading",
					label:  filepath.Base(req.Path),
					value:  progress,
				},
			}
		})
		streamCh <- attachmentUploadResultMsg{record: record, err: err, cleanup: cleanup}
	}()

	return waitForAttachmentStream(streamCh)
}

func startAttachmentOpenCmd(client attachmentClient, attachmentID string) tea.Cmd {
	streamCh := make(chan tea.Msg, 8)

	go func() {
		defer close(streamCh)
		path, err := client.Open(context.Background(), attachmentID, func(progress attachment.Progress) {
			streamCh <- attachmentProgressMsg{
				update: attachmentProgressUpdate{
					action: "opening",
					label:  attachmentID,
					value:  progress,
				},
			}
		})
		streamCh <- attachmentTransferResultMsg{action: "opened", path: path, err: err}
	}()

	return waitForAttachmentStream(streamCh)
}

func startAttachmentDownloadCmd(client attachmentClient, attachmentID, destPath string) tea.Cmd {
	streamCh := make(chan tea.Msg, 8)

	go func() {
		defer close(streamCh)
		path, err := client.DownloadToPath(context.Background(), attachmentID, destPath, func(progress attachment.Progress) {
			streamCh <- attachmentProgressMsg{
				update: attachmentProgressUpdate{
					action: "downloading",
					label:  attachmentID,
					value:  progress,
				},
			}
		})
		streamCh <- attachmentTransferResultMsg{action: "downloaded", path: path, err: err}
	}()

	return waitForAttachmentStream(streamCh)
}

func attachmentKindFromPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg", ".heic", ".heif", ".tif", ".tiff":
		return attachment.KindImage
	default:
		return attachment.KindFile
	}
}

func formatAttachmentBody(body string) (string, bool) {
	msg, ok := attachment.ParseChatMessage(body)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("[%s] %s (%s) #%s", msg.Kind, msg.Name, humanBytes(msg.Size), msg.ID), true
}

func humanBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	value := float64(size)
	suffixes := []string{"KB", "MB", "GB", "TB"}
	suffix := suffixes[0]
	for i := 0; i < len(suffixes) && value >= unit; i++ {
		value /= unit
		suffix = suffixes[i]
	}
	return fmt.Sprintf("%.1f %s", value, suffix)
}

func formatAttachmentProgress(update attachmentProgressUpdate) string {
	if update.value.Total > 0 {
		percent := int((update.value.Transferred * 100) / update.value.Total)
		if percent > 100 {
			percent = 100
		}
		return fmt.Sprintf("%s %s %d%%", update.action, update.label, percent)
	}
	if update.value.Transferred > 0 {
		return fmt.Sprintf("%s %s %s", update.action, update.label, humanBytes(update.value.Transferred))
	}
	return fmt.Sprintf("%s %s", update.action, update.label)
}

func splitCommandRemainder(text string) (string, string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", ""
	}
	index := strings.IndexAny(text, " \t")
	if index < 0 {
		return text, ""
	}
	return text[:index], strings.TrimSpace(text[index+1:])
}

func splitFirstToken(text string) (string, string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", ""
	}
	index := strings.IndexAny(text, " \t")
	if index < 0 {
		return text, ""
	}
	return text[:index], strings.TrimSpace(text[index+1:])
}

func (m *model) handleAttachmentStream(msg attachmentStreamMsg) (tea.Model, tea.Cmd) {
	nextCmd := waitForAttachmentStream(msg.ch)
	switch inner := msg.msg.(type) {
	case attachmentProgressMsg:
		m.handleAttachmentProgress(inner)
		return *m, nextCmd
	case attachmentUploadResultMsg:
		m.handleAttachmentUploadResult(inner)
		return *m, nextCmd
	case attachmentTransferResultMsg:
		m.handleAttachmentTransferResult(inner)
		return *m, nextCmd
	default:
		return *m, nextCmd
	}
}

func (m *model) handleAttachmentProgress(msg attachmentProgressMsg) {
	m.operationNotice = formatAttachmentProgress(msg.update)
	m.operationNoticeIsError = false
}

func (m *model) handleAttachmentUploadResult(msg attachmentUploadResultMsg) {
	if msg.cleanup != nil {
		defer msg.cleanup()
	}
	if msg.err != nil {
		m.operationNotice = msg.err.Error()
		m.operationNoticeIsError = true
		return
	}
	if err := m.publishUploadedAttachment(msg.record); err != nil {
		m.operationNotice = err.Error()
		m.operationNoticeIsError = true
		return
	}
	m.operationNotice = fmt.Sprintf("shared %s", msg.record.FileName)
	m.operationNoticeIsError = false
}

func (m *model) handleAttachmentTransferResult(msg attachmentTransferResultMsg) {
	m.activeClickHistoryIndex = -1
	m.refreshViewport(false)
	if msg.err != nil {
		m.operationNotice = msg.err.Error()
		m.operationNoticeIsError = true
		return
	}
	m.operationNotice = fmt.Sprintf("%s: %s", msg.action, msg.path)
	m.operationNoticeIsError = false
}

func (m *model) startAttachCommand(path string) (tea.Model, tea.Cmd) {
	req, err := m.buildAttachmentUploadRequest(path)
	if err != nil {
		m.addErrorEntry(err.Error())
		return *m, m.flushScrollbackCmd()
	}
	m.operationNotice = fmt.Sprintf("uploading %s", filepath.Base(req.Path))
	m.operationNoticeIsError = false
	return *m, startAttachmentUploadCmd(m.attachmentClient, req, nil)
}

func (m *model) handleAttachmentPaste(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	if m.uiMode != uiModeTUI || !msg.Paste {
		return *m, nil, false
	}
	pasted, err := m.readClipboardAttachment()
	if err != nil {
		if errors.Is(err, errPasteUnsupported) || errors.Is(err, errPasteEmpty) {
			return *m, nil, false
		}
		return *m, nil, false
	}
	req, err := m.buildAttachmentUploadRequestWithKind(pasted.Path, pasted.Kind)
	if err != nil {
		if pasted.Cleanup != nil {
			pasted.Cleanup()
		}
		m.addErrorEntry(err.Error())
		return *m, m.flushScrollbackCmd(), true
	}
	m.operationNotice = fmt.Sprintf("uploading %s", filepath.Base(req.Path))
	m.operationNoticeIsError = false
	return *m, startAttachmentUploadCmd(m.attachmentClient, req, pasted.Cleanup), true
}

func (m *model) startPasteCommand() (tea.Model, tea.Cmd) {
	pasted, err := m.readClipboardAttachment()
	if err != nil {
		m.addErrorEntry(err.Error())
		return *m, m.flushScrollbackCmd()
	}
	req, err := m.buildAttachmentUploadRequestWithKind(pasted.Path, pasted.Kind)
	if err != nil {
		if pasted.Cleanup != nil {
			pasted.Cleanup()
		}
		m.addErrorEntry(err.Error())
		return *m, m.flushScrollbackCmd()
	}
	m.operationNotice = fmt.Sprintf("uploading %s", filepath.Base(req.Path))
	m.operationNoticeIsError = false
	return *m, startAttachmentUploadCmd(m.attachmentClient, req, pasted.Cleanup)
}

func (m *model) startOpenCommand(attachmentID string) (tea.Model, tea.Cmd) {
	if m.attachmentClient == nil {
		m.addErrorEntry("attachments unavailable")
		return *m, m.flushScrollbackCmd()
	}
	attachmentID = strings.TrimSpace(attachmentID)
	if attachmentID == "" {
		m.addErrorEntry("usage: /open <attachment-id>")
		return *m, m.flushScrollbackCmd()
	}
	m.operationNotice = fmt.Sprintf("opening %s", attachmentID)
	m.operationNoticeIsError = false
	return *m, startAttachmentOpenCmd(m.attachmentClient, attachmentID)
}

func (m *model) startDownloadCommand(attachmentID, destPath string) (tea.Model, tea.Cmd) {
	if m.attachmentClient == nil {
		m.addErrorEntry("attachments unavailable")
		return *m, m.flushScrollbackCmd()
	}
	attachmentID = strings.TrimSpace(attachmentID)
	if attachmentID == "" {
		m.addErrorEntry("usage: /download <attachment-id> [dest]")
		return *m, m.flushScrollbackCmd()
	}
	m.operationNotice = fmt.Sprintf("downloading %s", attachmentID)
	m.operationNoticeIsError = false
	return *m, startAttachmentDownloadCmd(m.attachmentClient, attachmentID, strings.TrimSpace(destPath))
}

func (m model) selectedAttachmentMessage() (attachment.ChatMessage, bool) {
	index := m.selectedCopyHistoryIndex()
	if index < 0 || index >= len(m.history) {
		return attachment.ChatMessage{}, false
	}
	entry := m.history[index]
	if entry.kind != historyKindMessage || entry.revoked {
		return attachment.ChatMessage{}, false
	}
	return attachment.ParseChatMessage(entry.body)
}

func (m *model) buildAttachmentUploadRequest(path string) (attachment.UploadPathRequest, error) {
	return m.buildAttachmentUploadRequestWithKind(path, "")
}

func (m *model) buildAttachmentUploadRequestWithKind(path, kind string) (attachment.UploadPathRequest, error) {
	if m.attachmentClient == nil {
		return attachment.UploadPathRequest{}, fmt.Errorf("attachments unavailable")
	}
	if m.session == nil {
		return attachment.UploadPathRequest{}, fmt.Errorf("not connected yet")
	}
	if err := m.ensureIdentityLoaded(); err != nil {
		return attachment.UploadPathRequest{}, err
	}
	if err := m.ensureRoomAuthorization(m.conversationKeyForPeer(m.currentPeer)); err != nil {
		return attachment.UploadPathRequest{}, err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return attachment.UploadPathRequest{}, fmt.Errorf("usage: /file <path>")
	}
	if strings.TrimSpace(kind) == "" {
		kind = attachmentKindFromPath(path)
	}
	return attachment.UploadPathRequest{
		RoomKey:       m.roomAuthorization.RoomKey,
		OwnerName:     m.localRequesterName(),
		OwnerIdentity: m.identityID,
		Path:          path,
		Kind:          kind,
	}, nil
}

func (m *model) readClipboardAttachment() (clipboardAttachment, error) {
	if m.clipboardReader == nil {
		return clipboardAttachment{}, errPasteUnsupported
	}
	pasted, err := m.clipboardReader(context.Background())
	if err != nil {
		return clipboardAttachment{}, err
	}
	if strings.TrimSpace(pasted.Path) == "" {
		return clipboardAttachment{}, errPasteEmpty
	}
	if strings.TrimSpace(pasted.Kind) == "" {
		pasted.Kind = attachmentKindFromPath(pasted.Path)
	}
	return pasted, nil
}

func (m *model) publishUploadedAttachment(record attachment.Record) error {
	if m.session == nil {
		_ = m.attachmentClient.Delete(context.Background(), record.ID)
		return fmt.Errorf("attachment uploaded but chat is disconnected")
	}

	body := attachment.FormatChatMessage(attachment.ChatMessage{
		Version: 1,
		ID:      record.ID,
		Kind:    record.Kind,
		Name:    record.FileName,
		Size:    record.Size,
	})
	message, err := m.session.Send(body)
	if err != nil {
		_ = m.attachmentClient.Delete(context.Background(), record.ID)
		return err
	}

	m.pending[message.ID] = message
	m.addMessageEntry(message, true, transcript.StatusSending, true)
	if err := m.attachmentClient.BindMessage(context.Background(), record.ID, message.ID); err != nil {
		return err
	}
	return nil
}
