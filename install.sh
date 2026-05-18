#!/usr/bin/env bash

set -euo pipefail

readonly REPO_NAME="ai-toolkit"
readonly REMOTE_TARBALL_URL="https://codeload.github.com/hydronica/ai-toolkit/tar.gz/refs/heads/main"
readonly RESOURCE_TYPES=("rules" "commands" "skills" "agents")
readonly BIN_SOURCE_DIR="scripts"
readonly BIN_TARGET="${HOME}/.cursor/${REPO_NAME}"

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
