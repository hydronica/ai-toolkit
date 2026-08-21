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

Branch push status checks every remote (not only origin) and reports whether
HEAD matches the remote branch. PR target is derived from the push remote.
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

# First configured remote name, if any.
first_remote() {
  local remote=""
  while IFS= read -r remote; do
    [[ -n "${remote}" ]] || continue
    echo "${remote}"
    return 0
  done < <(git remote)
  return 1
}

# GitHub owner/repo slug for a remote, or empty.
remote_repo_slug() {
  local url=""
  url="$(git remote get-url "$1" 2>/dev/null || true)"
  [[ -n "${url}" ]] || return 1
  parse_repo_slug "${url}"
}

# SHA of refs/heads/<branch> on <remote> (exact match), or empty.
remote_branch_sha() {
  local remote="$1"
  local branch="$2"
  local line=""
  line="$(git ls-remote --heads "${remote}" "refs/heads/${branch}" 2>/dev/null || true)"
  printf '%s\n' "${line}" | awk -v ref="refs/heads/${branch}" '$2 == ref { print $1; exit }'
}

# Configured git push remote for a local branch (pushRemote, pushDefault, @{push}, or upstream).
configured_push_remote() {
  local branch="$1"
  local remote="" push_ref="" rest=""
  remote="$(git config --get "branch.${branch}.pushRemote" 2>/dev/null || true)"
  if [[ -n "${remote}" ]]; then
    echo "${remote}"
    return 0
  fi
  remote="$(git config --get remote.pushDefault 2>/dev/null || true)"
  if [[ -n "${remote}" ]]; then
    echo "${remote}"
    return 0
  fi
  push_ref="$(git rev-parse --symbolic-full-name '@{push}' 2>/dev/null || true)"
  case "${push_ref}" in
    refs/remotes/*)
      rest="${push_ref#refs/remotes/}"
      echo "${rest%%/*}"
      return 0
      ;;
  esac
  remote="$(git config --get "branch.${branch}.remote" 2>/dev/null || true)"
  if [[ -n "${remote}" ]]; then
    echo "${remote}"
    return 0
  fi
  return 1
}

# Compare HEAD SHA to a remote branch SHA: up-to-date|ahead|behind|diverged|differ|missing.
compare_head_to_remote_sha() {
  local remote_sha="$1"
  local head_sha="$2"
  if [[ -z "${remote_sha}" ]]; then
    echo "missing"
    return 0
  fi
  if [[ "${remote_sha}" == "${head_sha}" ]]; then
    echo "up-to-date"
    return 0
  fi
  if git cat-file -e "${remote_sha}^{commit}" 2>/dev/null; then
    if git merge-base --is-ancestor "${remote_sha}" "${head_sha}" 2>/dev/null; then
      echo "ahead"
      return 0
    fi
    if git merge-base --is-ancestor "${head_sha}" "${remote_sha}" 2>/dev/null; then
      echo "behind"
      return 0
    fi
    echo "diverged"
    return 0
  fi
  echo "differ"
}

# SHA for a remote recorded in PUSH_MATCHES (remote<TAB>sha lines).
sha_from_push_matches() {
  local want="$1"
  local remote="" sha=""
  [[ -n "${PUSH_MATCHES}" ]] || return 1
  while IFS=$'\t' read -r remote sha; do
    if [[ "${remote}" == "${want}" ]]; then
      echo "${sha}"
      return 0
    fi
  done <<< "${PUSH_MATCHES}"
  return 1
}

# Append a unique remote name to PUSH_PICK_ORDER.
append_push_pick_order() {
  local r="$1"
  [[ -n "${r}" ]] || return 0
  : "${PUSH_PICK_ORDER:=}"
  case $'\n'"${PUSH_PICK_ORDER}"$'\n' in
    *$'\n'"${r}"$'\n'*) return 0 ;;
  esac
  PUSH_PICK_ORDER+="${r}"$'\n'
}

# Pick a remote from PUSH_MATCHES. Mode: uptodate (HEAD match) or any.
pick_push_remote_from_matches() {
  local mode="$1"
  local configured="$2"
  local head_sha="$3"
  local candidate="" sha=""
  PUSH_PICK_ORDER=""

  append_push_pick_order "${configured}"
  append_push_pick_order "origin"
  while IFS= read -r candidate; do
    append_push_pick_order "${candidate}"
  done < <(git remote)

  while IFS= read -r candidate; do
    [[ -n "${candidate}" ]] || continue
    sha="$(sha_from_push_matches "${candidate}" || true)"
    [[ -n "${sha}" ]] || continue
    if [[ "${mode}" == "uptodate" && "${sha}" != "${head_sha}" ]]; then
      continue
    fi
    echo "${candidate}"
    return 0
  done <<< "${PUSH_PICK_ORDER}"
  return 1
}

# Populate PUSH_* globals: which remotes have CURRENT_BRANCH and whether HEAD is pushed.
resolve_branch_push() {
  local branch="$1"
  local head_sha="$2"
  local remote="" sha="" configured=""

  PUSH_REMOTE=""
  PUSH_REMOTE_SHA=""
  PUSH_STATUS="missing"
  PUSH_AHEAD=""
  PUSH_BEHIND=""
  PUSH_MATCHES=""
  PUSH_HINT_REMOTE=""
  BRANCH_PUSH_OK="false"

  if [[ "${branch}" == "HEAD" || -z "${branch}" ]]; then
    PUSH_STATUS="detached"
    return 0
  fi

  configured="$(configured_push_remote "${branch}" || true)"
  if [[ -n "${configured}" ]]; then
    PUSH_HINT_REMOTE="${configured}"
  elif git remote get-url origin >/dev/null 2>&1; then
    PUSH_HINT_REMOTE="origin"
  else
    PUSH_HINT_REMOTE="$(first_remote || true)"
  fi

  while IFS= read -r remote; do
    [[ -n "${remote}" ]] || continue
    sha="$(remote_branch_sha "${remote}" "${branch}")"
    if [[ -n "${sha}" ]]; then
      PUSH_MATCHES+="${remote}"$'\t'"${sha}"$'\n'
    fi
  done < <(git remote)

  PUSH_REMOTE="$(pick_push_remote_from_matches "uptodate" "${configured}" "${head_sha}" || true)"
  if [[ -z "${PUSH_REMOTE}" ]]; then
    PUSH_REMOTE="$(pick_push_remote_from_matches "any" "${configured}" "${head_sha}" || true)"
  fi

  if [[ -z "${PUSH_REMOTE}" ]]; then
    PUSH_STATUS="missing"
    return 0
  fi

  PUSH_REMOTE_SHA="$(sha_from_push_matches "${PUSH_REMOTE}" || true)"
  PUSH_STATUS="$(compare_head_to_remote_sha "${PUSH_REMOTE_SHA}" "${head_sha}")"
  case "${PUSH_STATUS}" in
    up-to-date)
      BRANCH_PUSH_OK="true"
      ;;
    ahead)
      PUSH_AHEAD="$(git rev-list --count "${PUSH_REMOTE_SHA}..${head_sha}" 2>/dev/null || echo "?")"
      ;;
    behind)
      PUSH_BEHIND="$(git rev-list --count "${head_sha}..${PUSH_REMOTE_SHA}" 2>/dev/null || echo "?")"
      ;;
  esac
}

# Human-readable status of one remote-tracking branch vs HEAD.
describe_remote_branch_status() {
  local remote="$1"
  local sha="$2"
  local status="" n=""
  status="$(compare_head_to_remote_sha "${sha}" "${HEAD_SHA}")"
  case "${status}" in
    up-to-date)
      echo "${remote}/${CURRENT_BRANCH} (up to date)"
      ;;
    ahead)
      n="$(git rev-list --count "${sha}..${HEAD_SHA}" 2>/dev/null || echo "?")"
      echo "${remote}/${CURRENT_BRANCH} (${n} unpushed commit(s))"
      ;;
    behind)
      n="$(git rev-list --count "${HEAD_SHA}..${sha}" 2>/dev/null || echo "?")"
      echo "${remote}/${CURRENT_BRANCH} (${n} commit(s) behind remote)"
      ;;
    diverged)
      echo "${remote}/${CURRENT_BRANCH} (diverged from local)"
      ;;
    *)
      echo "${remote}/${CURRENT_BRANCH} (differs from local)"
      ;;
  esac
}

print_branch_push_status() {
  local remote="" sha="" extra="false"
  if [[ "${PUSH_STATUS}" == "detached" ]]; then
    echo "Detached HEAD state - cannot check push status"
    return 0
  fi

  case "${PUSH_STATUS}" in
    up-to-date)
      echo "✓ Branch '${CURRENT_BRANCH}' is pushed and up-to-date with ${PUSH_REMOTE}/${CURRENT_BRANCH}"
      ;;
    missing)
      echo "✗ Branch '${CURRENT_BRANCH}' not found on any remote"
      if [[ -n "${PUSH_HINT_REMOTE}" ]]; then
        echo "Run: git push -u ${PUSH_HINT_REMOTE} ${CURRENT_BRANCH}"
      else
        echo "No git remotes configured."
      fi
      ;;
    ahead)
      echo "✗ Branch '${CURRENT_BRANCH}' exists on ${PUSH_REMOTE} but local is ahead"
      echo "Unpushed commits: ${PUSH_AHEAD}"
      echo "Run: git push ${PUSH_REMOTE} ${CURRENT_BRANCH}"
      ;;
    behind)
      echo "✗ Branch '${CURRENT_BRANCH}' exists on ${PUSH_REMOTE} but local is behind"
      echo "Remote is ${PUSH_BEHIND} commit(s) ahead of local"
      echo "Run: git fetch ${PUSH_REMOTE} && git status"
      ;;
    diverged)
      echo "✗ Branch '${CURRENT_BRANCH}' exists on ${PUSH_REMOTE} but local has diverged"
      echo "Run: git fetch ${PUSH_REMOTE} && git status"
      ;;
    *)
      echo "✗ Branch '${CURRENT_BRANCH}' exists on ${PUSH_REMOTE} but local differs from remote"
      echo "Run: git fetch ${PUSH_REMOTE} && git status"
      ;;
  esac

  if [[ -n "${PUSH_MATCHES}" ]]; then
    while IFS=$'\t' read -r remote sha; do
      [[ -n "${remote}" ]] || continue
      [[ "${remote}" != "${PUSH_REMOTE}" ]] || continue
      if [[ "${extra}" != "true" ]]; then
        echo "Also found on:"
        extra="true"
      fi
      echo "  $(describe_remote_branch_status "${remote}" "${sha}")"
    done <<< "${PUSH_MATCHES}"
  fi
}

# Detect PR target repo from the push remote (fork parent, else upstream, else same repo).
detect_pr_target() {
  local head_remote="" url="" repo_slug="" parent="" upstream_slug=""
  local gh_ok="false"

  head_remote="${PUSH_REMOTE:-${PUSH_HINT_REMOTE}}"
  if [[ -n "${head_remote}" ]]; then
    url="$(git remote get-url "${head_remote}" 2>/dev/null || true)"
  fi
  if [[ -z "${url}" ]]; then
    url="$(git remote get-url origin 2>/dev/null || true)"
  fi
  if [[ -z "${url}" ]]; then
    head_remote="$(first_remote || true)"
    if [[ -n "${head_remote}" ]]; then
      url="$(git remote get-url "${head_remote}" 2>/dev/null || true)"
    fi
  fi
  if [[ -z "${url}" ]]; then
    return 0
  fi

  repo_slug="$(parse_repo_slug "${url}")"
  if [[ ! "${repo_slug}" =~ ^[^/]+/[^/]+$ ]]; then
    echo "${repo_slug}"
    return 0
  fi

  if [[ "${GH_AUTH_OK:-}" == "true" ]]; then
    gh_ok="true"
  elif command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
    gh_ok="true"
  fi

  if [[ "${gh_ok}" == "true" ]]; then
    parent="$(gh api "repos/${repo_slug}" --jq 'if .fork then .parent.full_name else empty end' 2>/dev/null || true)"
    if [[ -n "${parent}" ]]; then
      echo "${parent}"
      return 0
    fi
  fi

  if git remote get-url upstream >/dev/null 2>&1; then
    upstream_slug="$(remote_repo_slug upstream || true)"
    if [[ "${upstream_slug}" =~ ^[^/]+/[^/]+$ && "${upstream_slug}" != "${repo_slug}" ]]; then
      echo "${upstream_slug}"
      return 0
    fi
  fi

  echo "${repo_slug}"
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

CURRENT_BRANCH="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo 'HEAD')"
BRANCH_PUSH_OK="false"
PUSH_REMOTE=""
PUSH_HINT_REMOTE=""
PUSH_MATCHES=""
PUSH_STATUS="missing"
resolve_branch_push "${CURRENT_BRANCH}" "${HEAD_SHA}"

if [[ -n "${PUSH_REMOTE}" && "${PUSH_REMOTE}" != "${DEFAULT_REMOTE}" && "${PUSH_REMOTE}" != "${INTEGRATION_REMOTE}" && "${PUSH_REMOTE}" != "${BASE_REMOTE}" ]]; then
  try_fetch "${PUSH_REMOTE}"
fi

section "Context"
printf "Repository: %s\n" "${TOPLEVEL}"
printf "HEAD:       %s (%s)\n" "${CURRENT_BRANCH}" "${HEAD_SHA}"
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
MERGE_OK="false"

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

case "${PUSH_STATUS}" in
  up-to-date)
    echo "✓ Branch '${CURRENT_BRANCH}' is pushed and up-to-date with ${PUSH_REMOTE}"
    ;;
  detached)
    echo "✗ Detached HEAD state"
    ;;
  missing)
    echo "✗ Branch '${CURRENT_BRANCH}' not pushed to any remote"
    ;;
  ahead)
    echo "✗ Branch '${CURRENT_BRANCH}' has unpushed commits (${PUSH_REMOTE})"
    ;;
  behind)
    echo "✗ Branch '${CURRENT_BRANCH}' is behind ${PUSH_REMOTE}/${CURRENT_BRANCH}"
    ;;
  diverged)
    echo "✗ Branch '${CURRENT_BRANCH}' has diverged from ${PUSH_REMOTE}/${CURRENT_BRANCH}"
    ;;
  *)
    echo "✗ Branch '${CURRENT_BRANCH}' differs from ${PUSH_REMOTE}/${CURRENT_BRANCH}"
    ;;
esac

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
print_branch_push_status

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
  echo "Target repository: ${PR_TARGET_REPO}"
  push_head_remote="${PUSH_REMOTE:-${PUSH_HINT_REMOTE}}"
  push_slug=""
  if [[ -n "${push_head_remote}" ]]; then
    push_slug="$(remote_repo_slug "${push_head_remote}" || true)"
  fi
  if [[ -n "${push_slug}" && "${PR_TARGET_REPO}" != "${push_slug}" ]]; then
    echo "Fork workflow detected:"
    echo "  Push to: ${push_slug}:${CURRENT_BRANCH}"
    echo "  PR to: ${PR_TARGET_REPO}"
  else
    echo "Direct workflow: PR within ${PR_TARGET_REPO}"
  fi
else
  echo "Could not determine PR target (gh cli may not be authenticated, or no GitHub remote)"
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
