#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="${ROOT}/scripts/release-manual.sh"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

make_fake_bin() {
  local bin_dir="$1"
  mkdir -p "$bin_dir"

  cat >"${bin_dir}/git" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

case "${1:-}" in
  branch)
    echo "* ${FAKE_GIT_BRANCH:-main}"
    ;;
  status)
    if [[ "${FAKE_GIT_DIRTY:-0}" == "1" ]]; then
      echo " M README.md"
    fi
    ;;
  tag)
    if [[ "${2:-}" == "--list" ]]; then
      if [[ "${FAKE_GIT_TAG_EXISTS:-0}" == "1" ]]; then
        echo "${3:-v0.0.0}"
      fi
    elif [[ -n "${FAKE_LOG_FILE:-}" ]]; then
      echo "git tag ${2:-}" >>"${FAKE_LOG_FILE}"
    fi
    ;;
  ls-remote)
    if [[ "${FAKE_GIT_REMOTE_TAG_EXISTS:-0}" == "1" ]]; then
      echo "deadbeef refs/tags/${@: -1}"
    fi
    ;;
  rev-parse)
    echo "${FAKE_GIT_ROOT:-$PWD}"
    ;;
  push)
    if [[ -n "${FAKE_LOG_FILE:-}" ]]; then
      echo "git push ${*:2}" >>"${FAKE_LOG_FILE}"
    fi
    ;;
  add|commit)
    ;;
  *)
    ;;
esac
EOF

  cat >"${bin_dir}/gh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "release" && "${2:-}" == "create" ]]; then
  if [[ -n "${FAKE_LOG_FILE:-}" ]]; then
    echo "gh release create ${3:-}" >>"${FAKE_LOG_FILE}"
    printf '%s\n' "$*" >"${FAKE_LOG_FILE}.gh.args"
  fi
  if [[ "${FAKE_GH_RELEASE_FAIL:-0}" == "1" ]]; then
    echo "release create failed" >&2
    exit 1
  fi
  exit 0
fi
exit 0
EOF

  cat >"${bin_dir}/go" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "build" ]]; then
  output=""
  prev=""
  for arg in "$@"; do
    if [[ "$prev" == "-o" ]]; then
      output="$arg"
      break
    fi
    prev="$arg"
  done
  if [[ -n "$output" ]]; then
    mkdir -p "$(dirname "$output")"
    printf 'fake-binary' >"$output"
  fi
fi
exit 0
EOF

  cat >"${bin_dir}/tar" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
output=""
prev=""
for arg in "$@"; do
  if [[ "$prev" == "-czf" ]]; then
    output="$arg"
    break
  fi
  prev="$arg"
done
if [[ -n "$output" ]]; then
  : >"$output"
fi
exit 0
EOF

  cat >"${bin_dir}/shasum" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
for arg in "$@"; do
  if [[ "$arg" == *.tar.gz ]]; then
    echo "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  $(basename "$arg")"
  fi
done
EOF

  chmod +x "${bin_dir}/git" "${bin_dir}/gh" "${bin_dir}/go" "${bin_dir}/tar" "${bin_dir}/shasum"
}

run_script_expect_failure() {
  local expected="$1"
  shift

  local temp_dir
  temp_dir="$(mktemp -d)"
  trap "rm -rf '${temp_dir}'" RETURN

  make_fake_bin "${temp_dir}/bin"
  local output
  if output="$(PATH="${temp_dir}/bin:${PATH}" bash "$SCRIPT" "$@" 2>&1)"; then
    fail "expected command to fail, got success: ${output}"
  fi
  if [[ "${output}" != *"${expected}"* ]]; then
    fail "expected output to contain '${expected}', got: ${output}"
  fi
}

run_script_expect_success() {
  local temp_dir
  temp_dir="$(mktemp -d)"
  trap "rm -rf '${temp_dir}'" RETURN

  make_fake_bin "${temp_dir}/bin"
  : >"${temp_dir}/commands.log"
  local output
  if ! output="$(FAKE_LOG_FILE="${temp_dir}/commands.log" PATH="${temp_dir}/bin:${PATH}" bash "$SCRIPT" "$@" 2>&1)"; then
    fail "expected command to succeed, got failure: ${output}"
  fi
  cat "${temp_dir}/commands.log"
}

test_missing_version_fails() {
  run_script_expect_failure "usage"
}

test_dirty_worktree_fails() {
  FAKE_GIT_DIRTY=1 run_script_expect_failure "working tree is not clean" "v0.1.3"
}

test_existing_tag_fails() {
  FAKE_GIT_TAG_EXISTS=1 run_script_expect_failure "tag already exists locally" "v0.1.3"
}

