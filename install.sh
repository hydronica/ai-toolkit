#!/usr/bin/env bash

set -euo pipefail

readonly REPO_NAME="ai-toolkit"
readonly RESOURCE_TYPES=("commands" "skills" "agents")
readonly BIN_SOURCE_DIR="scripts"
readonly RULES_DIR="rules"
readonly BIN_TARGET="${HOME}/.cursor/${REPO_NAME}"
readonly RULES_TARGET="${BIN_TARGET}/rules-source"
readonly REGISTRY_FILE="${BIN_TARGET}/projects.registry"
readonly GITHUB_REPO="hydronica/ai-toolkit"
readonly GITHUB_API="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"
readonly INSTALL_MANIFEST="${BIN_TARGET}/install-manifest.json"
ONLINE_RELEASE_TAG=""
ONLINE_RELEASE_COMMIT=""
readonly MANAGED_BINARIES=("cuse" "db-query")
readonly LOCAL_BINARY_PATHS=("cmd/cuse" "cmd/db-query" "Makefile")

# Set by plan_install: skip | install | repair
PLAN_ASSETS_ACTION=""
PLAN_BINARIES_ACTION=""
PLAN_SYNC_RULES="false"
PLAN_SOURCE_COMMIT=""
PLAN_SOURCE_VERSION=""
PLAN_SOURCE_KIND=""
PLAN_MODE=""
PLAN_INSTALLED_COMMIT=""
PLAN_INSTALLED_VERSION=""
PLAN_INSTALLED_MODE=""
PLAN_INSTALLED_SOURCE=""

usage() {
  cat <<'EOF'
Usage: install.sh [--link|--copy] [--check] [--force]
                  [--sync-rules] [--no-sync-rules] [--check-attribution]

  --link               Link from local ai-toolkit source when available
  --copy               Copy from local/online source
  --check              Print install plan and exit (no changes)
  --force              Reinstall assets and binaries even when unchanged
  --sync-rules         Sync registered project rules even when assets unchanged
  --no-sync-rules      Skip syncing registered project rules after install
  --check-attribution  Print CLI/IDE attribution status and exit (no install)
  -h, --help           Show this help

Installs to ${HOME}/.cursor/(commands|skills|agents)/ai-toolkit/
Installs scripts/, rules-source/, and registry support under ${HOME}/.cursor/ai-toolkit/
Records install state in ~/.cursor/ai-toolkit/install-manifest.json (not projects.registry).
Syncs registered project rules when assets change — see scripts/install-rules.sh.

jq is recommended for install-manifest tracking and attribution checks; install succeeds
without it but smart updates and attribution status may be unavailable.
EOF
}

die() {
  echo "Error: $*" >&2
  exit 1
}

require_command() {
  local cmd="$1"
  command -v "${cmd}" >/dev/null 2>&1 || die "Missing required command: ${cmd}"
}

detect_platform() {
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"

  case "${arch}" in
    x86_64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    *) die "Unsupported architecture: ${arch}" ;;
  esac

  case "${os}" in
    darwin|linux) ;;
    mingw*|msys*|cygwin*) os="windows" ;;
    *) die "Unsupported OS: ${os}" ;;
  esac

  echo "${os}_${arch}"
}

resolve_local_source_root() {
  local top top_base
  if ! top="$(git -C "$(pwd -P)" rev-parse --show-toplevel 2>/dev/null)"; then
    return 1
  fi
  top_base="$(basename "${top}")"
  if [[ "$(printf '%s' "${top_base}" | tr '[:upper:]' '[:lower:]')" != "${REPO_NAME}" ]]; then
    return 1
  fi
  echo "${top}"
}

validate_source_root() {
  local source_root="$1"
  local resource
  for resource in "${RESOURCE_TYPES[@]}"; do
    [[ -d "${source_root}/${resource}" ]] || die "Source missing directory: ${resource}"
  done
  [[ -d "${source_root}/${BIN_SOURCE_DIR}" ]] || die "Source missing directory: ${BIN_SOURCE_DIR}"
  [[ -d "${source_root}/${RULES_DIR}" ]] || die "Source missing directory: ${RULES_DIR}"
}

remote_tarball_url() {
  local commit="$1"
  [[ -n "${commit}" ]] || die "Remote tarball URL requires commit SHA"
  printf 'https://codeload.github.com/%s/tar.gz/%s' "${GITHUB_REPO}" "${commit}"
}

