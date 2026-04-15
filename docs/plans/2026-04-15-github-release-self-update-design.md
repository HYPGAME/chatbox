# GitHub Release Self-Update Design

## Goal

Allow `chatbox` to update itself from GitHub Releases and notify the user about newer versions on startup, without requiring Go to be installed on the target machine.

## Product Decisions

- Distribution source: GitHub Releases for `HYPGAME/chatbox`
- Target platforms for the first version: `darwin/arm64` and `darwin/amd64`
- Update command: `chatbox self-update`
- Startup check: always run on startup, asynchronously, without blocking chat startup
- Prompting behavior: print a single upgrade hint when a newer stable release exists
- Replacement strategy: try in-place replacement of the running binary first
- Fallback strategy: if in-place replacement fails, write the new binary next to the current executable and print a manual replacement instruction
- Integrity protection: verify downloaded release assets with SHA256 checksums
- Release cadence: publish from git tags with GitHub Actions
- Release channel: stable releases only for the initial implementation; ignore prereleases

## Why This Design

The user wants a real upgrade flow, not just source synchronization. `git pull && go build` works for developers, but it pushes build tooling requirements onto every machine and does not fit the expected UX of a CLI that can keep itself current.

GitHub Releases is the simplest stable distribution layer for this project:

- it matches the current repository plan
- it supports platform-specific binaries
- it has a well-defined latest release API
- it works well with GitHub Actions automation

This gives us one release pipeline and one client update path. That keeps the release contract small and predictable.

## Release Contract

Each stable tag such as `v0.1.0` will produce a GitHub Release containing:

- `chatbox_darwin_arm64.tar.gz`
- `chatbox_darwin_amd64.tar.gz`
- `checksums.txt`

Each archive will contain exactly one executable named `chatbox`.

`checksums.txt` will contain SHA256 entries for all uploaded assets. The updater will only trust assets whose checksum entry matches the downloaded file.

The CLI will embed a version string:

- development builds: `dev`
- tagged releases: semantic version tag such as `v0.1.0`

## Client Architecture

### Version Source

Add a small internal version package or build variable so the CLI can expose its current version consistently to:

- `chatbox version`
- startup update checks
- `chatbox self-update`

### Release Discovery

The updater will call the GitHub Releases API for the latest stable release of `HYPGAME/chatbox`.

It will extract:

- latest tag name
- asset list
- release URL for user-facing messages

It will then compare the latest release tag with the current binary version.

Rules:

- if current version is `dev`, startup may still notify that a stable release exists
- if current version equals the latest tag, no message is shown
- if the latest release is older or unparsable, do nothing

### Asset Selection

Asset choice is based on `runtime.GOOS` and `runtime.GOARCH`.

Initial mapping:

- `darwin/arm64` -> `chatbox_darwin_arm64.tar.gz`
- `darwin/amd64` -> `chatbox_darwin_amd64.tar.gz`

If no matching asset exists, `self-update` fails with a clear message rather than attempting a partial update.

### Integrity Validation

`self-update` downloads:

1. `checksums.txt`
2. the platform archive

It computes SHA256 for the archive and verifies it against `checksums.txt` before extracting anything into the executable location.

Checksum mismatch is a hard failure.

### Replacement Flow

Recommended replacement sequence:

1. Resolve the current executable path.
2. Download the new archive to a temporary directory.
3. Verify SHA256.
4. Extract `chatbox`.
5. Apply executable permissions.
6. Write a sibling temp file near the current binary.
7. Rename current binary to a backup path such as `chatbox.old`.
8. Rename the new temp file into the final binary path.
9. Remove the backup on success.

If step 6-8 fails because of permissions or filesystem constraints:

- keep the current binary untouched
- write the extracted replacement as a sibling file if possible, for example `chatbox.new`
- print a precise manual replacement instruction

We should prefer atomic rename over in-place write because it reduces the chance of ending up with a truncated binary.

## Startup Update Check

The startup check should run in a background goroutine launched near process startup.

Behavior:

- do not block `host`, `join`, or `keygen`
- do not run for `self-update` itself
- fetch the latest stable release
- compare against current version
- if a newer release exists, print one concise line to stderr

Example:

```text
new version available: v0.1.1 (current: v0.1.0)
run: chatbox self-update
```

Failures such as network errors, GitHub rate limits, or malformed responses should fail silently by default. They must never stop chat startup.

## CLI Surface

Recommended command additions:

- `chatbox version`
- `chatbox self-update`

Optional future additions, but not required now:

- `chatbox self-update --check`
- `chatbox self-update --channel prerelease`

For the first implementation, keep the surface minimal.

## GitHub Actions Release Flow

Release automation will be tag-driven.

Trigger:

- push tag matching `v*`

Workflow responsibilities:

1. check out code
2. build `chatbox` for `darwin/arm64` and `darwin/amd64`
3. inject the tag version at build time
4. package binaries as tar.gz
5. generate `checksums.txt`
6. create or publish the GitHub Release
7. upload all assets
8. optionally generate release notes

This gives the project one clear publishing motion:

```bash
git tag v0.1.0
git push origin v0.1.0
```

## Repository Prerequisite

The current local project directory is not yet a git repository, and the remote GitHub repository is currently empty.

Before automated releases can work, the project needs:

- `git init`
- a remote such as `origin` pointing to `https://github.com/HYPGAME/chatbox.git`
- an initial push of the current project state

This prerequisite is operational, not architectural, but it must be completed before the release workflow can be exercised.

## Failure Strategy

### Startup Check

- network failure: ignore
- API parse failure: ignore
- missing version metadata: ignore

### Self-Update

- missing matching asset: fail with message
- checksum mismatch: fail with message
- extraction failure: fail with message
- replacement failure: preserve current binary and fall back to manual replacement artifact when possible

### Release Automation

- workflow failure should fail the release build before publishing incomplete assets
- do not publish a release missing `checksums.txt`

## Testing Strategy

### Unit Tests

- version comparison logic
- asset name selection from platform tuple
- checksum parsing and validation
- release payload parsing
- updater fallback behavior when rename/write fails
- startup notifier only emits when current version is older than latest stable release

### Integration Tests

- self-update against a stub HTTP server serving a fake release payload and archive
- extraction and replacement logic using temp directories

### Manual Validation

1. Build a local binary with an older version string.
2. Publish or simulate a newer release payload.
3. Start `chatbox host ...` and confirm the update hint appears asynchronously.
4. Run `chatbox self-update`.
5. Confirm the binary reports the new version afterward.
6. Validate the fallback path by running from a non-writable install location.

## Non-Goals

- Homebrew distribution
- Linux or Windows update assets
- prerelease channels
- signed binaries or notarization
- automatic rollback after a successfully swapped but later-failing binary
