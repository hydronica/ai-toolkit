#!/usr/bin/env bash
# Rule install manifest (sourced by scripts/install-rules.sh)
#
# org:     ORG_RULES — always installed when non-empty (optional)
# stacks:  STACK_NAMES plus STACK_<name>_DETECT and STACK_<name>_RULES per stack

ORG_RULES=(
)

STACK_NAMES=(go)

STACK_go_DETECT=(go.mod go.work)
STACK_go_RULES=(
  go-standards.mdc
  go-testing.mdc
  go-project-structure.md
)
