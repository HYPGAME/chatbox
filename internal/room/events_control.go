package room

import (
	"encoding/json"
	"strings"
)

const (
	eventsControlPrefix  = "\x00chatbox:events:"
	eventsRequestPayload = eventsControlPrefix + "request"
	eventsResponsePrefix = eventsControlPrefix + "response:"
)

func EventsRequestBody() string {
	return eventsRequestPayload
}

func IsEventsRequest(body string) bool {
	return body == eventsRequestPayload
}

func EventsResponseBody(events []Event) string {
	payload, err := json.Marshal(events)
	if err != nil {
		return eventsResponsePrefix + "[]"
	}
	return eventsResponsePrefix + string(payload)
}

func ParseEventsResponse(body string) ([]Event, bool) {
	if !strings.HasPrefix(body, eventsResponsePrefix) {
		return nil, false
	}
	var events []Event
	if err := json.Unmarshal([]byte(strings.TrimPrefix(body, eventsResponsePrefix)), &events); err != nil {
		return nil, false
	}
	return events, true
}
