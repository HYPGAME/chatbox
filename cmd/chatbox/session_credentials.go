package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"

	"chatbox/internal/keys"
)

type sessionCredentials struct {
	psk           []byte
	transcriptKey string
}

var stdinIsTerminal = func() bool {
	return term.IsTerminal(os.Stdin.Fd())
}

var promptForGroupPassword = func(groupName string) (string, error) {
	if !stdinIsTerminal() {
		return "", errors.New("group mode without --group-password requires an interactive terminal; pass --group-password explicitly in non-interactive mode")
	}

	normalizedName := strings.TrimSpace(groupName)
	if _, err := fmt.Fprintf(stderr, "group password for %s: ", normalizedName); err != nil {
		return "", err
	}
	secret, err := term.ReadPassword(os.Stdin.Fd())
	_, _ = fmt.Fprintln(stderr)
	if err != nil {
		return "", fmt.Errorf("read group password: %w", err)
	}
	return string(secret), nil
}

func loadGroupPasswordFromFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read group password file: %w", err)
	}
	password := string(data)
	if idx := strings.IndexByte(password, '\n'); idx >= 0 {
		password = password[:idx]
	}
	password = strings.TrimSuffix(password, "\r")
	if password == "" {
		return "", errors.New("group password file must contain a non-empty first line")
	}
	return password, nil
}

func resolveSessionCredentials(pskFile, groupName, groupPassword, groupPasswordFile string) (sessionCredentials, error) {
	trimmedFile := strings.TrimSpace(pskFile)
	trimmedGroupName := strings.TrimSpace(groupName)
	trimmedGroupPasswordFile := strings.TrimSpace(groupPasswordFile)

	if trimmedFile != "" && (trimmedGroupName != "" || groupPassword != "" || trimmedGroupPasswordFile != "") {
		return sessionCredentials{}, errors.New("cannot combine --psk-file with --group-name/--group-password/--group-password-file")
	}
	if trimmedFile != "" {
		psk, err := keys.LoadPSKFromFile(trimmedFile)
		if err != nil {
			return sessionCredentials{}, err
		}
		return sessionCredentials{psk: psk}, nil
	}

	if groupPassword != "" && trimmedGroupName == "" {
		return sessionCredentials{}, errors.New("--group-password requires --group-name")
	}
	if trimmedGroupPasswordFile != "" && trimmedGroupName == "" {
		return sessionCredentials{}, errors.New("--group-password-file requires --group-name")
	}
	if trimmedGroupName == "" {
		return sessionCredentials{}, errors.New("requires either --psk-file or --group-name")
	}

	password := groupPassword
	if password == "" && trimmedGroupPasswordFile != "" {
		var err error
		password, err = loadGroupPasswordFromFile(trimmedGroupPasswordFile)
		if err != nil {
			return sessionCredentials{}, err
		}
	}
	if password == "" {
		var err error
		password, err = promptForGroupPassword(trimmedGroupName)
		if err != nil {
			return sessionCredentials{}, err
		}
	}

	creds, err := keys.DeriveGroupCredentials(trimmedGroupName, password)
	if err != nil {
		return sessionCredentials{}, err
	}
	return sessionCredentials{
		psk:           creds.PSK,
		transcriptKey: creds.RoomKey,
	}, nil
}
