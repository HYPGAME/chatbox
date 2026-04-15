package update

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type Release struct {
	TagName string
	HTMLURL string
	Assets  []ReleaseAsset
}

type ReleaseAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
}

type githubRelease struct {
	TagName string         `json:"tag_name"`
	HTMLURL string         `json:"html_url"`
	Assets  []ReleaseAsset `json:"assets"`
}

func parseLatestRelease(payload []byte) (Release, error) {
	var raw githubRelease
	if err := json.Unmarshal(payload, &raw); err != nil {
		return Release{}, fmt.Errorf("parse release json: %w", err)
	}
	return Release{
		TagName: raw.TagName,
		HTMLURL: raw.HTMLURL,
		Assets:  raw.Assets,
	}, nil
}

func (r Release) AssetByName(name string) (ReleaseAsset, bool) {
	for _, asset := range r.Assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return ReleaseAsset{}, false
}

func isNewerRelease(currentVersion string, latestVersion string) bool {
	if currentVersion == "dev" {
		_, ok := parseVersionTag(latestVersion)
		return ok
	}

	current, ok := parseVersionTag(currentVersion)
	if !ok {
		return false
	}
	latest, ok := parseVersionTag(latestVersion)
	if !ok {
		return false
	}

	for i := range current {
		if latest[i] > current[i] {
			return true
		}
		if latest[i] < current[i] {
			return false
		}
	}
	return false
}

func parseVersionTag(raw string) ([3]int, bool) {
	var version [3]int

	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "v") {
		return version, false
	}

	parts := strings.Split(strings.TrimPrefix(raw, "v"), ".")
	if len(parts) != 3 {
		return version, false
	}

	for i, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil {
			return version, false
		}
		version[i] = value
	}

	return version, true
}

func selectAssetName(goos string, goarch string) (string, error) {
	switch {
	case goos == "darwin" && goarch == "arm64":
		return "chatbox_darwin_arm64.tar.gz", nil
	case goos == "darwin" && goarch == "amd64":
		return "chatbox_darwin_amd64.tar.gz", nil
	default:
		return "", fmt.Errorf("unsupported platform %s/%s", goos, goarch)
	}
}
