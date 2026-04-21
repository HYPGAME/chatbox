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
	Notes   string
	Assets  []ReleaseAsset
}

type ReleaseAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
}

type comparableVersion struct {
	core       [3]int
	prerelease bool
}

type githubRelease struct {
	TagName string         `json:"tag_name"`
	HTMLURL string         `json:"html_url"`
	Body    string         `json:"body"`
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
		Notes:   raw.Body,
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
	current, ok := parseComparableVersion(currentVersion)
	if !ok {
		return false
	}
	latest, ok := parseComparableVersion(latestVersion)
	if !ok {
		return false
	}

	for i := range current.core {
		if latest.core[i] > current.core[i] {
			return true
		}
		if latest.core[i] < current.core[i] {
			return false
		}
	}
	if current.prerelease != latest.prerelease {
		return current.prerelease && !latest.prerelease
	}
	return false
}

func parseComparableVersion(raw string) (comparableVersion, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "dev" {
		return comparableVersion{prerelease: true}, true
	}

	prerelease := false
	base := raw
	if index := strings.Index(base, "-"); index >= 0 {
		prerelease = true
		base = base[:index]
	}

	core, ok := parseVersionTag(base)
	if !ok {
		return comparableVersion{}, false
	}
	return comparableVersion{
		core:       core,
		prerelease: prerelease,
	}, true
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
	case goos == "linux" && goarch == "arm64":
		return "chatbox_linux_arm64.tar.gz", nil
	case goos == "android" && goarch == "arm64":
		return "chatbox_android_arm64.tar.gz", nil
	default:
		return "", fmt.Errorf("unsupported platform %s/%s", goos, goarch)
	}
}
