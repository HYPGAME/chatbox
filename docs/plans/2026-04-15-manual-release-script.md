# Manual Release Script Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a one-command local manual release script that validates the repo state, builds macOS release artifacts, tags the release, pushes it, and publishes a GitHub Release.

**Architecture:** Keep the user-facing entrypoint as `scripts/release-manual.sh`, but move the most failure-prone contract checks into small, testable logic so release validation and artifact naming do not live only inside ad hoc shell branches. The shell script will orchestrate git, go, tar, shasum, and `gh`, while tests lock down the release contract and refusal conditions.

**Tech Stack:** Bash, Go, git, GitHub CLI, tar.gz archives, SHA256 checksums

---

### Task 1: Release contract helpers

**Files:**
- Create: `internal/release/manual.go`
- Create: `internal/release/manual_test.go`

**Step 1: Write the failing test**

Add tests that verify:

```go
func TestValidateVersionAcceptsSemanticTag(t *testing.T) { ... }
func TestValidateVersionRejectsInvalidFormats(t *testing.T) { ... }
func TestArtifactNamesForVersionedRelease(t *testing.T) { ... }
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/release -run 'Test(ValidateVersionAcceptsSemanticTag|ValidateVersionRejectsInvalidFormats|ArtifactNamesForVersionedRelease)'`

Expected: FAIL because the release helper package does not exist yet.

**Step 3: Write minimal implementation**

- add version validation for `vMAJOR.MINOR.PATCH`
- expose the artifact names used by manual release
- keep the helper package small and shell-oriented

**Step 4: Run test to verify it passes**

Run: `go test ./internal/release -run 'Test(ValidateVersionAcceptsSemanticTag|ValidateVersionRejectsInvalidFormats|ArtifactNamesForVersionedRelease)'`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/release/manual.go internal/release/manual_test.go
git commit -m "feat: add manual release contract helpers"
```

### Task 2: Shell release script validation and orchestration

**Files:**
- Create: `scripts/release-manual.sh`
- Create: `scripts/release-manual_test.sh`

**Step 1: Write the failing test**

Add shell-level tests that verify:

```bash
test_missing_version_fails
test_dirty_worktree_fails
test_existing_tag_fails
```

Use temporary directories and fake `git` / `gh` / `go` commands in `PATH` so the script can be exercised without touching the real repository.

**Step 2: Run test to verify it fails**

Run: `bash scripts/release-manual_test.sh`

Expected: FAIL because the script does not exist yet.

**Step 3: Write minimal implementation**

- require one version argument
- check required commands
- validate current branch is `main`
- validate working tree is clean
- refuse if local or remote tag already exists
- run `go test ./...`
- build both darwin archives
- generate basename-only `checksums.txt`

**Step 4: Run test to verify it passes**

Run: `bash scripts/release-manual_test.sh`

Expected: PASS

**Step 5: Commit**

```bash
git add scripts/release-manual.sh scripts/release-manual_test.sh
git commit -m "feat: add manual release script"
```

### Task 3: Publishing flow coverage

**Files:**
- Modify: `scripts/release-manual.sh`
- Modify: `scripts/release-manual_test.sh`

**Step 1: Write the failing test**

Extend the shell tests to verify:

```bash
test_release_flow_pushes_main_then_tag_then_release
test_release_create_failure_reports_recovery_steps
```

Capture command invocations in fake binaries so the exact order can be asserted.

**Step 2: Run test to verify it fails**

Run: `bash scripts/release-manual_test.sh`

Expected: FAIL because publish orchestration and error reporting are not complete yet.

**Step 3: Write minimal implementation**

- `git push origin main`
- `git tag <version>`
- `git push origin refs/tags/<version>`
- `gh release create <version> ...`
- on release-create failure, print non-destructive recovery guidance

**Step 4: Run test to verify it passes**

Run: `bash scripts/release-manual_test.sh`

Expected: PASS

**Step 5: Commit**

```bash
git add scripts/release-manual.sh scripts/release-manual_test.sh
git commit -m "feat: automate manual GitHub release publishing"
```

### Task 4: Documentation

**Files:**
- Modify: `README.md`

**Step 1: Write the failing test**

Add a lightweight documentation verification step:

- inspect `README.md` and note that it does not yet document the new script command

**Step 2: Run verification to confirm the precondition**

Run: `rg -n "release-manual\\.sh" README.md`

Expected: no matches

**Step 3: Write minimal implementation**

- add a `Manual Release` section
- document `./scripts/release-manual.sh v0.1.3`
- explain that this is the supported fallback while GitHub Actions remains blocked by billing state

**Step 4: Run verification to confirm it is documented**

Run: `rg -n "release-manual\\.sh|Manual Release" README.md`

Expected: matches for the new section and example command

**Step 5: Commit**

```bash
git add README.md
git commit -m "docs: add manual release fallback instructions"
```

### Task 5: Full verification and live dry run

**Files:**
- No code changes required unless verification finds defects.

**Step 1: Run the full automated checks**

Run:

```bash
go test ./...
go test -race ./cmd/chatbox ./internal/update ./internal/tui ./internal/session
bash scripts/release-manual_test.sh
```

Expected: PASS

**Step 2: Run a local dry verification of the script against the real repo**

Run:

```bash
bash scripts/release-manual.sh v9.9.9-test
```

Expected: the script should fail safely before publishing if you deliberately point it at a protected or non-releaseable version, or complete if you choose a real release version.

If publishing a real version is not desired at this stage, adapt the script to support a safe `DRY_RUN=1` mode before live release.

**Step 3: Clean up any validation-only tags if created**

Run only if you intentionally created a disposable validation tag:

```bash
git tag -d v9.9.9-test
git push origin :refs/tags/v9.9.9-test
```

Expected: cleanup complete and real release history remains tidy.

**Step 4: Commit**

```bash
git add .
git commit -m "feat: add one-command manual release flow"
```
