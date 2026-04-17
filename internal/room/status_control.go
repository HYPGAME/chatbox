package room

import (
	"fmt"
	"sort"
	"strings"
)

const (
	statusControlPrefix  = "\x00chatbox:status:"
	statusRequestPayload = statusControlPrefix + "request"
	statusResponsePrefix = statusControlPrefix + "response:"
)

func StatusControlPrefix() string {
	return statusControlPrefix
}

func StatusRequestBody() string {
	return statusRequestPayload
}

func IsStatusRequest(body string) bool {
	return body == statusRequestPayload
}

func StatusResponseBody(names []string) string {
	return statusResponsePrefix + FormatOnlineStatus(names)
}

func ParseStatusResponse(body string) (string, bool) {
	if !strings.HasPrefix(body, statusResponsePrefix) {
		return "", false
	}
	return strings.TrimPrefix(body, statusResponsePrefix), true
}

func FormatOnlineStatus(names []string) string {
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	if len(sorted) == 0 {
		return "online (0): none"
	}
	return fmt.Sprintf("online (%d): %s", len(sorted), strings.Join(sorted, ", "))
}
