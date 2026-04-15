# GitHub Release Self-Update Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add GitHub Releases based version checking and self-update support to `chatbox`, with automated tag-driven release publishing for macOS binaries.

**Architecture:** Introduce a small update subsystem that knows the current build version, queries the latest stable GitHub Release, selects a platform-specific asset, verifies it with `checksums.txt`, and swaps the current executable atomically when possible. Wire an asynchronous startup check into the CLI entrypoint and add a GitHub Actions workflow that builds and publishes release assets from git tags.

**Tech Stack:** Go, GitHub Releases REST API, SHA256 verification, tar.gz archives, GitHub Actions

---

### Task 1: Repository and build metadata bootstrap

**Files:**
- Create: `.gitignore`
- Modify: `cmd/chatbox/main.go`
- Create: `internal/version/version.go`
- Test: `cmd/chatbox/main_test.go`

**Step 1: Write the failing test**

Add tests that verify:

```go
func TestRunVersionPrintsCurrentVersion(t *testing.T) { ... }
func TestUsageIncludesVersionAndSelfUpdateCommands(t *testing.T) { ... }
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/chatbox -run 'TestRunVersionPrintsCurrentVersion|TestUsageIncludesVersionAndSelfUpdateCommands'`

Expected: FAIL because no version command or version package exists.

**Step 3: Write minimal implementation**

- add `internal/version/version.go` with a default exported version string set to `dev`
- add `version` command handling in `cmd/chatbox/main.go`
- update usage text to mention `version` and `self-update`
- add `.gitignore` entries for `chatbox`, `*.psk`, and release artifacts

**Step 4: Run test to verify it passes**

Run: `go test ./cmd/chatbox -run 'TestRunVersionPrintsCurrentVersion|TestUsageIncludesVersionAndSelfUpdateCommands'`

Expected: PASS

**Step 5: Commit**

Run if repo is initialized:

```bash
git add .gitignore cmd/chatbox/main.go cmd/chatbox/main_test.go internal/version/version.go
git commit -m "feat: add version command and build metadata"
```

If git is not initialized yet, record that the commit step was skipped.

### Task 2: Release parsing and version comparison

**Files:**
- Create: `internal/update/release.go`
- Create: `internal/update/release_test.go`

**Step 1: Write the failing test**

Add tests that verify:

```go
func TestParseLatestReleaseExtractsStableAssets(t *testing.T) { ... }
func TestIsNewerReleaseComparesSemanticTags(t *testing.T) { ... }
func TestIsNewerReleaseTreatsDevAsOlderThanStable(t *testing.T) { ... }
func TestSelectAssetNameForPlatform(t *testing.T) { ... }
```

Include fixture JSON that matches the fields returned by the GitHub latest release endpoint.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/update -run 'Test(ParseLatestReleaseExtractsStableAssets|IsNewerReleaseComparesSemanticTags|IsNewerReleaseTreatsDevAsOlderThanStable|SelectAssetNameForPlatform)'`

Expected: FAIL because no update package exists.

**Step 3: Write minimal implementation**

- add release model structs for the latest release payload
- parse tag name and asset metadata
- implement semantic tag comparison for `vMAJOR.MINOR.PATCH`
- implement platform-to-asset-name mapping for `darwin/arm64` and `darwin/amd64`

**Step 4: Run test to verify it passes**

Run: `go test ./internal/update -run 'Test(ParseLatestReleaseExtractsStableAssets|IsNewerReleaseComparesSemanticTags|IsNewerReleaseTreatsDevAsOlderThanStable|SelectAssetNameForPlatform)'`

Expected: PASS

**Step 5: Commit**

Run if repo is initialized:

```bash
git add internal/update/release.go internal/update/release_test.go
git commit -m "feat: parse latest release metadata"
```

If git is not initialized yet, record that the commit step was skipped.

### Task 3: Checksum verification and archive extraction

**Files:**
- Create: `internal/update/download.go`
- Create: `internal/update/download_test.go`

**Step 1: Write the failing test**

Add tests that verify:

```go
func TestParseChecksumsFindsExpectedAsset(t *testing.T) { ... }
func TestVerifyChecksumRejectsMismatch(t *testing.T) { ... }
func TestExtractChatboxBinaryFromTarGz(t *testing.T) { ... }
```

Use in-memory checksum text and an in-memory tar.gz payload built in the test.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/update -run 'Test(ParseChecksumsFindsExpectedAsset|VerifyChecksumRejectsMismatch|ExtractChatboxBinaryFromTarGz)'`

