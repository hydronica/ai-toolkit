#!/usr/bin/env bash
#
# Collect context for a local PR-style code review.
# Safe to run from any directory inside a Git work tree (uses repo root).
#
# Usage: scripts/review_sum.sh [--base <ref>] [--scope branch|uncommitted] [--no-fetch] [-h|--help]
# Env:   PR_BASE — default base ref if --base is not passed

set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/review_sum.sh [--base <ref>] [--scope branch|uncommitted] [--no-fetch] [-h|--help]

  --base <ref>          Integration base (e.g. origin/main). Overrides PR_BASE.
  --scope branch        Review branch changes vs merge-base plus working tree (default).
  --scope uncommitted   Review only staged, unstaged, and untracked changes.
  --no-fetch            Do not run git fetch before comparing to remote refs.
  -h, --help            Show this help.

Run from anywhere inside a Git repository. Output is human-readable sections
for the pr-ready skill or /ai-toolkit/pr_ready command.
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

section() {
  echo ""
  echo "=== $* ==="
}

try_fetch() {
  local remote="$1"
  [[ "${DO_FETCH}" == "true" ]] || return 0
  [[ -n "${remote}" ]] || return 0
  git remote get-url "${remote}" >/dev/null 2>&1 || return 0
  section "Fetch (${remote})"
  if git fetch --prune "${remote}" >/dev/null 2>&1; then
    echo "✓ Fetched ${remote} successfully"
  else
    echo "Warning: git fetch ${remote} failed. Continuing with local refs." >&2
  fi
}

parse_base_remote_branch() {
  BASE_REMOTE=""
  BASE_BRANCH=""
  local sym rest
  sym="$(git rev-parse --verify --symbolic-full-name "${BASE}" 2>/dev/null || true)"
  case "${sym}" in
    refs/remotes/*)
      rest="${sym#refs/remotes/}"
      BASE_REMOTE="${rest%%/*}"
      BASE_BRANCH="${rest#*/}"
      ;;
    refs/heads/*)
      BASE_BRANCH="${sym#refs/heads/}"
      ;;
  esac
}

# Convert a simple glob (with **) to an extended regex for matching paths.
# Self-check (bash [[ path =~ regex ]]):
#   **/*.go       matches main.go and pkg/main.go
#   **/*_test.go  matches foo_test.go and x/foo_test.go
glob_to_regex() {
  local glob="$1"
  local regex="" ch
  regex="^"
  while ((${#glob} > 0)); do
    ch="${glob:0:1}"
    glob="${glob:1}"
    case "${ch}" in
      '*')
        if [[ "${glob:0:1}" == '*' && "${glob:1:1}" == '/' ]]; then
          regex+="(.*/)?"
          glob="${glob:2}"
        elif [[ "${glob:0:1}" == '*' ]]; then
          regex+=".*"
          glob="${glob:1}"
        else
          regex+="[^/]*"
        fi
        ;;
      '?') regex+="." ;;
      '.') regex+="\\." ;;
      '/') regex+="/" ;;
      '['|']'|'('|')'|'+'|'^'|'$'|'|'|'\\')
        regex+="\\${ch}"
        ;;
      *) regex+="${ch}" ;;
    esac
  done
  regex+="$"
  printf '%s' "${regex}"
}

path_matches_glob() {
  local path="$1"
  local glob="$2"
  local regex
  regex="$(glob_to_regex "${glob}")"
  [[ "${path}" =~ ${regex} ]]
}

# Read YAML frontmatter value (single-line scalars only).
frontmatter_value() {
  local file="$1"
  local key="$2"
  awk -v key="${key}" '
    BEGIN { in_fm = 0; closed = 0 }
    NR == 1 && $0 == "---" { in_fm = 1; next }
    in_fm && $0 == "---" { closed = 1; exit }
    in_fm && $0 ~ "^" key ":" {
      sub("^" key ":[[:space:]]*", "")
      gsub(/^["'\'']|["'\'']$/, "")
      print
      exit
    }
  ' "${file}"
}

