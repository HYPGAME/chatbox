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

func StartBackgroundCheckChannel(ctx context.Context, source latestReleaseSource, currentVersion string, out chan<- string) {
	go func() {
		defer close(out)
		checkAndNotifyChannel(ctx, source, currentVersion, out)
	}()
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

func checkAndNotifyChannel(ctx context.Context, source latestReleaseSource, currentVersion string, out chan<- string) {
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

	select {
	case out <- fmt.Sprintf("new version available: %s (current: %s)\nrun: chatbox self-update", release.TagName, currentVersion):
	default:
	}
}
