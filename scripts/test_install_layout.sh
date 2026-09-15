#!/usr/bin/env bash
# Tests legacy install layout migration helpers from install.sh.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmpdir="${ROOT}/.tmp-test-install-layout-$$"
mkdir -p "${tmpdir}/home/.cursor"
export HOME="${tmpdir}/home"

# shellcheck source=../install.sh
source "${ROOT}/install.sh"

failures=0

assert_ok() {
  local name="$1"
  shift
  if "$@"; then
    return 0
  fi
  echo "FAIL ${name}" >&2
  failures=$((failures + 1))
}

assert_eq() {
  local name="$1" got="$2" want="$3"
  if [[ "${got}" != "${want}" ]]; then
    echo "FAIL ${name}: got '${got}' want '${want}'" >&2
    failures=$((failures + 1))
  fi
}

trap 'rm -rf "${tmpdir}"' EXIT

test_paths_same() {
  local a="${tmpdir}/a" b="${tmpdir}/b"
  mkdir -p "${tmpdir}/dir"
  ln -s "${tmpdir}/dir" "${a}"
  ln -s "${tmpdir}/dir" "${b}"
  assert_ok "paths_same symlink equivalents" paths_same "${a}" "${b}"
}

test_safe_symlink_skips_self() {
  local file="${tmpdir}/same-file"
  printf 'x\n' > "${file}"
  safe_symlink "${file}" "${file}" 2>/dev/null
  assert_ok "safe_symlink leaves regular file" test -f "${file}"
  assert_ok "safe_symlink did not self-link" test ! -L "${file}"
}

test_migrate_legacy_scripts_symlink() {
  local repo="${tmpdir}/repo"
  rm -rf "${HOME}/.cursor"
  mkdir -p "${HOME}/.cursor"
  mkdir -p "${repo}/scripts"
  printf 'registry\n' > "${repo}/scripts/projects.registry"
  printf '#!/bin/sh\n' > "${repo}/scripts/install-rules.sh"
  chmod +x "${repo}/scripts/install-rules.sh"
  ln -s cuse "${repo}/scripts/cuse"
  ln -s db-query "${repo}/scripts/db-query"

  ln -s "${repo}/scripts" "${HOME}/.cursor/ai-toolkit"

  migrate_legacy_bin_target

  assert_ok "BIN_TARGET is a directory" test -d "${BIN_TARGET}"
  assert_ok "BIN_TARGET is not a symlink" test ! -L "${BIN_TARGET}"
  assert_eq "registry preserved" "$(cat "${BIN_TARGET}/projects.registry")" "registry"
  assert_ok "repo scripts dir remains" test -d "${repo}/scripts"
  assert_ok "repo install-rules.sh remains" test -f "${repo}/scripts/install-rules.sh"
  assert_ok "self-linked cuse removed from scripts" test ! -L "${repo}/scripts/cuse"
  assert_ok "self-linked db-query removed from scripts" test ! -L "${repo}/scripts/db-query"
}

test_install_bin_after_legacy_migration() {
  local repo="${tmpdir}/repo2"
  rm -rf "${HOME}/.cursor"
  mkdir -p "${HOME}/.cursor"
  mkdir -p "${repo}/scripts" "${repo}/rules"
  printf '#!/bin/sh\n' > "${repo}/scripts/install-rules.sh"
  chmod +x "${repo}/scripts/install-rules.sh"
  printf 'cuse-bin\n' > "${repo}/scripts/cuse"
  chmod +x "${repo}/scripts/cuse"
  ln -s "${repo}/scripts" "${HOME}/.cursor/ai-toolkit"

  install_bin "${repo}" "link"
  repair_managed_binaries "${repo}"

  assert_ok "install-rules.sh is outward symlink" test -L "${BIN_TARGET}/install-rules.sh"
  assert_eq "install-rules link target" "$(readlink "${BIN_TARGET}/install-rules.sh")" "${repo}/scripts/install-rules.sh"
  assert_ok "cuse is outward symlink" test -L "${BIN_TARGET}/cuse"
  assert_eq "cuse link target" "$(readlink "${BIN_TARGET}/cuse")" "${repo}/scripts/cuse"
  assert_ok "cuse is not a self-link" test "$(readlink "${BIN_TARGET}/cuse")" != "cuse"
}

test_paths_same
test_safe_symlink_skips_self
test_migrate_legacy_scripts_symlink
test_install_bin_after_legacy_migration

if [[ "${failures}" -ne 0 ]]; then
  echo "${failures} test(s) failed" >&2
  exit 1
fi

echo "ok"