array_contains() {
  local needle="$1"
  shift
  (($# == 0)) && return 1
  local item
  for item in "$@"; do
    [[ "${item}" == "${needle}" ]] && return 0
  done
  return 1
}

collect_changed_files() {
  CHANGED_FILES=()
  local f

  add_file() {
    local path="$1"
    [[ -n "${path}" ]] || return 0
    if ((${#CHANGED_FILES[@]} > 0)) && array_contains "${path}" "${CHANGED_FILES[@]}"; then
      return 0
    fi
    CHANGED_FILES+=("${path}")
  }

  if [[ "${REVIEW_SCOPE}" == "uncommitted" ]]; then
    while IFS= read -r f; do add_file "${f}"; done < <(git diff --name-only 2>/dev/null || true)
    while IFS= read -r f; do add_file "${f}"; done < <(git diff --cached --name-only 2>/dev/null || true)
  else
    while IFS= read -r f; do add_file "${f}"; done < <(git diff --name-only "${BASE}...HEAD" 2>/dev/null || true)
    while IFS= read -r f; do add_file "${f}"; done < <(git diff --name-only 2>/dev/null || true)
    while IFS= read -r f; do add_file "${f}"; done < <(git diff --cached --name-only 2>/dev/null || true)
  fi
  while IFS= read -r f; do add_file "${f}"; done < <(git ls-files --others --exclude-standard 2>/dev/null || true)
}

discover_policy_docs() {
  POLICY_DOCS=()
  local dir path rel

  add_policy() {
    local doc="$1"
    [[ -f "${doc}" ]] || return 0
    if ((${#POLICY_DOCS[@]} > 0)) && array_contains "${doc}" "${POLICY_DOCS[@]}"; then
      return 0
    fi
    POLICY_DOCS+=("${doc}")
  }

  add_policy "${TOPLEVEL}/CONTRIBUTING.md"

  if ((${#CHANGED_FILES[@]} == 0)); then
    add_policy "${TOPLEVEL}/AGENTS.md"
    return 0
  fi

  for path in "${CHANGED_FILES[@]}"; do
    dir="${TOPLEVEL}"
    if [[ "${path}" == */* ]]; then
      dir="${TOPLEVEL}/$(dirname "${path}")"
    fi
    while [[ "${dir}" == "${TOPLEVEL}"* ]]; do
      add_policy "${dir}/AGENTS.md"
      [[ "${dir}" == "${TOPLEVEL}" ]] && break
      dir="$(dirname "${dir}")"
    done
  done
}

discover_applicable_rules() {
  RULE_FILES=()
  ALWAYS_APPLY_RULES=()
  GLOB_MATCHED_RULES=()
  DESCRIPTION_RULES=()
  local rules_root="${TOPLEVEL}/.cursor/rules"
  local rules_source="${HOME}/.cursor/ai-toolkit/rules-source"
  local rule path glob always_apply description matched entry
  local has_go_changes="false" has_go_test_changes="false"

  add_rule() {
    local r="$1"
    [[ -f "${r}" ]] || return 0
    if ((${#RULE_FILES[@]} > 0)) && array_contains "${r}" "${RULE_FILES[@]}"; then
      return 0
    fi
    RULE_FILES+=("${r}")
  }

  for path in "${CHANGED_FILES[@]}"; do
    case "${path}" in
      *_test.go)
        has_go_test_changes="true"
        has_go_changes="true"
        ;;
      *.go) has_go_changes="true" ;;
      *go-standards.mdc|*go-testing.mdc|*go-project-structure.md)
        has_go_changes="true"
        if [[ "${path}" == *go-testing* ]]; then
          has_go_test_changes="true"
        fi
        ;;
    esac
    case "${path}" in
      *.mdc|rules/*.md|.cursor/rules/*)
        if [[ "${path}" == *.mdc ]]; then
          add_rule "${TOPLEVEL}/${path}"
        elif [[ "${path}" == rules/*.md ]]; then
          add_rule "${TOPLEVEL}/${path}"
        fi
        ;;
    esac
  done

  if [[ -d "${rules_root}" ]]; then
    while IFS= read -r rule; do
      [[ "${rule}" == *.mdc ]] || continue
      always_apply="$(frontmatter_value "${rule}" "alwaysApply")"
      if [[ "${always_apply}" == "true" ]]; then
        add_rule "${rule}"
        if ! ((${#ALWAYS_APPLY_RULES[@]} > 0)) || ! array_contains "${rule}" "${ALWAYS_APPLY_RULES[@]}"; then
          ALWAYS_APPLY_RULES+=("${rule}")
        fi
      fi

      glob="$(frontmatter_value "${rule}" "globs")"
      if [[ -n "${glob}" ]]; then
        for path in "${CHANGED_FILES[@]}"; do
          if path_matches_glob "${path}" "${glob}"; then
            add_rule "${rule}"
            entry="${rule}::${glob}"
            if ! ((${#GLOB_MATCHED_RULES[@]} > 0)) || ! array_contains "${entry}" "${GLOB_MATCHED_RULES[@]}"; then
              GLOB_MATCHED_RULES+=("${entry}")
            fi
            break
          fi
        done
      fi

      description="$(frontmatter_value "${rule}" "description")"
      if [[ -n "${description}" && "${always_apply}" != "true" && -z "${glob}" ]]; then
        if ! ((${#DESCRIPTION_RULES[@]} > 0)) || ! array_contains "${rule}" "${DESCRIPTION_RULES[@]}"; then
          DESCRIPTION_RULES+=("${rule}")
        fi
      fi
    done < <(find "${rules_root}" -type f -name '*.mdc' 2>/dev/null | sort)
  fi

  # Explicit Go stack hints when project rules are missing or globs did not match.
  if [[ "${has_go_changes}" == "true" || "${has_go_test_changes}" == "true" ]]; then
    rule_basename_present() {
      local base="$1"
      local existing
      ((${#RULE_FILES[@]} == 0)) && return 1
      for existing in "${RULE_FILES[@]}"; do
        [[ "$(basename "${existing}")" == "${base}" ]] && return 0
      done
      return 1
    }
    for candidate in \
      "${rules_root}/ai-toolkit/go-standards.mdc" \
      "${rules_root}/ai-toolkit/go-testing.mdc" \
      "${rules_source}/go-standards.mdc" \
      "${rules_source}/go-testing.mdc"; do
      if [[ -f "${candidate}" ]]; then
        if [[ "${candidate}" == *go-standards* && "${has_go_changes}" == "true" ]] \
          && ! rule_basename_present "go-standards.mdc"; then
          add_rule "${candidate}"
        fi
        if [[ "${candidate}" == *go-testing* && "${has_go_test_changes}" == "true" ]] \
          && ! rule_basename_present "go-testing.mdc"; then
          add_rule "${candidate}"
        fi
      fi
    done
    for companion in \
      "${rules_root}/ai-toolkit/go-project-structure.md" \
      "${rules_source}/go-project-structure.md"; do
      if [[ -f "${companion}" && "${has_go_changes}" == "true" ]] \
        && ! rule_basename_present "go-project-structure.md"; then
        add_rule "${companion}"
      fi
    done
  fi

  RULES_ROOT="${rules_root}"
  RULES_SOURCE="${rules_source}"
  HAS_PROJECT_RULES="false"
  if [[ -d "${rules_root}" ]]; then
    if find "${rules_root}" -type f -name '*.mdc' 2>/dev/null | head -n1 | grep -q .; then
      HAS_PROJECT_RULES="true"
    fi
  fi
}

print_review_checklist_seed() {
  cat <<'EOF'
# PR Ready

## Summary
<fill: 1-3 sentences on what changed and overall readiness>

## Compliance
| Check | Status | Notes |
|-------|--------|-------|
| CONTRIBUTING.md | pass/fail/n-a | |
| AGENTS.md | pass/fail/n-a | |
| Rules applied | pass/fail/n-a | |

## Findings
| Severity | Location | Source | Finding |
|----------|----------|--------|---------|
| | | | |

## Test plan
<fill: commands or manual checks>

## Verdict
<Approve / Approve with nits / Request changes — include blocker count>
EOF
}

BASE_OVERRIDE=""
DO_FETCH="true"
REVIEW_SCOPE="branch"
while (($# > 0)); do
  case "$1" in
    --base)
      (($# >= 2)) || die "--base requires a value"
      BASE_OVERRIDE="$2"
      shift 2
      ;;
    --scope)
      (($# >= 2)) || die "--scope requires a value"
      REVIEW_SCOPE="$2"
      shift 2
      ;;
    --no-fetch)
      DO_FETCH="false"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "Unknown option: $1 (use --help)"
      ;;
  esac
done

case "${REVIEW_SCOPE}" in
  branch|uncommitted) ;;
  *) die "Invalid --scope: ${REVIEW_SCOPE} (use branch or uncommitted)" ;;
esac

require_command git

TOPLEVEL="$(git rev-parse --show-toplevel 2>/dev/null)" || die "Not inside a Git repository"
cd "${TOPLEVEL}"

export GIT_PAGER=cat

if [[ -f .git/shallow ]]; then
  echo "Warning: shallow clone (.git/shallow). Merge-base may be incomplete until history is deep enough." >&2
fi

DEFAULT_REMOTE=""
if git remote get-url origin >/dev/null 2>&1; then
  DEFAULT_REMOTE="origin"
fi

INTEGRATION_REMOTE=""
if git remote get-url upstream >/dev/null 2>&1; then
  INTEGRATION_REMOTE="upstream"
elif [[ -n "${DEFAULT_REMOTE}" ]]; then
  INTEGRATION_REMOTE="${DEFAULT_REMOTE}"
fi

if [[ "${DO_FETCH}" == "true" ]]; then
  if [[ -n "${DEFAULT_REMOTE}" ]]; then
    try_fetch "${DEFAULT_REMOTE}"
  fi
  if [[ -n "${INTEGRATION_REMOTE}" && "${INTEGRATION_REMOTE}" != "${DEFAULT_REMOTE}" ]]; then
    try_fetch "${INTEGRATION_REMOTE}"
  fi
fi

resolve_base_ref() {
  local candidate="" sym="" remote=""
  if [[ -n "${BASE_OVERRIDE}" ]]; then
    git rev-parse --verify "${BASE_OVERRIDE}^{commit}" >/dev/null 2>&1 || die "Base ref not found: ${BASE_OVERRIDE}"
    echo "${BASE_OVERRIDE}"
    return 0
  fi
  if [[ -n "${PR_BASE:-}" ]]; then
    git rev-parse --verify "${PR_BASE}^{commit}" >/dev/null 2>&1 || die "PR_BASE not found: ${PR_BASE}"
    echo "${PR_BASE}"
    return 0
  fi
  remote="${INTEGRATION_REMOTE}"
  [[ -n "${remote}" ]] || die "Could not resolve base ref. Set PR_BASE, pass --base, or add an origin/upstream remote."
  sym="$(git symbolic-ref -q "refs/remotes/${remote}/HEAD" 2>/dev/null || true)"
  if [[ -n "${sym}" ]]; then
    candidate="${sym#refs/remotes/}"
    if git rev-parse --verify "${candidate}^{commit}" >/dev/null 2>&1; then
      echo "${candidate}"
      return 0
    fi
  fi
  for candidate in "${remote}/main" "${remote}/master"; do
    if git rev-parse --verify "${candidate}^{commit}" >/dev/null 2>&1; then
      echo "${candidate}"
      return 0
    fi
  done
  die "Could not resolve base ref. Set PR_BASE, pass --base, or ensure ${remote}/main (or ${remote}/master / ${remote}/HEAD) exists after fetch."
}

BASE="$(resolve_base_ref)"
BASE_SHA="$(git rev-parse "${BASE}^{commit}")"
HEAD_SHA="$(git rev-parse HEAD)"
CURRENT_BRANCH="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo 'HEAD')"
MB="$(git merge-base HEAD "${BASE}" 2>/dev/null || true)"

parse_base_remote_branch
if [[ -n "${BASE_REMOTE}" && "${BASE_REMOTE}" != "${DEFAULT_REMOTE}" ]]; then
  try_fetch "${BASE_REMOTE}"
fi

collect_changed_files
discover_policy_docs
discover_applicable_rules

section "Repository"
printf "Repository: %s\n" "${TOPLEVEL}"
printf "Branch:     %s\n" "${CURRENT_BRANCH}"
printf "HEAD:       %s\n" "${HEAD_SHA}"
printf "Base:       %s (%s)\n" "${BASE}" "${BASE_SHA}"
if [[ -n "${MB}" ]]; then
  printf "Merge-base: %s\n" "${MB}"
fi

section "Scope"
echo "Review scope: ${REVIEW_SCOPE}"
if [[ "${REVIEW_SCOPE}" == "branch" ]]; then
  echo "Includes committed changes (${BASE}...HEAD) plus staged, unstaged, and untracked working tree changes."
else
  echo "Includes staged, unstaged, and untracked working tree changes only."
fi
echo "Changed file count: ${#CHANGED_FILES[@]}"

if [[ "${REVIEW_SCOPE}" == "branch" ]]; then
  section "Commits (${BASE}..HEAD)"
  commit_count="$(git rev-list --count "${BASE}..HEAD" 2>/dev/null || echo 0)"
  if [[ "${commit_count}" -gt 0 ]]; then
    git log --oneline --no-decorate "${BASE}..HEAD"
  else
    echo "(no commits in range ${BASE}..HEAD)"
  fi
fi

section "Files changed"
if ((${#CHANGED_FILES[@]} == 0)); then
  echo "No changed files for scope '${REVIEW_SCOPE}'."
else
  if [[ "${REVIEW_SCOPE}" == "branch" ]]; then
    SHORTSTAT="$(git diff --shortstat "${BASE}...HEAD" 2>/dev/null || true)"
    if [[ -n "${SHORTSTAT}" ]]; then
      echo "Committed range (${BASE}...HEAD): ${SHORTSTAT}"
      echo ""
    fi
  fi
  WORK_SHORTSTAT="$(git diff --shortstat 2>/dev/null || true)"
  CACHED_SHORTSTAT="$(git diff --cached --shortstat 2>/dev/null || true)"
  UNTRACKED_COUNT="$(git ls-files --others --exclude-standard 2>/dev/null | wc -l | tr -d ' ')"
  [[ -n "${WORK_SHORTSTAT}" ]] && echo "Unstaged: ${WORK_SHORTSTAT}"
  [[ -n "${CACHED_SHORTSTAT}" ]] && echo "Staged: ${CACHED_SHORTSTAT}"
  [[ "${UNTRACKED_COUNT}" -gt 0 ]] && echo "Untracked: ${UNTRACKED_COUNT} file(s)"
  echo ""
  echo "paths:"
  for path in "${CHANGED_FILES[@]}"; do
    echo "  ${path}"
  done
fi

section "Policy documents"
if [[ -f "${TOPLEVEL}/CONTRIBUTING.md" ]]; then
  echo "✓ CONTRIBUTING.md"
else
  echo "○ CONTRIBUTING.md (not found at repo root)"
fi

agents_found="false"
for doc in "${POLICY_DOCS[@]}"; do
  case "${doc}" in
    */AGENTS.md)
      agents_found="true"
      rel="${doc#${TOPLEVEL}/}"
      echo "✓ ${rel}"
      ;;
  esac
done
if [[ "${agents_found}" == "false" ]]; then
  echo "○ AGENTS.md (not found at repo root or along changed paths)"
fi

section "Applicable rules"
if [[ "${HAS_PROJECT_RULES}" == "true" ]]; then
  echo "Project rules root: ${RULES_ROOT}"
else
  echo "○ No project .mdc rules found under .cursor/rules/"
  if [[ -d "${RULES_SOURCE}" ]]; then
    echo "  Fallback source: ${RULES_SOURCE}"
    echo "  Suggestion: run install_rules to install project rules."
  else
    echo "  Fallback source not found at ${RULES_SOURCE}"
    echo "  Suggestion: run install.sh from ai-toolkit, then install_rules in this project."
  fi
fi

if ((${#ALWAYS_APPLY_RULES[@]} > 0)); then
  echo ""
  echo "alwaysApply:"
  for rule in "${ALWAYS_APPLY_RULES[@]}"; do
    echo "  ${rule#${TOPLEVEL}/}"
  done
fi

if ((${#GLOB_MATCHED_RULES[@]} > 0)); then
  echo ""
  echo "glob-matched:"
  for entry in "${GLOB_MATCHED_RULES[@]}"; do
    rule="${entry%%::*}"
    glob="${entry#*::}"
    echo "  ${rule#${TOPLEVEL}/} (globs: ${glob})"
  done
fi

if ((${#DESCRIPTION_RULES[@]} > 0)); then
  echo ""
  echo "description-only (agent may include when task matches):"
  for rule in "${DESCRIPTION_RULES[@]}"; do
    echo "  ${rule#${TOPLEVEL}/}"
  done
fi

if ((${#RULE_FILES[@]} > 0)); then
  echo ""
  echo "read for review:"
  for rule in "${RULE_FILES[@]}"; do
    if [[ "${rule}" == "${TOPLEVEL}/"* ]]; then
      echo "  ${rule#${TOPLEVEL}/}"
    else
      echo "  ${rule}"
    fi
  done
else
  echo ""
  echo "(no applicable rules matched changed files)"
fi

section "Review checklist seed"
print_review_checklist_seed
