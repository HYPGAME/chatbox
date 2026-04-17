package headless

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"chatbox/internal/room"
	"chatbox/internal/session"
)

func RunHost(ctx context.Context, host *session.Host, localName string, out io.Writer) error {
	if host == nil {
		return errors.New("headless host requires listener")
	}
	if out == nil {
		out = io.Discard
	}

	hostRoom := room.NewHostRoom(localName)
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

func logf(out io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(out, format+"\n", args...)
}