fetch_commit_for_ref() {
  local ref="$1" sha response
  require_command curl
  response="$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/commits/${ref}")" \
    || die "Failed to resolve commit for ref: ${ref}"
  if jq_available; then
    sha="$(printf '%s' "${response}" | jq -er '.sha' 2>/dev/null || true)"
  else
    sha="$(printf '%s' "${response}" | grep '"sha"' | head -1 | sed 's/.*"sha": *"\([^"]*\)".*/\1/')"
  fi
  [[ -n "${sha}" ]] || die "Failed to resolve commit for ref: ${ref}"
  echo "${sha}"
}

ensure_online_release_resolved() {
  local release_json tag
  if [[ -n "${ONLINE_RELEASE_COMMIT}" ]]; then
    return 0
  fi
  require_command curl
  release_json="$(curl -fsSL "${GITHUB_API}")" || die "Failed to fetch release info"
  tag="$(echo "${release_json}" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"
  [[ -n "${tag}" ]] || die "Failed to parse latest release tag"
  ONLINE_RELEASE_TAG="${tag}"
  ONLINE_RELEASE_COMMIT="$(fetch_commit_for_ref "${tag}")"
}

local_is_at_or_ahead_of_release() {
  local source_root="$1" release_commit="$2"
  local local_sha
  local_sha="$(git -C "${source_root}" rev-parse HEAD)"
  git -C "${source_root}" merge-base --is-ancestor "${release_commit}" "${local_sha}" 2>/dev/null
}

fetch_remote_source_root() {
  local commit="$1"
  require_command curl
  require_command tar
  [[ -n "${commit}" ]] || die "Remote fetch requires commit SHA"

  local tmpdir archive tarball_url extracted_root
  tmpdir="$(mktemp -d)"
  archive="${tmpdir}/repo.tar.gz"
  tarball_url="$(remote_tarball_url "${commit}")"

  curl -fsSL "${tarball_url}" -o "${archive}" || die "Failed to download remote repository archive (${commit})"
  tar -xzf "${archive}" -C "${tmpdir}" || die "Failed to extract remote repository archive"

  extracted_root="$(find "${tmpdir}" -mindepth 1 -maxdepth 1 -type d | head -1)"
  [[ -n "${extracted_root}" && -d "${extracted_root}" ]] || die "Could not locate extracted remote source directory"
  echo "${tmpdir}:${extracted_root}"
}

install_resource() {
  local resource="$1"
  local source_root="$2"
  local mode="$3"
  local target="${HOME}/.cursor/${resource}/${REPO_NAME}"

  mkdir -p "$(dirname "${target}")"
  rm -rf "${target}"

  if [[ "${mode}" == "link" ]]; then
    ln -s "${source_root}/${resource}" "${target}"
  else
    cp -R "${source_root}/${resource}" "${target}"
  fi
}

install_bin() {
  local source_root="$1"
  local mode="$2"
  local entry base name found
  local -a source_names=()

  # Managed artifacts: entries from scripts/ (except Go binaries, handled by ensure_binaries).
  # Preserved state: projects.registry, .env, rules-source/, install-manifest.json, binaries.
  mkdir -p "${BIN_TARGET}"

  for entry in "${source_root}/${BIN_SOURCE_DIR}"/*; do
    [[ -e "${entry}" ]] || continue
    base="$(basename "${entry}")"

    case "${base}" in
      cuse|db-query) continue ;;
    esac
    if [[ "${base}" == ".env" && -f "${BIN_TARGET}/.env" ]]; then
      continue
    fi

    source_names+=("${base}")
    rm -rf "${BIN_TARGET}/${base}"

    if [[ "${mode}" == "link" ]]; then
      ln -s "${entry}" "${BIN_TARGET}/${base}"
    elif [[ -d "${entry}" ]]; then
      cp -R "${entry}" "${BIN_TARGET}/${base}"
    else
      cp "${entry}" "${BIN_TARGET}/${base}"
      [[ "${base}" == *.sh ]] && chmod +x "${BIN_TARGET}/${base}"
    fi
  done

  for entry in "${BIN_TARGET}"/*; do
    [[ -e "${entry}" ]] || continue
    base="$(basename "${entry}")"
    case "${base}" in
      projects.registry|.env|rules-source|cuse|db-query|install-manifest.json) continue ;;
    esac
    found="false"
    for name in "${source_names[@]}"; do
      if [[ "${name}" == "${base}" ]]; then
        found="true"
        break
      fi
    done
    if [[ "${found}" == "false" ]]; then
      rm -rf "${entry}"
    fi
  done
}

install_rules_source() {
  local source_root="$1"
  local mode="$2"

  rm -rf "${RULES_TARGET}"

  if [[ "${mode}" == "link" ]]; then
    ln -s "${source_root}/${RULES_DIR}" "${RULES_TARGET}"
  else
    cp -R "${source_root}/${RULES_DIR}" "${RULES_TARGET}"
  fi
}

sync_registered_projects() {
  local install_rules="${BIN_TARGET}/install-rules.sh"
  [[ -x "${install_rules}" ]] || install_rules="${BIN_TARGET}/install-rules.sh"
  if [[ ! -f "${install_rules}" ]]; then
    echo "Warning: install-rules.sh not found; skipping project rule sync." >&2
    return 0
  fi
  bash "${install_rules}" --sync-all
}

check_gh() {
  if ! command -v gh >/dev/null 2>&1; then
    echo "GitHub CLI (gh) is not installed. pr_sum.sh and release_sum.sh require it."
    echo "  Install: https://cli.github.com/"
    echo "  Then run: gh auth login"
    return 0
  fi
  if ! gh auth status >/dev/null 2>&1; then
    echo "GitHub CLI is installed but not authenticated."
    echo "  Run: gh auth login"
    echo "  Required for pr_sum.sh and release_sum.sh."
  fi
}

jq_available() {
  command -v jq >/dev/null 2>&1
}

jq_install_hint() {
  local os
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "${os}" in
    darwin)
      echo "brew install jq"
      ;;
    linux)
      echo "sudo apt install jq  # or your distro package manager"
      ;;
    mingw*|msys*|cygwin*)
      echo "winget install jqlang.jq  # or see https://jqlang.org/download/"
      ;;
    *)
      echo "see https://jqlang.org/download/"
      ;;
  esac
}

manifest_field() {
  local field="$1" value
  if [[ ! -f "${INSTALL_MANIFEST}" ]] || ! jq_available; then
    return 0
  fi
  value="$(jq -r --arg f "${field}" '
    if type != "object" then empty
    elif ($f | contains(".")) then (getpath($f | split(".")) // empty)
    else (.[$f] // empty)
    end
  ' "${INSTALL_MANIFEST}" 2>/dev/null || true)"
  if [[ -n "${value}" && "${value}" != "null" ]]; then
    printf '%s\n' "${value}"
  fi
}

write_install_manifest() {
  local commit="$1" version="$2" source="$3" mode="$4"
  local cuse_version="" db_query_version="" installed_at
  mkdir -p "${BIN_TARGET}"
  if [[ -x "${BIN_TARGET}/cuse" ]]; then
    cuse_version="$("${BIN_TARGET}/cuse" -version 2>/dev/null || true)"
  fi
  if [[ -x "${BIN_TARGET}/db-query" ]]; then
    db_query_version="$("${BIN_TARGET}/db-query" -version 2>/dev/null || true)"
  fi
  if ! jq_available; then
    echo "Warning: jq not found; could not write ${INSTALL_MANIFEST}" >&2
    echo "  Install jq: $(jq_install_hint)" >&2
    return 0
  fi
  installed_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
  if ! jq -n \
    --arg version "${version}" \
    --arg commit "${commit}" \
    --arg source "${source}" \
    --arg mode "${mode}" \
    --arg installed_at "${installed_at}" \
    --arg cuse "${cuse_version}" \
    --arg db_query "${db_query_version}" \
    '{
      version: $version,
      commit: $commit,
      source: $source,
      mode: $mode,
      installed_at: $installed_at,
      binaries: (
        {}
        | if $cuse != "" then . + {cuse: $cuse} else . end
        | if $db_query != "" then . + {"db-query": $db_query} else . end
      )
    }' > "${INSTALL_MANIFEST}"; then
    echo "Warning: jq failed writing ${INSTALL_MANIFEST}" >&2
    return 0
  fi
}

source_identity() {
  local source_root="$1" source_kind="$2"
  local commit version
  if [[ "${source_kind}" == "local" ]]; then
    [[ -n "${source_root}" ]] || die "Local install requires ai-toolkit source root"
    commit="$(git -C "${source_root}" rev-parse HEAD)"
    version="$(git -C "${source_root}" describe --tags --always --dirty)"
  else
    ensure_online_release_resolved
    commit="${ONLINE_RELEASE_COMMIT}"
    version="${ONLINE_RELEASE_TAG}@${commit:0:7}"
  fi
  printf '%s\n%s\n' "${commit}" "${version}"
}

link_target_matches() {
  local path="$1" expected="$2"
  [[ -L "${path}" ]] && [[ "$(readlink "${path}")" == "${expected}" ]]
}

copy_target_present() {
  local path="$1"
  [[ -e "${path}" ]] && [[ ! -L "${path}" ]]
}

managed_script_entry_needs_repair() {
  local source_root="$1" mode="$2" entry="$3"
  local base="${entry##*/}"
  case "${base}" in
    cuse|db-query) return 1 ;;
  esac
  if [[ "${base}" == ".env" && -f "${BIN_TARGET}/.env" ]]; then
    return 1
  fi
  if [[ "${mode}" == "link" ]]; then
    if ! link_target_matches "${BIN_TARGET}/${base}" "${entry}"; then
      return 0
    fi
  elif ! copy_target_present "${BIN_TARGET}/${base}"; then
    return 0
  fi
  return 1
}

