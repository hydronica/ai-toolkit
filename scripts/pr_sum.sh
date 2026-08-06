#!/usr/bin/env bash
#
# Summarize the current branch against an integration base for PR prep.
# Safe to run from any directory inside a Git work tree (uses repo root).
#
# Usage: scripts/pr_sum.sh [--base <ref>] [--no-fetch] [-h|--help]
# Env:   PR_BASE — default base ref if --base is not passed
#
# Base resolution (after optional fetch): --base, then PR_BASE, then
# upstream/HEAD|main|master when an upstream remote exists, else the same on origin.

set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/pr_sum.sh [--base <ref>] [--no-fetch] [-h|--help]

  --base <ref>   Integration base (e.g. origin/main). Overrides PR_BASE.
  --no-fetch     Do not run git fetch before comparing to remote refs.
  -h, --help     Show this help.

Run from anywhere inside a Git repository. Output is human-readable sections
suitable for pasting into a PR description or command follow-up.

When an upstream remote exists, the integration base defaults to upstream (not
origin), so fork workflows with a stale origin/main still compare against the
real merge target.
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

# Standard repo-relative paths GitHub recognizes for pull request templates.
PR_TEMPLATE_CANDIDATES=(
  ".github/pull_request_template.md"
  ".github/PULL_REQUEST_TEMPLATE.md"
  "pull_request_template.md"
  "PULL_REQUEST_TEMPLATE.md"
  "docs/pull_request_template.md"
  "docs/PULL_REQUEST_TEMPLATE.md"
)

PR_TEMPLATE_DIR_CANDIDATES=(
  ".github/PULL_REQUEST_TEMPLATE"
  ".github/pull_request_template"
  "PULL_REQUEST_TEMPLATE"
  "pull_request_template"
  "docs/PULL_REQUEST_TEMPLATE"
  "docs/pull_request_template"
)

# Find a repo-local PR template on disk (file or directory).
find_local_pr_template() {
  local candidate="" dir="" first=""
  for candidate in "${PR_TEMPLATE_CANDIDATES[@]}"; do
    if [[ -f "${candidate}" ]]; then
      echo "${candidate}"
      return 0
    fi
  done
  for dir in "${PR_TEMPLATE_DIR_CANDIDATES[@]}"; do
    if [[ -d "${dir}" ]]; then
      first="$(list_pr_template_dir_files "${dir}" | head -n1 || true)"
      if [[ -n "${first}" ]]; then
        echo "${dir}/"
        return 0
      fi
    fi
  done
  return 1
}

# List markdown templates inside a template directory (relative paths).
list_pr_template_dir_files() {
  local dir="${1%/}"
  local file=""
  while IFS= read -r file; do
    [[ -n "${file}" ]] || continue
    echo "${dir}/${file}"
  done < <(find "${dir}" -maxdepth 1 -type f \( -iname '*.md' -o -iname '*.markdown' \) -exec basename {} \; 2>/dev/null | sort)
}

# Query GitHub for PR templates on the default branch of owner/repo.
# Prints lines: filename<TAB>source (repository|organization)
fetch_remote_pr_templates() {
  local repo_slug="$1"
  local owner="${repo_slug%%/*}"
  local name="${repo_slug#*/}"
  local query='query($owner:String!,$name:String!){repository(owner:$owner,name:$name){pullRequestTemplates{filename}}}'

  if ! command -v gh >/dev/null 2>&1 || [[ "${GH_AUTH_OK}" != "true" ]]; then
    return 1
  fi

  local filename="" source=""
  while IFS= read -r filename; do
    [[ -n "${filename}" ]] || continue
    source="repository"
    if ! gh api "repos/${repo_slug}/contents/${filename}" >/dev/null 2>&1; then
      source="organization"
    fi
    printf '%s\t%s\n' "${filename}" "${source}"
  done < <(gh api graphql -f query="${query}" -f owner="${owner}" -f name="${name}" \
    --jq '.data.repository.pullRequestTemplates[]?.filename' 2>/dev/null || true)
}