Expected: FAIL because checksum parsing and extraction do not exist.

**Step 3: Write minimal implementation**

- parse `checksums.txt` lines into filename/hash entries
- compute SHA256 for a downloaded archive
- verify the selected asset hash
- extract the `chatbox` executable bytes from tar.gz content

**Step 4: Run test to verify it passes**

Run: `go test ./internal/update -run 'Test(ParseChecksumsFindsExpectedAsset|VerifyChecksumRejectsMismatch|ExtractChatboxBinaryFromTarGz)'`

Expected: PASS

**Step 5: Commit**

Run if repo is initialized:

```bash
git add internal/update/download.go internal/update/download_test.go
git commit -m "feat: verify release checksums"
```

If git is not initialized yet, record that the commit step was skipped.

### Task 4: Self-update replacement flow

**Files:**
- Create: `internal/update/apply.go`
- Create: `internal/update/apply_test.go`

**Step 1: Write the failing test**

Add tests that verify:

```go
func TestApplyUpdateReplacesBinaryAtomically(t *testing.T) { ... }
func TestApplyUpdateFallsBackToSiblingFileWhenDirectReplaceFails(t *testing.T) { ... }
```

Inject filesystem operations so the fallback path can be exercised deterministically without relying on OS-specific permission behavior.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/update -run 'TestApplyUpdate(ReplacesBinaryAtomically|FallsBackToSiblingFileWhenDirectReplaceFails)'`

Expected: FAIL because replacement logic does not exist.

**Step 3: Write minimal implementation**

- resolve executable path
- write extracted binary to a sibling temp file
- preserve executable permissions
- rename current binary to backup then swap the new binary into place
- on failure, keep the original binary intact and emit fallback metadata for user messaging

**Step 4: Run test to verify it passes**

Run: `go test ./internal/update -run 'TestApplyUpdate(ReplacesBinaryAtomically|FallsBackToSiblingFileWhenDirectReplaceFails)'`

Expected: PASS

**Step 5: Commit**

Run if repo is initialized:

```bash
git add internal/update/apply.go internal/update/apply_test.go
git commit -m "feat: add binary replacement flow"
```

If git is not initialized yet, record that the commit step was skipped.

### Task 5: GitHub client and self-update command

**Files:**
- Create: `internal/update/client.go`
- Create: `internal/update/client_test.go`
- Modify: `cmd/chatbox/main.go`
- Test: `cmd/chatbox/main_test.go`

**Step 1: Write the failing test**

Add tests that verify:

```go
func TestSelfUpdateDownloadsAndAppliesLatestMatchingRelease(t *testing.T) { ... }
func TestSelfUpdateFailsWhenNoMatchingAssetExists(t *testing.T) { ... }
```

Use an `httptest.Server` to serve:

- latest release JSON
- `checksums.txt`
- platform archive

Stub the apply step so the test focuses on command orchestration rather than filesystem swap mechanics.

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/chatbox ./internal/update -run 'Test(SelfUpdateDownloadsAndAppliesLatestMatchingRelease|SelfUpdateFailsWhenNoMatchingAssetExists)'`

Expected: FAIL because the updater command path does not exist.

**Step 3: Write minimal implementation**

- add a GitHub release client with an injectable base URL and HTTP client
- download and validate the latest matching asset
- call the apply-update logic
- add `self-update` command wiring in `cmd/chatbox/main.go`
- print success and fallback/manual-replacement messages clearly

**Step 4: Run test to verify it passes**

Run: `go test ./cmd/chatbox ./internal/update -run 'Test(SelfUpdateDownloadsAndAppliesLatestMatchingRelease|SelfUpdateFailsWhenNoMatchingAssetExists)'`

Expected: PASS

**Step 5: Commit**

Run if repo is initialized:

```bash
git add cmd/chatbox/main.go cmd/chatbox/main_test.go internal/update/client.go internal/update/client_test.go
git commit -m "feat: add self-update command"
```

If git is not initialized yet, record that the commit step was skipped.

### Task 6: Asynchronous startup update notification

**Files:**
- Create: `internal/update/notify.go`
- Create: `internal/update/notify_test.go`
- Modify: `cmd/chatbox/main.go`
- Test: `cmd/chatbox/main_test.go`

