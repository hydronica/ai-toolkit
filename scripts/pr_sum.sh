#!/usr/bin/env bash
#
# Summarize the current branch against an integration base for PR prep.
# Safe to run from any directory inside a Git work tree (uses repo root).
#
# Usage: scripts/pr_sum.sh [--base <ref>] [--no-fetch] [-h|--help]
# Env:   PR_BASE — default base ref if --base is not passed
#
# Base resolution (after optional fetch): --base, then PR_BASE, then
# origin/HEAD (if set), else origin/main, else origin/master.

set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/pr_sum.sh [--base <ref>] [--no-fetch] [-h|--help]

  --base <ref>   Integration base (e.g. origin/main). Overrides PR_BASE.
  --no-fetch     Do not run git fetch before comparing to remote refs.
  -h, --help     Show this help.

Run from anywhere inside a Git repository. Output is human-readable sections
suitable for pasting into a PR description or command follow-up.
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

# Fetch remote if DO_FETCH and remote exists; warn on failure.
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

# Parse BASE into BASE_REMOTE / BASE_BRANCH when it is refs/remotes/r/b or refs/heads/b.
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

# Parse GitHub repo slug from git URL
parse_repo_slug() {
  local url="$1"
  # Handle git@github.com:owner/repo.git or https://github.com/owner/repo
  echo "${url}" | sed -E 's#.*github\.com[:/]([^/]+/[^/]+)\.git$#\1#; s#.*github\.com[:/]([^/]+/[^/]+)$#\1#'
}

# Detect if origin is a fork and find PR target repo
detect_pr_target() {
  local origin_url=""
  origin_url="$(git remote get-url origin 2>/dev/null || true)"
  
  if [[ -z "${origin_url}" ]]; then
    echo "origin" # fallback
    return
  fi
  
  # Extract owner/repo from origin URL
  local repo_slug=""
  repo_slug="$(parse_repo_slug "${origin_url}")"
  
  # Count number of remotes
  local remote_count=""
  remote_count="$(git remote | wc -l | tr -d ' ')"
  
  # If only one remote, treat it as the main repo (no fork workflow)
  if [[ "${remote_count}" -eq 1 ]]; then
    echo "${repo_slug}" # Single remote = direct workflow, PR to same repo
    return
  fi
  
  # Multiple remotes: check if origin is a fork using gh api
  if command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
    local fork_info=""
    fork_info="$(gh api "repos/${repo_slug}" --jq '.fork, .parent.full_name' 2>/dev/null || true)"
    
    if echo "${fork_info}" | head -n1 | grep -q "true"; then
      local parent_repo=""
      parent_repo="$(echo "${fork_info}" | tail -n1)"
      echo "${parent_repo}" # Return parent repo for PR target
      return
    fi
  fi
  
  echo "${repo_slug}" # Not a fork, PR to same repo
}

