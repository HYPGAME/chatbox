# Manual Release Script Design

## Goal

Add a single local command that performs the current fallback release flow end-to-end without relying on GitHub Actions.

## Product Decision

- Entry point: `./scripts/release-manual.sh vX.Y.Z`
- Version input: explicit, required, semantic tag format only
- Release type: stable release only
- Output contract:
  - `chatbox_darwin_arm64.tar.gz`
  - `chatbox_darwin_amd64.tar.gz`
  - `checksums.txt`
- Publishing target: `HYPGAME/chatbox`
- Distribution path: GitHub Releases via `gh release create`
- Build source: current local `main` branch state

## Why This Design

GitHub Actions is currently blocked by the account billing lock, but the release artifacts and `self-update` flow already work when a valid GitHub Release exists. The shortest path is therefore not more CI work. It is a reliable local release command that reproduces the same artifact contract every time.

The user asked for full automation. That means the script should not stop at "build locally". It should validate the repo state, build, tag, push, publish, and then print the release URL.

Using an explicit version argument is the safest option:

- no hidden version bump logic
- no ambiguity about the release being published
- easy to audit in shell history

## Script Responsibilities

The script should perform these steps in order:

1. Validate prerequisites.
2. Validate repository state.
3. Run tests.
4. Build macOS release artifacts.
5. Generate `checksums.txt`.
6. Push `main`.
7. Create and push the tag.
8. Create the GitHub Release with assets.
9. Print the release URL and next-step hints.

## Validation Rules

### Prerequisites

The script must fail fast with a clear error if any of these are unavailable:

- `git`
- `gh`
- `go`
- `tar`
- `shasum`

It should also require:

- `gh auth status` succeeds
- current directory is inside the project git repository

### Version Validation

The version argument must:

- be present
- match `^v[0-9]+\.[0-9]+\.[0-9]+$`

Rejected examples:

- `0.1.3`
- `v1`
- `latest`

### Repository Validation

The script should require:

- current branch is `main`
- working tree is clean
- local tag does not already exist
- remote tag does not already exist

The script should push `main` before pushing the tag so the release commit is guaranteed to exist on the remote.

## Build Contract

Build commands should mirror the release workflow contract already used by the project:

- `GOOS=darwin GOARCH=arm64`
- `GOOS=darwin GOARCH=amd64`
- `-ldflags "-X chatbox/internal/version.Version=<version>"`

Artifacts should be assembled into `dist/`:

- `dist/chatbox_darwin_arm64.tar.gz`
- `dist/chatbox_darwin_amd64.tar.gz`
- `dist/checksums.txt`

`checksums.txt` must list bare asset names, not `dist/`-prefixed paths, because the updater matches against release asset names.

## Publishing Behavior

Tagging and release creation should happen only after a successful build and checksum generation.

Recommended flow:

1. `git push origin main`
2. `git tag <version>`
3. `git push origin refs/tags/<version>`
4. `gh release create <version> ...`

Release notes can stay minimal for now. A short fixed note is enough because the primary goal is to automate the already-tested fallback path.

## Failure Strategy

### Before Tag Creation

If validation, tests, build, or `main` push fails:

- exit non-zero
- do not create a tag
- do not create a release

### After Tag Push But Before Release Creation

If the tag already exists remotely but `gh release create` fails:

- exit non-zero
- print the exact tag and remote state
- print suggested recovery commands
- do not auto-delete the tag

This keeps the script non-destructive.

### After Release Creation

If release creation succeeds:

- print the release URL
- print a reminder that collaborators can update with `chatbox self-update`

## Testing Strategy

Because the user asked for one-command automation, the critical risks are validation gaps and accidental publication with bad state. The implementation should therefore separate shell orchestration from logic that can be unit-tested.

Recommended split:

- shell wrapper: `scripts/release-manual.sh`
- small Go helper package or helper command for validation and release-plan logic, if needed for testability

At minimum, tests should cover:

- version format validation
- artifact naming
- checksum generation contract
- refusal on dirty working tree
- refusal when tag already exists

## Documentation

README should gain a `Manual Release` section showing:

```bash
./scripts/release-manual.sh v0.1.3
```

It should also explain that this is the supported fallback while GitHub Actions remains blocked by the account billing issue.

## Non-Goals

- automatic semantic version bumping
- prerelease channel support
- automatic rollback of pushed tags
- Linux or Windows release artifacts
- replacing GitHub Actions once the billing issue is resolved
