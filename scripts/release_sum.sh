#!/usr/bin/env bash
#
# Summarize release context for a repository: last tag, bump level, and commits.
# Safe to run from any directory inside a Git work tree (uses repo root).
#
# Usage: scripts/release_sum.sh [--pre-release <suffix>] [--no-fetch] [-h|--help]
#
# Handles both squash-merge and merge-commit histories. Merge commits are
# flagged in output but excluded from bump analysis.

set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/release_sum.sh [--pre-release <suffix>] [--no-fetch] [-h|--help]

  --pre-release <suffix>  Append a pre-release suffix to the suggested tag
                          (e.g. --pre-release rc.1 → v1.3.0-rc.1).
  --no-fetch              Do not run git fetch --tags before analyzing.
  -h, --help              Show this help.

Run from anywhere inside a Git repository. Output is human-readable sections
suitable for pasting into a release description or command follow-up.
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

# ── Argument parsing ─────────────────────────────────────────────────────────

PRE_RELEASE_SUFFIX=""
DO_FETCH="true"

while (($# > 0)); do
  case "$1" in
    --pre-release)
      (($# >= 2)) || die "--pre-release requires a value"
      PRE_RELEASE_SUFFIX="$2"
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

# ── Prerequisites ─────────────────────────────────────────────────────────────

require_command git

TOPLEVEL="$(git rev-parse --show-toplevel 2>/dev/null)" || die "Not inside a Git repository"
cd "${TOPLEVEL}"

export GIT_PAGER=cat

# ── Fetch tags ────────────────────────────────────────────────────────────────

if [[ "${DO_FETCH}" == "true" ]]; then
  DEFAULT_REMOTE=""
  if git remote get-url origin >/dev/null 2>&1; then
    DEFAULT_REMOTE="origin"
  fi

  if [[ -n "${DEFAULT_REMOTE}" ]]; then
    section "Fetch (${DEFAULT_REMOTE})"
    if git fetch --prune --tags "${DEFAULT_REMOTE}" >/dev/null 2>&1; then
      echo "✓ Fetched ${DEFAULT_REMOTE} and updated tags"
    else
      echo "Warning: git fetch ${DEFAULT_REMOTE} failed. Continuing with local refs." >&2
    fi
  else
    echo "Warning: no 'origin' remote; skipping fetch." >&2
  fi
fi

# ── Tag detection ─────────────────────────────────────────────────────────────

HEAD_SHA="$(git rev-parse HEAD)"
CURRENT_BRANCH="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo 'HEAD')"

LAST_TAG=""
if LAST_TAG="$(git describe --tags --abbrev=0 2>/dev/null)"; then
  LAST_TAG_SHA="$(git rev-parse "${LAST_TAG}^{commit}" 2>/dev/null || true)"
else
  LAST_TAG=""
  LAST_TAG_SHA=""
fi

# ── Already-tagged check ──────────────────────────────────────────────────────

if [[ -n "${LAST_TAG}" && "${HEAD_SHA}" == "${LAST_TAG_SHA}" ]]; then
  section "Release Readiness"
  echo "✗ HEAD is already tagged as ${LAST_TAG} — nothing to release"
  exit 0
fi

# ── Version bump logic ────────────────────────────────────────────────────────

# Returns: major, minor, or patch
determine_bump() {
  local range="$1"
  local breaking=0
  local has_feat=0

  while IFS= read -r subject; do
    # Skip merge commits
    if echo "${subject}" | grep -qE '^Merge (pull request|branch)'; then
      continue
    fi
    # Breaking change in subject: type!: or type(scope)!:
    if echo "${subject}" | grep -qE '^[a-z]+(\([^)]+\))?!:'; then
      breaking=1
      break
    fi
    # Feature
    if echo "${subject}" | grep -qE '^feat(\([^)]+\))?:'; then
      has_feat=1
    fi
  done < <(git log "${range}" --pretty=tformat:"%s" 2>/dev/null)

  if [[ "${breaking}" -eq 0 ]]; then
    # Also scan commit bodies for BREAKING CHANGE:
    while IFS= read -r line; do
      if echo "${line}" | grep -qiE '^BREAKING[- ]CHANGE:'; then
        breaking=1
        break
      fi
    done < <(git log "${range}" --pretty=tformat:"%b" 2>/dev/null)
  fi

  if [[ "${breaking}" -eq 1 ]]; then
    echo "major"
  elif [[ "${has_feat}" -eq 1 ]]; then
    echo "minor"
  else
    echo "patch"
  fi
}

# Increment a semver string (without leading v)
increment_version() {
  local version="$1"
  local bump="$2"
  local major minor patch

  IFS='.' read -r major minor patch <<< "${version}"
  major="${major:-0}"
  minor="${minor:-0}"
  patch="${patch:-0}"

  case "${bump}" in
    major) major=$((major + 1)); minor=0; patch=0 ;;
    minor) minor=$((minor + 1)); patch=0 ;;
    patch) patch=$((patch + 1)) ;;
  esac

  echo "${major}.${minor}.${patch}"
}

# ── Compute suggested tag ─────────────────────────────────────────────────────

if [[ -z "${LAST_TAG}" ]]; then
  BUMP_LEVEL="none"
  BUMP_REASON="no previous tags; defaulting to initial release"
  BASE_VERSION="0.1.0"
  SUGGESTED_TAG="v0.1.0"
