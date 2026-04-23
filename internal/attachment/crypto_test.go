package attachment

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	psk := bytes.Repeat([]byte{7}, 32)
	plaintext := make([]byte, 2<<20+123)
	if _, err := rand.Read(plaintext); err != nil {
		t.Fatalf("rand.Read returned error: %v", err)
	}

	var encrypted bytes.Buffer
	summary, err := EncryptStream(bytes.NewReader(plaintext), &encrypted, psk, nil)
	if err != nil {
		t.Fatalf("EncryptStream returned error: %v", err)
	}
	if summary.PlainSize != int64(len(plaintext)) {
		t.Fatalf("expected plain size %d, got %d", len(plaintext), summary.PlainSize)
	}

	var decrypted bytes.Buffer
	if _, err := DecryptStream(&encrypted, &decrypted, psk, nil); err != nil {
		t.Fatalf("DecryptStream returned error: %v", err)
	}
	if !bytes.Equal(plaintext, decrypted.Bytes()) {
		t.Fatal("expected decrypt to recover original plaintext")
	}
}

func TestDecryptRejectsWrongPSK(t *testing.T) {
	psk := bytes.Repeat([]byte{7}, 32)

	var encrypted bytes.Buffer
	if _, err := EncryptStream(bytes.NewReader([]byte("secret")), &encrypted, psk, nil); err != nil {
		t.Fatalf("EncryptStream returned error: %v", err)
	}
	if _, err := DecryptStream(bytes.NewReader(encrypted.Bytes()), io.Discard, bytes.Repeat([]byte{8}, 32), nil); err == nil {
		t.Fatal("expected wrong psk to fail")
	}
}
