#!/usr/bin/env bash
#
# Install ai-toolkit Cursor rules into the current project at .cursor/rules/ai-toolkit/
#
# Usage: install-rules.sh [--filter auto|all|org|<stack>[,<stack>...]] [--link|--copy] [--project <dir>] [--dry-run] [-h|--help]
#
# Run from anywhere inside a Git work tree (uses repo root) or pass --project.
# Source resolution: script location → cwd ai-toolkit clone → remote tarball.

set -euo pipefail

readonly REPO_NAME="ai-toolkit"
readonly RULES_DIR="rules"
readonly MANIFEST_NAME="manifest.sh"
readonly REMOTE_TARBALL_URL="https://codeload.github.com/hydronica/ai-toolkit/tar.gz/refs/heads/main"

SOURCE_KIND=""
SOURCE_RULES=""
TEMP_ROOT=""

ORG_RULES=()
STACK_NAMES=()

usage() {
  cat <<'EOF'
Usage: install-rules.sh [--filter auto|all|org|<stack>[,<stack>...]] [--link|--copy] [--project <dir>] [--dry-run] [-h|--help]

  --filter <spec>  Which rules to install (default: auto)
                   auto  — org rules + stacks detected in the project (see rules/manifest.sh)
                   all   — org rules + every stack in the manifest
                   org   — org rules only
                   go    — org + go stack (comma-separate for multiple stacks)
  --link           Symlink each rule file (default when source is local)
  --copy           Copy each rule file (default when source is remote)
  --project <dir>  Project root (default: Git root of current directory, else pwd)
  --dry-run        Print selected rules and exit without installing
  -h, --help       Show this help

Installs selected rules to <project>/.cursor/rules/ai-toolkit/

Source (first match wins):
  1. ai-toolkit repo adjacent to this script
  2. Local ai-toolkit clone (when cwd is that repository)
  3. Remote tarball from GitHub
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

resolve_project_root() {
  local explicit="${1:-}"
  if [[ -n "${explicit}" ]]; then
    [[ -d "${explicit}" ]] || die "Project directory not found: ${explicit}"
    cd "${explicit}" && pwd -P
    return
  fi

  local top
  if top="$(git rev-parse --show-toplevel 2>/dev/null)"; then
    (cd "${top}" && pwd -P)
    return
  fi

  pwd -P
}

resolve_local_toolkit_root() {
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

validate_rules_source() {
  local source_rules="$1"
  local manifest="${source_rules}/${MANIFEST_NAME}"
  [[ -d "${source_rules}" ]] || die "Rules source missing: ${source_rules}"
  [[ -f "${manifest}" ]] || die "Rules manifest missing: ${manifest}"
}

fetch_remote_rules_dir() {
  require_command curl
  require_command tar

  local tmpdir archive extracted_root
  tmpdir="$(mktemp -d)"
  archive="${tmpdir}/repo.tar.gz"

  curl -fsSL "${REMOTE_TARBALL_URL}" -o "${archive}" || die "Failed to download remote repository"
  tar -xzf "${archive}" -C "${tmpdir}" || die "Failed to extract remote repository archive"

  extracted_root="${tmpdir}/${REPO_NAME}-main"
  [[ -d "${extracted_root}/${RULES_DIR}" ]] || die "Remote source missing directory: ${RULES_DIR}"
  echo "${tmpdir}:${extracted_root}/${RULES_DIR}"
}

resolve_script_dir() {
  local src="${BASH_SOURCE[0]}"
  while [[ -L "${src}" ]]; do
    local dir
    dir="$(cd "$(dirname "${src}")" && pwd -P)"
    src="$(readlink "${src}")"
    [[ "${src}" != /* ]] && src="${dir}/${src}"
  done
  cd "$(dirname "${src}")" && pwd -P
}

resolve_rules_source() {
  local toolkit_root rules_dir script_dir

  script_dir="$(resolve_script_dir)"
  toolkit_root="$(cd "${script_dir}/.." && pwd -P)"
  rules_dir="${toolkit_root}/${RULES_DIR}"
  if [[ -f "${rules_dir}/${MANIFEST_NAME}" ]]; then
    validate_rules_source "${rules_dir}"
    SOURCE_KIND="local"
    SOURCE_RULES="${rules_dir}"
    return
  fi

  if toolkit_root="$(resolve_local_toolkit_root)"; then
    rules_dir="${toolkit_root}/${RULES_DIR}"
    validate_rules_source "${rules_dir}"
    SOURCE_KIND="local"
    SOURCE_RULES="${rules_dir}"
    return
  fi

  local remote_info
  remote_info="$(fetch_remote_rules_dir)"
  TEMP_ROOT="${remote_info%%:*}"
  rules_dir="${remote_info#*:}"
  validate_rules_source "${rules_dir}"
  SOURCE_KIND="online"
  SOURCE_RULES="${rules_dir}"
}

load_manifest() {
  local source_rules="$1"
  local manifest="${source_rules}/${MANIFEST_NAME}"

  ORG_RULES=()
  STACK_NAMES=()

  # shellcheck source=/dev/null
  source "${manifest}"

  ((${#ORG_RULES[@]} > 0)) || die "Manifest defines no org rules: ${manifest}"
}

stack_detect_files() {
  local stack="$1"
  eval "printf '%s\n' \"\${STACK_${stack}_DETECT[@]}\""
}

stack_rule_files() {
  local stack="$1"
  eval "printf '%s\n' \"\${STACK_${stack}_RULES[@]}\""
}

stack_is_known() {
  local stack="$1"
  local name
  for name in "${STACK_NAMES[@]}"; do
    [[ "${name}" == "${stack}" ]] && return 0
  done
  return 1
}

stack_detected_in_project() {
  local project_root="$1"
  local stack="$2"
  local detect_file
  while IFS= read -r detect_file; do
    [[ -n "${detect_file}" ]] || continue
    [[ -e "${project_root}/${detect_file}" ]] && return 0
  done < <(stack_detect_files "${stack}")
  return 1
}

SELECTED_RULES=()
SELECTED_STACKS=()

add_rule_if_new() {
  local candidate="$1"
  local existing
  for existing in "${SELECTED_RULES[@]}"; do
    [[ "${existing}" == "${candidate}" ]] && return 0
  done
  SELECTED_RULES+=("${candidate}")
}

append_stack_rules() {
  local stack="$1"
  local rule
  while IFS= read -r rule; do
    [[ -n "${rule}" ]] || continue
    add_rule_if_new "${rule}"
  done < <(stack_rule_files "${stack}")
}

resolve_selected_rules() {
  local project_root="$1"
  local source_rules="$2"
  local filter_spec="$3"
  local stack rule

  load_manifest "${source_rules}"

  SELECTED_RULES=()
  SELECTED_STACKS=()

  for rule in "${ORG_RULES[@]}"; do
    add_rule_if_new "${rule}"
  done

  case "${filter_spec}" in
    org)
      ;;
    all)
      for stack in "${STACK_NAMES[@]}"; do
        SELECTED_STACKS+=("${stack}")
        append_stack_rules "${stack}"
      done
      ;;
    auto)
      for stack in "${STACK_NAMES[@]}"; do
        if stack_detected_in_project "${project_root}" "${stack}"; then
          SELECTED_STACKS+=("${stack}")
          append_stack_rules "${stack}"
        fi
      done
      ;;
    *)
      local IFS=','
      read -ra SELECTED_STACKS <<< "${filter_spec}"
      for stack in "${SELECTED_STACKS[@]}"; do
        stack="${stack// /}"
        [[ -n "${stack}" ]] || continue
        stack_is_known "${stack}" || die "Unknown stack in --filter: ${stack} (see ${MANIFEST_NAME})"
        append_stack_rules "${stack}"
      done
      ;;
  esac

  local file
  for file in "${SELECTED_RULES[@]}"; do
    [[ -f "${source_rules}/${file}" ]] || die "Rule file missing in source: ${file}"
  done
}

install_selected_rules() {
  local source_rules="$1"
  local mode="$2"
  local project_root="$3"
  shift 3
  local -a files=("$@")
  local target="${project_root}/.cursor/rules/${REPO_NAME}"
  local file

  mkdir -p "$(dirname "${target}")"
  rm -rf "${target}"
  mkdir -p "${target}"

  for file in "${files[@]}"; do
    if [[ "${mode}" == "link" ]]; then
      ln -s "${source_rules}/${file}" "${target}/${file}"
    else
      cp "${source_rules}/${file}" "${target}/${file}"
    fi
  done

  echo "${target}"
}

main() {
  local requested_mode="" project_arg="" filter_spec="auto" dry_run="false"
  while (($# > 0)); do
    case "$1" in
      --link)
        requested_mode="link"
        ;;
      --copy)
        requested_mode="copy"
        ;;
      --project)
        shift
        [[ $# -gt 0 ]] || die "--project requires a directory"
        project_arg="$1"
        ;;
      --filter)
        shift
        [[ $# -gt 0 ]] || die "--filter requires a value"
        filter_spec="$1"
        ;;
      --dry-run)
        dry_run="true"
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

  local project_root
  project_root="$(resolve_project_root "${project_arg}")"

  resolve_rules_source
  resolve_selected_rules "${project_root}" "${SOURCE_RULES}" "${filter_spec}"

  local -a selected_rules=("${SELECTED_RULES[@]}")
  local -a selected_stacks=()
  if ((${#SELECTED_STACKS[@]} > 0)); then
    selected_stacks=("${SELECTED_STACKS[@]}")
  fi

  if ((${#selected_rules[@]} == 0)); then
    die "No rules selected for --filter ${filter_spec}"
  fi

  local mode used_fallback
  used_fallback="false"
  if [[ -n "${requested_mode}" ]]; then
    mode="${requested_mode}"
  elif [[ "${SOURCE_KIND}" == "online" ]]; then
    mode="copy"
  else
    mode="link"
  fi

  if [[ "${mode}" == "link" && "${SOURCE_KIND}" == "online" ]]; then
    mode="copy"
    used_fallback="true"
  fi

  echo "Project: ${project_root}"
  echo "Filter:  ${filter_spec}"
  if ((${#selected_stacks[@]} > 0)); then
    echo "Stacks:  ${selected_stacks[*]}"
  elif [[ "${filter_spec}" == "auto" ]]; then
    echo "Stacks:  (none detected — org rules only)"
  fi
  echo "Rules:"
  local file
  for file in "${selected_rules[@]}"; do
    echo "  - ${file}"
  done

  if [[ "${dry_run}" == "true" ]]; then
    echo ""
    echo "Dry run — no files written."
    exit 0
  fi

  local target
  target="$(install_selected_rules "${SOURCE_RULES}" "${mode}" "${project_root}" "${selected_rules[@]}")"

  if [[ -n "${TEMP_ROOT}" ]]; then
    rm -rf "${TEMP_ROOT}"
  fi

  echo ""
  if [[ "${used_fallback}" == "true" ]]; then
    echo "Installed project rules to ${target} using copy from online (fallback from --link)."
  else
    echo "Installed project rules to ${target} using ${mode} from ${SOURCE_KIND}."
  fi
  echo ""
  echo "Reload the workspace in Cursor if rules do not appear immediately."
  echo "See docs/cursor.md for known Cursor limitations around nested .cursor/rules/ paths."
}

main "$@"
