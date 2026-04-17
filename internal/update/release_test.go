package update

import "testing"

func TestParseLatestReleaseExtractsStableAssets(t *testing.T) {
	t.Parallel()

	payload := []byte(`{
		"tag_name": "v0.1.0",
		"html_url": "https://github.com/HYPGAME/chatbox/releases/tag/v0.1.0",
		"assets": [
			{
				"name": "chatbox_darwin_arm64.tar.gz",
				"browser_download_url": "https://example.com/chatbox_darwin_arm64.tar.gz"
			},
			{
				"name": "checksums.txt",
				"browser_download_url": "https://example.com/checksums.txt"
			}
		]
	}`)

	release, err := parseLatestRelease(payload)
	if err != nil {
		t.Fatalf("parseLatestRelease returned error: %v", err)
	}
	if release.TagName != "v0.1.0" {
		t.Fatalf("expected tag %q, got %q", "v0.1.0", release.TagName)
	}
	if release.HTMLURL != "https://github.com/HYPGAME/chatbox/releases/tag/v0.1.0" {
		t.Fatalf("expected html url to be parsed, got %q", release.HTMLURL)
	}

	asset, ok := release.AssetByName("chatbox_darwin_arm64.tar.gz")
	if !ok {
		t.Fatal("expected darwin arm64 asset to be present")
	}
	if asset.DownloadURL != "https://example.com/chatbox_darwin_arm64.tar.gz" {
		t.Fatalf("expected asset url to be parsed, got %q", asset.DownloadURL)
	}
}

func TestIsNewerReleaseComparesSemanticTags(t *testing.T) {
	t.Parallel()

	if !isNewerRelease("v0.1.0", "v0.1.1") {
		t.Fatal("expected patch release to compare newer")
	}
	if isNewerRelease("v0.2.0", "v0.1.9") {
		t.Fatal("expected older release not to compare newer")
	}
}

func TestIsNewerReleaseTreatsDevAsOlderThanStable(t *testing.T) {
	t.Parallel()

	if !isNewerRelease("dev", "v0.1.0") {
		t.Fatal("expected stable tag to be newer than dev build")
	}
}

func TestIsNewerReleaseTreatsPrereleaseBuildsAsOlderThanStable(t *testing.T) {
	t.Parallel()

	if !isNewerRelease("v0.0.0-dev", "v0.1.4") {
		t.Fatal("expected stable tag to be newer than prerelease dev build")
	}
	if !isNewerRelease("v0.1.4-dev", "v0.1.4") {
		t.Fatal("expected stable tag to be newer than same-version prerelease build")
	}
}

func TestSelectAssetNameForPlatform(t *testing.T) {
	t.Parallel()

	name, err := selectAssetName("darwin", "arm64")
	if err != nil {
		t.Fatalf("selectAssetName returned error: %v", err)
	}
	if name != "chatbox_darwin_arm64.tar.gz" {
		t.Fatalf("expected darwin arm64 asset, got %q", name)
	}

	if _, err := selectAssetName("linux", "amd64"); err == nil {
		t.Fatal("expected unsupported platform to fail")
	}
}

func TestSelectAssetNameSupportsAndroidArm64(t *testing.T) {
	t.Parallel()

	name, err := selectAssetName("android", "arm64")
	if err != nil {
		t.Fatalf("selectAssetName returned error: %v", err)
	}
	if name != "chatbox_android_arm64.tar.gz" {
		t.Fatalf("expected android arm64 asset, got %q", name)
	}
}

func TestSelectAssetNameSupportsLinuxArm64(t *testing.T) {
	t.Parallel()

	name, err := selectAssetName("linux", "arm64")
	if err != nil {
		t.Fatalf("selectAssetName returned error: %v", err)
	}
	if name != "chatbox_linux_arm64.tar.gz" {
		t.Fatalf("expected linux arm64 asset, got %q", name)
	}
}
