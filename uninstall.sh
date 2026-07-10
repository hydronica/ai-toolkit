#!/usr/bin/env bash

set -euo pipefail

readonly REPO_NAME="ai-toolkit"
readonly RESOURCE_TYPES=("commands" "skills" "agents")
readonly BIN_TARGET="${HOME}/.cursor/${REPO_NAME}"

usage() {
  cat <<'EOF'
Usage: uninstall.sh [--purge-project-rules]

  --purge-project-rules  Remove .cursor/rules/ai-toolkit/ from all registered
                         projects before removing global toolkit files
  -h, --help             Show this help

Removes ${HOME}/.cursor/(commands|skills|agents)/ai-toolkit
Removes ${HOME}/.cursor/ai-toolkit (scripts, rules-source, projects.registry)

By default, project-level rules under .cursor/rules/ai-toolkit/ are left in place.
Link-mode projects will have dangling symlinks after uninstall unless you purge first.
EOF
}

main() {
  local purge_projects="false"

  while (($# > 0)); do
    case "$1" in
      --purge-project-rules)
        purge_projects="true"
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        echo "Error: Unknown argument: $1" >&2
        exit 1
        ;;
    esac
    shift
  done

  if [[ "${purge_projects}" == "true" ]]; then
    local install_rules="${BIN_TARGET}/install-rules.sh"
    if [[ -f "${install_rules}" ]]; then
      bash "${install_rules}" --purge-all
    else
      echo "Warning: install-rules.sh not found; skipping project rule purge." >&2
    fi
  fi

  local resource target
  for resource in "${RESOURCE_TYPES[@]}"; do
    target="${HOME}/.cursor/${resource}/${REPO_NAME}"
    rm -rf "${target}"
  done

  rm -rf "${BIN_TARGET}"

  echo "Removed ${HOME}/.cursor/{commands,skills,agents}/${REPO_NAME}."
  echo "Removed ${BIN_TARGET}."
  if [[ "${purge_projects}" != "true" ]]; then
    echo ""
    echo "Project rules under .cursor/rules/ai-toolkit/ were not removed."
    echo "Run uninstall.sh --purge-project-rules for full cleanup."
  fi
}

main "$@"
