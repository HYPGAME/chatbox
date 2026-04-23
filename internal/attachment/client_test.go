package attachment

import (
	"path/filepath"
	"testing"
)

func TestResolveDestinationDefaultsToDownloadsDir(t *testing.T) {
	t.Parallel()

	client := Client{}
	targetPath, err := client.resolveDestination(Record{FileName: "report.pdf"}, "")
	if err != nil {
		t.Fatalf("resolveDestination returned error: %v", err)
	}
	if got, want := filepath.Base(targetPath), "report.pdf"; got != want {
		t.Fatalf("expected destination basename %q, got %q", want, got)
	}
	if got, want := filepath.Base(filepath.Dir(targetPath)), "Downloads"; got != want {
		t.Fatalf("expected default destination under %q, got %q", want, targetPath)
	}
}

func TestOpenResolveDestinationUsesCacheDir(t *testing.T) {
	t.Parallel()

	client := Client{CacheDir: "/tmp/chatbox-cache"}
	targetPath, err := client.resolveOpenDestination(Record{FileName: "cat.gif"}, "")
	if err != nil {
		t.Fatalf("resolveOpenDestination returned error: %v", err)
	}
	if got, want := targetPath, "/tmp/chatbox-cache/cat.gif"; got != want {
		t.Fatalf("expected open destination %q, got %q", want, got)
	}
}
