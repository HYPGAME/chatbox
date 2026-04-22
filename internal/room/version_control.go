package room

import (
	"encoding/json"
	"strings"
)

const versionControlPrefix = "\x00chatbox:version:"

type VersionAnnounce struct {
	Version       int    `json:"version"`
	ClientVersion string `json:"client_version,omitempty"`
}

func IsVersionControl(body string) bool {
	return strings.HasPrefix(body, versionControlPrefix)
}

func VersionAnnounceBody(announce VersionAnnounce) string {
	data, err := json.Marshal(announce)
	if err != nil {
		return versionControlPrefix + "announce:{}"
	}
	return versionControlPrefix + "announce:" + string(data)
}

func ParseVersionAnnounce(body string) (VersionAnnounce, bool) {
	var announce VersionAnnounce
	prefix := versionControlPrefix + "announce:"
	if !strings.HasPrefix(body, prefix) {
		return announce, false
	}
	return announce, json.Unmarshal([]byte(strings.TrimPrefix(body, prefix)), &announce) == nil
}