else
  # Strip leading 'v' for arithmetic
  STRIPPED="${LAST_TAG#v}"
  COMMIT_RANGE="${LAST_TAG}..HEAD"
  BUMP_LEVEL="$(determine_bump "${COMMIT_RANGE}")"

  case "${BUMP_LEVEL}" in
    major) BUMP_REASON="breaking change found in commit history" ;;
    minor) BUMP_REASON="feat commit(s) found, no breaking changes" ;;
    patch) BUMP_REASON="no feat or breaking changes found" ;;
  esac

  BASE_VERSION="$(increment_version "${STRIPPED}" "${BUMP_LEVEL}")"
  SUGGESTED_TAG="v${BASE_VERSION}"
fi

if [[ -n "${PRE_RELEASE_SUFFIX}" ]]; then
  SUGGESTED_TAG="${SUGGESTED_TAG}-${PRE_RELEASE_SUFFIX}"
fi

# ── Commit type counts ────────────────────────────────────────────────────────

COMMIT_RANGE_FOR_LOG=""
if [[ -n "${LAST_TAG}" ]]; then
  COMMIT_RANGE_FOR_LOG="${LAST_TAG}..HEAD"
else
  COMMIT_RANGE_FOR_LOG="HEAD"
fi

# Collect all subjects into a temp file to allow multiple passes without
# re-running git. Bash 3.x (macOS) has no associative arrays so we use
# sorted counts via sort | uniq -c instead.
_TMP_SUBJECTS="$(mktemp)"
_TMP_BODIES="$(mktemp)"
trap 'rm -f "${_TMP_SUBJECTS}" "${_TMP_BODIES}"' EXIT

git log "${COMMIT_RANGE_FOR_LOG}" --pretty=tformat:"%s" 2>/dev/null > "${_TMP_SUBJECTS}"
git log "${COMMIT_RANGE_FOR_LOG}" --pretty=tformat:"%b" 2>/dev/null > "${_TMP_BODIES}"

MERGE_COUNT=0
COMMIT_COUNT=0
BREAKING_LIST=()
TYPE_LINES=""

while IFS= read -r subject; do
  [[ -z "${subject}" ]] && continue
  COMMIT_COUNT=$((COMMIT_COUNT + 1))

  if echo "${subject}" | grep -qE '^Merge (pull request|branch)'; then
    MERGE_COUNT=$((MERGE_COUNT + 1))
    continue
  fi

  # Extract conventional commit type (with optional ! for breaking)
  raw_type="$(echo "${subject}" | sed -nE 's/^([a-z]+)(\([^)]+\))?(!)?: .*/\1\3/p')"
  if [[ -n "${raw_type}" ]]; then
    base_type="${raw_type%!}"
    TYPE_LINES="${TYPE_LINES}${base_type}"$'\n'
    if echo "${subject}" | grep -qE '^[a-z]+(\([^)]+\))?!:'; then
      BREAKING_LIST+=("${subject}")
    fi
  fi
done < "${_TMP_SUBJECTS}"

# Also collect BREAKING CHANGE footers from bodies
while IFS= read -r line; do
  if echo "${line}" | grep -qiE '^BREAKING[- ]CHANGE:'; then
    BREAKING_LIST+=("${line}")
  fi
done < "${_TMP_BODIES}"

# ── gh auth check ─────────────────────────────────────────────────────────────

GH_AUTH_OK="false"
GH_AUTH_ERROR=""
if command -v gh >/dev/null 2>&1; then
  GH_AUTH_OUTPUT="$(gh auth status 2>&1)"
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

# ── Output ────────────────────────────────────────────────────────────────────

section "Context"
printf "Repository:     %s\n" "${TOPLEVEL}"
printf "HEAD:           %s (%s)\n" "${CURRENT_BRANCH}" "${HEAD_SHA}"
printf "Last tag:       %s\n" "${LAST_TAG:-none}"
printf "Suggested tag:  %s\n" "${SUGGESTED_TAG}"

section "Release Readiness"
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

echo "✓ HEAD is not tagged (clear to release)"
printf "✓ %d commit(s) since %s\n" "${COMMIT_COUNT}" "${LAST_TAG:-beginning}"

section "Bump Analysis"
printf "Bump level: %s\n" "${BUMP_LEVEL}"
printf "Reason:     %s\n" "${BUMP_REASON}"

section "Breaking Changes"
if [[ "${#BREAKING_LIST[@]}" -eq 0 ]]; then
  echo "(none)"
else
  for item in "${BREAKING_LIST[@]}"; do
    echo "  ${item}"
  done
fi

section "Commit Type Counts"
if [[ -z "${TYPE_LINES}" ]]; then
  echo "(no conventional commits found)"
else
  echo "${TYPE_LINES}" | sort | uniq -c | sort -rn | while read -r count type; do
    printf "  %-12s %d\n" "${type}:" "${count}"
  done
fi
if [[ "${MERGE_COUNT}" -gt 0 ]]; then
  printf "  %-12s %d  (excluded from bump analysis)\n" "merge:" "${MERGE_COUNT}"
fi

section "Commits since ${LAST_TAG:-beginning}"
while IFS= read -r full_hash; do
  [[ -z "${full_hash}" ]] && continue
  short_hash="$(git log -1 --pretty=tformat:"%h" "${full_hash}")"
  subject="$(git log -1 --pretty=tformat:"%s" "${full_hash}")"
  body="$(git log -1 --pretty=tformat:"%b" "${full_hash}")"

  if echo "${subject}" | grep -qE '^Merge (pull request|branch)'; then
    echo "  [merge] ${short_hash} ${subject}"
  else
    echo "  ${short_hash} ${subject}"
    if [[ -n "${body}" ]]; then
      while IFS= read -r bline; do
        [[ -z "${bline}" ]] && continue
        echo "    ${bline}"
      done <<< "${body}"
    fi
  fi
done < <(git log "${COMMIT_RANGE_FOR_LOG}" --pretty=tformat:"%H" 2>/dev/null)
