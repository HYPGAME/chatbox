package attachment

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

func BaseURLFromPeer(peer string) (string, error) {
	host, port, err := net.SplitHostPort(strings.TrimSpace(peer))
	if err != nil {
		return "", fmt.Errorf("split peer host/port: %w", err)
	}
	attachmentPort, err := attachmentPortFromChatPort(port)
	if err != nil {
		return "", err
	}
	return "http://" + net.JoinHostPort(host, attachmentPort), nil
}

func BaseURLFromListenAddr(listenAddr string) (string, error) {
	host, port, err := net.SplitHostPort(strings.TrimSpace(listenAddr))
	if err != nil {
		return "", fmt.Errorf("split listen host/port: %w", err)
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	attachmentPort, err := attachmentPortFromChatPort(port)
	if err != nil {
		return "", err
	}
	return "http://" + net.JoinHostPort(host, attachmentPort), nil
}

func attachmentPortFromChatPort(port string) (string, error) {
	value, err := strconv.Atoi(strings.TrimSpace(port))
	if err != nil {
		return "", fmt.Errorf("parse port %q: %w", port, err)
	}
	return strconv.Itoa(value + 1), nil
}