# Parse args
BASE_OVERRIDE=""
DO_FETCH="true"
while (($# > 0)); do
  case "$1" in
    --base)
      (($# >= 2)) || die "--base requires a value"
      BASE_OVERRIDE="$2"
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

require_command git

TOPLEVEL="$(git rev-parse --show-toplevel 2>/dev/null)" || die "Not inside a Git repository"
cd "${TOPLEVEL}"

# Send all git output to the terminal (avoid less/whatever is in core.pager).
export GIT_PAGER=cat

if [[ -f .git/shallow ]]; then
  echo "Warning: shallow clone (.git/shallow). Merge-base and conflict simulation may be incomplete until history is deep enough (e.g. git fetch --unshallow)." >&2
fi

if [[ -n "$(git status --porcelain 2>/dev/null)" ]]; then
  echo "Warning: working tree has uncommitted changes; commit list and diff stats describe committed history only." >&2
fi

DEFAULT_REMOTE=""
if git remote get-url origin >/dev/null 2>&1; then
  DEFAULT_REMOTE="origin"
fi

if [[ "${DO_FETCH}" == "true" ]]; then
  if [[ -n "${DEFAULT_REMOTE}" ]]; then
    try_fetch "${DEFAULT_REMOTE}"
  else
    echo "Warning: no 'origin' remote; skipping initial fetch." >&2
  fi
fi

resolve_base_ref() {
  local candidate="" sym=""
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
  sym="$(git symbolic-ref -q refs/remotes/origin/HEAD 2>/dev/null || true)"
  if [[ -n "${sym}" ]]; then
    candidate="${sym#refs/remotes/}"
    if git rev-parse --verify "${candidate}^{commit}" >/dev/null 2>&1; then
      echo "${candidate}"
      return 0
    fi
  fi
  for candidate in origin/main origin/master; do
    if git rev-parse --verify "${candidate}^{commit}" >/dev/null 2>&1; then
      echo "${candidate}"
      return 0
    fi
  done
  die "Could not resolve base ref. Set PR_BASE, pass --base, or ensure origin/main (or origin/master / origin/HEAD) exists after fetch."
}

BASE="$(resolve_base_ref)"
BASE_SHA="$(git rev-parse "${BASE}^{commit}")"
HEAD_SHA="$(git rev-parse HEAD)"

parse_base_remote_branch

if [[ -n "${BASE_REMOTE}" && "${BASE_REMOTE}" != "${DEFAULT_REMOTE}" ]]; then
  try_fetch "${BASE_REMOTE}"
fi

section "Context"
printf "Repository: %s\n" "${TOPLEVEL}"
printf "HEAD:       %s (%s)\n" "$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo 'HEAD')" "${HEAD_SHA}"
printf "Base:       %s (%s)\n" "${BASE}" "${BASE_SHA}"

MB="$(git merge-base HEAD "${BASE}" 2>/dev/null || true)"
if [[ -n "${MB}" ]]; then
  printf "Merge-base: %s\n" "${MB}"
fi

# Collect PR readiness status for summary
GH_AUTH_OK="false"
GH_AUTH_ERROR=""
BRANCH_PUSH_OK="false"
MERGE_OK="false"
CURRENT_BRANCH="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo 'HEAD')"

if command -v gh >/dev/null 2>&1; then
  GH_AUTH_OUTPUT="$(gh auth status 2>&1)"
  if echo "${GH_AUTH_OUTPUT}" | grep -q "Logged in"; then
    GH_AUTH_OK="true"
  else
    # Capture error type for better messaging
    if echo "${GH_AUTH_OUTPUT}" | grep -qi "network\|connection\|unreachable\|timeout"; then
      GH_AUTH_ERROR="network"
    elif echo "${GH_AUTH_OUTPUT}" | grep -qi "token.*invalid\|authentication failed"; then
      GH_AUTH_ERROR="auth"
    else
      GH_AUTH_ERROR="unknown"
    fi
  fi
fi

if [[ "${CURRENT_BRANCH}" != "HEAD" ]]; then
  if git ls-remote --heads origin "${CURRENT_BRANCH}" 2>/dev/null | grep -q "${CURRENT_BRANCH}"; then
    local_sha="$(git rev-parse HEAD)"
    remote_sha="$(git rev-parse "origin/${CURRENT_BRANCH}" 2>/dev/null || echo "")"
    if [[ "${local_sha}" == "${remote_sha}" ]]; then
      BRANCH_PUSH_OK="true"
    fi
  fi
fi

MERGE_TREE_OUT=""
if MERGE_TREE_OUT="$(git merge-tree --write-tree "${BASE}" HEAD 2>&1)"; then
  line_count="$(printf '%s' "${MERGE_TREE_OUT}" | awk 'END{print NR}')"
  if [[ "${line_count}" -eq 1 ]] && printf '%s' "${MERGE_TREE_OUT}" | grep -qE '^[0-9a-f]{40}$'; then
    MERGE_OK="true"
  fi
fi

PR_TARGET_REPO="$(detect_pr_target)"

section "PR Readiness"
if [[ "${GH_AUTH_OK}" == "true" ]]; then
  echo "✓ GitHub CLI authenticated"
elif [[ "${GH_AUTH_ERROR}" == "network" ]]; then
  echo "✗ GitHub CLI cannot reach GitHub (network blocked or offline)"
  echo "  Note: If running in Cursor sandbox, use required_permissions: [\"full_network\"]"
elif [[ "${GH_AUTH_ERROR}" == "auth" ]]; then
  echo "✗ GitHub CLI authentication failed (run: gh auth login)"
elif ! command -v gh >/dev/null 2>&1; then
  echo "✗ gh command not found (install from https://cli.github.com/)"
else
  echo "✗ GitHub CLI check failed (run: gh auth status for details)"
fi

if [[ "${BRANCH_PUSH_OK}" == "true" ]]; then
  echo "✓ Branch '${CURRENT_BRANCH}' is pushed and up-to-date"
elif [[ "${CURRENT_BRANCH}" == "HEAD" ]]; then
  echo "✗ Detached HEAD state"
else
  if git ls-remote --heads origin "${CURRENT_BRANCH}" 2>/dev/null | grep -q "${CURRENT_BRANCH}"; then
    echo "✗ Branch '${CURRENT_BRANCH}' has unpushed commits"
  else
    echo "✗ Branch '${CURRENT_BRANCH}' not pushed to origin"
  fi
fi

if [[ "${MERGE_OK}" == "true" ]]; then
  echo "✓ No merge conflicts detected"
else
  echo "⚠ Potential merge conflicts (see Merge simulation section)"
fi

if [[ "${PR_TARGET_REPO}" =~ / ]]; then
  echo "→ Ready to create PR to ${PR_TARGET_REPO}"
else
  echo "⚠ Could not determine PR target repository"
fi

section "Branch push status"

if [[ "${CURRENT_BRANCH}" == "HEAD" ]]; then
  echo "Detached HEAD state - cannot check push status"
else
  # Check if branch exists on origin
  if git ls-remote --heads origin "${CURRENT_BRANCH}" | grep -q "${CURRENT_BRANCH}"; then
    local_sha="$(git rev-parse HEAD)"
    remote_sha="$(git rev-parse "origin/${CURRENT_BRANCH}" 2>/dev/null || echo "")"
    
    if [[ "${local_sha}" == "${remote_sha}" ]]; then
      echo "✓ Branch '${CURRENT_BRANCH}' is pushed and up-to-date with origin/${CURRENT_BRANCH}"
    else
      echo "✗ Branch '${CURRENT_BRANCH}' exists on origin but local differs from remote"
      unpushed="$(git rev-list --count "origin/${CURRENT_BRANCH}..HEAD" 2>/dev/null || echo "?")"
      echo "Unpushed commits: ${unpushed}"
      echo "Run: git push origin ${CURRENT_BRANCH}"
    fi
  else
    echo "✗ Branch '${CURRENT_BRANCH}' not found on origin"
    echo "Run: git push -u origin ${CURRENT_BRANCH}"
  fi
fi

section "Commits (${BASE}..HEAD)"
if ! git merge-base --is-ancestor "${BASE}" HEAD 2>/dev/null; then
  if ! git merge-base --is-ancestor HEAD "${BASE}" 2>/dev/null; then
    echo "Warning: ${BASE} and HEAD diverged."
  fi
fi
commit_count="$(git rev-list --count "${BASE}..HEAD" 2>/dev/null || echo 0)"
if [[ "${commit_count}" -gt 0 ]]; then
  git log --oneline --no-decorate "${BASE}..HEAD"
else
  echo "(no commits in range ${BASE}..HEAD)"
fi

section "Merge simulation"
if [[ "${MERGE_OK}" == "true" ]]; then
  echo "No conflicts."
elif [[ -n "${MERGE_TREE_OUT}" ]]; then
  echo "Potential conflicts — rebase likely."
  printf '%s\n' "${MERGE_TREE_OUT}"
else
  echo "Merge check skipped (git merge-tree --write-tree failed; need Git 2.38+ and full objects)."
fi

section "Files changed (three-dot vs base)"
SHORTSTAT="$(git diff --shortstat "${BASE}...HEAD" 2>/dev/null || true)"
if [[ -z "${SHORTSTAT}" ]]; then
  echo "No diff (no changes vs merge-base range)."
else
  echo "${SHORTSTAT}"
  echo ""
  echo "added   removed  path"
  git diff --numstat "${BASE}...HEAD" || true
  echo ""
  echo "(Note: '-\t-' indicates binary files)"
fi

section "Pull request target"
if [[ "${PR_TARGET_REPO}" =~ / ]]; then
  # Contains slash, likely owner/repo format
  echo "Target repository: ${PR_TARGET_REPO}"
  
  # Check if this looks like a fork workflow
  origin_slug="$(parse_repo_slug "$(git remote get-url origin 2>/dev/null || echo "")")"
  if [[ "${PR_TARGET_REPO}" != "${origin_slug}" ]]; then
    echo "Fork workflow detected:"
    echo "  Push to: ${origin_slug}:${CURRENT_BRANCH}"
    echo "  PR to: ${PR_TARGET_REPO}"
  else
    echo "Direct workflow: PR within ${PR_TARGET_REPO}"
  fi
else
  echo "Could not determine PR target (gh cli may not be authenticated)"
fi
