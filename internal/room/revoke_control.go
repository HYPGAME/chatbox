package room

import (
	"encoding/json"
	"strings"
	"time"
)

const revokeControlPrefix = "\x00chatbox:revoke:"

type Revoke struct {
	Version          int       `json:"version"`
	RoomKey          string    `json:"room_key"`
	OperatorIdentity string    `json:"operator_identity"`
	TargetMessageID  string    `json:"target_message_id"`
	At               time.Time `json:"at"`
}

func IsRevokeControl(body string) bool {
	return strings.HasPrefix(body, revokeControlPrefix)
}

func RevokeBody(revoke Revoke) string {
	data, err := json.Marshal(revoke)
	if err != nil {
		return revokeControlPrefix + "{}"
	}
	return revokeControlPrefix + string(data)
}

func ParseRevoke(body string) (Revoke, bool) {
	var revoke Revoke
	if !strings.HasPrefix(body, revokeControlPrefix) {
		return revoke, false
	}
	return revoke, json.Unmarshal([]byte(strings.TrimPrefix(body, revokeControlPrefix)), &revoke) == nil
}
