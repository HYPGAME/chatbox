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
  push|add|commit)
    ;;
  *)
    ;;
esac
EOF

  cat >"${bin_dir}/gh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
exit 0
EOF

  cat >"${bin_dir}/go" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
exit 0
EOF

  cat >"${bin_dir}/tar" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
exit 0
EOF

  cat >"${bin_dir}/shasum" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
exit 0
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

test_missing_version_fails() {
  run_script_expect_failure "usage"
}

test_dirty_worktree_fails() {
  FAKE_GIT_DIRTY=1 run_script_expect_failure "working tree is not clean" "v0.1.3"
}

test_existing_tag_fails() {
  FAKE_GIT_TAG_EXISTS=1 run_script_expect_failure "tag already exists locally" "v0.1.3"
}

test_missing_version_fails
test_dirty_worktree_fails
test_existing_tag_fails

echo "ok"
