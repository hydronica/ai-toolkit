#!/usr/bin/env bash

set -euo pipefail

readonly REPO_NAME="ai-toolkit"
readonly RESOURCE_TYPES=("rules" "commands" "skills" "agents")

usage() {
  cat <<'EOF'
Usage: uninstall.sh

Removes ${HOME}/.cursor/(rules|commands|skills|agents)/ai-toolkit
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

  echo "Removed ${HOME}/.cursor/{rules,commands,skills,agents}/${REPO_NAME}."
}

main "$@"