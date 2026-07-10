#!/usr/bin/env bash
#
# Install ai-toolkit Cursor rules into projects at .cursor/rules/ai-toolkit/
#
# Usage: install-rules.sh [options]
#
# Run from anywhere inside a Git work tree (uses repo root) or pass --project.
# Source resolution: canonical rules-source → adjacent repo → cwd clone → remote tarball.

set -euo pipefail

readonly REPO_NAME="ai-toolkit"
readonly RULES_DIR="rules"
readonly MANIFEST_NAME="manifest.sh"
readonly REMOTE_TARBALL_URL="https://codeload.github.com/hydronica/ai-toolkit/tar.gz/refs/heads/main"
readonly TOOLKIT_HOME="${HOME}/.cursor/${REPO_NAME}"
readonly CANONICAL_RULES="${TOOLKIT_HOME}/rules-source"
readonly REGISTRY_FILE="${TOOLKIT_HOME}/projects.registry"
readonly REGISTRY_HEADER='# ai-toolkit project registry — managed by install-rules.sh; do not edit by hand unless you know the format
# columns: absolute_path|filter|mode'

SOURCE_KIND=""
SOURCE_RULES=""
TEMP_ROOT=""

ORG_RULES=()
STACK_NAMES=()
SELECTED_RULES=()
SELECTED_STACKS=()

usage() {
  cat <<'EOF'
Usage: install-rules.sh [options]

Project install (default):
  --filter <spec>  Which rules to install (default: auto)
                   auto  — org rules (if any) + stacks detected in the project
                   all   — org rules (if any) + every stack in the manifest
                   org   — org rules only
                   go    — org + go stack (comma-separate for multiple stacks)
  --link           Symlink each rule file (default when source is local/canonical)
  --copy           Copy each rule file (default when source is remote)
  --project <dir>  Project root (default: Git root of cwd, else pwd)
  --no-register    Install without adding the project to the registry
  --dry-run        Print selected rules and exit without installing

Registry management:
  --sync-all       Reinstall rules for every registered project
  --list           List registered projects
  --unregister --project <dir>
                   Remove a project from the registry (rules on disk unchanged)
  --purge --project <dir>
                   Remove .cursor/rules/ai-toolkit/ from one project and unregister
  --purge-all      Remove rules from all registered projects and clear the registry

  -h, --help       Show this help

Installs selected rules to <project>/.cursor/rules/ai-toolkit/

Source (first match wins):
  1. ~/.cursor/ai-toolkit/rules-source/ (after install.sh)
  2. ai-toolkit repo adjacent to this script
  3. Local ai-toolkit clone (when cwd is that repository)
  4. Remote tarball from GitHub
EOF
}

die() {
  echo "Error: $*" >&2
  exit 1
}