assets_need_repair() {
  local source_root="$1" mode="$2"
  local resource target expected entry
  for resource in "${RESOURCE_TYPES[@]}"; do
    target="${HOME}/.cursor/${resource}/${REPO_NAME}"
    if [[ "${mode}" == "link" ]]; then
      expected="${source_root}/${resource}"
      if ! link_target_matches "${target}" "${expected}"; then
        return 0
      fi
    elif ! copy_target_present "${target}"; then
      return 0
    fi
  done
  if [[ "${mode}" == "link" ]]; then
    if ! link_target_matches "${RULES_TARGET}" "${source_root}/${RULES_DIR}"; then
      return 0
    fi
  elif ! copy_target_present "${RULES_TARGET}"; then
    return 0
  fi
  if [[ "${mode}" == "link" && -n "${source_root}" ]]; then
    local name expected dest
    for name in "${MANAGED_BINARIES[@]}"; do
      dest="${BIN_TARGET}/${name}"
      expected="${source_root}/${BIN_SOURCE_DIR}/${name}"
      if ! link_target_matches "${dest}" "${expected}"; then
        return 0
      fi
    done
  fi
  if [[ -n "${source_root}" ]]; then
    for entry in "${source_root}/${BIN_SOURCE_DIR}"/*; do
      [[ -e "${entry}" ]] || continue
      if managed_script_entry_needs_repair "${source_root}" "${mode}" "${entry}"; then
        return 0
      fi
    done
  elif [[ "${mode}" == "copy" && ! -f "${BIN_TARGET}/install-rules.sh" ]]; then
    return 0
  fi
  return 1
}

repair_assets() {
  local source_root="$1" mode="$2"
  local resource target expected entry
  for resource in "${RESOURCE_TYPES[@]}"; do
    target="${HOME}/.cursor/${resource}/${REPO_NAME}"
    if [[ "${mode}" == "link" ]]; then
      expected="${source_root}/${resource}"
      if ! link_target_matches "${target}" "${expected}"; then
        install_resource "${resource}" "${source_root}" "link"
      fi
    elif ! copy_target_present "${target}"; then
      install_resource "${resource}" "${source_root}" "copy"
    fi
  done
  if [[ "${mode}" == "link" ]]; then
    if ! link_target_matches "${RULES_TARGET}" "${source_root}/${RULES_DIR}"; then
      install_rules_source "${source_root}" "link"
    fi
  elif ! copy_target_present "${RULES_TARGET}"; then
    install_rules_source "${source_root}" "copy"
  fi
  install_bin "${source_root}" "${mode}"
  if [[ "${mode}" == "link" ]]; then
    repair_managed_binaries "${source_root}"
  fi
}

repair_managed_binaries() {
  local source_root="$1"
  local name src dest version
  for name in "${MANAGED_BINARIES[@]}"; do
    src="${source_root}/${BIN_SOURCE_DIR}/${name}"
    dest="${BIN_TARGET}/${name}"
    if link_target_matches "${dest}" "${src}"; then
      continue
    fi
    if [[ ! -f "${src}" ]] && command -v go >/dev/null 2>&1 && [[ -f "${source_root}/Makefile" ]]; then
      version="$(local_binary_source_version "${source_root}")"
      make -C "${source_root}" "${name}" VERSION="${version}" >/dev/null
    fi
    if [[ ! -f "${src}" ]]; then
      continue
    fi
    rm -f "${dest}"
    ln -s "${src}" "${dest}"
  done
}

binary_installed_version() {
  local name="$1"
  local path="${BIN_TARGET}/${name}"
  if [[ ! -x "${path}" ]]; then
    return 0
  fi
  "${path}" -version 2>/dev/null || true
}

normalize_version() {
  local v="${1#v}"
  echo "${v}"
}

versions_match() {
  local a b
  a="$(normalize_version "$1")"
  b="$(normalize_version "$2")"
  [[ -n "${a}" && "${a}" == "${b}" ]]
}

fetch_latest_release_tag() {
  ensure_online_release_resolved
  echo "${ONLINE_RELEASE_TAG}"
}

sha256_hex() {
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 | awk '{print $1}'
  elif command -v sha256sum >/dev/null 2>&1; then
    sha256sum | awk '{print $1}'
  else
    die "Need shasum or sha256sum to hash local binary source"
  fi
}

local_binary_source_version() {
  local source_root="$1"
  local base hash
  base="$(git -C "${source_root}" describe --tags --always)"
  if git -C "${source_root}" diff --quiet HEAD -- "${LOCAL_BINARY_PATHS[@]}" 2>/dev/null \
     && git -C "${source_root}" diff --cached --quiet -- "${LOCAL_BINARY_PATHS[@]}" 2>/dev/null; then
    echo "${base}"
    return 0
  fi
  hash="$(
    {
      git -C "${source_root}" diff HEAD -- "${LOCAL_BINARY_PATHS[@]}" 2>/dev/null
      git -C "${source_root}" diff --cached -- "${LOCAL_BINARY_PATHS[@]}" 2>/dev/null
    } | sha256_hex | cut -c1-12
  )"
  echo "${base}-dirty@${hash}"
}

binaries_need_update() {
  local source_kind="$1" source_root="$2" source_commit="$3" force="$4" mode="${5:-}"
  local name path latest_tag installed
  if [[ "${force}" == "true" ]]; then
    return 0
  fi
  if [[ "${source_kind}" == "local" ]] && command -v go >/dev/null 2>&1 && [[ -f "${source_root}/Makefile" ]]; then
    local expected dest src manifest_ver
    expected="$(local_binary_source_version "${source_root}")"
    for name in "${MANAGED_BINARIES[@]}"; do
      path="${BIN_TARGET}/${name}"
      if [[ ! -x "${path}" ]]; then
        return 0
      fi
      if [[ "${mode:-}" == "link" ]]; then
        src="${source_root}/${BIN_SOURCE_DIR}/${name}"
        if ! link_target_matches "${path}" "${src}"; then
          return 0
        fi
      fi
      installed="$(binary_installed_version "${name}")"
      if ! versions_match "${installed}" "${expected}"; then
        return 0
      fi
    done
    manifest_ver="$(manifest_field "binaries.cuse")"
    if [[ -n "${manifest_ver}" ]] && ! versions_match "${manifest_ver}" "${expected}"; then
      return 0
    fi
    return 1
  fi
  latest_tag="$(fetch_latest_release_tag)"
  for name in "${MANAGED_BINARIES[@]}"; do
    path="${BIN_TARGET}/${name}"
    if [[ ! -x "${path}" ]]; then
      return 0
    fi
    installed="$(binary_installed_version "${name}")"
    if ! versions_match "${installed}" "${latest_tag}"; then
      return 0
    fi
  done
  return 1
}

plan_install() {
  local source_root="$1" source_kind="$2" mode="$3" force="$4" force_sync_rules="$5" no_sync_rules="$6"
  local identity commit version installed_commit installed_version installed_mode installed_source assets_unchanged

  identity="$(source_identity "${source_root}" "${source_kind}")"
  commit="${identity%%$'\n'*}"
  version="${identity#*$'\n'}"

  installed_commit="$(manifest_field commit)"
  installed_version="$(manifest_field version)"
  installed_mode="$(manifest_field mode)"
  installed_source="$(manifest_field source)"
  PLAN_INSTALLED_VERSION="${installed_version}"
  PLAN_INSTALLED_COMMIT="${installed_commit}"
  PLAN_INSTALLED_MODE="${installed_mode}"
  PLAN_INSTALLED_SOURCE="${installed_source}"
  PLAN_SOURCE_COMMIT="${commit}"
  PLAN_SOURCE_VERSION="${version}"
  PLAN_SOURCE_KIND="${source_kind}"
  PLAN_MODE="${mode}"

  if [[ "${force}" == "true" ]]; then
    PLAN_ASSETS_ACTION="install"
  elif [[ -z "${installed_commit}" ]]; then
    PLAN_ASSETS_ACTION="install"
  elif [[ "${installed_commit}" != "${commit}" || "${installed_mode}" != "${mode}" || "${installed_source}" != "${source_kind}" || ( "${mode}" == "copy" && "${installed_version}" != "${version}" ) ]]; then
    PLAN_ASSETS_ACTION="install"
  elif assets_need_repair "${source_root}" "${mode}"; then
    if [[ -z "${source_root}" ]]; then
      PLAN_ASSETS_ACTION="install"
    else
      PLAN_ASSETS_ACTION="repair"
    fi
  else
    PLAN_ASSETS_ACTION="skip"
  fi

  if binaries_need_update "${source_kind}" "${source_root}" "${commit}" "${force}" "${mode}"; then
    PLAN_BINARIES_ACTION="install"
  else
    PLAN_BINARIES_ACTION="skip"
  fi

  if [[ "${no_sync_rules}" == "true" ]]; then
    PLAN_SYNC_RULES="false"
  elif [[ "${force_sync_rules}" == "true" ]]; then
    PLAN_SYNC_RULES="true"
  elif [[ "${PLAN_ASSETS_ACTION}" == "install" || "${PLAN_ASSETS_ACTION}" == "repair" ]]; then
    PLAN_SYNC_RULES="true"
  else
    PLAN_SYNC_RULES="false"
  fi
}

print_install_plan() {
  local assets_line binaries_line sync_line cuse_ver db_ver
  echo "ai-toolkit install plan"
  if [[ -n "${PLAN_INSTALLED_COMMIT}" ]]; then
    echo "  Installed:  ${PLAN_INSTALLED_COMMIT:0:7} (${PLAN_INSTALLED_VERSION:-unknown}) ${PLAN_INSTALLED_MODE:-?} ${PLAN_INSTALLED_SOURCE:-?}"
  else
    echo "  Installed:  (none)"
  fi
  echo "  Source:     ${PLAN_SOURCE_COMMIT:0:7} (${PLAN_SOURCE_VERSION}) ${PLAN_MODE} ${PLAN_SOURCE_KIND}"
  case "${PLAN_ASSETS_ACTION}" in
    skip) assets_line="skip (unchanged)" ;;
    repair) assets_line="repair (fix missing or broken assets)" ;;
    *) assets_line="install" ;;
  esac
  echo "  Assets:     ${assets_line}"
  case "${PLAN_BINARIES_ACTION}" in
    skip)
      cuse_ver="$(binary_installed_version cuse)"
      db_ver="$(binary_installed_version db-query)"
      binaries_line="skip (cuse ${cuse_ver:-missing}, db-query ${db_ver:-missing})"
      ;;
    install)
      if [[ "${PLAN_SOURCE_KIND}" == "local" ]] && command -v go >/dev/null 2>&1 && [[ -f "${1}/Makefile" ]]; then
        if [[ "${PLAN_MODE}" == "link" ]]; then
          binaries_line="link from local source (make cuse updates in place)"
        else
          binaries_line="build from local source"
        fi
      else
        binaries_line="download from GitHub releases"
      fi
      ;;
    *) binaries_line="skip" ;;
  esac
  echo "  Binaries:   ${binaries_line}"
  if [[ "${PLAN_SYNC_RULES}" == "true" ]]; then
    sync_line="run"
  else
    sync_line="skip (unchanged; use --sync-rules to force)"
  fi
  echo "  Rule sync:  ${sync_line}"
}

download_release_binary() {
  local name="$1" latest_tag="$2" platform="$3"
  local version_number asset_name download_url dest="${BIN_TARGET}/${name}"
  version_number="${latest_tag#v}"
  asset_name="${name}_${version_number}_${platform}"
  download_url="$(curl -fsSL "${GITHUB_API}" | grep "browser_download_url.*${asset_name}" | head -1 | sed 's/.*"\(https[^"]*\)".*/\1/')"
  [[ -n "${download_url}" ]] || die "No release found for ${name} on platform: ${platform}"
  echo "Downloading ${name} ${latest_tag} for ${platform}..."
  curl -fsSL "${download_url}" -o "${dest}" || die "Failed to download ${name}"
  chmod +x "${dest}"
  echo "Installed ${name} ${latest_tag} to ${dest}"
}

