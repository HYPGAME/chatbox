package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

func parseChecksums(payload []byte, assetName string) (string, error) {
	lines := strings.Split(string(payload), "\n")
	for _, line := range lines {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) != 2 {
			continue
		}
		if fields[1] == assetName {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("checksum for %q not found", assetName)
}

func verifyChecksum(payload []byte, expected string) error {
	sum := sha256.Sum256(payload)
	actual := hex.EncodeToString(sum[:])
	if actual != strings.ToLower(strings.TrimSpace(expected)) {
		return fmt.Errorf("checksum mismatch: expected %s got %s", expected, actual)
	}
	return nil
}

func extractChatboxBinaryFromTarGz(payload []byte) ([]byte, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("open gzip archive: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("chatbox binary not found in archive")
		}
		if err != nil {
			return nil, fmt.Errorf("read tar entry: %w", err)
		}
		if header.Name != "chatbox" {
			continue
		}
		binary, err := io.ReadAll(tarReader)
		if err != nil {
			return nil, fmt.Errorf("read chatbox binary: %w", err)
		}
		return binary, nil
	}
}
