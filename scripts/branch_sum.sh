#!/usr/bin/env bash
#
# Audit remote and local Git branches for cleanup candidates.
# Read-only: never deletes branches. Safe to run from any directory inside a
# Git work tree (uses repo root).
#
# Usage: scripts/branch_sum.sh [options]
#
# Full PR / protected-branch metadata requires gh + jq. Without them the
# script still lists local and remote refs with git-only merged checks.

set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/branch_sum.sh [options]

  --remote <name>       Remote to audit for delete suggestions (default: origin)
  --repo <owner/repo>   Override repo slug (default: from remote URL)
  --base <ref>          Integration base for merged/ahead checks
                        (default: upstream default when upstream exists,
                        else audited remote default)
  --keep-pattern <glob> Repeatable; branches matching glob are always kept
  --min-score <N>       Only show review branches with score >= N (default: 0)
  --scope <mode>        remote | local | both (default: both)
  --json                Machine-readable JSON on stdout; suppress human sections
  --no-fetch            Skip git fetch --prune
  -h, --help            Show this help

Run from anywhere inside a Git repository. PR metadata comes from
`gh pr list --state all --limit 500` (warns if truncated). Never deletes
branches; prints dry-run delete suggestions only.

Without gh or jq, PR and protected-branch signals are omitted; git-only
merged-into-base checks still run for both scopes.
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
  if [[ "${JSON_MODE}" != "true" ]]; then
    section "Fetch (${remote})"
  fi
  if git fetch --prune "${remote}" >/dev/null 2>&1; then
    FETCH_STATUS="ok"
    if [[ "${JSON_MODE}" != "true" ]]; then
      echo "✓ Fetched ${remote} successfully"
    fi
  else
    FETCH_STATUS="failed"
    echo "Warning: git fetch ${remote} failed. Continuing with local refs." >&2
  fi
}

# Parse GitHub repo slug from git URL
parse_repo_slug() {
  local url="$1"
  echo "${url}" | sed -E 's#.*github\.com[:/]([^/]+/[^/]+)\.git$#\1#; s#.*github\.com[:/]([^/]+/[^/]+)$#\1#'
}

first_remote() {
  local remote=""
  while IFS= read -r remote; do
    [[ -n "${remote}" ]] || continue
    echo "${remote}"
    return 0
  done < <(git remote)
  return 1
}

remote_repo_slug() {
  local url=""
  url="$(git remote get-url "$1" 2>/dev/null || true)"
  [[ -n "${url}" ]] || return 1
  parse_repo_slug "${url}"
}

# Return 0 if branch name matches any keep pattern (bash glob).
matches_keep_pattern() {
  local branch="$1"
  local pat=""
  for pat in "${KEEP_PATTERNS[@]+"${KEEP_PATTERNS[@]}"}"; do
    # shellcheck disable=SC2254
    case "${branch}" in
      ${pat}) return 0 ;;
    esac
  done
  return 1
}

# Days between epoch seconds and now (non-negative integer).
days_since_epoch() {
  local ts="$1"
  local now
  now="$(date +%s)"
  echo $(( (now - ts) / 86400 ))
}

# Clamp integer to 0..100.
clamp_score() {
  local n="$1"
  if (( n < 0 )); then
    echo 0
  elif (( n > 100 )); then
    echo 100
  else
    echo "${n}"
  fi
}

