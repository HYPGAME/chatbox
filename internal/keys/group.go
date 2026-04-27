package keys

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/hkdf"
)

const (
	groupPSKInfo             = "chatbox group room psk"
	groupRoomFingerprintSize = 8
)

type GroupCredentials struct {
	GroupName string
	PSK       []byte
	RoomKey   string
}

func DeriveGroupCredentials(groupName, password string) (GroupCredentials, error) {
	normalizedName := strings.TrimSpace(groupName)
	if normalizedName == "" {
		return GroupCredentials{}, errors.New("group name must not be empty")
	}
	if password == "" {
		return GroupCredentials{}, errors.New("group password must not be empty")
	}

	reader := hkdf.New(sha256.New, []byte(normalizedName+"\x00"+password), nil, []byte(groupPSKInfo))
	psk := make([]byte, pskSize)
	if _, err := io.ReadFull(reader, psk); err != nil {
		return GroupCredentials{}, fmt.Errorf("derive group psk: %w", err)
	}

	return GroupCredentials{
		GroupName: normalizedName,
		PSK:       psk,
		RoomKey:   GroupRoomKey(normalizedName, psk),
	}, nil
}

func GroupRoomKey(groupName string, psk []byte) string {
	normalizedName := strings.TrimSpace(groupName)
	fingerprint := sha256.Sum256(psk)
	return fmt.Sprintf(
		"group:%s:%s",
		normalizedName,
		hex.EncodeToString(fingerprint[:groupRoomFingerprintSize]),
	)
}
