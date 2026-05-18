#!/usr/bin/env bash

set -euo pipefail

readonly REPO_NAME="ai-toolkit"
readonly REMOTE_TARBALL_URL="https://codeload.github.com/hydronica/ai-toolkit/tar.gz/refs/heads/main"
readonly RESOURCE_TYPES=("rules" "commands" "skills" "agents")
readonly BIN_SOURCE_DIR="scripts"
readonly BIN_TARGET="${HOME}/.cursor/${REPO_NAME}"
readonly GITHUB_REPO="hydronica/ai-toolkit"
readonly GITHUB_API="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"

usage() {
  cat <<'EOF'
Usage: install.sh [--link|--copy]

  --link     Link from local ai-toolkit source when available
  --copy     Copy from local/online source
  -h, --help Show this help

Installs to ${HOME}/.cursor/(rules|commands|skills|agents)/ai-toolkit/
Installs scripts/ as a bin directory at ${HOME}/.cursor/ai-toolkit/
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
}

fetch_remote_source_root() {
  require_command curl
  require_command tar

  local tmpdir archive
  tmpdir="$(mktemp -d)"
  archive="${tmpdir}/repo.tar.gz"

  curl -fsSL "${REMOTE_TARBALL_URL}" -o "${archive}" || die "Failed to download remote repository"
  tar -xzf "${archive}" -C "${tmpdir}" || die "Failed to extract remote repository archive"

  local extracted_root
  extracted_root="${tmpdir}/${REPO_NAME}-main"
  [[ -d "${extracted_root}" ]] || die "Could not locate extracted remote source directory"
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

  mkdir -p "$(dirname "${BIN_TARGET}")"
  rm -rf "${BIN_TARGET}"

  if [[ "${mode}" == "link" ]]; then
    ln -s "${source_root}/${BIN_SOURCE_DIR}" "${BIN_TARGET}"
  else
    cp -R "${source_root}/${BIN_SOURCE_DIR}" "${BIN_TARGET}"
    find "${BIN_TARGET}" -type f -name '*.sh' -exec chmod +x {} +
  fi
}

ensure_cuse() {
  local cuse_path="${BIN_TARGET}/cuse"
  local platform latest_version local_version download_url asset_name

  platform="$(detect_platform)"

  # Fetch latest release info
  local release_json
  release_json="$(curl -fsSL "${GITHUB_API}")" || die "Failed to fetch release info"
  latest_version="$(echo "${release_json}" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"

  # Check local version
  local_version=""
  if [[ -x "${cuse_path}" ]]; then
    local_version="$("${cuse_path}" -version 2>/dev/null || echo "")"
  fi

  # Compare versions (strip leading 'v' for comparison)
  if [[ "${local_version}" == "${latest_version#v}" || "v${local_version}" == "${latest_version}" ]]; then
    echo "cuse is up to date (${latest_version})"
    return 0
  fi

  # Find download URL for this platform (strip 'v' prefix to match GoReleaser naming)
  local version_number="${latest_version#v}"
  asset_name="cuse_${version_number}_${platform}"
  download_url="$(echo "${release_json}" | grep "browser_download_url.*${asset_name}" | head -1 | sed 's/.*"\(https[^"]*\)".*/\1/')"

  [[ -n "${download_url}" ]] || die "No release found for platform: ${platform}"

  echo "Downloading cuse ${latest_version} for ${platform}..."
  curl -fsSL "${download_url}" -o "${cuse_path}" || die "Failed to download cuse"
  chmod +x "${cuse_path}"
  echo "Installed cuse ${latest_version} to ${cuse_path}"
}

main() {
  local requested_mode=""
  while (($# > 0)); do
    case "$1" in
      --link)
        requested_mode="link"
        ;;
      --copy)
        requested_mode="copy"
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

  local source_kind source_root temp_root mode used_fallback
  source_kind="online"
  source_root=""
  temp_root=""
  used_fallback="false"

  if source_root="$(resolve_local_source_root)"; then
    source_kind="local"
  else
    local remote_info
    remote_info="$(fetch_remote_source_root)"
    temp_root="${remote_info%%:*}"
    source_root="${remote_info#*:}"
  fi

  validate_source_root "${source_root}"

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

  local resource
  for resource in "${RESOURCE_TYPES[@]}"; do
    install_resource "${resource}" "${source_root}" "${mode}"
  done

  install_bin "${source_root}" "${mode}"

  # Ensure cuse binary is installed/updated
  ensure_cuse

  if [[ -n "${temp_root}" ]]; then
    rm -rf "${temp_root}"
  fi

  if [[ "${used_fallback}" == "true" ]]; then
    echo "Installed to ${HOME}/.cursor/{rules,commands,skills,agents}/${REPO_NAME} using copy from online (fallback from --link)."
  else
    echo "Installed to ${HOME}/.cursor/{rules,commands,skills,agents}/${REPO_NAME} using ${mode} from ${source_kind}."
  fi
  echo "Installed bin directory at ${BIN_TARGET} (from ${BIN_SOURCE_DIR}/)."
  echo ""
  echo "To run the bundled scripts from anywhere, add this to your shell config (e.g. ~/.zshrc or ~/.bashrc):"
  echo "  export PATH=\"\${HOME}/.cursor/${REPO_NAME}:\${PATH}\""
}

main "$@"
