# Self-Update Release Notes Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Show newly released feature notes after `chatbox self-update` successfully updates to a newer version.

**Architecture:** Extend release parsing so `internal/update` can carry GitHub Release notes through `SelfUpdateResult`. Keep the CLI output logic in `cmd/chatbox/main.go`: after a real update, print the new version plus a short release-notes section, with a safe fallback to the release URL when notes are empty.

**Tech Stack:** Go, GitHub Releases API payload parsing, existing `internal/update` self-update flow, Go tests.

---

### Task 1: Carry release notes through update metadata

**Files:**
- Modify: `internal/update/release.go`
- Modify: `internal/update/client.go`
- Test: `internal/update/release_test.go`
- Test: `internal/update/client_test.go`

**Step 1: Write the failing tests**

- Add a release parsing test that expects `body` to populate a release notes field.
- Add a self-update test that expects `SelfUpdateResult` to expose those notes after a successful update.

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/update -run 'TestParseLatestRelease|TestSelfUpdateDownloadsAndAppliesLatestMatchingRelease' -count=1`

Expected: FAIL because release notes are not parsed or returned yet.

**Step 3: Write minimal implementation**

- Add a `Notes` field to the release model and JSON parsing layer.
- Add a `ReleaseNotes` field to `SelfUpdateResult`.
- Copy release notes from `LatestRelease` into the update result.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/update -run 'TestParseLatestRelease|TestSelfUpdateDownloadsAndAppliesLatestMatchingRelease' -count=1`

Expected: PASS

### Task 2: Print release notes after successful self-update

**Files:**
- Modify: `cmd/chatbox/main.go`
- Test: `cmd/chatbox/main_test.go`

**Step 1: Write the failing test**

- Add a CLI test that stubs the self-update path and expects:
  - `updated chatbox to vX.Y.Z`
  - a `what's new:` section when notes exist
  - fallback `release:` output when notes are empty

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/chatbox -run 'TestRunSelfUpdate' -count=1`

Expected: FAIL because current CLI only prints the version line.

**Step 3: Write minimal implementation**

- Refactor the CLI self-update output into a small formatter helper.
- On real updates, print the version line plus:
  - trimmed release notes if present
  - otherwise the release URL if present

**Step 4: Run tests to verify it passes**

Run: `go test ./cmd/chatbox -run 'TestRunSelfUpdate' -count=1`

Expected: PASS

### Task 3: Keep release publishing compatible with feature display

**Files:**
- Modify: `scripts/release-manual.sh`
- Test: `scripts/release-manual_test.sh`

**Step 1: Write the failing test**

- Extend the release script test to expect `gh release create` to use generated notes instead of the previous static text-only note body.

**Step 2: Run test to verify it fails**

Run: `bash scripts/release-manual_test.sh`

Expected: FAIL because the script currently uses a static `--notes` string.

**Step 3: Write minimal implementation**

- Switch manual release creation to `--generate-notes`.
- Keep the existing manual-fallback context as a short prepended note.

**Step 4: Run tests to verify it passes**

Run: `bash scripts/release-manual_test.sh`

Expected: PASS

### Task 4: Verify the whole flow

**Files:**
- Modify: none

**Step 1: Run focused tests**

Run: `go test ./internal/update ./cmd/chatbox -count=1`

Expected: PASS

**Step 2: Run release script tests**

Run: `bash scripts/release-manual_test.sh`

Expected: PASS

**Step 3: Run full test suite**

Run: `go test ./... -count=1`

Expected: PASS