# Check org/.github community defaults when the target repo has no template.
fetch_org_pr_template() {
  local org="$1"
  local candidate="" dir_entry="" name=""

  if ! command -v gh >/dev/null 2>&1 || [[ "${GH_AUTH_OK}" != "true" ]]; then
    return 1
  fi

  for candidate in pull_request_template.md PULL_REQUEST_TEMPLATE.md; do
    if gh api "repos/${org}/.github/contents/${candidate}" >/dev/null 2>&1; then
      printf '%s\torganization\n' "${candidate}"
      return 0
    fi
  done

  for candidate in PULL_REQUEST_TEMPLATE pull_request_template; do
    dir_entry="$(gh api "repos/${org}/.github/contents/${candidate}" --jq '.[].name' 2>/dev/null || true)"
    if [[ -n "${dir_entry}" ]]; then
      while IFS= read -r name; do
        [[ -n "${name}" ]] || continue
        case "${name}" in
          *.md|*.markdown)
            printf '%s\torganization\n' "${candidate}/${name}"
            ;;
        esac
      done <<< "${dir_entry}"
      return 0
    fi
  done

  return 1
}

default_pr_template_body() {
  cat <<'EOF'
### Quality check

- [ ] Documentation included
- [ ] Test coverage

### Summary

<fill from commit messages and changed paths>
EOF
}

# Print PR template body from disk or GitHub (stdout). Returns 1 if unavailable.
load_pr_template_body() {
  local body="" org="" org_api_path=""

  if [[ "${PR_TEMPLATE_USE_BUILTIN}" == "true" ]]; then
    default_pr_template_body
    return 0
  fi

  if [[ -f "${PR_TEMPLATE_PATH}" ]]; then
    cat "${PR_TEMPLATE_PATH}"
    return 0
  fi

  if [[ "${GH_AUTH_OK}" != "true" ]] || [[ ! "${PR_TARGET_REPO}" =~ / ]]; then
    echo "Warning: could not load template content for ${PR_TEMPLATE_PATH} (gh auth or target repo required)." >&2
    return 1
  fi

  case "${PR_TEMPLATE_SOURCE}" in
    remote)
      if [[ "${PR_TEMPLATE_SCOPE}" == "organization" ]]; then
        org="${PR_TARGET_REPO%%/*}"
        org_api_path="${PR_TEMPLATE_PATH#.github/}"
        body="$(gh api "repos/${org}/.github/contents/${org_api_path}" --jq -r '.content' 2>/dev/null | base64 -d 2>/dev/null || true)"
      else
        body="$(gh api "repos/${PR_TARGET_REPO}/contents/${PR_TEMPLATE_PATH}" --jq -r '.content' 2>/dev/null | base64 -d 2>/dev/null || true)"
      fi
      ;;
    org-default)
      org="${PR_TARGET_REPO%%/*}"
      body="$(gh api "repos/${org}/.github/contents/${PR_TEMPLATE_PATH}" --jq -r '.content' 2>/dev/null | base64 -d 2>/dev/null || true)"
      ;;
    *)
      echo "Warning: could not load template content for ${PR_TEMPLATE_PATH}." >&2
      return 1
      ;;
  esac

  if [[ -n "${body}" ]]; then
    printf '%s\n' "${body}"
    return 0
  fi

  echo "Warning: could not load template content for ${PR_TEMPLATE_PATH}." >&2
  return 1
}

