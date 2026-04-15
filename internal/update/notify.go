package update

import (
	"context"
	"fmt"
	"io"
)

type latestReleaseSource interface {
	LatestRelease(context.Context) (Release, error)
}

func StartBackgroundCheck(ctx context.Context, source latestReleaseSource, currentVersion string, out io.Writer) {
	go checkAndNotify(ctx, source, currentVersion, out)
}

func checkAndNotify(ctx context.Context, source latestReleaseSource, currentVersion string, out io.Writer) {
	if source == nil || out == nil {
		return
	}

	release, err := source.LatestRelease(ctx)
	if err != nil {
		return
	}
	if !isNewerRelease(currentVersion, release.TagName) {
		return
	}

	_, _ = fmt.Fprintf(out, "new version available: %s (current: %s)\nrun: chatbox self-update\n", release.TagName, currentVersion)
}
