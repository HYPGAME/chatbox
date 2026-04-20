package room

import (
	"encoding/json"
	"strings"
	"time"

	"chatbox/internal/transcript"
)

const historySyncControlPrefix = "\x00chatbox:sync:"

type HistorySyncSummary struct {
	Count  int       `json:"count"`
	Oldest time.Time `json:"oldest,omitempty"`
	Newest time.Time `json:"newest,omitempty"`
}

type HistorySyncHello struct {
	Version    int                `json:"version"`
	IdentityID string             `json:"identity_id"`
	RoomKey    string             `json:"room_key"`
	Summary    HistorySyncSummary `json:"summary"`
}

type HistorySyncOffer struct {
	Version        int                `json:"version"`
	SourceIdentity string             `json:"source_identity"`
	TargetIdentity string             `json:"target_identity"`
	RoomKey        string             `json:"room_key"`
	Summary        HistorySyncSummary `json:"summary"`
}

type HistorySyncRequest struct {
	Version        int       `json:"version"`
	SourceIdentity string    `json:"source_identity"`
	TargetIdentity string    `json:"target_identity"`
	RoomKey        string    `json:"room_key"`
	Since          time.Time `json:"since"`
}

type HistorySyncChunk struct {
	Version        int                 `json:"version"`
	SourceIdentity string              `json:"source_identity"`
	TargetIdentity string              `json:"target_identity"`
	RoomKey        string              `json:"room_key"`
	Records        []transcript.Record `json:"records"`
}

func IsHistorySyncControl(body string) bool {
	return strings.HasPrefix(body, historySyncControlPrefix)
}

func HistorySyncHelloBody(hello HistorySyncHello) string {
	return marshalHistorySyncControl("hello", hello)
}

func ParseHistorySyncHello(body string) (HistorySyncHello, bool) {
	var hello HistorySyncHello
	return hello, parseHistorySyncControl(body, "hello", &hello)
}

func HistorySyncOfferBody(offer HistorySyncOffer) string {
	return marshalHistorySyncControl("offer", offer)
}

func ParseHistorySyncOffer(body string) (HistorySyncOffer, bool) {
	var offer HistorySyncOffer
	return offer, parseHistorySyncControl(body, "offer", &offer)
}

func HistorySyncRequestBody(request HistorySyncRequest) string {
	return marshalHistorySyncControl("request", request)
}

func ParseHistorySyncRequest(body string) (HistorySyncRequest, bool) {
	var request HistorySyncRequest
	return request, parseHistorySyncControl(body, "request", &request)
}

func HistorySyncChunkBody(chunk HistorySyncChunk) string {
	return marshalHistorySyncControl("chunk", chunk)
}

func ParseHistorySyncChunk(body string) (HistorySyncChunk, bool) {
	var chunk HistorySyncChunk
	return chunk, parseHistorySyncControl(body, "chunk", &chunk)
}

func marshalHistorySyncControl(kind string, payload any) string {
	data, err := json.Marshal(payload)
	if err != nil {
		return historySyncControlPrefix + kind + ":{}"
	}
	return historySyncControlPrefix + kind + ":" + string(data)
}

func parseHistorySyncControl(body, kind string, out any) bool {
	prefix := historySyncControlPrefix + kind + ":"
	if !strings.HasPrefix(body, prefix) {
		return false
	}
	return json.Unmarshal([]byte(strings.TrimPrefix(body, prefix)), out) == nil
}
