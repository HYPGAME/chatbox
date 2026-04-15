package update

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
)

const defaultAPIBaseURL = "https://api.github.com"

type Client struct {
	BaseURL        string
	Repository     string
	HTTPClient     *http.Client
	CurrentVersion string
	GOOS           string
	GOARCH         string
	ExecutablePath func() (string, error)
	ApplyUpdate    func(string, []byte) (ApplyResult, error)
}

type SelfUpdateResult struct {
	CurrentVersion string
	LatestVersion  string
	ReleaseURL     string
	FallbackPath   string
	Updated        bool
}

func (c Client) LatestRelease(ctx context.Context) (Release, error) {
	endpoint := strings.TrimRight(c.baseURL(), "/") + "/repos/" + c.repository() + "/releases/latest"
	payload, err := c.download(ctx, endpoint)
	if err != nil {
		return Release{}, err
	}
	return parseLatestRelease(payload)
}

func (c Client) SelfUpdate(ctx context.Context) (SelfUpdateResult, error) {
	release, err := c.LatestRelease(ctx)
	if err != nil {
		return SelfUpdateResult{}, err
	}

	result := SelfUpdateResult{
		CurrentVersion: c.CurrentVersion,
		LatestVersion:  release.TagName,
		ReleaseURL:     release.HTMLURL,
	}
	if c.CurrentVersion != "" && !isNewerRelease(c.CurrentVersion, release.TagName) {
		return result, nil
	}

	assetName, err := selectAssetName(c.goos(), c.goarch())
	if err != nil {
		return SelfUpdateResult{}, err
	}
	archiveAsset, ok := release.AssetByName(assetName)
	if !ok {
		return SelfUpdateResult{}, fmt.Errorf("release %s is missing asset %q", release.TagName, assetName)
	}
	checksumAsset, ok := release.AssetByName("checksums.txt")
	if !ok {
		return SelfUpdateResult{}, fmt.Errorf("release %s is missing asset %q", release.TagName, "checksums.txt")
	}

	checksumPayload, err := c.download(ctx, checksumAsset.DownloadURL)
	if err != nil {
		return SelfUpdateResult{}, err
	}
	expectedChecksum, err := parseChecksums(checksumPayload, assetName)
	if err != nil {
		return SelfUpdateResult{}, err
	}

	archivePayload, err := c.download(ctx, archiveAsset.DownloadURL)
	if err != nil {
		return SelfUpdateResult{}, err
	}
	if err := verifyChecksum(archivePayload, expectedChecksum); err != nil {
		return SelfUpdateResult{}, err
	}

	binary, err := extractChatboxBinaryFromTarGz(archivePayload)
	if err != nil {
		return SelfUpdateResult{}, err
	}
	executablePath, err := c.executablePath()()
	if err != nil {
		return SelfUpdateResult{}, fmt.Errorf("resolve executable path: %w", err)
	}
	applyResult, err := c.applyUpdate()(executablePath, binary)
	if err != nil {
		return SelfUpdateResult{}, err
	}

	result.FallbackPath = applyResult.FallbackPath
	result.Updated = true
	return result, nil
}

func (c Client) download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: unexpected status %s", url, resp.Status)
	}

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", url, err)
	}
	return payload, nil
}

func (c Client) baseURL() string {
	if c.BaseURL == "" {
		return defaultAPIBaseURL
	}
	return c.BaseURL
}

func (c Client) repository() string {
	if c.Repository == "" {
		return "HYPGAME/chatbox"
	}
	return c.Repository
}

func (c Client) httpClient() *http.Client {
	if c.HTTPClient == nil {
		return http.DefaultClient
	}
	return c.HTTPClient
}

func (c Client) goos() string {
	if c.GOOS == "" {
		return runtime.GOOS
	}
	return c.GOOS
}

func (c Client) goarch() string {
	if c.GOARCH == "" {
		return runtime.GOARCH
	}
	return c.GOARCH
}

func (c Client) executablePath() func() (string, error) {
	if c.ExecutablePath == nil {
		return os.Executable
	}
	return c.ExecutablePath
}

func (c Client) applyUpdate() func(string, []byte) (ApplyResult, error) {
	if c.ApplyUpdate == nil {
		return func(path string, binary []byte) (ApplyResult, error) {
			return applyUpdateAtPath(path, binary, defaultApplyFileOps())
		}
	}
	return c.ApplyUpdate
}
