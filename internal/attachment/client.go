package attachment

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Client struct {
	BaseURL    string
	PSK        []byte
	HTTPClient *http.Client
	CacheDir   string
	OpenFile   func(string) error
}

type UploadRequest struct {
	RoomKey       string
	OwnerName     string
	OwnerIdentity string
	FileName      string
	Kind          string
	Body          []byte
}

type UploadPathRequest struct {
	RoomKey       string
	OwnerName     string
	OwnerIdentity string
	Path          string
	Kind          string
}

func (c Client) UploadBytes(ctx context.Context, req UploadRequest) (Record, error) {
	reader := bytes.NewReader(req.Body)
	return c.upload(ctx, req.RoomKey, req.OwnerName, req.OwnerIdentity, req.FileName, req.Kind, int64(len(req.Body)), reader, nil)
}

func (c Client) UploadPath(ctx context.Context, req UploadPathRequest, progress ProgressFunc) (Record, error) {
	file, err := os.Open(req.Path)
	if err != nil {
		return Record{}, fmt.Errorf("open attachment file: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return Record{}, fmt.Errorf("stat attachment file: %w", err)
	}

	return c.upload(ctx, req.RoomKey, req.OwnerName, req.OwnerIdentity, filepath.Base(req.Path), req.Kind, info.Size(), file, progress)
}

func (c Client) BindMessage(ctx context.Context, attachmentID, messageID string) error {
	body, err := json.Marshal(struct {
		MessageID string `json:"message_id"`
	}{MessageID: strings.TrimSpace(messageID)})
	if err != nil {
		return fmt.Errorf("marshal bind request: %w", err)
	}

	req, err := c.signedRequest(ctx, http.MethodPost, "/v1/attachments/"+strings.TrimSpace(attachmentID)+"/bind-message", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("bind attachment message: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return responseError("bind attachment message", resp)
	}
	return nil
}

func (c Client) FetchMeta(ctx context.Context, attachmentID string) (Record, error) {
	req, err := c.signedRequest(ctx, http.MethodGet, "/v1/attachments/"+strings.TrimSpace(attachmentID)+"/meta", nil)
	if err != nil {
		return Record{}, err
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return Record{}, fmt.Errorf("fetch attachment metadata: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Record{}, responseError("fetch attachment metadata", resp)
	}

	var record Record
	if err := json.NewDecoder(resp.Body).Decode(&record); err != nil {
		return Record{}, fmt.Errorf("decode attachment metadata: %w", err)
	}
	return record, nil
}

func (c Client) DownloadBytes(ctx context.Context, attachmentID string) ([]byte, error) {
	req, err := c.signedRequest(ctx, http.MethodGet, "/v1/attachments/"+strings.TrimSpace(attachmentID)+"/blob", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("download attachment blob: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, responseError("download attachment blob", resp)
	}

	var decrypted bytes.Buffer
	if _, err := DecryptStream(resp.Body, &decrypted, c.PSK, nil); err != nil {
		return nil, fmt.Errorf("decrypt attachment blob: %w", err)
	}
	return decrypted.Bytes(), nil
}

func (c Client) DownloadToPath(ctx context.Context, attachmentID, destPath string, progress ProgressFunc) (string, error) {
	meta, err := c.FetchMeta(ctx, attachmentID)
	if err != nil {
		return "", err
	}

	targetPath, err := c.resolveDestination(meta, destPath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
		return "", fmt.Errorf("create attachment destination dir: %w", err)
	}

	req, err := c.signedRequest(ctx, http.MethodGet, "/v1/attachments/"+strings.TrimSpace(attachmentID)+"/blob", nil)
	if err != nil {
		return "", err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("download attachment blob: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", responseError("download attachment blob", resp)
	}

	tempFile, err := os.CreateTemp(filepath.Dir(targetPath), filepath.Base(targetPath)+".*.partial")
	if err != nil {
		return "", fmt.Errorf("create partial attachment file: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
	}()

	wrappedProgress := func(p Progress) {
		if progress == nil {
			return
		}
		p.Total = meta.Size
		progress(p)
	}
	if _, err := DecryptStream(resp.Body, tempFile, c.PSK, wrappedProgress); err != nil {
		return "", fmt.Errorf("decrypt attachment blob: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return "", fmt.Errorf("close partial attachment file: %w", err)
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		return "", fmt.Errorf("finalize attachment file: %w", err)
	}
	return targetPath, nil
}

func (c Client) Open(ctx context.Context, attachmentID string, progress ProgressFunc) (string, error) {
	path, err := c.DownloadToPath(ctx, attachmentID, "", progress)
	if err != nil {
		return "", err
	}
	if err := c.open(path); err != nil {
		return path, err
	}
	return path, nil
}

func (c Client) Delete(ctx context.Context, attachmentID string) error {
	req, err := c.signedRequest(ctx, http.MethodDelete, "/v1/attachments/"+strings.TrimSpace(attachmentID), nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("delete attachment: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return responseError("delete attachment", resp)
	}
	return nil
}

func (c Client) upload(ctx context.Context, roomKey, ownerName, ownerIdentity, fileName, kind string, size int64, plaintext io.Reader, progress ProgressFunc) (Record, error) {
	pipeReader, pipeWriter := io.Pipe()
	encryptErrCh := make(chan error, 1)
	wrappedProgress := func(p Progress) {
		if progress == nil {
			return
		}
		p.Total = size
		progress(p)
	}

	go func() {
		_, err := EncryptStream(plaintext, pipeWriter, c.PSK, wrappedProgress)
		if err != nil {
			_ = pipeWriter.CloseWithError(err)
			encryptErrCh <- err
			return
		}
		encryptErrCh <- pipeWriter.Close()
	}()

	req, err := c.signedRequest(ctx, http.MethodPost, "/v1/attachments", pipeReader)
	if err != nil {
		_ = pipeReader.Close()
		_ = pipeWriter.Close()
		return Record{}, err
	}
	req.Header.Set(roomKeyHeader, strings.TrimSpace(roomKey))
	req.Header.Set(ownerNameHeader, strings.TrimSpace(ownerName))
	req.Header.Set(ownerIdentityHeader, strings.TrimSpace(ownerIdentity))
	req.Header.Set(fileNameHeader, strings.TrimSpace(fileName))
	req.Header.Set(fileKindHeader, strings.TrimSpace(kind))
	req.Header.Set(fileSizeHeader, fmt.Sprintf("%d", size))

	resp, err := c.httpClient().Do(req)
	encryptErr := <-encryptErrCh
	if err != nil {
		if encryptErr != nil && !errors.Is(encryptErr, io.ErrClosedPipe) {
			return Record{}, fmt.Errorf("encrypt attachment: %w", encryptErr)
		}
		return Record{}, fmt.Errorf("upload attachment: %w", err)
	}
	defer resp.Body.Close()
	if encryptErr != nil {
		return Record{}, fmt.Errorf("encrypt attachment: %w", encryptErr)
	}
	if resp.StatusCode != http.StatusOK {
		return Record{}, responseError("upload attachment", resp)
	}

	var record Record
	if err := json.NewDecoder(resp.Body).Decode(&record); err != nil {
		return Record{}, fmt.Errorf("decode upload response: %w", err)
	}
	return record, nil
}

func (c Client) signedRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	if strings.TrimSpace(c.BaseURL) == "" {
		return nil, errors.New("attachment base url is required")
	}

	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, body)
	if err != nil {
		return nil, fmt.Errorf("build attachment request: %w", err)
	}
	issuedAt := time.Now()
	signature, err := requestSignature(c.PSK, method, path, issuedAt)
	if err != nil {
		return nil, err
	}
	req.Header.Set(issuedAtHeader, fmt.Sprintf("%d", issuedAt.Unix()))
	req.Header.Set(signatureHeader, signature)
	return req, nil
}

func (c Client) resolveDestination(meta Record, destPath string) (string, error) {
	destPath = strings.TrimSpace(destPath)
	if destPath == "" {
		cacheDir := strings.TrimSpace(c.CacheDir)
		if cacheDir == "" {
			var err error
			cacheDir, err = DefaultCacheDir()
			if err != nil {
				return "", err
			}
		}
		return filepath.Join(cacheDir, meta.FileName), nil
	}

	info, err := os.Stat(destPath)
	if err == nil && info.IsDir() {
		return filepath.Join(destPath, meta.FileName), nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("stat attachment destination: %w", err)
	}
	return destPath, nil
}

func (c Client) open(path string) error {
	if c.OpenFile != nil {
		return c.OpenFile(path)
	}
	return openFile(path)
}

func (c Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

func requestSignature(psk []byte, method, path string, issuedAt time.Time) (string, error) {
	signingKey, err := deriveAttachmentKey(psk, "request")
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, signingKey)
	fmt.Fprintf(mac, "%s\n%s\n%d", method, path, issuedAt.Unix())
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func responseError(action string, resp *http.Response) error {
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%s failed with %s", action, resp.Status)
	}
	detail := strings.TrimSpace(string(payload))
	if detail == "" {
		return fmt.Errorf("%s failed with %s", action, resp.Status)
	}
	return fmt.Errorf("%s failed with %s: %s", action, resp.Status, detail)
}
