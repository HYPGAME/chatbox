package release

import (
	"fmt"
	"regexp"
)

var versionPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)

func ValidateVersion(version string) error {
	if !versionPattern.MatchString(version) {
		return fmt.Errorf("invalid version %q: use vMAJOR.MINOR.PATCH", version)
	}
	return nil
}

func ArtifactNames() []string {
	return []string{
		"chatbox_darwin_arm64.tar.gz",
		"chatbox_darwin_amd64.tar.gz",
		"chatbox_android_arm64.tar.gz",
		"checksums.txt",
	}
}