# Score a review-candidate branch. Sets SCORE and REASON_TOKENS (comma-separated).
# Args: days_ago ahead pr_state pr_merged pr_closed_days has_pr_data author_is_user
# has_pr_data: "true" if gh PR index was loaded; "false" means do not apply orphan +10
# pr_state: OPEN|MERGED|CLOSED|"" ; pr_merged: true|false|""
# pr_closed_days: days since closedAt, or "" if unknown
score_branch() {
  local days_ago="$1"
  local ahead="$2"
  local pr_state="$3"
  local pr_merged="$4"
  local pr_closed_days="$5"
  local has_pr_data="$6"
  local author_is_user="$7"

  local score=50
  local reasons=()
  local delta=0

  if [[ "${pr_state}" == "OPEN" ]]; then
    SCORE=0
    REASON_TOKENS="open-pr →0"
    return 0
  fi

  if [[ "${pr_merged}" == "true" || "${pr_state}" == "MERGED" ]]; then
    SCORE=100
    REASON_TOKENS="merged-pr →100"
    return 0
  fi

  # Fully merged into default is handled before calling score for Bucket B;
  # when scoring review branches that are also ancestors, still apply +35.
  if [[ "${BRANCH_IS_MERGED:-false}" == "true" ]]; then
    score=$((score + 35))
    reasons+=("merged-into-base +35")
  fi

  if (( days_ago > 180 )); then
    score=$((score + 25))
    reasons+=("age:${days_ago}d +25")
  elif (( days_ago > 90 )); then
    score=$((score + 15))
    reasons+=("age:${days_ago}d +15")
  elif (( days_ago < 14 )); then
    score=$((score - 25))
    reasons+=("age:${days_ago}d -25")
  fi

  if [[ "${pr_state}" == "CLOSED" && "${pr_merged}" != "true" && -n "${pr_closed_days}" ]]; then
    if (( pr_closed_days > 60 )); then
      score=$((score + 20))
      reasons+=("closed-pr:${pr_closed_days}d +20")
    elif (( pr_closed_days < 14 )); then
      score=$((score - 15))
      reasons+=("closed-pr:${pr_closed_days}d -15")
    fi
  fi

  if (( ahead > 0 )); then
    delta=$(( ahead < 30 ? ahead : 30 ))
    score=$((score - delta))
    reasons+=("ahead:${ahead} -${delta}")
  fi

  if [[ "${has_pr_data}" == "true" && -z "${pr_state}" ]]; then
    score=$((score + 10))
    reasons+=("no-pr +10")
  fi

  if [[ "${author_is_user}" == "true" ]] && (( days_ago < 30 )); then
    score=$((score - 20))
    reasons+=("author-recent -20")
  fi

  SCORE="$(clamp_score "${score}")"
  if ((${#reasons[@]} > 0)); then
    local IFS=', '
    REASON_TOKENS="${reasons[*]}"
  else
    REASON_TOKENS="base:50"
  fi
}

# Look up PR fields for a branch name from PR_INDEX_FILE.
# Sets: PR_NUMBER PR_STATE PR_MERGED PR_CLOSED_DAYS PR_UPDATED
lookup_pr() {
  local branch="$1"
  PR_NUMBER=""
  PR_STATE=""
  PR_MERGED=""
  PR_CLOSED_DAYS=""
  PR_UPDATED=""
  [[ -f "${PR_INDEX_FILE}" ]] || return 1
  local line=""
  line="$(awk -F'\t' -v b="${branch}" '$1 == b { print; exit }' "${PR_INDEX_FILE}")"
  [[ -n "${line}" ]] || return 1
  IFS=$'\t' read -r _ PR_NUMBER PR_STATE PR_MERGED _closed_at PR_UPDATED _author <<< "${line}"
  if [[ -n "${_closed_at}" && "${_closed_at}" != "null" && "${_closed_at}" != "" ]]; then
    local closed_epoch=""
    closed_epoch="$(date -j -f "%Y-%m-%dT%H:%M:%SZ" "${_closed_at}" "+%s" 2>/dev/null \
      || date -d "${_closed_at}" "+%s" 2>/dev/null \
      || true)"
    if [[ -n "${closed_epoch}" ]]; then
      PR_CLOSED_DAYS="$(days_since_epoch "${closed_epoch}")"
    fi
  fi
  return 0
}

# ── Argument parsing ─────────────────────────────────────────────────────────

REMOTE_NAME=""
REPO_SLUG_OVERRIDE=""
BASE_REF=""
KEEP_PATTERNS=()
MIN_SCORE=0
SCOPE="both"
JSON_MODE="false"
DO_FETCH="true"

while (($# > 0)); do
  case "$1" in
    --remote)
      (($# >= 2)) || die "--remote requires a value"
      REMOTE_NAME="$2"
      shift 2
      ;;
    --repo)
      (($# >= 2)) || die "--repo requires a value"
      REPO_SLUG_OVERRIDE="$2"
      shift 2
      ;;
    --base)
      (($# >= 2)) || die "--base requires a value"
      BASE_REF="$2"
      shift 2
      ;;
    --keep-pattern)
      (($# >= 2)) || die "--keep-pattern requires a value"
      KEEP_PATTERNS+=("$2")
      shift 2
      ;;
    --min-score)
      (($# >= 2)) || die "--min-score requires a value"
      [[ "$2" =~ ^[0-9]+$ ]] || die "--min-score must be a non-negative integer"
      MIN_SCORE="$2"
      shift 2
      ;;
    --scope)
      (($# >= 2)) || die "--scope requires a value"
      case "$2" in
        remote|local|both) SCOPE="$2" ;;
        *) die "--scope must be remote, local, or both" ;;
      esac
      shift 2
      ;;
    --json)
      JSON_MODE="true"
      shift
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

# ── Prerequisites ────────────────────────────────────────────────────────────

require_command git

TOPLEVEL="$(git rev-parse --show-toplevel 2>/dev/null)" || die "Not inside a Git repository"
cd "${TOPLEVEL}"
export GIT_PAGER=cat

WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/branch_sum.XXXXXX")"
trap 'rm -rf "${WORKDIR}"' EXIT

PR_INDEX_FILE="${WORKDIR}/prs.tsv"
PROTECTED_FILE="${WORKDIR}/protected.txt"
OPEN_PR_FILE="${WORKDIR}/open_prs.txt"
MERGED_PR_FILE="${WORKDIR}/merged_prs.tsv"
REMOTE_RESULT="${WORKDIR}/remote_result.tsv"
LOCAL_RESULT="${WORKDIR}/local_result.tsv"
LINKAGE_FILE="${WORKDIR}/linkage.tsv"
WORKTREE_BRANCHES="${WORKDIR}/worktrees.txt"
: >"${PR_INDEX_FILE}"
: >"${PROTECTED_FILE}"
: >"${OPEN_PR_FILE}"
: >"${MERGED_PR_FILE}"
: >"${REMOTE_RESULT}"
: >"${LOCAL_RESULT}"
: >"${LINKAGE_FILE}"
: >"${WORKTREE_BRANCHES}"

FETCH_STATUS="skipped"
if [[ "${DO_FETCH}" != "true" ]]; then
  FETCH_STATUS="skipped"
fi

# Resolve audited remote (delete suggestions).
if [[ -z "${REMOTE_NAME}" ]]; then
  if git remote get-url origin >/dev/null 2>&1; then
    REMOTE_NAME="origin"
  else
    REMOTE_NAME="$(first_remote || true)"
  fi
fi
[[ -n "${REMOTE_NAME}" ]] || die "No git remote found. Pass --remote."
git remote get-url "${REMOTE_NAME}" >/dev/null 2>&1 || die "Remote '${REMOTE_NAME}' not found"

# Never suggest deletes on upstream; warn if user asked for it.
if [[ "${REMOTE_NAME}" == "upstream" ]]; then
  echo "Warning: auditing 'upstream' remote; delete suggestions will be suppressed (fork parent)." >&2
  SUPPRESS_REMOTE_DELETE="true"
else
  SUPPRESS_REMOTE_DELETE="false"
fi

# Integration remote for merge/ahead checks (prefer upstream when present).
INTEGRATION_REMOTE="${REMOTE_NAME}"
if git remote get-url upstream >/dev/null 2>&1; then
  INTEGRATION_REMOTE="upstream"
fi

try_fetch "${REMOTE_NAME}"
if [[ "${INTEGRATION_REMOTE}" != "${REMOTE_NAME}" ]]; then
  try_fetch "${INTEGRATION_REMOTE}"
fi

# Repo slug for gh.
REPO_SLUG=""
if [[ -n "${REPO_SLUG_OVERRIDE}" ]]; then
  REPO_SLUG="${REPO_SLUG_OVERRIDE}"
else
  REPO_SLUG="$(remote_repo_slug "${REMOTE_NAME}" || true)"
fi

# gh / jq availability
GH_AVAILABLE="false"
JQ_AVAILABLE="false"
GH_AUTH_OK="false"
GH_AUTH_ERROR=""
HAS_PR_DATA="false"
CURRENT_GH_USER=""

if command -v jq >/dev/null 2>&1; then
  JQ_AVAILABLE="true"
fi
if command -v gh >/dev/null 2>&1; then
  GH_AVAILABLE="true"
  GH_AUTH_OUTPUT="$(gh auth status 2>&1)" || true
  if echo "${GH_AUTH_OUTPUT}" | grep -q "Logged in"; then
    GH_AUTH_OK="true"
  else
    if echo "${GH_AUTH_OUTPUT}" | grep -qi "network\|connection\|unreachable\|timeout"; then
      GH_AUTH_ERROR="network"
    elif echo "${GH_AUTH_OUTPUT}" | grep -qi "token.*invalid\|authentication failed"; then
      GH_AUTH_ERROR="auth"
    else
      GH_AUTH_ERROR="unknown"
    fi
  fi
fi

if [[ "${JSON_MODE}" == "true" && "${JQ_AVAILABLE}" != "true" ]]; then
  die "--json requires jq (install: https://jqlang.org/download/)"
fi

# Default / base branch resolution
DEFAULT_BRANCH=""
BASE_SHA=""
BASE_DISPLAY=""

if [[ -n "${BASE_REF}" ]]; then
  BASE_DISPLAY="${BASE_REF}"
  BASE_SHA="$(git rev-parse --verify "${BASE_REF}^{commit}" 2>/dev/null)" \
    || die "Could not resolve --base '${BASE_REF}'"
  # Best-effort default branch name for Keep checks
  if [[ "${GH_AUTH_OK}" == "true" && -n "${REPO_SLUG}" ]]; then
    DEFAULT_BRANCH="$(gh repo view "${REPO_SLUG}" --json defaultBranchRef --jq '.defaultBranchRef.name' 2>/dev/null || true)"
  fi
  if [[ -z "${DEFAULT_BRANCH}" ]]; then
    DEFAULT_BRANCH="$(git symbolic-ref -q --short "refs/remotes/${INTEGRATION_REMOTE}/HEAD" 2>/dev/null | sed "s#^${INTEGRATION_REMOTE}/##" || true)"
  fi
  [[ -n "${DEFAULT_BRANCH}" ]] || DEFAULT_BRANCH="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo main)"
else
  if [[ "${GH_AUTH_OK}" == "true" && -n "${REPO_SLUG}" ]]; then
    DEFAULT_BRANCH="$(gh repo view "${REPO_SLUG}" --json defaultBranchRef --jq '.defaultBranchRef.name' 2>/dev/null || true)"
  fi
  if [[ -z "${DEFAULT_BRANCH}" ]]; then
    DEFAULT_BRANCH="$(git symbolic-ref -q --short "refs/remotes/${INTEGRATION_REMOTE}/HEAD" 2>/dev/null | sed "s#^${INTEGRATION_REMOTE}/##" || true)"
  fi
  if [[ -z "${DEFAULT_BRANCH}" ]]; then
    for candidate in main master; do
      if git rev-parse --verify "${INTEGRATION_REMOTE}/${candidate}" >/dev/null 2>&1; then
        DEFAULT_BRANCH="${candidate}"
        break
      fi
    done
  fi
  [[ -n "${DEFAULT_BRANCH}" ]] || die "Could not resolve default branch. Pass --base."
  BASE_DISPLAY="${INTEGRATION_REMOTE}/${DEFAULT_BRANCH}"
  BASE_SHA="$(git rev-parse --verify "${BASE_DISPLAY}^{commit}" 2>/dev/null)" \
    || die "Could not resolve base '${BASE_DISPLAY}'. Pass --base or fetch remotes."
fi

CURRENT_BRANCH="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo HEAD)"
if [[ "${CURRENT_BRANCH}" == "HEAD" ]]; then
  CURRENT_BRANCH=""
fi

# Worktree checkouts (branch names)
while IFS= read -r line; do
  case "${line}" in
    branch\ refs/heads/*)
      echo "${line#branch refs/heads/}" >>"${WORKTREE_BRANCHES}"
      ;;
  esac
done < <(git worktree list --porcelain 2>/dev/null || true)

is_worktree_branch() {
  local b="$1"
  [[ -f "${WORKTREE_BRANCHES}" ]] || return 1
  grep -Fxq "${b}" "${WORKTREE_BRANCHES}" 2>/dev/null
}

# ── Load gh metadata ─────────────────────────────────────────────────────────

PR_TRUNCATED="false"

if [[ "${GH_AUTH_OK}" == "true" && "${JQ_AVAILABLE}" == "true" && -n "${REPO_SLUG}" ]]; then
  CURRENT_GH_USER="$(gh api user --jq '.login' 2>/dev/null || true)"

  # Protected branch names (paginate)
  if gh api --paginate "repos/${REPO_SLUG}/branches?per_page=100" \
    --jq '.[] | select(.protected==true) | .name' >"${PROTECTED_FILE}" 2>/dev/null; then
    :
  else
    # Fallback: at least mark default if protection lookup fails later per-candidate
    : >"${PROTECTED_FILE}"
  fi

  # PR index (latest per headRefName by updatedAt)
  PR_JSON="$(gh pr list --repo "${REPO_SLUG}" --state all --limit 500 \
    --json number,title,state,mergedAt,closedAt,headRefName,author,updatedAt 2>/dev/null || true)"
  if [[ -n "${PR_JSON}" ]]; then
    pr_count="$(printf '%s' "${PR_JSON}" | jq 'length')"
    if [[ "${pr_count}" -ge 500 ]]; then
      PR_TRUNCATED="true"
      echo "Warning: PR list hit --limit 500; metadata may be incomplete." >&2
    fi
    # Sort by updatedAt desc, unique by headRefName
    printf '%s' "${PR_JSON}" | jq -r '
      sort_by(.updatedAt) | reverse |
      unique_by(.headRefName) |
      .[] |
      [
        .headRefName,
        (.number|tostring),
        .state,
        (if .mergedAt != null then "true" else "false" end),
        (.closedAt // ""),
        (.updatedAt // ""),
        (.author.login // "")
      ] | @tsv
    ' >"${PR_INDEX_FILE}"

    awk -F'\t' '$3 == "OPEN" { print $1 }' "${PR_INDEX_FILE}" >"${OPEN_PR_FILE}"
    awk -F'\t' '$4 == "true" { print $1 "\t" $2 }' "${PR_INDEX_FILE}" >"${MERGED_PR_FILE}"
    HAS_PR_DATA="true"
  fi
fi

is_protected() {
  local b="$1"
  grep -Fxq "${b}" "${PROTECTED_FILE}" 2>/dev/null
}

is_open_pr_head() {
  local b="$1"
  grep -Fxq "${b}" "${OPEN_PR_FILE}" 2>/dev/null
}

merged_pr_number() {
  local b="$1"
  awk -F'\t' -v br="${b}" '$1 == br { print $2; exit }' "${MERGED_PR_FILE}"
}

# Protection-unknown flag for candidates (when API list failed empty but gh present)
PROTECTION_LOOKUP_OK="true"
if [[ "${GH_AUTH_OK}" == "true" && "${JQ_AVAILABLE}" == "true" && -n "${REPO_SLUG}" ]]; then
  # If protected file is empty, try verifying default branch once
  if [[ ! -s "${PROTECTED_FILE}" ]]; then
    prot_json="$(gh api "repos/${REPO_SLUG}/branches/${DEFAULT_BRANCH}" --jq 'has("protection") or .protected' 2>/dev/null || true)"
    if [[ "${prot_json}" == "true" ]]; then
      echo "${DEFAULT_BRANCH}" >>"${PROTECTED_FILE}"
    elif [[ -z "${prot_json}" ]]; then
      PROTECTION_LOOKUP_OK="false"
    fi
  fi
fi

# ── Collect remote heads ─────────────────────────────────────────────────────

REMOTE_BRANCHES_FILE="${WORKDIR}/remote_branches.tsv"
: >"${REMOTE_BRANCHES_FILE}"

while IFS= read -r line; do
  [[ -n "${line}" ]] || continue
  name="${line%%$'\t'*}"
  rest="${line#*$'\t'}"
  sha="${rest%%$'\t'*}"
  rest2="${rest#*$'\t'}"
  cdate="${rest2%%$'\t'*}"
  author="${rest2#*$'\t'}"
  [[ "${name}" == "HEAD" ]] && continue
  [[ -n "${author}" ]] || author="-"
  [[ -n "${cdate}" ]] || cdate="-"
  printf '%s\t%s\t%s\t%s\n' "${name}" "${sha}" "${cdate}" "${author}" >>"${REMOTE_BRANCHES_FILE}"
done < <(
  git for-each-ref --format='%(refname:strip=3)%09%(objectname)%09%(committerdate:short)%09%(authorname)' \
    "refs/remotes/${REMOTE_NAME}/" 2>/dev/null || true
)

# ── Collect local heads ──────────────────────────────────────────────────────

LOCAL_BRANCHES_FILE="${WORKDIR}/local_branches.tsv"
: >"${LOCAL_BRANCHES_FILE}"

while IFS= read -r line; do
  [[ -n "${line}" ]] || continue
  name="${line%%$'\t'*}"
  rest="${line#*$'\t'}"
  sha="${rest%%$'\t'*}"
  rest2="${rest#*$'\t'}"
  cdate="${rest2%%$'\t'*}"
  rest3="${rest2#*$'\t'}"
  author="${rest3%%$'\t'*}"
  rest4="${rest3#*$'\t'}"
  upstream="${rest4%%$'\t'*}"
  track="${rest4#*$'\t'}"
  [[ -n "${upstream}" ]] || upstream="-"
  [[ -n "${track}" ]] || track="-"
  [[ -n "${author}" ]] || author="-"
  [[ -n "${cdate}" ]] || cdate="-"
  printf '%s\t%s\t%s\t%s\t%s\t%s\n' "${name}" "${sha}" "${cdate}" "${author}" "${upstream}" "${track}" >>"${LOCAL_BRANCHES_FILE}"
done < <(
  git for-each-ref --format='%(refname:short)%09%(objectname)%09%(committerdate:short)%09%(authorname)%09%(upstream:short)%09%(upstream:track,nobracket)' \
    refs/heads/ 2>/dev/null || true
)

# ── Classify one branch into result TSV ──────────────────────────────────────
# Result columns:
# bucket|branch|score|reason|last_commit|ahead|pr_number|pr_state|pr_merged|merged_pr|sha|days_ago|author

classify_branch() {
  local scope="$1" # remote|local
  local branch="$2"
  local sha="$3"
  local cdate="$4"
  local author="$5"
  local upstream="${6:-}"
  local track="${7:-}"

  local bucket="" keep_reason="" score="" reasons=""
  local ahead=0 merged="false"
  local days_ago=0
  local commit_epoch=""
  local pr_number="" pr_state="" pr_merged="" pr_closed_days=""
  local author_is_user="false"
  local merged_pr=""
  local protection_unknown="false"

  commit_epoch="$(git log -1 --format=%ct "${sha}" 2>/dev/null || true)"
  if [[ -n "${commit_epoch}" ]]; then
    days_ago="$(days_since_epoch "${commit_epoch}")"
  fi

  if git merge-base --is-ancestor "${sha}" "${BASE_SHA}" 2>/dev/null; then
    merged="true"
  fi

  ahead="$(git rev-list --count "${BASE_SHA}..${sha}" 2>/dev/null || echo 0)"

  lookup_pr "${branch}" || true
  pr_number="${PR_NUMBER}"
  pr_state="${PR_STATE}"
  pr_merged="${PR_MERGED}"
  pr_closed_days="${PR_CLOSED_DAYS}"

  if [[ -n "${CURRENT_GH_USER}" ]]; then
    # Prefer PR author match; fall back to git author name containing login (best-effort)
    local pr_author=""
    pr_author="$(awk -F'\t' -v b="${branch}" '$1 == b { print $7; exit }' "${PR_INDEX_FILE}" 2>/dev/null || true)"
    if [[ "${pr_author}" == "${CURRENT_GH_USER}" ]]; then
      author_is_user="true"
    fi
  fi

  merged_pr="$(merged_pr_number "${branch}" || true)"

  # ── Bucket A: Keep ─────────────────────────────────────────────────────────
  if [[ "${branch}" == "${DEFAULT_BRANCH}" ]]; then
    bucket="keep"
    keep_reason="default"
  elif is_protected "${branch}"; then
    bucket="keep"
    keep_reason="protected"
  elif is_open_pr_head "${branch}"; then
    bucket="keep"
    keep_reason="open-pr"
  elif matches_keep_pattern "${branch}"; then
    bucket="keep"
    keep_reason="keep-pattern"
  elif [[ "${scope}" == "local" && -n "${CURRENT_BRANCH}" && "${branch}" == "${CURRENT_BRANCH}" ]]; then
    bucket="keep"
    keep_reason="current-checkout"
  elif [[ "${scope}" == "local" ]] && is_worktree_branch "${branch}"; then
    bucket="keep"
    keep_reason="worktree-checkout"
  elif [[ "${scope}" == "remote" && -n "${CURRENT_BRANCH}" && "${branch}" == "${CURRENT_BRANCH}" ]]; then
    bucket="keep"
    keep_reason="current-checkout"
  elif [[ "${scope}" == "local" ]]; then
    # Track remote that is protected or open-PR head
    local tracked_name="${upstream#"${REMOTE_NAME}/"}"
    if [[ -n "${upstream}" && "${upstream}" != "-" && "${upstream}" == "${REMOTE_NAME}/"* ]]; then
      if is_protected "${tracked_name}" || is_open_pr_head "${tracked_name}"; then
        bucket="keep"
        if is_open_pr_head "${tracked_name}"; then
          keep_reason="tracks-open-pr"
        else
          keep_reason="tracks-protected"
        fi
      fi
    fi
  fi

  if [[ -z "${bucket}" && "${PROTECTION_LOOKUP_OK}" != "true" && "${merged}" == "true" ]]; then
    # Do not put in safe_remove when protection unknown
    protection_unknown="true"
  fi

  # ── Bucket B: Safe remove ──────────────────────────────────────────────────
  if [[ -z "${bucket}" ]]; then
    if [[ -n "${merged_pr}" || "${pr_merged}" == "true" ]]; then
      if [[ "${protection_unknown}" == "true" ]]; then
        bucket="review"
        BRANCH_IS_MERGED="true"
        score_branch "${days_ago}" "${ahead}" "${pr_state}" "${pr_merged}" "${pr_closed_days}" "${HAS_PR_DATA}" "${author_is_user}"
        reasons="${REASON_TOKENS},protection-unknown"
        # Force into review with merged-pr signal still visible
        score="${SCORE}"
      else
        bucket="safe_remove"
        score=100
        reasons="merged-pr →100"
        [[ -n "${merged_pr}" ]] || merged_pr="${pr_number}"
      fi
    elif [[ "${merged}" == "true" ]]; then
      if [[ "${protection_unknown}" == "true" ]]; then
        bucket="review"
        BRANCH_IS_MERGED="true"
        score_branch "${days_ago}" "${ahead}" "${pr_state}" "${pr_merged}" "${pr_closed_days}" "${HAS_PR_DATA}" "${author_is_user}"
        reasons="${REASON_TOKENS},protection-unknown"
        score="${SCORE}"
      else
        bucket="safe_remove"
        score=100
        reasons="merged-into-base →100"
      fi
    fi
  fi

  # ── Bucket C: Review ───────────────────────────────────────────────────────
  if [[ -z "${bucket}" ]]; then
    bucket="review"
    BRANCH_IS_MERGED="false"
    score_branch "${days_ago}" "${ahead}" "${pr_state}" "${pr_merged}" "${pr_closed_days}" "${HAS_PR_DATA}" "${author_is_user}"
    score="${SCORE}"
    reasons="${REASON_TOKENS}"
  fi

  if [[ "${bucket}" == "keep" ]]; then
    score=0
    reasons="${keep_reason}"
  fi

  # Bash 3.2 read collapses empty TSV fields; use "-" placeholders.
  [[ -n "${pr_number}" ]] || pr_number="-"
  [[ -n "${pr_state}" ]] || pr_state="-"
  [[ -n "${pr_merged}" ]] || pr_merged="-"
  [[ -n "${merged_pr}" ]] || merged_pr="-"
  [[ -n "${author}" ]] || author="-"
  [[ -n "${cdate}" ]] || cdate="-"
  [[ -n "${reasons}" ]] || reasons="-"

  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
    "${bucket}" "${branch}" "${score}" "${reasons}" "${cdate}" "${ahead}" \
    "${pr_number}" "${pr_state}" "${pr_merged}" "${merged_pr}" "${sha}" "${days_ago}" "${author}"
}

# Normalize "-" placeholders back to empty for display / JSON.
dash_empty() {
  if [[ "$1" == "-" ]]; then
    echo ""
  else
    echo "$1"
  fi
}

# ── Run classification ───────────────────────────────────────────────────────

REMOTE_KEEP=0
REMOTE_SAFE=0
REMOTE_REVIEW=0
LOCAL_KEEP=0
LOCAL_SAFE=0
LOCAL_REVIEW=0

if [[ "${SCOPE}" == "remote" || "${SCOPE}" == "both" ]]; then
  while IFS=$'\t' read -r name sha cdate author; do
    [[ -n "${name}" ]] || continue
    row="$(classify_branch remote "${name}" "${sha}" "${cdate}" "${author}")"
    echo "${row}" >>"${REMOTE_RESULT}"
    bucket="${row%%$'\t'*}"
    case "${bucket}" in
      keep) REMOTE_KEEP=$((REMOTE_KEEP + 1)) ;;
      safe_remove) REMOTE_SAFE=$((REMOTE_SAFE + 1)) ;;
      review) REMOTE_REVIEW=$((REMOTE_REVIEW + 1)) ;;
    esac
  done <"${REMOTE_BRANCHES_FILE}"
fi

if [[ "${SCOPE}" == "local" || "${SCOPE}" == "both" ]]; then
  while IFS=$'\t' read -r name sha cdate author upstream track; do
    [[ -n "${name}" ]] || continue
    row="$(classify_branch local "${name}" "${sha}" "${cdate}" "${author}" "${upstream}" "${track}")"
    echo "${row}" >>"${LOCAL_RESULT}"
    bucket="${row%%$'\t'*}"
    case "${bucket}" in
      keep) LOCAL_KEEP=$((LOCAL_KEEP + 1)) ;;
      safe_remove) LOCAL_SAFE=$((LOCAL_SAFE + 1)) ;;
      review) LOCAL_REVIEW=$((LOCAL_REVIEW + 1)) ;;
    esac
  done <"${LOCAL_BRANCHES_FILE}"
fi

# ── Linkage ──────────────────────────────────────────────────────────────────

LINK_PAIRED=0
LINK_LOCAL_ONLY=0
LINK_REMOTE_ONLY=0
LINK_GONE=0
LINK_DIVERGED=0

# Build name sets
REMOTE_NAMES="${WORKDIR}/remote_names.txt"
LOCAL_NAMES="${WORKDIR}/local_names.txt"
awk -F'\t' '{ print $1 }' "${REMOTE_BRANCHES_FILE}" | sort -u >"${REMOTE_NAMES}"
awk -F'\t' '{ print $1 }' "${LOCAL_BRANCHES_FILE}" | sort -u >"${LOCAL_NAMES}"

# Local linkage
if [[ -s "${LOCAL_BRANCHES_FILE}" ]]; then
  while IFS=$'\t' read -r name sha cdate author upstream track; do
    [[ -n "${name}" ]] || continue
    status=""
    if [[ "${track}" == *gone* ]]; then
      status="gone"
      LINK_GONE=$((LINK_GONE + 1))
    elif [[ -n "${upstream}" && "${upstream}" != "-" && "${upstream}" == "${REMOTE_NAME}/"* ]]; then
      remote_sha="$(awk -F'\t' -v n="${name}" '$1 == n { print $2; exit }' "${REMOTE_BRANCHES_FILE}")"
      if [[ -z "${remote_sha}" ]]; then
        status="gone"
        LINK_GONE=$((LINK_GONE + 1))
      elif [[ "${remote_sha}" == "${sha}" ]]; then
        status="paired"
        LINK_PAIRED=$((LINK_PAIRED + 1))
      else
        if git merge-base --is-ancestor "${sha}" "${remote_sha}" 2>/dev/null \
          || git merge-base --is-ancestor "${remote_sha}" "${sha}" 2>/dev/null; then
          # ahead/behind still count as paired with note — plan uses diverged for neither-ancestor
          status="paired"
          LINK_PAIRED=$((LINK_PAIRED + 1))
        else
          status="diverged"
          LINK_DIVERGED=$((LINK_DIVERGED + 1))
        fi
      fi
    elif grep -Fxq "${name}" "${REMOTE_NAMES}" 2>/dev/null; then
      remote_sha="$(awk -F'\t' -v n="${name}" '$1 == n { print $2; exit }' "${REMOTE_BRANCHES_FILE}")"
      if [[ "${remote_sha}" == "${sha}" ]]; then
        status="paired"
        LINK_PAIRED=$((LINK_PAIRED + 1))
      else
        if git merge-base --is-ancestor "${sha}" "${remote_sha}" 2>/dev/null \
          || git merge-base --is-ancestor "${remote_sha}" "${sha}" 2>/dev/null; then
          status="paired"
          LINK_PAIRED=$((LINK_PAIRED + 1))
        else
          status="diverged"
          LINK_DIVERGED=$((LINK_DIVERGED + 1))
        fi
      fi
    else
      status="local-only"
      LINK_LOCAL_ONLY=$((LINK_LOCAL_ONLY + 1))
    fi
    printf '%s\t%s\n' "${name}" "${status}" >>"${LINKAGE_FILE}"
  done <"${LOCAL_BRANCHES_FILE}"
fi

# Remote-only
if [[ -s "${REMOTE_BRANCHES_FILE}" ]]; then
  while IFS=$'\t' read -r name _rest; do
    [[ -n "${name}" ]] || continue
    if ! grep -Fxq "${name}" "${LOCAL_NAMES}" 2>/dev/null; then
      printf '%s\t%s\n' "${name}" "remote-only" >>"${LINKAGE_FILE}"
      LINK_REMOTE_ONLY=$((LINK_REMOTE_ONLY + 1))
    fi
  done <"${REMOTE_BRANCHES_FILE}"
fi

# ── Output helpers ───────────────────────────────────────────────────────────

print_keep_section() {
  local file="$1"
  local title="$2"
  section "${title}"
  local found=0
  while IFS=$'\t' read -r bucket branch score reasons last_commit ahead pr_number pr_state pr_merged merged_pr sha days_ago author; do
    [[ "${bucket}" == "keep" ]] || continue
    printf "%-40s %s\n" "${branch}" "$(dash_empty "${reasons}")"
    found=1
  done < <(sort -t$'\t' -k2,2 "${file}")
  if [[ "${found}" -eq 0 ]]; then
    echo "(none)"
  fi
}

print_safe_section() {
  local file="$1"
  local title="$2"
  section "${title}"
  local found=0
  printf "%-5s %-36s %-12s %s\n" "score" "branch" "merged PR" "last commit"
  while IFS=$'\t' read -r bucket branch score reasons last_commit ahead pr_number pr_state pr_merged merged_pr sha days_ago author; do
    [[ "${bucket}" == "safe_remove" ]] || continue
    pr_number="$(dash_empty "${pr_number}")"
    merged_pr="$(dash_empty "${merged_pr}")"
    local pr_disp="${merged_pr:-${pr_number}}"
    [[ -n "${pr_disp}" ]] || pr_disp="-"
    printf "%-5s %-36s %-12s %s\n" "${score}" "${branch}" "${pr_disp}" "$(dash_empty "${last_commit}")"
    found=1
  done < <(sort -t$'\t' -k2,2 "${file}")
  if [[ "${found}" -eq 0 ]]; then
    echo "(none)"
  fi
}

print_review_section() {
  local file="$1"
  local title="$2"
  section "${title}"
  local found=0
  printf "%-5s %-32s %-12s %-6s %-14s %s\n" "score" "branch" "last commit" "ahead" "PR" "reasons"
  while IFS=$'\t' read -r bucket branch score reasons last_commit ahead pr_number pr_state pr_merged merged_pr sha days_ago author; do
    [[ "${bucket}" == "review" ]] || continue
    if (( score < MIN_SCORE )); then
      continue
    fi
    pr_number="$(dash_empty "${pr_number}")"
    pr_state="$(dash_empty "${pr_state}")"
    local pr_disp="-"
    if [[ -n "${pr_number}" ]]; then
      pr_disp="#${pr_number}/${pr_state}"
    fi
    printf "%-5s %-32s %-12s %-6s %-14s %s\n" \
      "${score}" "${branch}" "$(dash_empty "${last_commit}")" "${ahead}" "${pr_disp}" "$(dash_empty "${reasons}")"
    found=1
  done < <(sort -t$'\t' -k3,3nr -k2,2 "${file}")
  if [[ "${found}" -eq 0 ]]; then
    echo "(none)"
  fi
}

print_suggested_commands() {
  section "Suggested commands (dry-run)"
  echo "# Not executed by this script."
  if [[ "${SCOPE}" == "remote" || "${SCOPE}" == "both" ]]; then
    if [[ "${SUPPRESS_REMOTE_DELETE}" == "true" ]]; then
      echo "# Remote deletes suppressed (upstream / fork parent)."
    else
      echo "# Remote:"
      local any=0
      while IFS=$'\t' read -r bucket branch score reasons last_commit ahead pr_number pr_state pr_merged merged_pr sha days_ago author; do
        [[ "${bucket}" == "safe_remove" ]] || continue
        echo "git push ${REMOTE_NAME} --delete ${branch}"
        any=1
      done <"${REMOTE_RESULT}"
      if [[ "${any}" -eq 0 ]]; then
        echo "# (no remote safe-remove candidates)"
      fi
    fi
  fi
  if [[ "${SCOPE}" == "local" || "${SCOPE}" == "both" ]]; then
    echo "# Local:"
    local any=0
    while IFS=$'\t' read -r bucket branch score reasons last_commit ahead pr_number pr_state pr_merged merged_pr sha days_ago author; do
      [[ "${bucket}" == "safe_remove" ]] || continue
      echo "git branch -d ${branch}"
      any=1
    done <"${LOCAL_RESULT}"
    if [[ "${any}" -eq 0 ]]; then
      echo "# (no local safe-remove candidates)"
    fi
  fi
}

# ── JSON output ──────────────────────────────────────────────────────────────

result_file_to_json() {
  local file="$1"
  local keep_json safe_json review_json
  keep_json="$(awk -F'\t' '
    function empty(s) { return (s == "-" || s == "") }
    function jqstr(s) {
      if (empty(s)) s = ""
      gsub(/\\/, "\\\\", s); gsub(/"/, "\\\"", s); return "\"" s "\""
    }
    $1 == "keep" {
      printf "{\"branch\":%s,\"reason\":%s}\n", jqstr($2), jqstr($4)
    }
  ' "${file}" | jq -s '.' 2>/dev/null || echo '[]')"

  safe_json="$(awk -F'\t' '
    function empty(s) { return (s == "-" || s == "") }
    function jqstr(s) {
      if (empty(s)) s = ""
      gsub(/\\/, "\\\\", s); gsub(/"/, "\\\"", s); return "\"" s "\""
    }
    $1 == "safe_remove" {
      mp = (empty($10) ? "null" : ($10 + 0))
      printf "{\"branch\":%s,\"score\":%s,\"merged_pr\":%s,\"last_commit\":%s,\"reasons\":%s}\n",
        jqstr($2), $3, mp, jqstr($5), jqstr($4)
    }
  ' "${file}" | jq -s '.' 2>/dev/null || echo '[]')"

  review_json="$(awk -F'\t' -v min="${MIN_SCORE}" '
    function empty(s) { return (s == "-" || s == "") }
    function jqstr(s) {
      if (empty(s)) s = ""
      gsub(/\\/, "\\\\", s); gsub(/"/, "\\\"", s); return "\"" s "\""
    }
    $1 == "review" && ($3 + 0) >= min {
      pr = "null"
      if (!empty($7)) {
        merged = ($9 == "true" ? "true" : "false")
        pr = sprintf("{\"number\":%s,\"state\":%s,\"merged\":%s}", $7, jqstr($8), merged)
      }
      printf "{\"branch\":%s,\"score\":%s,\"last_commit\":%s,\"ahead\":%s,\"pr\":%s,\"reasons\":%s}\n",
        jqstr($2), $3, jqstr($5), $6, pr, jqstr($4)
    }
  ' "${file}" | jq -s '.' 2>/dev/null || echo '[]')"

  local k_count s_count r_count
  k_count="$(awk -F'\t' '$1=="keep"{c++} END{print c+0}' "${file}")"
  s_count="$(awk -F'\t' '$1=="safe_remove"{c++} END{print c+0}' "${file}")"
  r_count="$(awk -F'\t' '$1=="review"{c++} END{print c+0}' "${file}")"

  jq -n \
    --argjson keep "${keep_json}" \
    --argjson safe_remove "${safe_json}" \
    --argjson review "${review_json}" \
    --argjson k "${k_count}" \
    --argjson s "${s_count}" \
    --argjson r "${r_count}" \
    '{summary:{keep:$k, safe_remove:$s, review:$r}, keep:$keep, safe_remove:$safe_remove, review:$review}'
}

emit_json_fixed() {
  local remote_obj local_obj linkage_arr
  remote_obj="$(result_file_to_json "${REMOTE_RESULT}")"
  local_obj="$(result_file_to_json "${LOCAL_RESULT}")"

  linkage_arr="$(
    if [[ -s "${LINKAGE_FILE}" ]]; then
      awk -F'\t' 'NF>=2 {
        gsub(/\\/, "\\\\", $1); gsub(/"/, "\\\"", $1)
        gsub(/\\/, "\\\\", $2); gsub(/"/, "\\\"", $2)
        printf "{\"branch\":\"%s\",\"status\":\"%s\"}\n", $1, $2
      }' "${LINKAGE_FILE}" | jq -s '.'
    else
      echo '[]'
    fi
  )"

  jq -n \
    --arg repo "${REPO_SLUG}" \
    --arg default_branch "${DEFAULT_BRANCH}" \
    --arg remote_name "${REMOTE_NAME}" \
    --arg base "${BASE_DISPLAY}" \
    --arg scope "${SCOPE}" \
    --arg fetch_status "${FETCH_STATUS}" \
    --argjson gh_auth "$( [[ "${GH_AUTH_OK}" == "true" ]] && echo true || echo false )" \
    --argjson has_pr_data "$( [[ "${HAS_PR_DATA}" == "true" ]] && echo true || echo false )" \
    --argjson pr_truncated "$( [[ "${PR_TRUNCATED}" == "true" ]] && echo true || echo false )" \
    --argjson remote_obj "${remote_obj}" \
    --argjson local_obj "${local_obj}" \
    --argjson linkage "${linkage_arr}" \
    --argjson paired "${LINK_PAIRED}" \
    --argjson local_only "${LINK_LOCAL_ONLY}" \
    --argjson remote_only "${LINK_REMOTE_ONLY}" \
    --argjson gone "${LINK_GONE}" \
    --argjson diverged "${LINK_DIVERGED}" \
    '{
      repo: $repo,
      default_branch: $default_branch,
      remote_name: $remote_name,
      base: $base,
      scope: $scope,
      fetch_status: $fetch_status,
      gh_auth: $gh_auth,
      has_pr_data: $has_pr_data,
      pr_truncated: $pr_truncated,
      remote: $remote_obj,
      local: $local_obj,
      linkage: $linkage,
      linkage_summary: {
        paired: $paired,
        "local-only": $local_only,
        "remote-only": $remote_only,
        gone: $gone,
        diverged: $diverged
      }
    }'
}

# ── Human output ─────────────────────────────────────────────────────────────

if [[ "${JSON_MODE}" == "true" ]]; then
  emit_json_fixed
  exit 0
fi

section "Context"
printf "Repository:     %s\n" "${TOPLEVEL}"
printf "Repo slug:      %s\n" "${REPO_SLUG:-"(unknown)"}"
printf "Default branch: %s\n" "${DEFAULT_BRANCH}"
printf "Audit remote:   %s\n" "${REMOTE_NAME}"
printf "Integration:    %s (%s)\n" "${BASE_DISPLAY}" "${BASE_SHA:0:12}"
printf "Scope:          %s\n" "${SCOPE}"
printf "Fetch:          %s\n" "${FETCH_STATUS}"
if [[ "${GH_AUTH_OK}" == "true" ]]; then
  echo "GitHub CLI:     authenticated"
elif [[ "${GH_AVAILABLE}" != "true" ]]; then
  echo "GitHub CLI:     not installed (PR/protected metadata skipped)"
elif [[ "${GH_AUTH_ERROR}" == "network" ]]; then
  echo "GitHub CLI:     network error (PR/protected metadata skipped)"
elif [[ "${GH_AUTH_ERROR}" == "auth" ]]; then
  echo "GitHub CLI:     not authenticated (run: gh auth login)"
else
  echo "GitHub CLI:     unavailable (PR/protected metadata skipped)"
fi
if [[ "${JQ_AVAILABLE}" != "true" ]]; then
  echo "jq:             not installed (PR/protected metadata skipped)"
fi
if [[ "${HAS_PR_DATA}" != "true" ]]; then
  echo "PR data:        unavailable — orphan +10 scoring disabled"
fi
if [[ "${PR_TRUNCATED}" == "true" ]]; then
  echo "PR data:        truncated at 500 PRs"
fi
printf "Remote heads:   %s\n" "$(wc -l <"${REMOTE_BRANCHES_FILE}" | tr -d ' ')"
printf "Local heads:    %s\n" "$(wc -l <"${LOCAL_BRANCHES_FILE}" | tr -d ' ')"

section "Summary"
if [[ "${SCOPE}" == "remote" || "${SCOPE}" == "both" ]]; then
  printf "Remote — Keep: %s | Safe remove: %s | Review: %s\n" "${REMOTE_KEEP}" "${REMOTE_SAFE}" "${REMOTE_REVIEW}"
fi
if [[ "${SCOPE}" == "local" || "${SCOPE}" == "both" ]]; then
  printf "Local  — Keep: %s | Safe remove: %s | Review: %s\n" "${LOCAL_KEEP}" "${LOCAL_SAFE}" "${LOCAL_REVIEW}"
fi
printf "Linkage — paired: %s | local-only: %s | remote-only: %s | gone upstream: %s | diverged: %s\n" \
  "${LINK_PAIRED}" "${LINK_LOCAL_ONLY}" "${LINK_REMOTE_ONLY}" "${LINK_GONE}" "${LINK_DIVERGED}"

if [[ "${SCOPE}" == "remote" || "${SCOPE}" == "both" ]]; then
  print_keep_section "${REMOTE_RESULT}" "Remote: Keep"
  print_safe_section "${REMOTE_RESULT}" "Remote: Safe to remove (merged)"
  print_review_section "${REMOTE_RESULT}" "Remote: Review (rated)"
fi

if [[ "${SCOPE}" == "local" || "${SCOPE}" == "both" ]]; then
  print_keep_section "${LOCAL_RESULT}" "Local: Keep"
  print_safe_section "${LOCAL_RESULT}" "Local: Safe to remove (merged)"
  print_review_section "${LOCAL_RESULT}" "Local: Review (rated)"
fi

print_suggested_commands
