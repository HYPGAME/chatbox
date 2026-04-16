package release

import "testing"

func TestValidateVersionAcceptsSemanticTag(t *testing.T) {
	t.Parallel()

	if err := ValidateVersion("v0.1.3"); err != nil {
		t.Fatalf("ValidateVersion returned error: %v", err)
	}
}

func TestValidateVersionRejectsInvalidFormats(t *testing.T) {
	t.Parallel()

	cases := []string{"0.1.3", "v1", "latest", "v1.2.3-rc1"}
	for _, tc := range cases {
		if err := ValidateVersion(tc); err == nil {
			t.Fatalf("expected invalid version %q to fail", tc)
		}
	}
}

func TestArtifactNamesForVersionedRelease(t *testing.T) {
	t.Parallel()

	assets := ArtifactNames()
	want := []string{
		"chatbox_darwin_arm64.tar.gz",
		"chatbox_darwin_amd64.tar.gz",
		"chatbox_android_arm64.tar.gz",
		"checksums.txt",
	}
	if len(assets) != len(want) {
		t.Fatalf("expected %d assets, got %d", len(want), len(assets))
	}
	for i := range want {
		if assets[i] != want[i] {
			t.Fatalf("expected asset %d to be %q, got %q", i, want[i], assets[i])
		}
	}
}

func TestReleaseArtifactsIncludeAndroidArm64Archive(t *testing.T) {
	t.Parallel()

	assets := ArtifactNames()
	found := false
	for _, asset := range assets {
		if asset == "chatbox_android_arm64.tar.gz" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected android arm64 archive in release artifacts")
	}
}
