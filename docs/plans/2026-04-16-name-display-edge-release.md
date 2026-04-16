# Name Display and Edge Release Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Show real sender names for all chat lines and publish a prerelease build for every push to `main` without disturbing the stable tag-based release flow.

**Architecture:** Remove the last-mile `you` label rewrite in TUI rendering so outgoing entries use the sender name already present in message/transcript data. Add a separate edge-release GitHub Actions workflow for `main` pushes that builds the same archives as stable releases, but publishes them as unique prereleases with edge version labels.

**Tech Stack:** Go, Bubble Tea TUI rendering, existing transcript/retry flow, GitHub Actions release publishing

---

### Task 1: Remove the synthetic `you` label from rendered chat lines

**Files:**
- Modify: `internal/tui/model_test.go`
- Modify: `internal/tui/model.go`

**Step 1: Write the failing tests**

Update or add tests verifying:

```go
func TestModelSendsTypedMessageOnEnter(t *testing.T) { ... }        // expects local configured name
func TestScrollbackOutgoingReceiptDoesNotPrintDeliveryStatuses(t *testing.T) { ... } // expects local configured name
func TestScrollbackReconnectPrintsRetryMarkerForPendingMessage(t *testing.T) { ... } // expects local configured name
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run 'Test(ModelSendsTypedMessageOnEnter|ScrollbackOutgoingReceiptDoesNotPrintDeliveryStatuses|ScrollbackReconnectPrintsRetryMarkerForPendingMessage)'`

Expected: FAIL because outgoing messages are still rendered as `you`.

**Step 3: Write minimal implementation**

- stop replacing outgoing labels with `you`
- preserve outgoing-only status suffix logic

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run 'Test(ModelSendsTypedMessageOnEnter|ScrollbackOutgoingReceiptDoesNotPrintDeliveryStatuses|ScrollbackReconnectPrintsRetryMarkerForPendingMessage)'`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/model_test.go
git commit -m "feat: show sender names for outgoing chat lines"
```

### Task 2: Add edge release identity helpers and tests

**Files:**
- Create: `internal/release/edge.go`
- Create: `internal/release/edge_test.go`

**Step 1: Write the failing tests**

Add tests for helper functions like:

```go
func TestEdgeTagForCommit(t *testing.T) { ... }
func TestEdgeVersionForCommit(t *testing.T) { ... }
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/release -run 'TestEdge(TagForCommit|VersionForCommit)'`

Expected: FAIL because the helper does not exist yet.

**Step 3: Write minimal implementation**

- derive a stable edge tag from a commit SHA, e.g. `edge-abc1234`
- derive an embedded edge version string, e.g. `edge-abc1234`
- keep helper logic small and deterministic

**Step 4: Run test to verify it passes**

Run: `go test ./internal/release -run 'TestEdge(TagForCommit|VersionForCommit)'`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/release/edge.go internal/release/edge_test.go
git commit -m "feat: add edge release naming helpers"
```

### Task 3: Publish a prerelease on every push to `main`

**Files:**
- Create: `.github/workflows/edge-release.yml`

**Step 1: Write the failing static check**

Run: `test -f .github/workflows/edge-release.yml`

Expected: FAIL because the workflow does not exist yet.

**Step 2: Write minimal implementation**

- trigger on `push` to `main`
- build the same darwin/android archives as the stable release flow
- compute edge tag/version from `${GITHUB_SHA}`
- publish a GitHub prerelease with unique per-commit tag name
- upload the same four assets plus `checksums.txt`

**Step 3: Run static verification**

Run: `rg -n "branches:|main|prerelease: true|chatbox_android_arm64.tar.gz|edge-" .github/workflows/edge-release.yml`

Expected: matches for main-branch trigger, prerelease publishing, and the full asset list.

**Step 4: Commit**

```bash
git add .github/workflows/edge-release.yml
git commit -m "ci: publish edge prereleases on main"
```

### Task 4: Document edge prerelease behavior

**Files:**
- Modify: `README.md`

**Step 1: Write the failing check**

Run: `rg -n "prerelease|edge-|main push|nightly" README.md`

Expected: no relevant edge release documentation.

**Step 2: Write minimal implementation**

- document that tag pushes create stable releases
- document that pushes to `main` create prereleases
- clarify that `self-update` continues to target stable releases

**Step 3: Run documentation check**

Run: `rg -n "prerelease|edge-|main push|self-update" README.md`

Expected: matches for the new edge release section.

**Step 4: Commit**

```bash
git add README.md
git commit -m "docs: describe edge prerelease flow"
```

### Task 5: Full verification

**Files:**
- Modify: all touched files from Tasks 1-4

**Step 1: Run project tests**

Run: `go test ./...`

Expected: PASS

**Step 2: Cross-compile sanity for published assets**

Run: `GOOS=android GOARCH=arm64 go build -o /tmp/chatbox-edge-android ./cmd/chatbox`

Expected: PASS

**Step 3: Workflow static verification**

Run: `rg -n "prerelease: true|generate_release_notes|chatbox_darwin_arm64.tar.gz|chatbox_android_arm64.tar.gz" .github/workflows/edge-release.yml`

Expected: PASS with matches for prerelease publishing and all assets.