install_managed_binaries() {
  local source_root="$1" mode="$2"
  local name src dest version
  require_command go
  version="$(local_binary_source_version "${source_root}")"
  make -C "${source_root}" cuse db-query VERSION="${version}"
  for name in "${MANAGED_BINARIES[@]}"; do
    src="${source_root}/${BIN_SOURCE_DIR}/${name}"
    dest="${BIN_TARGET}/${name}"
    [[ -f "${src}" ]] || die "Built binary missing: ${src}"
    rm -f "${dest}"
    if [[ "${mode}" == "link" ]]; then
      ln -s "${src}" "${dest}"
    else
      cp "${src}" "${dest}"
      chmod +x "${dest}"
    fi
  done
  if [[ "${mode}" == "link" ]]; then
    echo "Linked binaries from ${source_root}/${BIN_SOURCE_DIR} into ${BIN_TARGET}"
  else
    echo "Built binaries from local source into ${BIN_TARGET}"
  fi
}

ensure_binaries() {
  local source_kind="$1" source_root="$2" mode="$3" force="$4"
  local platform latest_tag name installed
  if [[ "${source_kind}" == "local" ]] && command -v go >/dev/null 2>&1 && [[ -f "${source_root}/Makefile" ]]; then
    install_managed_binaries "${source_root}" "${mode}"
    return 0
  fi
  if [[ "${source_kind}" == "local" ]]; then
    echo "Warning: Go not available; downloading release binaries instead." >&2
  fi
  platform="$(detect_platform)"
  latest_tag="$(fetch_latest_release_tag)"
  for name in "${MANAGED_BINARIES[@]}"; do
    installed="$(binary_installed_version "${name}")"
    if [[ "${force}" == "true" ]] || [[ ! -x "${BIN_TARGET}/${name}" ]] || ! versions_match "${installed}" "${latest_tag}"; then
      download_release_binary "${name}" "${latest_tag}" "${platform}"
    else
      echo "${name} is up to date (${latest_tag})"
    fi
  done
}

