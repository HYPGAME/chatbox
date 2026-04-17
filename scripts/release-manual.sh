#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: ./scripts/release-manual.sh vMAJOR.MINOR.PATCH" >&2
}

run_publish_step() {
  if [[ "${DRY_RUN:-0}" == "1" ]]; then
    echo "dry-run: $*"
    return 0
  fi
  "$@"
}

require_command() {
  local name="$1"
  if ! command -v "$name" >/dev/null 2>&1; then
    echo "missing required command: $name" >&2
    exit 1
  fi
}

if [[ $# -ne 1 ]]; then
  usage
  exit 1
fi

VERSION="$1"
if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "invalid version '$VERSION': use vMAJOR.MINOR.PATCH" >&2
  exit 1
fi

require_command git
require_command gh
require_command go
require_command tar
require_command shasum

git rev-parse --show-toplevel >/dev/null
gh auth status >/dev/null

BRANCH="$(git branch --show-current 2>/dev/null || true)"
BRANCH="${BRANCH#\* }"
if [[ -z "$BRANCH" ]]; then
  BRANCH="$(git branch | sed -n 's/^\* //p')"
fi
if [[ "$BRANCH" != "main" ]]; then
  echo "current branch must be main, got: $BRANCH" >&2
  exit 1
fi

if [[ -n "$(git status --short)" ]]; then
  echo "working tree is not clean" >&2
  exit 1
fi

if [[ -n "$(git tag --list "$VERSION")" ]]; then
  echo "tag already exists locally: $VERSION" >&2
  exit 1
fi

if [[ -n "$(git ls-remote --tags origin "$VERSION" 2>/dev/null)" ]]; then
  echo "tag already exists on origin: $VERSION" >&2
  exit 1
fi

go test ./...

rm -rf dist
mkdir -p dist

for ARCH in arm64 amd64; do
  WORKDIR="dist/chatbox_darwin_${ARCH}"
  mkdir -p "$WORKDIR"
  GOOS=darwin GOARCH="$ARCH" go build -ldflags "-X chatbox/internal/version.Version=${VERSION}" -o "$WORKDIR/chatbox" ./cmd/chatbox
  tar -C "$WORKDIR" -czf "dist/chatbox_darwin_${ARCH}.tar.gz" chatbox
done

ANDROID_WORKDIR="dist/chatbox_android_arm64"
mkdir -p "$ANDROID_WORKDIR"
GOOS=android GOARCH=arm64 go build -ldflags "-X chatbox/internal/version.Version=${VERSION}" -o "$ANDROID_WORKDIR/chatbox" ./cmd/chatbox
tar -C "$ANDROID_WORKDIR" -czf "dist/chatbox_android_arm64.tar.gz" chatbox

LINUX_WORKDIR="dist/chatbox_linux_arm64"
mkdir -p "$LINUX_WORKDIR"
GOOS=linux GOARCH=arm64 go build -ldflags "-X chatbox/internal/version.Version=${VERSION}" -o "$LINUX_WORKDIR/chatbox" ./cmd/chatbox
tar -C "$LINUX_WORKDIR" -czf "dist/chatbox_linux_arm64.tar.gz" chatbox

(
  cd dist
  shasum -a 256 chatbox_darwin_arm64.tar.gz chatbox_darwin_amd64.tar.gz chatbox_linux_arm64.tar.gz chatbox_android_arm64.tar.gz
) > dist/checksums.txt

run_publish_step git push origin main
run_publish_step git tag "$VERSION"
run_publish_step git push origin "refs/tags/$VERSION"

if ! run_publish_step gh release create "$VERSION" \
  dist/chatbox_darwin_arm64.tar.gz \
  dist/chatbox_darwin_amd64.tar.gz \
  dist/chatbox_linux_arm64.tar.gz \
  dist/chatbox_android_arm64.tar.gz \
  dist/checksums.txt \
  --target main \
  --title "$VERSION" \
  --notes "Manual release fallback because GitHub Actions is currently blocked by repository billing status."
then
  echo "release creation failed for $VERSION" >&2
  echo "recovery options:" >&2
  echo "  gh release view $VERSION" >&2
  echo "  git push origin :refs/tags/$VERSION" >&2
  echo "  git tag -d $VERSION" >&2
  exit 1
fi

if [[ "${DRY_RUN:-0}" == "1" ]]; then
  echo "dry-run complete: release artifacts are ready in dist/"
  exit 0
fi

echo "release published: https://github.com/HYPGAME/chatbox/releases/tag/$VERSION"
echo "collaborators can update with: chatbox self-update"
