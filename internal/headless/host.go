package headless

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"chatbox/internal/historymeta"
	"chatbox/internal/hosthistory"
	"chatbox/internal/room"
	"chatbox/internal/session"
	"chatbox/internal/transcript"
)

type attachmentStore interface {
	CleanupExpired(context.Context, time.Time) (int, error)
	DeleteByMessageID(string) error
}

type retentionStore interface {
	AppendMessage(roomKey string, record transcript.Record, now time.Time) error
	AppendRevoke(roomKey string, revoke transcript.RevokeRecord, now time.Time) error
	LoadWindow(roomKey string, since, now time.Time) (hosthistory.Window, error)
	CleanupExpired(now time.Time) (int, error)
}

func RunHost(ctx context.Context, host *session.Host, localName string, psk []byte, roomKey string, out io.Writer, attachments attachmentStore, retained retentionStore) error {
	if host == nil {
		return errors.New("headless host requires listener")
	}
	if out == nil {
		out = io.Discard
	}

	hostRoom := room.NewHostRoom(localName)
	if retained == nil && len(psk) == 32 {
		baseDir, err := hosthistory.DefaultBaseDir()
		if err != nil {
			return err
		}
		retained, err = hosthistory.OpenStore(baseDir, psk)
		if err != nil {
			return err
		}
	}
	if retained != nil && roomKey != "" {
		baseDir, err := historymeta.DefaultBaseDir()
		if err != nil {
			return err
		}
		hostRoom.ConfigureHistoryRetention(retained, roomKey, func(roomKey, identityID string) (historymeta.Record, error) {
			return historymeta.OpenOrCreateFirstSeenRecord(baseDir, roomKey, identityID, time.Now)
		})
		if _, err := retained.CleanupExpired(time.Now()); err != nil {
			logf(out, "host history cleanup failed: %v", err)
		}
		go runHistoryCleanup(ctx, retained, out)
	}
	if attachments != nil {
		hostRoom.ConfigureAttachments(attachments)
		if _, err := attachments.CleanupExpired(ctx, time.Now()); err != nil && !errors.Is(err, context.Canceled) {
			logf(out, "attachment cleanup failed: %v", err)
		}
		go runAttachmentCleanup(ctx, attachments, out)
	}
	serveCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var shutdownOnce sync.Once
	shutdown := func() {
		shutdownOnce.Do(func() {
			_ = host.Close()
			_ = hostRoom.Close()
			cancel()
		})
	}
	defer shutdown()

	serveDone := make(chan struct{})
	go func() {
		defer close(serveDone)
		hostRoom.Serve(serveCtx, host)
		shutdown()
	}()

	logf(out, "headless host listening on %s as %s", host.Addr(), localName)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-serveDone:
			return nil
		case <-hostRoom.Done():
			return nil
		case event, ok := <-hostRoom.Events():
			if !ok {
				return nil
			}
			switch event.Kind {
			case room.EventPeerJoined:
				logf(out, "%s joined", event.PeerName)
			case room.EventPeerLeft:
				logf(out, "%s left", event.PeerName)
			}
		case _, ok := <-hostRoom.Messages():
			if !ok {
				return nil
			}
		case _, ok := <-hostRoom.Receipts():
			if !ok {
				return nil
			}
		}
	}
}

func runHistoryCleanup(ctx context.Context, retained retentionStore, out io.Writer) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := retained.CleanupExpired(time.Now()); err != nil {
				logf(out, "host history cleanup failed: %v", err)
			}
		}
	}
}

func runAttachmentCleanup(ctx context.Context, attachments attachmentStore, out io.Writer) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := attachments.CleanupExpired(ctx, time.Now()); err != nil && !errors.Is(err, context.Canceled) {
				logf(out, "attachment cleanup failed: %v", err)
			}
		}
	}
}

func logf(out io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(out, format+"\n", args...)
}
