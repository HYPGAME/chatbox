package attachment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	attachmentTTL         = 7 * 24 * time.Hour
	requestSignatureSkew  = 15 * time.Minute
	issuedAtHeader        = "X-Chatbox-Issued-At"
	signatureHeader       = "X-Chatbox-Signature"
	roomKeyHeader         = "X-Chatbox-Room-Key"
	ownerNameHeader       = "X-Chatbox-Owner-Name"
	ownerIdentityHeader   = "X-Chatbox-Owner-Identity"
	fileNameHeader        = "X-Chatbox-File-Name"
	fileKindHeader        = "X-Chatbox-File-Kind"
	fileSizeHeader        = "X-Chatbox-File-Size"
	fileDigestHeader      = "X-Chatbox-File-Digest"
)

type Service struct {
	store *Store
	psk   []byte
	now   func() time.Time
}

type Server struct {
	listener net.Listener
	http     *http.Server
}

func NewService(store *Store, psk []byte, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{
		store: store,
		psk:   append([]byte(nil), psk...),
		now:   now,
	}
}

func ListenAndServe(ctx context.Context, chatListenAddr string, svc *Service) (*Server, error) {
	host, port, err := net.SplitHostPort(strings.TrimSpace(chatListenAddr))
	if err != nil {
		return nil, fmt.Errorf("split chat listen addr: %w", err)
	}
	value, err := strconv.Atoi(port)
	if err != nil {
		return nil, fmt.Errorf("parse chat port %q: %w", port, err)
	}
	addr := net.JoinHostPort(host, strconv.Itoa(value+1))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen attachment service: %w", err)
	}

	server := &http.Server{Handler: svc.Handler()}
	go func() {
		<-ctx.Done()
		_ = server.Shutdown(context.Background())
	}()
	go func() {
		_ = server.Serve(listener)
	}()

	return &Server{listener: listener, http: server}, nil
}

func (s *Server) Close() error {
	if s == nil || s.http == nil {
		return nil
	}
	return s.http.Close()
}

func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/attachments", s.handleUpload)
	mux.HandleFunc("/v1/attachments/", s.handleAttachmentByID)
	return mux
}

func (s *Service) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorize(w, r) {
		return
	}

	recordID, err := generateAttachmentID()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	blobPath := s.store.BlobPath(recordID)
	if err := os.MkdirAll(filepath.Dir(blobPath), 0o700); err != nil {
		http.Error(w, fmt.Sprintf("create blob dir: %v", err), http.StatusInternalServerError)
		return
	}

	tempFile, err := os.CreateTemp(filepath.Dir(blobPath), recordID+".*.upload")
	if err != nil {
		http.Error(w, fmt.Sprintf("create temp blob: %v", err), http.StatusInternalServerError)
		return
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
	}()

	if _, err := io.Copy(tempFile, r.Body); err != nil {
		http.Error(w, fmt.Sprintf("write attachment blob: %v", err), http.StatusBadRequest)
		return
	}
	if err := tempFile.Close(); err != nil {
		http.Error(w, fmt.Sprintf("close attachment blob: %v", err), http.StatusInternalServerError)
		return
	}
	if err := os.Rename(tempPath, blobPath); err != nil {
		http.Error(w, fmt.Sprintf("finalize attachment blob: %v", err), http.StatusInternalServerError)
		return
	}

	size, err := strconv.ParseInt(strings.TrimSpace(r.Header.Get(fileSizeHeader)), 10, 64)
	if err != nil {
		size = 0
	}
	now := s.now()
	record, err := s.store.CreatePending(PendingRecord{
		ID:            recordID,
		RoomKey:       strings.TrimSpace(r.Header.Get(roomKeyHeader)),
		OwnerName:     strings.TrimSpace(r.Header.Get(ownerNameHeader)),
		OwnerIdentity: strings.TrimSpace(r.Header.Get(ownerIdentityHeader)),
		FileName:      strings.TrimSpace(r.Header.Get(fileNameHeader)),
		Kind:          strings.TrimSpace(r.Header.Get(fileKindHeader)),
		Size:          size,
		DigestHex:     strings.TrimSpace(r.Header.Get(fileDigestHeader)),
		BlobPath:      blobPath,
		CreatedAt:     now,
		ExpiresAt:     now.Add(attachmentTTL),
	})
	if err != nil {
		_ = os.Remove(blobPath)
		http.Error(w, fmt.Sprintf("persist attachment record: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, publicRecord(record), http.StatusOK)
}

func (s *Service) handleAttachmentByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/attachments/")
	if path == "" || path == r.URL.Path {
		http.NotFound(w, r)
		return
	}

	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		http.NotFound(w, r)
		return
	}
	id := strings.TrimSpace(parts[0])
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch {
	case r.Method == http.MethodGet && action == "meta":
		if !s.authorize(w, r) {
			return
		}
		record, err := s.store.Lookup(id)
		if err != nil {
			writeLookupError(w, err)
			return
		}
		writeJSON(w, publicRecord(record), http.StatusOK)
	case r.Method == http.MethodGet && action == "blob":
		if !s.authorize(w, r) {
			return
		}
		record, err := s.store.Lookup(id)
		if err != nil {
			writeLookupError(w, err)
			return
		}
		file, err := os.Open(record.BlobPath)
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, "attachment blob missing", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, fmt.Sprintf("open attachment blob: %v", err), http.StatusInternalServerError)
			return
		}
		defer file.Close()
		if _, err := io.Copy(w, file); err != nil {
			return
		}
	case r.Method == http.MethodPost && action == "bind-message":
		if !s.authorize(w, r) {
			return
		}
		var request struct {
			MessageID string `json:"message_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, fmt.Sprintf("decode bind request: %v", err), http.StatusBadRequest)
			return
		}
		if err := s.store.BindMessage(id, request.MessageID); err != nil {
			writeLookupError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodDelete && action == "":
		if !s.authorize(w, r) {
			return
		}
		if err := s.store.Delete(id); err != nil {
			writeLookupError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

func (s *Service) authorize(w http.ResponseWriter, r *http.Request) bool {
	issuedAtRaw := strings.TrimSpace(r.Header.Get(issuedAtHeader))
	signature := strings.TrimSpace(r.Header.Get(signatureHeader))
	if issuedAtRaw == "" || signature == "" {
		http.Error(w, "missing request signature", http.StatusUnauthorized)
		return false
	}

	issuedAtUnix, err := strconv.ParseInt(issuedAtRaw, 10, 64)
	if err != nil {
		http.Error(w, "invalid request timestamp", http.StatusUnauthorized)
		return false
	}
	issuedAt := time.Unix(issuedAtUnix, 0)
	now := s.now()
	if issuedAt.Before(now.Add(-requestSignatureSkew)) || issuedAt.After(now.Add(requestSignatureSkew)) {
		http.Error(w, "stale request signature", http.StatusUnauthorized)
		return false
	}

	expected, err := requestSignature(s.psk, r.Method, r.URL.Path, issuedAt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return false
	}
	if signature != expected {
		http.Error(w, "invalid request signature", http.StatusUnauthorized)
		return false
	}
	return true
}

func publicRecord(record Record) Record {
	record.BlobPath = ""
	return record
}

func writeLookupError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotFound) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func writeJSON(w http.ResponseWriter, value any, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func generateAttachmentID() (string, error) {
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate attachment id: %w", err)
	}
	return "att_" + hex.EncodeToString(random), nil
}