warn() {
  echo "Warning: $*" >&2
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

  if [[ -f "${CANONICAL_RULES}/${MANIFEST_NAME}" ]]; then
    validate_rules_source "${CANONICAL_RULES}"
    SOURCE_KIND="canonical"
    SOURCE_RULES="${CANONICAL_RULES}"
    return
  fi

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

add_rule_if_new() {
  local candidate="$1"
  local existing
  if ((${#SELECTED_RULES[@]} > 0)); then
    for existing in "${SELECTED_RULES[@]}"; do
      [[ "${existing}" == "${candidate}" ]] && return 0
    done
  fi
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
  local stack

  load_manifest "${source_rules}"

  SELECTED_RULES=()
  SELECTED_STACKS=()

  if ((${#ORG_RULES[@]} > 0)); then
    local rule
    for rule in "${ORG_RULES[@]}"; do
      add_rule_if_new "${rule}"
    done
  fi

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

  if ((${#SELECTED_RULES[@]} > 0)); then
    local file
    for file in "${SELECTED_RULES[@]}"; do
      [[ -f "${source_rules}/${file}" ]] || die "Rule file missing in source: ${file}"
    done
  fi
}

registry_ensure_dir() {
  mkdir -p "$(dirname "${REGISTRY_FILE}")"
}

registry_read_entries() {
  local line path filter mode
  REGISTRY_ENTRIES=()

  [[ -f "${REGISTRY_FILE}" ]] || return 0

  while IFS= read -r line || [[ -n "${line}" ]]; do
    [[ -z "${line}" ]] && continue
    [[ "${line}" =~ ^[[:space:]]*# ]] && continue
    IFS='|' read -r path filter mode <<< "${line}"
    if [[ -z "${path}" || -z "${filter}" || -z "${mode}" ]]; then
      warn "Skipping malformed registry line: ${line}"
      continue
    fi
    if [[ "${mode}" != "link" && "${mode}" != "copy" ]]; then
      warn "Skipping registry line with invalid mode '${mode}': ${line}"
      continue
    fi
    REGISTRY_ENTRIES+=("${path}|${filter}|${mode}")
  done < "${REGISTRY_FILE}"
}

registry_write_entries() {
  registry_ensure_dir
  {
    printf '%s\n' "${REGISTRY_HEADER}"
    if ((${#REGISTRY_ENTRIES[@]} > 0)); then
      local entry
      for entry in "${REGISTRY_ENTRIES[@]}"; do
        printf '%s\n' "${entry}"
      done
    fi
  } > "${REGISTRY_FILE}"
}

registry_register() {
  local path="$1"
  local filter="$2"
  local mode="$3"
  local entry existing path_part filter_part mode_part
  local -a new_entries=()

  registry_read_entries
  entry="${path}|${filter}|${mode}"

  if ((${#REGISTRY_ENTRIES[@]} > 0)); then
    for existing in "${REGISTRY_ENTRIES[@]}"; do
      IFS='|' read -r path_part filter_part mode_part <<< "${existing}"
      [[ "${path_part}" == "${path}" ]] && continue
      new_entries+=("${existing}")
    done
  fi
  new_entries+=("${entry}")
  REGISTRY_ENTRIES=("${new_entries[@]}")
  registry_write_entries
}

registry_unregister() {
  local path="$1"
  local entry existing path_part _filter _mode
  local -a new_entries=()
  local found="false"

  registry_read_entries
  if ((${#REGISTRY_ENTRIES[@]} > 0)); then
    for existing in "${REGISTRY_ENTRIES[@]}"; do
      IFS='|' read -r path_part _filter _mode <<< "${existing}"
      if [[ "${path_part}" == "${path}" ]]; then
        found="true"
        continue
      fi
      new_entries+=("${existing}")
    done
  fi

  if [[ "${found}" != "true" ]]; then
    die "Project not in registry: ${path}"
  fi

  REGISTRY_ENTRIES=("${new_entries[@]}")
  registry_write_entries
}

registry_unregister_if_present() {
  local path="$1"
  local entry existing path_part _filter _mode
  local -a new_entries=()

  [[ -f "${REGISTRY_FILE}" ]] || return 0

  registry_read_entries
  if ((${#REGISTRY_ENTRIES[@]} > 0)); then
    for existing in "${REGISTRY_ENTRIES[@]}"; do
      IFS='|' read -r path_part _filter _mode <<< "${existing}"
      if [[ "${path_part}" == "${path}" ]]; then
        continue
      fi
      new_entries+=("${existing}")
    done
  fi

  if ((${#new_entries[@]} < ${#REGISTRY_ENTRIES[@]})); then
    REGISTRY_ENTRIES=("${new_entries[@]}")
    registry_write_entries
  fi
}

registry_clear() {
  REGISTRY_ENTRIES=()
  registry_write_entries
}

purge_project_rules() {
  local project_root="$1"
  local target="${project_root}/.cursor/rules/${REPO_NAME}"
  local rules_parent="${project_root}/.cursor/rules"

  if [[ -d "${target}" ]]; then
    rm -rf "${target}"
    if [[ -d "${rules_parent}" ]] && [[ -z "$(ls -A "${rules_parent}" 2>/dev/null)" ]]; then
      rmdir "${rules_parent}" 2>/dev/null || true
    fi
    if [[ -d "${project_root}/.cursor" ]] && [[ -z "$(ls -A "${project_root}/.cursor" 2>/dev/null)" ]]; then
      rmdir "${project_root}/.cursor" 2>/dev/null || true
    fi
    echo "Purged ${target}"
  else
    warn "No rules directory at ${target}"
  fi
}

install_files_to_project() {
  local source_rules="$1"
  local mode="$2"
  local project_root="$3"
  shift 3
  local -a files=("$@")
  local target="${project_root}/.cursor/rules/${REPO_NAME}"
  local file effective_mode="${mode}"

  mkdir -p "$(dirname "${target}")"
  rm -rf "${target}"
  mkdir -p "${target}"

  for file in "${files[@]}"; do
    if [[ "${effective_mode}" == "link" ]]; then
      if ! ln -s "${source_rules}/${file}" "${target}/${file}" 2>/dev/null; then
        warn "Symlink failed for ${file}; falling back to copy for this project."
        rm -rf "${target}"
        mkdir -p "${target}"
        effective_mode="copy"
        break
      fi
    else
      cp "${source_rules}/${file}" "${target}/${file}"
    fi
  done

  if [[ "${effective_mode}" == "copy" && "${mode}" == "link" ]]; then
    for file in "${files[@]}"; do
      cp "${source_rules}/${file}" "${target}/${file}"
    done
  fi

  echo "${effective_mode}"
}

install_one_project() {
  local project_root="$1"
  local filter_spec="$2"
  local mode="$3"
  local dry_run="$4"
  local register="${5:-true}"
  local -a files=()
  local effective_mode file

  if [[ ! -d "${project_root}" ]]; then
    warn "Skipping missing project: ${project_root}"
    return 2
  fi

  resolve_selected_rules "${project_root}" "${SOURCE_RULES}" "${filter_spec}"

  if ((${#SELECTED_RULES[@]} == 0)); then
    warn "No rules selected for ${project_root} (filter=${filter_spec}); skipping."
    return 2
  fi

  files=("${SELECTED_RULES[@]}")

  echo "Project: ${project_root}"
  echo "Filter:  ${filter_spec}"
  if ((${#SELECTED_STACKS[@]} > 0)); then
    echo "Stacks:  ${SELECTED_STACKS[*]}"
  elif [[ "${filter_spec}" == "auto" ]]; then
    if ((${#ORG_RULES[@]} > 0)); then
      echo "Stacks:  (none detected — org rules only)"
    else
      echo "Stacks:  (none detected)"
    fi
  fi
  echo "Mode:    ${mode}"
  echo "Rules:"
  for file in "${files[@]}"; do
    echo "  - ${file}"
  done

  if [[ "${dry_run}" == "true" ]]; then
    echo ""
    return 0
  fi

  effective_mode="$(install_files_to_project "${SOURCE_RULES}" "${mode}" "${project_root}" "${files[@]}")"
  if [[ "${register}" == "true" ]]; then
    registry_register "${project_root}" "${filter_spec}" "${effective_mode}"
  fi
  echo "Installed to ${project_root}/.cursor/rules/${REPO_NAME}/ (${effective_mode})"
  echo ""
  return 0
}

cmd_list() {
  local entry path filter mode count=0

  registry_read_entries
  count="${#REGISTRY_ENTRIES[@]}"

  echo "REGISTERED PROJECTS (${count})"
  if ((${count} == 0)); then
    return 0
  fi

  if ((${#REGISTRY_ENTRIES[@]} > 0)); then
    for entry in "${REGISTRY_ENTRIES[@]}"; do
      IFS='|' read -r path filter mode <<< "${entry}"
      echo "  ${path}  filter=${filter}  mode=${mode}"
    done
  fi
}

cmd_sync_all() {
  local dry_run="$1"
  local entry path filter mode
  local updated=0 skipped=0 failed=0
  local -a failures=()

  registry_read_entries
  if ((${#REGISTRY_ENTRIES[@]} == 0)); then
    echo "No registered projects; nothing to sync."
    return 0
  fi

  resolve_rules_source

  echo "Syncing ${#REGISTRY_ENTRIES[@]} registered project(s) from ${SOURCE_RULES}"
  echo ""

  if ((${#REGISTRY_ENTRIES[@]} > 0)); then
    for entry in "${REGISTRY_ENTRIES[@]}"; do
      IFS='|' read -r path filter mode <<< "${entry}"
      if install_one_project "${path}" "${filter}" "${mode}" "${dry_run}" "true"; then
        updated=$((updated + 1))
      else
        local rc=$?
        if [[ "${rc}" -eq 2 ]]; then
          skipped=$((skipped + 1))
          failures+=("${path} (skipped)")
        else
          failed=$((failed + 1))
          failures+=("${path} (failed)")
        fi
      fi
    done
  fi

  if [[ -n "${TEMP_ROOT}" ]]; then
    rm -rf "${TEMP_ROOT}"
  fi

  echo "Sync summary: updated=${updated} skipped=${skipped} failed=${failed}"
  if ((${#failures[@]} > 0)); then
    echo "Issues:"
    local failure
    for failure in "${failures[@]}"; do
      echo "  - ${failure}"
    done
  fi

  if [[ "${dry_run}" == "true" ]]; then
    echo ""
    echo "Dry run — no files written."
    return 0
  fi

  if ((failed > 0)); then
    return 1
  fi
  return 0
}

cmd_purge_all() {
  local entry path _filter _mode

  registry_read_entries
  if ((${#REGISTRY_ENTRIES[@]} == 0)); then
    echo "No registered projects; nothing to purge."
    registry_clear
    return 0
  fi

  if ((${#REGISTRY_ENTRIES[@]} > 0)); then
    for entry in "${REGISTRY_ENTRIES[@]}"; do
      IFS='|' read -r path _filter _mode <<< "${entry}"
      if [[ -d "${path}" ]]; then
        purge_project_rules "${path}"
      else
        warn "Skipping missing project: ${path}"
      fi
    done
  fi

  registry_clear
  echo "Cleared registry at ${REGISTRY_FILE}"
}

main() {
  local requested_mode="" project_arg="" filter_spec="auto" dry_run="false"
  local register="true"
  local action="install"

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
      --no-register)
        register="false"
        ;;
      --sync-all)
        action="sync-all"
        ;;
      --list)
        action="list"
        ;;
      --unregister)
        action="unregister"
        ;;
      --purge)
        action="purge"
        ;;
      --purge-all)
        action="purge-all"
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

  case "${action}" in
    list)
      cmd_list
      exit 0
      ;;
    sync-all)
      cmd_sync_all "${dry_run}"
      exit $?
      ;;
    purge-all)
      [[ "${dry_run}" != "true" ]] || die "--dry-run is not supported with --purge-all"
      cmd_purge_all
      exit 0
      ;;
    unregister)
      [[ -n "${project_arg}" ]] || die "--unregister requires --project <dir>"
      local unregister_root
      unregister_root="$(resolve_project_root "${project_arg}")"
      registry_unregister "${unregister_root}"
      echo "Unregistered ${unregister_root}"
      exit 0
      ;;
    purge)
      [[ -n "${project_arg}" ]] || die "--purge requires --project <dir>"
      local purge_root
      purge_root="$(resolve_project_root "${project_arg}")"
      purge_project_rules "${purge_root}"
      registry_unregister_if_present "${purge_root}"
      exit 0
      ;;
  esac

  local project_root
  project_root="$(resolve_project_root "${project_arg}")"

  resolve_rules_source

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

  if [[ "${dry_run}" == "true" ]]; then
    install_one_project "${project_root}" "${filter_spec}" "${mode}" "true" "${register}"
    echo "Dry run — no files written."
    exit 0
  fi

  local rc=0
  install_one_project "${project_root}" "${filter_spec}" "${mode}" "false" "${register}" || rc=$?

  if [[ "${rc}" -eq 2 ]]; then
    case "${filter_spec}" in
      org)
        die "No rules selected for --filter org (manifest defines no org rules)."
        ;;
      auto)
        die "No rules selected for --filter auto (no org rules and no stacks detected). Pass an explicit stack, e.g. --filter go."
        ;;
      *)
        die "No rules selected for --filter ${filter_spec}."
        ;;
    esac
  fi

  if [[ -n "${TEMP_ROOT}" ]]; then
    rm -rf "${TEMP_ROOT}"
  fi

  if [[ "${used_fallback}" == "true" ]]; then
    echo "Installed using copy from online (fallback from --link)."
  fi
  echo "Reload the workspace in Cursor if rules do not appear immediately."
  echo "See docs/cursor.md for known Cursor limitations around nested .cursor/rules/ paths."
}

REGISTRY_ENTRIES=()
main "$@"