**Step 1: Write the failing test**

Add tests that verify:

```go
func TestBackgroundUpdateCheckPrintsHintForNewerStableRelease(t *testing.T) { ... }
func TestBackgroundUpdateCheckDoesNothingWhenAlreadyCurrent(t *testing.T) { ... }
func TestBackgroundUpdateCheckDoesNotRunForSelfUpdateCommand(t *testing.T) { ... }
```

Inject the notifier output writer and release client so the check can be deterministic and non-networked.

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/chatbox ./internal/update -run 'Test(BackgroundUpdateCheckPrintsHintForNewerStableRelease|BackgroundUpdateCheckDoesNothingWhenAlreadyCurrent|BackgroundUpdateCheckDoesNotRunForSelfUpdateCommand)'`

Expected: FAIL because startup checking is not wired in.

**Step 3: Write minimal implementation**

- add a non-blocking background check helper
- launch it near the top of `run(...)`
- skip the check for `self-update`
- print one concise line when a newer stable release exists
- ignore network and parse failures by default

**Step 4: Run test to verify it passes**

Run: `go test ./cmd/chatbox ./internal/update -run 'Test(BackgroundUpdateCheckPrintsHintForNewerStableRelease|BackgroundUpdateCheckDoesNothingWhenAlreadyCurrent|BackgroundUpdateCheckDoesNotRunForSelfUpdateCommand)'`

Expected: PASS

**Step 5: Commit**

Run if repo is initialized:

```bash
git add cmd/chatbox/main.go cmd/chatbox/main_test.go internal/update/notify.go internal/update/notify_test.go
git commit -m "feat: add background update notifications"
```

If git is not initialized yet, record that the commit step was skipped.

### Task 7: Release workflow and documentation

**Files:**
- Create: `.github/workflows/release.yml`
- Modify: `README.md`

**Step 1: Write the failing test**

Add a documentation or configuration verification step instead of a Go unit test:

- inspect the workflow file path and make sure it does not exist yet
- define the expected release assets and tag trigger before writing the workflow

**Step 2: Run verification to confirm the precondition**

Run: `test ! -f .github/workflows/release.yml`

Expected: exit code `0` because the workflow is not present yet.

**Step 3: Write minimal implementation**

- create a GitHub Actions workflow that:
  - triggers on tags matching `v*`
  - builds `darwin/arm64` and `darwin/amd64`
  - injects version via `-ldflags`
  - packages `chatbox` into tar.gz archives
  - generates `checksums.txt`
  - publishes a GitHub Release with uploaded assets
- document release and update usage in `README.md`

**Step 4: Run verification to confirm the files look correct**

Run:

```bash
sed -n '1,240p' .github/workflows/release.yml
sed -n '1,240p' README.md
```

Expected: workflow shows `v*` tag trigger and the three release assets; README documents `version`, `self-update`, and release publishing.

**Step 5: Commit**

Run if repo is initialized:

```bash
git add .github/workflows/release.yml README.md
git commit -m "ci: automate release publishing"
```

If git is not initialized yet, record that the commit step was skipped.

### Task 8: Full verification and repository bootstrap

**Files:**
- No code changes required unless verification finds defects.

**Step 1: Run the full Go test suite**

Run: `go test ./...`

Expected: PASS

**Step 2: Run race checks on the update-sensitive packages**

Run: `go test -race ./cmd/chatbox ./internal/update ./internal/tui ./internal/session`

Expected: PASS

**Step 3: Build a local binary with an explicit version**

Run: `go build -ldflags \"-X chatbox/internal/version.Version=v0.0.0-dev\" -o ./chatbox ./cmd/chatbox`

Expected: PASS

**Step 4: Bootstrap git and connect the remote**

Run after code is ready:

```bash
git init
git branch -M main
git remote add origin https://github.com/HYPGAME/chatbox.git
git add .
git commit -m "feat: add GitHub release self-update flow"
git push -u origin main
```

Expected: local repository initialized and remote updated.

**Step 5: Manual release validation**

Run:

1. create and push a test tag such as `v0.1.0`
2. wait for GitHub Actions to publish the release
3. download a release binary and run `./chatbox version`
4. run `./chatbox self-update` from an older binary
5. run `./chatbox host --psk-file ...` and confirm the asynchronous upgrade hint behavior on a newer available release

Expected: release assets publish correctly, checksum validation succeeds, and the binary updates in place when writable.
