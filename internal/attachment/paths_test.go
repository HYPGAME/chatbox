package attachment

import (
	"strings"
	"testing"
)

func TestDefaultDirsEndWithExpectedSuffixes(t *testing.T) {
	hostDir, err := DefaultHostDir()
	if err != nil {
		t.Fatalf("DefaultHostDir returned error: %v", err)
	}
	cacheDir, err := DefaultCacheDir()
	if err != nil {
		t.Fatalf("DefaultCacheDir returned error: %v", err)
	}
	if hostDir == cacheDir {
		t.Fatalf("expected host and cache dirs to differ, got %q", hostDir)
	}
	if !strings.HasSuffix(hostDir, "chatbox/attachments/host") {
		t.Fatalf("expected host dir suffix %q, got %q", "chatbox/attachments/host", hostDir)
	}
	if !strings.HasSuffix(cacheDir, "chatbox/attachments/cache") {
		t.Fatalf("expected cache dir suffix %q, got %q", "chatbox/attachments/cache", cacheDir)
	}
}
