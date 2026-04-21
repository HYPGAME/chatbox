package update

import (
	"context"
	"testing"
)

func TestPerformUpdateToVersionUsesExplicitRelease(t *testing.T) {
	t.Parallel()

	client := Client{
		Repository:     "HYPGAME/chatbox",
		CurrentVersion: "v0.1.23",
		ReleaseByTag: func(context.Context, string) (Release, error) {
			return Release{
				TagName: "v0.1.24",
				Assets: []ReleaseAsset{
					{Name: "chatbox_darwin_arm64.tar.gz", DownloadURL: "https://example.invalid/chatbox_darwin_arm64.tar.gz"},
					{Name: "checksums.txt", DownloadURL: "https://example.invalid/checksums.txt"},
				},
			}, nil
		},
	}

	release, err := client.resolveRelease(context.Background(), "v0.1.24")
	if err != nil {
		t.Fatalf("resolveRelease returned error: %v", err)
	}
	if release.TagName != "v0.1.24" {
		t.Fatalf("expected explicit release tag %q, got %#v", "v0.1.24", release)
	}
}

func TestPerformUpdateReturnsAlreadyUpToDateForMatchingExplicitVersion(t *testing.T) {
	t.Parallel()

	client := Client{
		Repository:     "HYPGAME/chatbox",
		CurrentVersion: "v0.1.24",
		GOOS:           "darwin",
		GOARCH:         "arm64",
		ReleaseByTag: func(context.Context, string) (Release, error) {
			return Release{TagName: "v0.1.24"}, nil
		},
	}

	outcome, err := client.PerformUpdate(context.Background(), "v0.1.24")
	if err != nil {
		t.Fatalf("PerformUpdate returned error: %v", err)
	}
	if outcome.Status != "already-up-to-date" {
		t.Fatalf("expected already-up-to-date outcome, got %#v", outcome)
	}
	if outcome.LatestVersion != "v0.1.24" {
		t.Fatalf("expected latest version %q, got %#v", "v0.1.24", outcome)
	}
}