install_assets() {
  local source_root="$1" mode="$2" action="$3"
  local resource
  case "${action}" in
    skip)
      echo "Assets unchanged; skipping reinstall."
      return 0
      ;;
    repair)
      echo "Repairing missing or broken ${mode}-mode assets..."
      repair_assets "${source_root}" "${mode}"
      return 0
      ;;
  esac
  for resource in "${RESOURCE_TYPES[@]}"; do
    install_resource "${resource}" "${source_root}" "${mode}"
  done
  install_bin "${source_root}" "${mode}"
  install_rules_source "${source_root}" "${mode}"
}

cursor_cli_config_path() {
  if [[ -n "${CURSOR_CONFIG_DIR:-}" ]]; then
    echo "${CURSOR_CONFIG_DIR%/}/cli-config.json"
    return 0
  fi
  local os
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  if [[ "${os}" == "linux" && -n "${XDG_CONFIG_HOME:-}" ]]; then
    echo "${XDG_CONFIG_HOME%/}/cursor/cli-config.json"
    return 0
  fi
  echo "${HOME}/.cursor/cli-config.json"
}

cursor_ide_vscdb_path() {
  local os
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "${os}" in
    darwin)
      echo "${HOME}/Library/Application Support/Cursor/User/globalStorage/state.vscdb"
      ;;
    linux)
      echo "${HOME}/.config/Cursor/User/globalStorage/state.vscdb"
      ;;
    mingw*|msys*|cygwin*)
      if [[ -n "${APPDATA:-}" ]]; then
        echo "${APPDATA}/Cursor/User/globalStorage/state.vscdb"
      else
        return 1
      fi
      ;;
    *)
      return 1
      ;;
  esac
}

