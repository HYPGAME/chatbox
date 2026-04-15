package keys

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
)

const (
	pskSize       = 32
	requiredPerms = 0o600
)

func GeneratePSKFile(path string) error {
	psk := make([]byte, pskSize)
	if _, err := rand.Read(psk); err != nil {
		return fmt.Errorf("generate PSK: %w", err)
	}

	if err := os.WriteFile(path, psk, requiredPerms); err != nil {
		return fmt.Errorf("write PSK file: %w", err)
	}

	return nil
}

func LoadPSKFromFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat PSK file: %w", err)
	}
	if info.Mode().Perm() != requiredPerms {
		return nil, fmt.Errorf("PSK file permissions must be %o, got %o", requiredPerms, info.Mode().Perm())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read PSK file: %w", err)
	}
	if len(data) != pskSize {
		return nil, errors.New("PSK file must contain exactly 32 bytes")
	}

	return data, nil
}
