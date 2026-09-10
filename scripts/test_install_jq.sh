#!/usr/bin/env bash
# Tests jq-based JSON helpers from install.sh (attribution + manifest).
set -euo pipefail

if ! command -v jq >/dev/null 2>&1; then
  echo "test_install_jq: jq is required for these tests" >&2
  exit 1
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmpdir="${ROOT}/.tmp-test-install-jq-$$"
mkdir -p "${tmpdir}/home/.cursor/ai-toolkit"
export HOME="${tmpdir}/home"

# shellcheck source=../install.sh
source "${ROOT}/install.sh"

failures=0

assert_eq() {
  local name="$1" got="$2" want="$3"
  if [[ "${got}" != "${want}" ]]; then
    echo "FAIL ${name}: got '${got}' want '${want}'" >&2
    failures=$((failures + 1))
  fi
}

trap 'rm -rf "${tmpdir}"' EXIT

test_read_cli_attribution_flag() {
  local config="${tmpdir}/cli-config.json"

  printf '%s\n' '{"attribution":{"attributeCommitsToAgent":true,"attributePRsToAgent":false}}' > "${config}"
  assert_eq "commits true" "$(read_cli_attribution_flag "${config}" "attributeCommitsToAgent")" "true"
  assert_eq "prs false" "$(read_cli_attribution_flag "${config}" "attributePRsToAgent")" "false"

  printf '%s\n' '{}' > "${config}"
  assert_eq "missing attribution defaults true" "$(read_cli_attribution_flag "${config}" "attributeCommitsToAgent")" "true"

  printf '%s\n' '{"attribution":"not-an-object"}' > "${config}"
  assert_eq "attribution string" "$(read_cli_attribution_flag "${config}" "attributeCommitsToAgent")" "unknown"

  printf '%s\n' '[]' > "${config}"
  assert_eq "root array" "$(read_cli_attribution_flag "${config}" "attributeCommitsToAgent")" "unknown"

  printf '%s\n' '{not json' > "${config}"
  assert_eq "malformed json" "$(read_cli_attribution_flag "${config}" "attributeCommitsToAgent")" "unknown"

  assert_eq "missing file defaults true" "$(read_cli_attribution_flag "${tmpdir}/missing.json" "attributeCommitsToAgent")" "true"
}

test_attribution_status_label() {
  assert_eq "enabled" "$(attribution_status_label true)" "enabled"
  assert_eq "disabled" "$(attribution_status_label false)" "disabled"
  assert_eq "unknown" "$(attribution_status_label unknown)" "unknown"
}

test_manifest_roundtrip() {
  write_install_manifest "abc123" "main@abc123" "local" "link"
  assert_eq "manifest commit" "$(manifest_field commit)" "abc123"
  assert_eq "manifest mode" "$(manifest_field mode)" "link"
  assert_eq "manifest source" "$(manifest_field source)" "local"
  assert_eq "manifest version" "$(manifest_field version)" "main@abc123"

  printf '%s\n' '{bad json' > "${INSTALL_MANIFEST}"
  assert_eq "malformed manifest read" "$(manifest_field commit)" ""
}

test_read_cli_attribution_flag
test_attribution_status_label
test_manifest_roundtrip

if [[ "${failures}" -ne 0 ]]; then
  echo "${failures} test(s) failed" >&2
  exit 1
fi

echo "ok"
