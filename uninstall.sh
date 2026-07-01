#!/usr/bin/env bash

set -euo pipefail

readonly REPO_NAME="ai-toolkit"
readonly RESOURCE_TYPES=("commands" "skills" "agents")
readonly BIN_TARGET="${HOME}/.cursor/${REPO_NAME}"

usage() {
  cat <<'EOF'
Usage: uninstall.sh

Removes ${HOME}/.cursor/(commands|skills|agents)/ai-toolkit
Removes ${HOME}/.cursor/ai-toolkit (bin directory)

Does not remove project-level rules under .cursor/rules/ai-toolkit/ in your repos.
EOF
}

main() {
  if (($# > 0)); then
    case "$1" in
      -h|--help)
        usage
        exit 0
        ;;
      *)
        echo "Error: Unknown argument: $1" >&2
        exit 1
        ;;
    esac
  fi

  local resource target
  for resource in "${RESOURCE_TYPES[@]}"; do
    target="${HOME}/.cursor/${resource}/${REPO_NAME}"
    rm -rf "${target}"
  done

  rm -rf "${BIN_TARGET}"

  echo "Removed ${HOME}/.cursor/{commands,skills,agents}/${REPO_NAME}."
  echo "Removed ${BIN_TARGET}."
}

main "$@"