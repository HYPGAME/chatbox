package update

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

type Outcome struct {
	Status         string
	Detail         string
	CurrentVersion string
	LatestVersion  string
	FallbackPath   string
	Restartable    bool
}

type appliedRelease struct {
	ExecutablePath string
	FallbackPath   string
}

func (c Client) PerformUpdate(ctx context.Context, targetVersion string) (Outcome, error) {
	if c.goos() == "android" {
		return Outcome{
			Status:         "android-manual-required",
			CurrentVersion: c.CurrentVersion,
		}, nil
	}

	release, err := c.resolveRelease(ctx, targetVersion)
	if err != nil {
		return Outcome{
			Status:         "resolve-latest-failed",
			Detail:         err.Error(),
			CurrentVersion: c.CurrentVersion,
		}, nil
	}
	if c.CurrentVersion != "" && !isNewerRelease(c.CurrentVersion, release.TagName) {
		return Outcome{
			Status:         "already-up-to-date",
			CurrentVersion: c.CurrentVersion,
			LatestVersion:  release.TagName,
		}, nil
	}

	result, err := c.applyRelease(ctx, release)
	if err != nil {
		return Outcome{
			Status:         classifyUpdateError(err),
			Detail:         err.Error(),
			CurrentVersion: c.CurrentVersion,
			LatestVersion:  release.TagName,
		}, nil
	}

	outcome := Outcome{
		Status:         "success",
		CurrentVersion: c.CurrentVersion,
		LatestVersion:  release.TagName,
		Restartable:    true,
	}
	if result.FallbackPath != "" {
		outcome.Status = "fallback-written"
		outcome.FallbackPath = result.FallbackPath
		outcome.Restartable = false
	}
	return outcome, nil
}

func (c Client) resolveRelease(ctx context.Context, targetVersion string) (Release, error) {
	targetVersion = strings.TrimSpace(targetVersion)
	if targetVersion == "" {
		return c.LatestRelease(ctx)
	}
	return c.releaseByTag(ctx, targetVersion)
}

func (c Client) releaseByTag(ctx context.Context, tag string) (Release, error) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return Release{}, fmt.Errorf("release tag is required")
	}
	if c.ReleaseByTag != nil {
		return c.ReleaseByTag(ctx, tag)
	}
	endpoint := strings.TrimRight(c.baseURL(), "/") + "/repos/" + c.repository() + "/releases/tags/" + url.PathEscape(tag)
	payload, err := c.download(ctx, endpoint)
	if err != nil {
		return Release{}, err
	}
	return parseLatestRelease(payload)
}

func (c Client) applyRelease(ctx context.Context, release Release) (appliedRelease, error) {
	assetName, err := selectAssetName(c.goos(), c.goarch())
	if err != nil {
		return appliedRelease{}, err
	}
	archiveAsset, ok := release.AssetByName(assetName)
	if !ok {
		return appliedRelease{}, fmt.Errorf("release %s is missing asset %q", release.TagName, assetName)
	}
	checksumAsset, ok := release.AssetByName("checksums.txt")
	if !ok {
		return appliedRelease{}, fmt.Errorf("release %s is missing asset %q", release.TagName, "checksums.txt")
	}

	checksumPayload, err := c.download(ctx, checksumAsset.DownloadURL)
	if err != nil {
		return appliedRelease{}, err
	}
	expectedChecksum, err := parseChecksums(checksumPayload, assetName)
	if err != nil {
		return appliedRelease{}, err
	}

	archivePayload, err := c.download(ctx, archiveAsset.DownloadURL)
	if err != nil {
		return appliedRelease{}, err
	}
	if err := verifyChecksum(archivePayload, expectedChecksum); err != nil {
		return appliedRelease{}, err
	}

	binary, err := extractChatboxBinaryFromTarGz(archivePayload)
	if err != nil {
		return appliedRelease{}, err
	}
	executablePath, err := c.executablePath()()
	if err != nil {
		return appliedRelease{}, fmt.Errorf("resolve executable path: %w", err)
	}
	applyResult, err := c.applyUpdate()(executablePath, binary)
	if err != nil {
		return appliedRelease{}, err
	}
	if applyResult.FallbackPath == "" {
		appliedVersion, err := c.readVersion()(executablePath)
		if err != nil {
			return appliedRelease{}, fmt.Errorf("verify updated binary version: %w", err)
		}
		if strings.TrimSpace(appliedVersion) != strings.TrimSpace(release.TagName) {
			return appliedRelease{}, fmt.Errorf("verify updated binary version: expected %s, got %s", release.TagName, strings.TrimSpace(appliedVersion))
		}
	}
	return appliedRelease{
		ExecutablePath: executablePath,
		FallbackPath:   applyResult.FallbackPath,
	}, nil
}

func classifyUpdateError(err error) string {
	if err == nil {
		return "success"
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "checksum"):
		return "checksum-failed"
	case strings.Contains(text, "extract"):
		return "extract-failed"
	case strings.Contains(text, "replace"), strings.Contains(text, "activate"):
		return "replace-failed"
	default:
		return "download-failed"
	}
}
