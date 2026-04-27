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

func resolveSessionCredentials(pskFile, groupName, groupPassword string) (sessionCredentials, error) {
	trimmedFile := strings.TrimSpace(pskFile)
	trimmedGroupName := strings.TrimSpace(groupName)

	if trimmedFile != "" && (trimmedGroupName != "" || groupPassword != "") {
		return sessionCredentials{}, errors.New("cannot combine --psk-file with --group-name/--group-password")
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
	if trimmedGroupName == "" {
		return sessionCredentials{}, errors.New("requires either --psk-file or --group-name")
	}

	password := groupPassword
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
