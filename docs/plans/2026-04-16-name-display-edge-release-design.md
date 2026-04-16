# Name Display and Edge Release Design

## Goal

Make chat rendering always show the sender's actual configured name instead of the synthetic `you` label, and publish a downloadable prerelease for every push to `main`.

## Product Decisions

### Message Labels

- All chat messages should render the sender name from the message/transcript data.
- Outgoing messages should no longer be rewritten to `you`.
- This applies equally to:
  - newly sent messages
  - retry markers such as `[retrying]`
  - transcript replay of outgoing history

### Edge Releases

- Keep the current tag-based stable release workflow unchanged for official versions.
- Add a second GitHub Actions workflow that runs on every push to `main`.
- Each push to `main` should create its own prerelease, not overwrite a shared nightly release.

## Why This Scope

The message label change is purely a rendering concern. The message payload already carries the real sender name, so the shortest path is to stop replacing it at render time.

The edge release requirement is best handled by a parallel prerelease pipeline rather than changing the stable release flow. That preserves:

- stable releases for `self-update`
- clear separation between tested tags and per-push builds
- rollback/debuggability because every push gets its own artifact set

## Rendering Design

Current behavior rewrites outgoing entries to `you` at the final render function.

New behavior should:

- always use `entry.from`
- keep outgoing-only status badges like `[retrying]` and `[failed]`
- leave system lines and error lines unchanged

No protocol, persistence, or retry semantics need to change.

## Edge Release Design

### Trigger

- GitHub Actions trigger: `push` on branch `main`

### Release Identity

- GitHub release tag: unique per commit, for example `edge-<short-sha>`
- GitHub release type: prerelease
- Embedded binary version: use a non-stable edge label such as `edge-<short-sha>`

Using a non-semver edge version is intentional. It keeps edge builds distinct from stable builds and avoids pretending they are official semantic versions.

### Assets

Edge prereleases should publish the same asset set as stable releases:

- `chatbox_darwin_arm64.tar.gz`
- `chatbox_darwin_amd64.tar.gz`
- `chatbox_android_arm64.tar.gz`
- `checksums.txt`

## Constraints

- Do not make every push produce a stable release.
- Do not change `self-update` to consume prereleases.
- Do not add new runtime behavior to distinguish "single chat" vs "group chat" names; rendering should be uniform.

## Testing

Add coverage for:

- outgoing messages render the local configured name instead of `you`
- outgoing scrollback/retry output also uses the configured name
- any new helper that computes edge release tags or version strings, if introduced

Workflow files themselves may still need static verification rather than full runtime tests, but any extracted helper logic should be unit-tested.
