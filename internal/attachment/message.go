package attachment

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	KindImage = "image"
	KindFile  = "file"
)

type ChatMessage struct {
	Version int
	ID      string
	Kind    string
	Name    string
	Size    int64
}

var chatMessagePattern = regexp.MustCompile(`^shared (image|file): (.+) \(([0-9]+) bytes\) #(att_[A-Za-z0-9]+)$`)

func FormatChatMessage(msg ChatMessage) string {
	return fmt.Sprintf("shared %s: %s (%d bytes) #%s", msg.Kind, msg.Name, msg.Size, msg.ID)
}

func ParseChatMessage(body string) (ChatMessage, bool) {
	matches := chatMessagePattern.FindStringSubmatch(strings.TrimSpace(body))
	if matches == nil {
		return ChatMessage{}, false
	}

	size, err := strconv.ParseInt(matches[3], 10, 64)
	if err != nil {
		return ChatMessage{}, false
	}

	return ChatMessage{
		Version: 1,
		Kind:    matches[1],
		Name:    matches[2],
		Size:    size,
		ID:      matches[4],
	}, true
}
