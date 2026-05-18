#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: ./scripts/release-verify.sh vMAJOR.MINOR.PATCH" >&2
}

require_command() {
  local name="$1"
  if ! command -v "$name" >/dev/null 2>&1; then
    echo "missing required command: $name" >&2
    exit 1
  fi
}

asset_for_current_platform() {
  local os arch
  os="$(go env GOOS)"
  arch="$(go env GOARCH)"
  case "${os}/${arch}" in
    darwin/arm64) echo "chatbox_darwin_arm64.tar.gz" ;;
    darwin/amd64) echo "chatbox_darwin_amd64.tar.gz" ;;
    linux/arm64) echo "chatbox_linux_arm64.tar.gz" ;;
    android/arm64) echo "chatbox_android_arm64.tar.gz" ;;
    *)
      echo "unsupported verification platform: ${os}/${arch}" >&2
      exit 1
      ;;
  esac
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

require_command gh
require_command git
require_command go
require_command shasum
require_command tar

gh auth status >/dev/null

latest_tag="$(gh release view --json tagName --jq .tagName)"
if [[ "$latest_tag" != "$VERSION" ]]; then
  echo "latest GitHub Release is ${latest_tag}, expected ${VERSION}" >&2
  exit 1
fi

release_tag="$(gh release view "$VERSION" --json tagName --jq .tagName)"
if [[ "$release_tag" != "$VERSION" ]]; then
  echo "release ${VERSION} was not found" >&2
  exit 1
fi

required_assets=(
  chatbox_darwin_arm64.tar.gz
  chatbox_darwin_amd64.tar.gz
  chatbox_linux_arm64.tar.gz
  chatbox_android_arm64.tar.gz
  checksums.txt
)

asset_names="$(gh release view "$VERSION" --json assets --jq '.assets[].name' | sort)"
for asset in "${required_assets[@]}"; do
  if ! grep -qx "$asset" <<<"$asset_names"; then
    echo "release ${VERSION} is missing asset ${asset}" >&2
    exit 1
  fi
done

verify_dir="$(mktemp -d "${TMPDIR:-/tmp}/chatbox-release-${VERSION}.XXXXXX")"
trap 'rm -rf "$verify_dir"' EXIT

current_asset="$(asset_for_current_platform)"
gh release download "$VERSION" \
  -p "$current_asset" \
  -p checksums.txt \
  -D "$verify_dir" \
  --clobber >/dev/null

(
  cd "$verify_dir"
  shasum -a 256 -c checksums.txt --ignore-missing
  tar -xzf "$current_asset"
  chmod +x ./chatbox
  actual_version="$(./chatbox version)"
  if [[ "$actual_version" != "$VERSION" ]]; then
    echo "downloaded binary reports ${actual_version}, expected ${VERSION}" >&2
    exit 1
  fi
)

echo "release verified: ${VERSION}"
