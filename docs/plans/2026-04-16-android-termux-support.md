# Android Termux Support Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add an Android Termux distribution and usage path for `chatbox` without changing the chat protocol or building a native Android app.

**Architecture:** Extend the existing release pipeline to publish an `android/arm64` binary archive, keep startup version checks intact, and make Android `self-update` fail fast with a clear manual-upgrade message. Document Termux installation and platform limitations in the README.

**Tech Stack:** Go, existing GitHub release/update flow, GitHub Actions, shell release script, README docs

---

### Task 1: Add Android asset selection coverage

**Files:**
- Modify: `internal/update/release_test.go`
- Modify: `internal/release/manual_test.go`

**Step 1: Write the failing tests**

Add tests that verify:

```go
func TestSelectAssetNameSupportsAndroidArm64(t *testing.T) { ... }
func TestReleaseArtifactsIncludeAndroidArm64Archive(t *testing.T) { ... }
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/update ./internal/release -run 'Test(SelectAssetNameSupportsAndroidArm64|ReleaseArtifactsIncludeAndroidArm64Archive)'`

Expected: FAIL because Android assets are not part of the current release model.

**Step 3: Write minimal implementation**

- add `chatbox_android_arm64.tar.gz` to asset selection
- add the same archive to the manual release artifact list

**Step 4: Run test to verify it passes**

Run: `go test ./internal/update ./internal/release -run 'Test(SelectAssetNameSupportsAndroidArm64|ReleaseArtifactsIncludeAndroidArm64Archive)'`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/update/release.go internal/update/release_test.go internal/release/manual.go internal/release/manual_test.go
git commit -m "feat: add android release asset support"
```

### Task 2: Make Android self-update fail fast with a clear message

**Files:**
- Modify: `internal/update/client_test.go`
- Modify: `internal/update/client.go`

**Step 1: Write the failing test**

Add a test verifying Android self-update returns a clear manual-upgrade error before download/extract logic runs.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/update -run 'TestSelfUpdateRejectsAndroidWithManualUpgradeMessage'`

Expected: FAIL because Android currently falls into an unsupported-platform path with a generic error.

**Step 3: Write minimal implementation**

- add an explicit Android guard in `SelfUpdate`
- return a user-facing error that points users to GitHub Releases/manual replacement

**Step 4: Run test to verify it passes**

Run: `go test ./internal/update -run 'TestSelfUpdateRejectsAndroidWithManualUpgradeMessage'`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/update/client.go internal/update/client_test.go
git commit -m "fix: clarify android self-update behavior"
```

### Task 3: Publish Android archives from both release paths

**Files:**
- Modify: `.github/workflows/release.yml`
- Modify: `scripts/release-manual.sh`
- Modify: `scripts/release-manual_test.sh`

**Step 1: Write the failing tests/checks**

Add or update tests/checks verifying:

- the manual release script emits `chatbox_android_arm64.tar.gz`
- dry-run output still covers publish flow with the extra artifact

**Step 2: Run test to verify it fails**

Run: `bash scripts/release-manual_test.sh`

Expected: FAIL because only macOS archives are built today.

**Step 3: Write minimal implementation**

- build `GOOS=android GOARCH=arm64`
- package it as `chatbox_android_arm64.tar.gz`
- add it to checksums and GitHub release uploads

**Step 4: Run test to verify it passes**

Run: `bash scripts/release-manual_test.sh`

Expected: PASS

### Task 4: Document the Android Termux usage path

**Files:**
- Modify: `README.md`

**Step 1: Write the failing check**

Run: `rg -n "Termux|Android|android/arm64" README.md`

Expected: no relevant Android usage section.

**Step 2: Write minimal implementation**

- add an Android/Termux section
- document install/build, run examples, and limitations
- update release/self-update wording so it no longer implies macOS-only behavior everywhere

**Step 3: Run documentation check**

Run: `rg -n "Termux|Android|android/arm64" README.md`

Expected: matches for the new section and release asset references.

**Step 4: Commit**

```bash
git add README.md .github/workflows/release.yml scripts/release-manual.sh scripts/release-manual_test.sh
git commit -m "docs: add android termux usage"
```

### Task 5: Full verification

**Files:**
- Modify: touched files from Tasks 1-4

**Step 1: Run project tests**

Run: `go test ./...`

Expected: PASS

**Step 2: Run shell release-script tests**

Run: `bash scripts/release-manual_test.sh`

Expected: PASS

**Step 3: Cross-compile sanity check**

Run: `GOOS=android GOARCH=arm64 go build -o /tmp/chatbox-android-arm64 ./cmd/chatbox`

Expected: PASS
