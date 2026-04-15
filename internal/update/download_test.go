package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

func TestParseChecksumsFindsExpectedAsset(t *testing.T) {
	t.Parallel()

	checksums := strings.Join([]string{
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  chatbox_darwin_arm64.tar.gz",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb  checksums.txt",
	}, "\n")

	sum, err := parseChecksums([]byte(checksums), "chatbox_darwin_arm64.tar.gz")
	if err != nil {
		t.Fatalf("parseChecksums returned error: %v", err)
	}
	if sum != strings.Repeat("a", 64) {
		t.Fatalf("expected checksum to be returned, got %q", sum)
	}
}

func TestParseChecksumsAcceptsPathPrefixedAssetNames(t *testing.T) {
	t.Parallel()

	checksums := strings.Join([]string{
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  dist/chatbox_darwin_arm64.tar.gz",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb  dist/checksums.txt",
	}, "\n")

	sum, err := parseChecksums([]byte(checksums), "chatbox_darwin_arm64.tar.gz")
	if err != nil {
		t.Fatalf("parseChecksums returned error: %v", err)
	}
	if sum != strings.Repeat("a", 64) {
		t.Fatalf("expected checksum to be returned, got %q", sum)
	}
}

func TestVerifyChecksumRejectsMismatch(t *testing.T) {
	t.Parallel()

	if err := verifyChecksum([]byte("payload"), strings.Repeat("0", 64)); err == nil {
		t.Fatal("expected checksum mismatch to fail")
	}
}

func TestExtractChatboxBinaryFromTarGz(t *testing.T) {
	t.Parallel()

	archive := buildTarGzArchive(t, map[string][]byte{
		"chatbox": []byte("new-binary"),
	})

	binary, err := extractChatboxBinaryFromTarGz(archive)
	if err != nil {
		t.Fatalf("extractChatboxBinaryFromTarGz returned error: %v", err)
	}
	if string(binary) != "new-binary" {
		t.Fatalf("expected extracted binary bytes, got %q", string(binary))
	}
}

func buildTarGzArchive(t *testing.T, files map[string][]byte) []byte {
	t.Helper()

	var archive bytes.Buffer
	gzipWriter := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(gzipWriter)

	for name, body := range files {
		header := &tar.Header{
			Name: name,
			Mode: 0o755,
			Size: int64(len(body)),
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("WriteHeader returned error: %v", err)
		}
		if _, err := tarWriter.Write(body); err != nil {
			t.Fatalf("Write returned error: %v", err)
		}
	}

	if err := tarWriter.Close(); err != nil {
		t.Fatalf("Close tar writer returned error: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("Close gzip writer returned error: %v", err)
	}

	return archive.Bytes()
}
