package headless

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"chatbox/internal/session"
)

func TestHeadlessHostRuntimeLogsLifecycle(t *testing.T) {
	t.Parallel()

	host, psk := newTestHost(t, "router")
	logs := &lockedBuffer{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- RunHost(ctx, host, "router", logs, nil)
	}()

	waitForLogContains(t, logs, "headless host listening on "+host.Addr())

	client, err := session.Dial(context.Background(), host.Addr(), session.Config{
		Name: "alice",
		PSK:  psk,
	})
	if err != nil {
		t.Fatalf("Dial returned error: %v", err)
	}

	waitForLogContains(t, logs, "alice joined")

	if err := client.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	waitForLogContains(t, logs, "alice left")

	cancel()
	waitForHeadlessExit(t, errCh)
}

func TestHeadlessHostDoesNotLogChatBodies(t *testing.T) {
	t.Parallel()

	host, psk := newTestHost(t, "router")
	logs := &lockedBuffer{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- RunHost(ctx, host, "router", logs, nil)
	}()

	waitForLogContains(t, logs, "headless host listening on "+host.Addr())

	client, err := session.Dial(context.Background(), host.Addr(), session.Config{
		Name: "alice",
		PSK:  psk,
	})
	if err != nil {
		t.Fatalf("Dial returned error: %v", err)
	}
	defer client.Close()

	message, err := client.Send("secret-body")
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}

	waitForReceipt(t, client.Receipts(), message.ID)

	if got := logs.String(); strings.Contains(got, "secret-body") {
		t.Fatalf("expected headless logs to omit chat body, got %q", got)
	}

	cancel()
	waitForHeadlessExit(t, errCh)
}

func TestHeadlessHostStopsOnContextCancel(t *testing.T) {
	t.Parallel()

	host, _ := newTestHost(t, "router")
	logs := &lockedBuffer{}
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- RunHost(ctx, host, "router", logs, nil)
	}()

	waitForLogContains(t, logs, "headless host listening on "+host.Addr())
	cancel()
	waitForHeadlessExit(t, errCh)
}

func TestHeadlessHostRunsAttachmentCleanupOnStart(t *testing.T) {
	t.Parallel()

	host, _ := newTestHost(t, "router")
	logs := &lockedBuffer{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := &fakeHeadlessAttachmentStore{cleanupSignal: make(chan struct{}, 1)}
	errCh := make(chan error, 1)
	go func() {
		errCh <- RunHost(ctx, host, "router", logs, store)
	}()

	waitForHeadlessCleanup(t, store.cleanupSignal)
	cancel()
	waitForHeadlessExit(t, errCh)
}

func newTestHost(t *testing.T, name string) (*session.Host, []byte) {
	t.Helper()

	psk := bytes.Repeat([]byte{0x44}, 32)
	host, err := session.Listen("127.0.0.1:0", session.Config{
		Name: name,
		PSK:  psk,
	})
	if err != nil {
		t.Fatalf("Listen returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = host.Close()
	})
	return host, psk
}

func waitForReceipt(t *testing.T, receipts <-chan session.Receipt, messageID string) {
	t.Helper()

	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()

	for {
		select {
		case receipt := <-receipts:
			if receipt.MessageID == messageID {
				return
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for receipt %q", messageID)
		}
	}
}

func waitForLogContains(t *testing.T, logs *lockedBuffer, needle string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(logs.String(), needle) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for log %q in %q", needle, logs.String())
}

func waitForHeadlessExit(t *testing.T, errCh <-chan error) {
	t.Helper()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunHost returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for headless host to exit")
	}
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

type fakeHeadlessAttachmentStore struct {
	cleanupSignal chan struct{}
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (f *fakeHeadlessAttachmentStore) CleanupExpired(context.Context, time.Time) (int, error) {
	select {
	case f.cleanupSignal <- struct{}{}:
	default:
	}
	return 0, nil
}

func (f *fakeHeadlessAttachmentStore) DeleteByMessageID(string) error {
	return nil
}

func waitForHeadlessCleanup(t *testing.T, signal <-chan struct{}) {
	t.Helper()

	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for headless cleanup")
	}
}