# read_cli_attribution_flag prints "true", "false", or "unknown".
# Missing keys default to true per Cursor docs.
read_cli_attribution_flag() {
  local config="$1" key="$2" value
  if [[ ! -f "${config}" ]]; then
    echo "true"
    return 0
  fi
  if ! jq_available; then
    echo "unknown"
    return 0
  fi
  value="$(jq -er --arg key "${key}" '
    if type != "object" then "unknown"
    elif (.attribution | type) == "object" then
      if .attribution[$key] == true then "true"
      elif .attribution[$key] == false then "false"
      else "true" end
    elif .attribution == null then "true"
    else "unknown" end
  ' "${config}" 2>/dev/null || echo "unknown")"
  echo "${value}"
}

attribution_status_label() {
  case "$1" in
    true) echo "enabled" ;;
    false) echo "disabled" ;;
    *) echo "unknown" ;;
  esac
}

cli_attribution_line() {
  local label="$1" value="$2"
  case "${value}" in
    true)
      echo "  ${label}: enabled"
      ;;
    false)
      echo "  ${label}: disabled"
      ;;
    *)
      echo "  ${label}: unknown (could not read cli-config.json)"
      ;;
  esac
}

# read_ide_attribution_flag prints "true" or "false". Missing keys default to true per Cursor docs.
read_ide_attribution_flag() {
  local vscdb="$1" storage_key="$2"
  local value
  value="$(sqlite3 "${vscdb}" "SELECT value FROM ItemTable WHERE key = '${storage_key}' LIMIT 1;" 2>/dev/null || true)"
  case "${value}" in
    true|1)
      echo "true"
      ;;
    false|0)
      echo "false"
      ;;
    *)
      echo "true"
      ;;
  esac
}

