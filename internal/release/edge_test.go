package release

import "testing"

func TestEdgeTagForCommit(t *testing.T) {
	t.Parallel()

	tag := EdgeTagForCommit("abcdef1234567890")
	if tag != "edge-abcdef1" {
		t.Fatalf("expected short edge tag, got %q", tag)
	}
}

func TestEdgeVersionForCommit(t *testing.T) {
	t.Parallel()

	version := EdgeVersionForCommit("abcdef1234567890")
	if version != "edge-abcdef1" {
		t.Fatalf("expected short edge version, got %q", version)
	}
}
