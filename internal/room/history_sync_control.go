package room

import (
	"encoding/json"
	"strings"
	"time"

	"chatbox/internal/transcript"
)

const historySyncControlPrefix = "\x00chatbox:sync:"
const hostHistorySyncControlPrefix = "\x00chatbox:hostsync:"

type HistorySyncSummary struct {
	Count  int       `json:"count"`
	Oldest time.Time `json:"oldest,omitempty"`
	Newest time.Time `json:"newest,omitempty"`
}

type HistorySyncHello struct {
	Version       int                `json:"version"`
	IdentityID    string             `json:"identity_id"`
	ClientVersion string             `json:"client_version,omitempty"`
	RoomKey       string             `json:"room_key"`
	Summary       HistorySyncSummary `json:"summary"`
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
	Version        int                       `json:"version"`
	SourceIdentity string                    `json:"source_identity"`
	TargetIdentity string                    `json:"target_identity"`
	RoomKey        string                    `json:"room_key"`
	Records        []transcript.Record       `json:"records"`
	Revokes        []transcript.RevokeRecord `json:"revokes,omitempty"`
}

type HostHistoryRequest struct {
	Version     int       `json:"version"`
	RoomKey     string    `json:"room_key"`
	IdentityID  string    `json:"identity_id"`
	JoinedAt    time.Time `json:"joined_at"`
	NewestLocal time.Time `json:"newest_local"`
}

type HostHistoryChunk struct {
	Version        int                       `json:"version"`
	RoomKey        string                    `json:"room_key"`
	TargetIdentity string                    `json:"target_identity"`
	Records        []transcript.Record       `json:"records"`
	Revokes        []transcript.RevokeRecord `json:"revokes,omitempty"`
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

func IsHostHistorySyncControl(body string) bool {
	return strings.HasPrefix(body, hostHistorySyncControlPrefix)
}

func HostHistoryRequestBody(request HostHistoryRequest) string {
	return marshalControl(hostHistorySyncControlPrefix, "request", request)
}

func ParseHostHistoryRequest(body string) (HostHistoryRequest, bool) {
	var request HostHistoryRequest
	return request, parseControl(hostHistorySyncControlPrefix, body, "request", &request)
}

func HostHistoryChunkBody(chunk HostHistoryChunk) string {
	return marshalControl(hostHistorySyncControlPrefix, "chunk", chunk)
}

func ParseHostHistoryChunk(body string) (HostHistoryChunk, bool) {
	var chunk HostHistoryChunk
	return chunk, parseControl(hostHistorySyncControlPrefix, body, "chunk", &chunk)
}

func marshalHistorySyncControl(kind string, payload any) string {
	return marshalControl(historySyncControlPrefix, kind, payload)
}

func marshalControl(prefix, kind string, payload any) string {
	data, err := json.Marshal(payload)
	if err != nil {
		return prefix + kind + ":{}"
	}
	return prefix + kind + ":" + string(data)
}

func parseHistorySyncControl(body, kind string, out any) bool {
	return parseControl(historySyncControlPrefix, body, kind, out)
}

func parseControl(prefix, body, kind string, out any) bool {
	expected := prefix + kind + ":"
	if !strings.HasPrefix(body, expected) {
		return false
	}
	return json.Unmarshal([]byte(strings.TrimPrefix(body, expected)), out) == nil
}