# Populate PR_TEMPLATE_* globals for the output section.
detect_pr_template() {
  PR_TEMPLATE_SOURCE=""
  PR_TEMPLATE_PATH=""
  PR_TEMPLATE_SCOPE=""
  PR_TEMPLATE_EXTRA=""
  PR_TEMPLATE_GH_FLAG=""
  PR_TEMPLATE_USE_BUILTIN="true"

  local local_path="" remote_line="" filename="" source="" org="" dir_file="" org_api_path=""
  local template_files=()

  if local_path="$(find_local_pr_template)"; then
    if [[ "${local_path}" == */ ]]; then
      template_files=()
      while IFS= read -r dir_file; do
        [[ -n "${dir_file}" ]] || continue
        template_files+=("${dir_file}")
      done < <(list_pr_template_dir_files "${local_path%/}")
      if ((${#template_files[@]} == 0)); then
        local_path=""
      else
        PR_TEMPLATE_SOURCE="local"
        PR_TEMPLATE_SCOPE="repository"
        PR_TEMPLATE_PATH="${template_files[0]}"
        PR_TEMPLATE_USE_BUILTIN="false"
        if ((${#template_files[@]} > 1)); then
          PR_TEMPLATE_EXTRA="Additional templates: $(IFS=', '; echo "${template_files[*]}")"
        fi
        PR_TEMPLATE_GH_FLAG="--template ${PR_TEMPLATE_PATH}"
        return 0
      fi
    else
      PR_TEMPLATE_SOURCE="local"
      PR_TEMPLATE_SCOPE="repository"
      PR_TEMPLATE_PATH="${local_path}"
      PR_TEMPLATE_USE_BUILTIN="false"
      PR_TEMPLATE_GH_FLAG="--template ${PR_TEMPLATE_PATH}"
      return 0
    fi
  fi

  if [[ "${PR_TARGET_REPO}" =~ / ]]; then
    remote_line="$(fetch_remote_pr_templates "${PR_TARGET_REPO}" | head -n1 || true)"
    if [[ -n "${remote_line}" ]]; then
      PR_TEMPLATE_SOURCE="remote"
      PR_TEMPLATE_PATH="${remote_line%%$'\t'*}"
      PR_TEMPLATE_SCOPE="${remote_line#*$'\t'}"
      PR_TEMPLATE_USE_BUILTIN="false"
      if [[ -f "${PR_TEMPLATE_PATH}" ]]; then
        PR_TEMPLATE_GH_FLAG="--template ${PR_TEMPLATE_PATH}"
      elif [[ "${PR_TEMPLATE_SCOPE}" == "organization" ]]; then
        PR_TEMPLATE_GH_FLAG="--template /tmp/pr-template.md"
      else
        PR_TEMPLATE_GH_FLAG="--template ${PR_TEMPLATE_PATH}"
      fi
      return 0
    fi

    org="${PR_TARGET_REPO%%/*}"
    remote_line="$(fetch_org_pr_template "${org}" | head -n1 || true)"
    if [[ -n "${remote_line}" ]]; then
      PR_TEMPLATE_SOURCE="org-default"
      PR_TEMPLATE_PATH="${remote_line%%$'\t'*}"
      PR_TEMPLATE_SCOPE="${remote_line#*$'\t'}"
      PR_TEMPLATE_USE_BUILTIN="false"
      PR_TEMPLATE_GH_FLAG="--template /tmp/pr-template.md"
      return 0
    fi
  fi

  PR_TEMPLATE_GH_FLAG="--body-file <filled-template-file>"
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

# Prefer upstream for integration base when present (fork workflow: origin/main is often stale).
INTEGRATION_REMOTE=""
if git remote get-url upstream >/dev/null 2>&1; then
  INTEGRATION_REMOTE="upstream"
elif [[ -n "${DEFAULT_REMOTE}" ]]; then
  INTEGRATION_REMOTE="${DEFAULT_REMOTE}"
fi

if [[ "${DO_FETCH}" == "true" ]]; then
  if [[ -n "${DEFAULT_REMOTE}" ]]; then
    try_fetch "${DEFAULT_REMOTE}"
  else
    echo "Warning: no 'origin' remote; skipping origin fetch." >&2
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

if [[ "${INTEGRATION_REMOTE}" == "upstream" && -n "${DEFAULT_REMOTE}" ]]; then
  for stale in "origin/main" "origin/master"; do
    if git rev-parse --verify "${stale}^{commit}" >/dev/null 2>&1; then
      if [[ "${stale}" != "${BASE}" ]]; then
        behind="$(git rev-list --count "${stale}..${BASE}" 2>/dev/null || echo 0)"
        if [[ "${behind}" -gt 0 ]]; then
          echo "Note: ${stale} is ${behind} commit(s) behind ${BASE}; using ${BASE} as integration base."
        fi
      fi
      break
    fi
  done
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
detect_pr_template

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

section "PR template"
if [[ "${PR_TEMPLATE_USE_BUILTIN}" == "false" ]]; then
  case "${PR_TEMPLATE_SOURCE}" in
    local)
      echo "✓ Repository template: ${PR_TEMPLATE_PATH}"
      ;;
    remote)
      echo "✓ Template on default branch: ${PR_TEMPLATE_PATH} (${PR_TEMPLATE_SCOPE})"
      if [[ ! -f "${PR_TEMPLATE_PATH}" ]]; then
        echo "  (fetched from GitHub below)"
      fi
      ;;
    org-default)
      echo "✓ Organization default template: ${PR_TARGET_REPO%%/*}/.github/${PR_TEMPLATE_PATH}"
      echo "  (fetched from GitHub below)"
      ;;
  esac
  if [[ -n "${PR_TEMPLATE_EXTRA}" ]]; then
    echo "${PR_TEMPLATE_EXTRA}"
  fi
  echo "Use with: gh pr create --title \"<title>\" ${PR_TEMPLATE_GH_FLAG}"
else
  echo "○ No repository or organization PR template found"
  echo "Use the default ai-toolkit pull_request template below."
fi
echo ""
load_pr_template_body || true