check_attribution() {
  local cli_config commits prs vscdb ide_commits ide_prs

  echo "Cursor attribution:"

  cli_config="$(cursor_cli_config_path)"
  if ! jq_available; then
    echo "  CLI commits: unknown (jq not installed)"
    echo "  CLI PRs:     unknown (jq not installed)"
    echo "    Install jq: $(jq_install_hint)"
  else
    commits="$(read_cli_attribution_flag "${cli_config}" "attributeCommitsToAgent")"
    prs="$(read_cli_attribution_flag "${cli_config}" "attributePRsToAgent")"
    cli_attribution_line "CLI commits" "${commits}"
    cli_attribution_line "CLI PRs" "${prs}"
    if [[ "${commits}" == "true" || "${prs}" == "true" ]]; then
      echo "    Disable in ${cli_config}:"
      echo "      attribution.attributeCommitsToAgent: false"
      echo "      attribution.attributePRsToAgent: false"
    fi
  fi

  if ! command -v sqlite3 >/dev/null 2>&1; then
    echo "  IDE commits: skipped (sqlite3 not installed)"
    echo "  IDE PRs:     skipped (sqlite3 not installed)"
    return 0
  fi

  if ! vscdb="$(cursor_ide_vscdb_path)" || [[ ! -f "${vscdb}" ]]; then
    echo "  IDE commits: unknown (Cursor state database not found)"
    echo "  IDE PRs:     unknown (Cursor state database not found)"
    echo "    Check Cursor Settings → Git & PRs → Attribution after opening Cursor"
    return 0
  fi

  ide_commits="$(read_ide_attribution_flag "${vscdb}" "cursor/attributeCommitsToAgent")"
  ide_prs="$(read_ide_attribution_flag "${vscdb}" "cursor/attributePRsToAgent")"
  echo "  IDE commits: $(attribution_status_label "${ide_commits}")"
  echo "  IDE PRs:     $(attribution_status_label "${ide_prs}")"
  if [[ "${ide_commits}" == "true" || "${ide_prs}" == "true" ]]; then
    echo "    Disable in Cursor Settings → Git & PRs → Attribution"
  fi
}

