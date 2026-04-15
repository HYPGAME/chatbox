package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSelfUpdateDownloadsAndAppliesLatestMatchingRelease(t *testing.T) {
	t.Parallel()

	archive := buildTarGzArchive(t, map[string][]byte{
		"chatbox": []byte("updated-binary"),
	})
	archiveChecksum := sha256.Sum256(archive)
	checksums := fmt.Sprintf("%s  chatbox_darwin_arm64.tar.gz\n", hex.EncodeToString(archiveChecksum[:]))

	var appliedPath string
	var appliedBinary []byte

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/HYPGAME/chatbox/releases/latest":
			_, _ = fmt.Fprintf(w, `{
				"tag_name": "v0.2.0",
				"html_url": "%s/releases/tag/v0.2.0",
				"assets": [
					{"name": "chatbox_darwin_arm64.tar.gz", "browser_download_url": "%s/assets/chatbox_darwin_arm64.tar.gz"},
					{"name": "checksums.txt", "browser_download_url": "%s/assets/checksums.txt"}
				]
			}`, server.URL, server.URL, server.URL)
		case "/assets/chatbox_darwin_arm64.tar.gz":
			_, _ = w.Write(archive)
		case "/assets/checksums.txt":
			_, _ = w.Write([]byte(checksums))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := Client{
		BaseURL:        server.URL,
		Repository:     "HYPGAME/chatbox",
		CurrentVersion: "v0.1.0",
		GOOS:           "darwin",
		GOARCH:         "arm64",
		ExecutablePath: func() (string, error) {
			return "/tmp/chatbox", nil
		},
		ApplyUpdate: func(path string, binary []byte) (ApplyResult, error) {
			appliedPath = path
			appliedBinary = append([]byte(nil), binary...)
			return ApplyResult{}, nil
		},
	}

	result, err := client.SelfUpdate(context.Background())
	if err != nil {
		t.Fatalf("SelfUpdate returned error: %v", err)
	}
	if result.LatestVersion != "v0.2.0" {
		t.Fatalf("expected latest version %q, got %q", "v0.2.0", result.LatestVersion)
	}
	if appliedPath != "/tmp/chatbox" {
		t.Fatalf("expected apply path to be used, got %q", appliedPath)
	}
	if string(appliedBinary) != "updated-binary" {
		t.Fatalf("expected extracted binary to be applied, got %q", string(appliedBinary))
	}
}

func TestSelfUpdateFailsWhenNoMatchingAssetExists(t *testing.T) {
	t.Parallel()

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/HYPGAME/chatbox/releases/latest" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprintf(w, `{
			"tag_name": "v0.2.0",
			"html_url": "%s/releases/tag/v0.2.0",
			"assets": [
				{"name": "chatbox_darwin_amd64.tar.gz", "browser_download_url": "%s/assets/chatbox_darwin_amd64.tar.gz"},
				{"name": "checksums.txt", "browser_download_url": "%s/assets/checksums.txt"}
			]
		}`, server.URL, server.URL, server.URL)
	}))
	defer server.Close()

	client := Client{
		BaseURL:        server.URL,
		Repository:     "HYPGAME/chatbox",
		CurrentVersion: "v0.1.0",
		GOOS:           "darwin",
		GOARCH:         "arm64",
	}

	if _, err := client.SelfUpdate(context.Background()); err == nil {
		t.Fatal("expected missing matching asset to fail")
	}
}

func TestSelfUpdateFallsBackToLatestDownloadEndpointsWhenAPIIsRateLimited(t *testing.T) {
	t.Parallel()

	archive := buildTarGzArchive(t, map[string][]byte{
		"chatbox": []byte("updated-binary"),
	})
	archiveChecksum := sha256.Sum256(archive)
	checksums := fmt.Sprintf("%s  chatbox_darwin_arm64.tar.gz\n", hex.EncodeToString(archiveChecksum[:]))

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/HYPGAME/chatbox/releases/latest":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
		case "/HYPGAME/chatbox/releases/latest":
			http.Redirect(w, r, server.URL+"/HYPGAME/chatbox/releases/tag/v0.2.0", http.StatusFound)
		case "/HYPGAME/chatbox/releases/latest/download/chatbox_darwin_arm64.tar.gz":
			_, _ = w.Write(archive)
		case "/HYPGAME/chatbox/releases/latest/download/checksums.txt":
			_, _ = w.Write([]byte(checksums))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := Client{
		BaseURL:        server.URL,
		WebBaseURL:     server.URL,
		Repository:     "HYPGAME/chatbox",
		CurrentVersion: "v0.1.0",
		GOOS:           "darwin",
		GOARCH:         "arm64",
		ExecutablePath: func() (string, error) {
			return "/tmp/chatbox", nil
		},
		ApplyUpdate: func(path string, binary []byte) (ApplyResult, error) {
			return ApplyResult{}, nil
		},
	}

	result, err := client.SelfUpdate(context.Background())
	if err != nil {
		t.Fatalf("SelfUpdate returned error: %v", err)
	}
	if result.LatestVersion != "v0.2.0" {
		t.Fatalf("expected fallback latest version %q, got %q", "v0.2.0", result.LatestVersion)
	}
}
