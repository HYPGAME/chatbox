package release

import "strings"

func EdgeTagForCommit(commit string) string {
	return "edge-" + shortCommit(commit)
}

func EdgeVersionForCommit(commit string) string {
	return "edge-" + shortCommit(commit)
}

func shortCommit(commit string) string {
	normalized := strings.ToLower(strings.TrimSpace(commit))
	if len(normalized) > 7 {
		return normalized[:7]
	}
	return normalized
}