main() {
  local requested_mode="" sync_rules="true" check_attribution_only="false"
  local check_only="false" force_install="false" force_sync_rules="false"
  while (($# > 0)); do
    case "$1" in
      --link)
        requested_mode="link"
        ;;
      --copy)
        requested_mode="copy"
        ;;
      --check)
        check_only="true"
        ;;
      --force)
        force_install="true"
        ;;
      --sync-rules)
        force_sync_rules="true"
        ;;
      --no-sync-rules)
        sync_rules="false"
        ;;
      --check-attribution)
        check_attribution_only="true"
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        die "Unknown argument: $1"
        ;;
    esac
    shift
  done

  if [[ "${check_attribution_only}" == "true" ]]; then
    check_attribution
    exit 0
  fi

  local source_kind source_root temp_root mode used_fallback
  local no_sync_rules="false"
  source_kind="online"
  source_root=""
  temp_root=""
  used_fallback="false"
  if [[ "${sync_rules}" == "false" ]]; then
    no_sync_rules="true"
  fi

  if local_root="$(resolve_local_source_root)"; then
    validate_source_root "${local_root}"
    ensure_online_release_resolved
    if local_is_at_or_ahead_of_release "${local_root}" "${ONLINE_RELEASE_COMMIT}"; then
      source_kind="local"
      source_root="${local_root}"
    fi
  fi

  if [[ -n "${requested_mode}" ]]; then
    mode="${requested_mode}"
  elif [[ "${source_kind}" == "local" ]]; then
    mode="link"
  else
    mode="copy"
  fi

  if [[ "${mode}" == "link" && "${source_kind}" == "online" ]]; then
    mode="copy"
    used_fallback="true"
  fi

  plan_install "${source_root}" "${source_kind}" "${mode}" "${force_install}" "${force_sync_rules}" "${no_sync_rules}"

  if [[ "${check_only}" == "true" ]]; then
    print_install_plan "${source_root}"
    exit 0
  fi

  if [[ "${PLAN_ASSETS_ACTION}" != "skip" && "${source_kind}" == "online" && -z "${source_root}" ]]; then
    ensure_online_release_resolved
    remote_info="$(fetch_remote_source_root "${ONLINE_RELEASE_COMMIT}")"
    temp_root="${remote_info%%:*}"
    source_root="${remote_info#*:}"
    validate_source_root "${source_root}"
  fi

  if [[ "${PLAN_ASSETS_ACTION}" != "skip" && -z "${source_root}" ]]; then
    die "Asset install requires source root"
  fi

  install_assets "${source_root}" "${mode}" "${PLAN_ASSETS_ACTION}"

  if [[ "${PLAN_BINARIES_ACTION}" == "install" ]]; then
    if [[ -z "${source_root}" && "${source_kind}" == "online" ]]; then
      source_root="${BIN_TARGET}"
    fi
    ensure_binaries "${source_kind}" "${source_root}" "${mode}" "${force_install}"
  fi

  if [[ -n "${temp_root}" ]]; then
    rm -rf "${temp_root}"
  fi

  write_install_manifest "${PLAN_SOURCE_COMMIT}" "${PLAN_SOURCE_VERSION}" "${PLAN_SOURCE_KIND}" "${PLAN_MODE}"

  if [[ "${PLAN_ASSETS_ACTION}" == "skip" && "${PLAN_BINARIES_ACTION}" == "skip" ]]; then
    echo "ai-toolkit is up to date (${PLAN_SOURCE_VERSION})."
  elif [[ "${used_fallback}" == "true" ]]; then
    echo "Installed to ${HOME}/.cursor/{commands,skills,agents}/${REPO_NAME} using copy from online (fallback from --link)."
  else
    echo "Installed to ${HOME}/.cursor/{commands,skills,agents}/${REPO_NAME} using ${mode} from ${source_kind}."
  fi
  if [[ "${PLAN_ASSETS_ACTION}" != "skip" ]]; then
    echo "Installed bin directory at ${BIN_TARGET} (from ${BIN_SOURCE_DIR}/)."
    echo "Installed rules source at ${RULES_TARGET} (from ${RULES_DIR}/)."
  fi

  if [[ "${PLAN_SYNC_RULES}" == "true" ]]; then
    echo ""
    sync_registered_projects
  fi
  echo ""
  echo "To run the bundled scripts from anywhere, add this to your shell config (e.g. ~/.zshrc or ~/.bashrc):"
  echo "  export PATH=\"\${HOME}/.cursor/${REPO_NAME}:\${PATH}\""
  echo ""
  check_attribution
  echo ""
  check_gh
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi
