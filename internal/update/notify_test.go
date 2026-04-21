package update

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestBackgroundUpdateCheckPrintsHintForNewerStableRelease(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	client := fakeLatestReleaseSource{
		release: Release{
			TagName: "v0.2.0",
			HTMLURL: "https://github.com/HYPGAME/chatbox/releases/tag/v0.2.0",
		},
	}

	checkAndNotify(context.Background(), client, "v0.1.0", &output)

	rendered := output.String()
	if !strings.Contains(rendered, "new version available: v0.2.0 (current: v0.1.0)") {
		t.Fatalf("expected upgrade hint, got %q", rendered)
	}
	if !strings.Contains(rendered, "run: chatbox self-update") {
		t.Fatalf("expected self-update hint, got %q", rendered)
	}
}

func TestBackgroundUpdateCheckDoesNothingWhenAlreadyCurrent(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	client := fakeLatestReleaseSource{
		release: Release{
			TagName: "v0.2.0",
		},
	}

	checkAndNotify(context.Background(), client, "v0.2.0", &output)

	if output.Len() != 0 {
		t.Fatalf("expected no output when already current, got %q", output.String())
	}
}

func TestBackgroundUpdateCheckChannelEmitsHintForNewerStableRelease(t *testing.T) {
	t.Parallel()

	output := make(chan string, 1)
	client := fakeLatestReleaseSource{
		release: Release{
			TagName: "v0.2.0",
			HTMLURL: "https://github.com/HYPGAME/chatbox/releases/tag/v0.2.0",
		},
	}

	checkAndNotifyChannel(context.Background(), client, "dev-91cd3e3", output)

	select {
	case rendered := <-output:
		if !strings.Contains(rendered, "new version available: v0.2.0 (current: dev-91cd3e3)") {
			t.Fatalf("expected upgrade hint, got %q", rendered)
		}
		if !strings.Contains(rendered, "run: chatbox self-update") {
			t.Fatalf("expected self-update hint, got %q", rendered)
		}
	default:
		t.Fatal("expected background update notice on channel")
	}
}

type fakeLatestReleaseSource struct {
	release Release
	err     error
}

func (f fakeLatestReleaseSource) LatestRelease(context.Context) (Release, error) {
	return f.release, f.err
}
