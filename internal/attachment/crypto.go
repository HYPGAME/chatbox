package attachment

import (
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	streamMagic    = "CBA1"
	chunkPlainSize = 1 << 20
)

type Progress struct {
	Transferred int64
	Total       int64
}

type ProgressFunc func(Progress)

type StreamSummary struct {
	PlainSize int64
	DigestHex string
}

type streamHeader struct {
	Version   int    `json:"version"`
	ChunkSize int    `json:"chunk_size"`
	NonceHex  string `json:"nonce_hex"`
}

func EncryptStream(src io.Reader, dst io.Writer, psk []byte, progress ProgressFunc) (StreamSummary, error) {
	aead, baseNonce, err := newAttachmentCipher(psk)
	if err != nil {
		return StreamSummary{}, err
	}

	headerPayload, err := json.Marshal(streamHeader{
		Version:   1,
		ChunkSize: chunkPlainSize,
		NonceHex:  hex.EncodeToString(baseNonce),
	})
	if err != nil {
		return StreamSummary{}, fmt.Errorf("marshal stream header: %w", err)
	}
	if _, err := dst.Write([]byte(streamMagic)); err != nil {
		return StreamSummary{}, fmt.Errorf("write stream magic: %w", err)
	}
	if err := binary.Write(dst, binary.BigEndian, uint32(len(headerPayload))); err != nil {
		return StreamSummary{}, fmt.Errorf("write header length: %w", err)
	}
	if _, err := dst.Write(headerPayload); err != nil {
		return StreamSummary{}, fmt.Errorf("write header payload: %w", err)
	}

	hasher := sha256.New()
	buf := make([]byte, chunkPlainSize)
	var plainSize int64
	var chunkIndex uint64

	for {
		n, readErr := io.ReadFull(src, buf)
		if errors.Is(readErr, io.EOF) && n == 0 {
			break
		}
		if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) {
			return StreamSummary{}, fmt.Errorf("read plaintext chunk: %w", readErr)
		}

		chunk := buf[:n]
		if _, err := hasher.Write(chunk); err != nil {
			return StreamSummary{}, fmt.Errorf("hash plaintext chunk: %w", err)
		}

		sealed := aead.Seal(nil, nonceForChunk(baseNonce, chunkIndex), chunk, []byte(streamMagic))
		if err := binary.Write(dst, binary.BigEndian, uint32(len(sealed))); err != nil {
			return StreamSummary{}, fmt.Errorf("write ciphertext length: %w", err)
		}
		if _, err := dst.Write(sealed); err != nil {
			return StreamSummary{}, fmt.Errorf("write ciphertext chunk: %w", err)
		}

		plainSize += int64(n)
		chunkIndex++
		if progress != nil {
			progress(Progress{Transferred: plainSize})
		}

		if errors.Is(readErr, io.ErrUnexpectedEOF) {
			break
		}
	}

	return StreamSummary{
		PlainSize: plainSize,
		DigestHex: hex.EncodeToString(hasher.Sum(nil)),
	}, nil
}

func DecryptStream(src io.Reader, dst io.Writer, psk []byte, progress ProgressFunc) (StreamSummary, error) {
	key, err := deriveAttachmentKey(psk, "content")
	if err != nil {
		return StreamSummary{}, err
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return StreamSummary{}, fmt.Errorf("create attachment cipher: %w", err)
	}

	magic := make([]byte, len(streamMagic))
	if _, err := io.ReadFull(src, magic); err != nil {
		return StreamSummary{}, fmt.Errorf("read stream magic: %w", err)
	}
	if string(magic) != streamMagic {
		return StreamSummary{}, fmt.Errorf("invalid attachment stream magic %q", string(magic))
	}

	var headerLength uint32
	if err := binary.Read(src, binary.BigEndian, &headerLength); err != nil {
		return StreamSummary{}, fmt.Errorf("read header length: %w", err)
	}
	headerPayload := make([]byte, headerLength)
	if _, err := io.ReadFull(src, headerPayload); err != nil {
		return StreamSummary{}, fmt.Errorf("read header payload: %w", err)
	}

	var header streamHeader
	if err := json.Unmarshal(headerPayload, &header); err != nil {
		return StreamSummary{}, fmt.Errorf("unmarshal stream header: %w", err)
	}
	baseNonce, err := hex.DecodeString(header.NonceHex)
	if err != nil {
		return StreamSummary{}, fmt.Errorf("decode nonce: %w", err)
	}
	if len(baseNonce) != chacha20poly1305.NonceSizeX {
		return StreamSummary{}, fmt.Errorf("invalid nonce size %d", len(baseNonce))
	}

	hasher := sha256.New()
	var plainSize int64
	var chunkIndex uint64

	for {
		var ciphertextLength uint32
		if err := binary.Read(src, binary.BigEndian, &ciphertextLength); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return StreamSummary{}, fmt.Errorf("read ciphertext length: %w", err)
		}

		ciphertext := make([]byte, ciphertextLength)
		if _, err := io.ReadFull(src, ciphertext); err != nil {
			return StreamSummary{}, fmt.Errorf("read ciphertext chunk: %w", err)
		}

		plaintext, err := aead.Open(nil, nonceForChunk(baseNonce, chunkIndex), ciphertext, []byte(streamMagic))
		if err != nil {
			return StreamSummary{}, fmt.Errorf("decrypt ciphertext chunk: %w", err)
		}
		if _, err := dst.Write(plaintext); err != nil {
			return StreamSummary{}, fmt.Errorf("write plaintext chunk: %w", err)
		}
		if _, err := hasher.Write(plaintext); err != nil {
			return StreamSummary{}, fmt.Errorf("hash plaintext chunk: %w", err)
		}

		plainSize += int64(len(plaintext))
		chunkIndex++
		if progress != nil {
			progress(Progress{Transferred: plainSize})
		}
	}

	return StreamSummary{
		PlainSize: plainSize,
		DigestHex: hex.EncodeToString(hasher.Sum(nil)),
	}, nil
}

func newAttachmentCipher(psk []byte) (cipher.AEAD, []byte, error) {
	key, err := deriveAttachmentKey(psk, "content")
	if err != nil {
		return nil, nil, err
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, nil, fmt.Errorf("create attachment cipher: %w", err)
	}
	baseNonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := rand.Read(baseNonce); err != nil {
		return nil, nil, fmt.Errorf("generate nonce: %w", err)
	}
	return aead, baseNonce, nil
}

func deriveAttachmentKey(psk []byte, label string) ([]byte, error) {
	if len(psk) != 32 {
		return nil, errors.New("attachment encryption requires a 32-byte PSK")
	}
	key, err := hkdf.Key(sha256.New, psk, nil, "chatbox attachment "+label, chacha20poly1305.KeySize)
	if err != nil {
		return nil, fmt.Errorf("derive attachment key: %w", err)
	}
	return key, nil
}

func nonceForChunk(baseNonce []byte, chunkIndex uint64) []byte {
	nonce := append([]byte(nil), baseNonce...)
	binary.BigEndian.PutUint64(nonce[len(nonce)-8:], chunkIndex)
	return nonce
}
