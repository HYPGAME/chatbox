package room

import (
	"encoding/json"
	"strings"
	"time"
)

const updateControlPrefix = "\x00chatbox:update:"

type UpdateRequest struct {
	Version           int       `json:"version"`
	RequestID         string    `json:"request_id"`
	RoomKey           string    `json:"room_key"`
	RequesterIdentity string    `json:"requester_identity"`
	RequesterName     string    `json:"requester_name"`
	TargetVersion     string    `json:"target_version,omitempty"`
	At                time.Time `json:"at"`
}

type UpdateExecute struct {
	Version           int       `json:"version"`
	RequestID         string    `json:"request_id"`
	RoomKey           string    `json:"room_key"`
	InitiatorIdentity string    `json:"initiator_identity"`
	InitiatorName     string    `json:"initiator_name"`
	TargetVersion     string    `json:"target_version"`
	At                time.Time `json:"at"`
}

type UpdateResult struct {
	Version        int       `json:"version"`
	RequestID      string    `json:"request_id"`
	RoomKey        string    `json:"room_key"`
	ReporterName   string    `json:"reporter_name"`
	ReporterID     string    `json:"reporter_id"`
	TargetVersion  string    `json:"target_version"`
	Status         string    `json:"status"`
	Detail         string    `json:"detail,omitempty"`
	CurrentVersion string    `json:"current_version,omitempty"`
	At             time.Time `json:"at"`
}

func IsUpdateControl(body string) bool {
	return strings.HasPrefix(body, updateControlPrefix)
}

func UpdateRequestBody(request UpdateRequest) string {
	return marshalUpdateControl("request", request)
}

func ParseUpdateRequest(body string) (UpdateRequest, bool) {
	var request UpdateRequest
	return request, parseUpdateControl(body, "request", &request)
}

func UpdateExecuteBody(execute UpdateExecute) string {
	return marshalUpdateControl("execute", execute)
}

func ParseUpdateExecute(body string) (UpdateExecute, bool) {
	var execute UpdateExecute
	return execute, parseUpdateControl(body, "execute", &execute)
}

func UpdateResultBody(result UpdateResult) string {
	return marshalUpdateControl("result", result)
}

func ParseUpdateResult(body string) (UpdateResult, bool) {
	var result UpdateResult
	return result, parseUpdateControl(body, "result", &result)
}

func marshalUpdateControl(kind string, payload any) string {
	data, err := json.Marshal(payload)
	if err != nil {
		return updateControlPrefix + kind + ":{}"
	}
	return updateControlPrefix + kind + ":" + string(data)
}

func parseUpdateControl(body, kind string, out any) bool {
	prefix := updateControlPrefix + kind + ":"
	if !strings.HasPrefix(body, prefix) {
		return false
	}
	return json.Unmarshal([]byte(strings.TrimPrefix(body, prefix)), out) == nil
}