test_release_flow_pushes_main_then_tag_then_release() {
  local log
  log="$(run_script_expect_success "v0.1.3")"
  local expected
  expected="$(cat <<'EOF'
git push origin main
git tag v0.1.3
git push origin refs/tags/v0.1.3
gh release create v0.1.3
EOF
)"
  if [[ "$log" != "$expected" ]]; then
    fail "unexpected publish order:\n$log"
  fi
}

test_release_create_failure_reports_recovery_steps() {
  local temp_dir
  temp_dir="$(mktemp -d)"
  trap "rm -rf '${temp_dir}'" RETURN

  make_fake_bin "${temp_dir}/bin"
  local output
  if output="$(FAKE_GH_RELEASE_FAIL=1 FAKE_LOG_FILE="${temp_dir}/commands.log" PATH="${temp_dir}/bin:${PATH}" bash "$SCRIPT" v0.1.3 2>&1)"; then
    fail "expected release create failure"
  fi
  if [[ "${output}" != *"release creation failed for v0.1.3"* ]]; then
    fail "expected release failure guidance, got: ${output}"
  fi
  if [[ "${output}" != *"git push origin :refs/tags/v0.1.3"* ]]; then
    fail "expected tag cleanup guidance, got: ${output}"
  fi
}

test_dry_run_prints_publish_plan_without_mutation() {
  local temp_dir
  temp_dir="$(mktemp -d)"
  trap "rm -rf '${temp_dir}'" RETURN

  make_fake_bin "${temp_dir}/bin"
  : >"${temp_dir}/commands.log"

  local output
  if ! output="$(DRY_RUN=1 FAKE_LOG_FILE="${temp_dir}/commands.log" PATH="${temp_dir}/bin:${PATH}" bash "$SCRIPT" v0.1.3 2>&1)"; then
    fail "expected dry run to succeed, got: ${output}"
  fi
  if [[ "${output}" != *"dry-run: git push origin main"* ]]; then
    fail "expected dry run push output, got: ${output}"
  fi
  if [[ "${output}" != *"dry-run: gh release create v0.1.3"* ]]; then
    fail "expected dry run release output, got: ${output}"
  fi
  if [[ "${output}" != *"--generate-notes"* ]]; then
    fail "expected dry run release output to generate notes, got: ${output}"
  fi
  if [[ "${output}" != *"dist/chatbox_android_arm64.tar.gz"* ]]; then
    fail "expected dry run to include android archive, got: ${output}"
  fi
  if [[ -s "${temp_dir}/commands.log" ]]; then
    fail "expected dry run not to execute push/tag/release commands"
  fi
}

test_release_generates_checksums_for_android_archive() {
  local temp_dir
  temp_dir="$(mktemp -d)"
  trap "rm -rf '${temp_dir}'" RETURN

  make_fake_bin "${temp_dir}/bin"

  local output
  if ! output="$(PATH="${temp_dir}/bin:${PATH}" bash "$SCRIPT" v0.1.3 2>&1)"; then
    fail "expected command to succeed, got failure: ${output}"
  fi

  if [[ ! -f "${ROOT}/dist/checksums.txt" ]]; then
    fail "expected dist/checksums.txt to be generated"
  fi
  if ! grep -q "chatbox_android_arm64.tar.gz" "${ROOT}/dist/checksums.txt"; then
    fail "expected checksums to include android archive"
  fi
  rm -rf "${ROOT}/dist"
}

test_release_uses_generated_notes() {
  local temp_dir
  temp_dir="$(mktemp -d)"
  trap "rm -rf '${temp_dir}'" RETURN

  make_fake_bin "${temp_dir}/bin"
  : >"${temp_dir}/commands.log"

  local output
  if ! output="$(FAKE_LOG_FILE="${temp_dir}/commands.log" PATH="${temp_dir}/bin:${PATH}" bash "$SCRIPT" v0.1.3 2>&1)"; then
    fail "expected command to succeed, got failure: ${output}"
  fi

  local args
  args="$(cat "${temp_dir}/commands.log.gh.args")"
  if [[ "${args}" != *"--generate-notes"* ]]; then
    fail "expected release create to generate notes, got: ${args}"
  fi
}

test_missing_version_fails
test_dirty_worktree_fails
test_existing_tag_fails
test_release_flow_pushes_main_then_tag_then_release
test_release_create_failure_reports_recovery_steps
test_dry_run_prints_publish_plan_without_mutation
test_release_generates_checksums_for_android_archive
test_release_uses_generated_notes

echo "ok"
