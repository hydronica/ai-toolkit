# GitHub branch cleanup: `branch_sum.sh` + `branch-cleanup` skill

Planning document for auditing and cleaning up stale GitHub branches in a repository. Delivers a report-first workflow (like `pr_sum.sh`) and an agent skill that deletes only after explicit user confirmation.

## Problem statement

Remote branches accumulate after merged PRs, abandoned experiments, and closed-but-not-merged work. Manual cleanup is tedious and error-prone:

- Easy to delete a branch tied to an **open PR** or the **default branch**
- **Merged** branches are obvious candidates but are often left behind when auto-delete is off
- **Closed, not merged** branches and **orphan** branches need judgment (age, unique commits, author)
- No bundled tool in ai-toolkit today summarizes branch hygiene before action

## Goals

1. **Report before delete** — gather data and present a structured audit; never delete silently
2. **Clear buckets** — keep vs safe-remove vs review (scored 0–100)
3. **Reuse repo patterns** — mirror `scripts/pr_sum.sh` conventions and the `pr-ready` skill workflow
4. **Agent-friendly** — human-readable sections plus optional `--json` for structured follow-up
5. **Safe defaults** — protect default, protected, open-PR, and current-checkout branches

## Non-goals (initial release)

- Bulk delete automation without user confirmation
- Org-wide multi-repo sweeps (single repo per invocation)
- GitHub Enterprise-only APIs beyond standard `gh` / REST / GraphQL
- Replacing `gh` extensions for branch cleanup (complementary, not a fork)

## Non-negotiable constraints

Any implementation must preserve:

1. **Read-only script** — `branch_sum.sh` never deletes branches; deletion is skill/command follow-up only after confirmation
2. **Public bootstrap compatibility** — script installs to `~/.cursor/ai-toolkit/` via existing `install.sh` (no new install path)
3. **Same ergonomics as sibling scripts** — runnable from any directory inside a git work tree; `set -euo pipefail`; `section()` output; `--help` / `--no-fetch`
4. **Fork awareness** — respect upstream vs origin patterns established in `pr_sum.sh` (do not suggest deleting branches on remotes the user does not own)
5. **gh optional degradation** — without `gh`, script can still list local/remote git branches and merged-into-default checks; PR metadata and protected-branch detection require `gh`

## Deliverables

| Artifact | Path | Role |
|----------|------|------|
| Audit script | `scripts/branch_sum.sh` | Fetch, classify, score; print report or JSON |
| Agent skill | `skills/branch-cleanup/SKILL.md` | Procedure, rubric interpretation, confirmation gates |
| Cursor command | `commands/branch_cleanup.md` | `/ai-toolkit/branch_cleanup` entry point |
| Docs | `README.md` | List script under gh-required helpers |

Installed paths after `install.sh`:

- `~/.cursor/ai-toolkit/branch_sum.sh`
- `~/.cursor/skills/ai-toolkit/branch-cleanup/SKILL.md` (via skills install)
- `~/.cursor/commands/ai-toolkit/branch_cleanup.md` (via commands install)

## Branch classification

### Bucket A — Keep (no score; always retain)

| Reason | Detection |
|--------|-----------|
| Default branch | `gh repo view --json defaultBranchRef` or `git symbolic-ref refs/remotes/{remote}/HEAD` |
| Protected branch | `gh api repos/{owner}/{repo}/branches/{branch}` → `protection` present |
| Open PR head | `gh pr list --state open --json headRefName` |
| Current checkout | `git rev-parse --abbrev-ref HEAD` |
| User keep patterns | `--keep-pattern 'release/*'` (repeatable flag) |

### Bucket B — Safe to remove (score = 100)

| Reason | Detection |
|--------|-----------|
| Merged PR branch | `gh pr list --state merged --json headRefName,mergedAt,number` |
| Fully merged into default (no PR) | `git merge-base --is-ancestor {branch_sha} {default_sha}` and not in Bucket A |

Note: many repos enable “Delete branch on merge”; Bucket B targets leftovers.

### Bucket C — Review (score 0–100)

Branches not in A or B:

- No associated PR
- Closed, not merged PR
- Stale with unique commits not in default
- Old feature / experiment branches

**Higher score = stronger case for removal.** Script prints score **and** reason tokens (e.g. `age:180d +25, ahead:3 -15`).

### Score interpretation (for skill report)

| Range | Meaning |
|-------|---------|
| 90–100 | Recommend delete (still requires user confirmation) |
| 60–89 | Probably stale; show PR link / commit summary before asking |
| 30–59 | Unclear; default to keep unless user opts in |
| 0–29 | Likely keep |

## Scoring rubric (v1)

Start at **50** for unknown branches. Apply adjustments; clamp to **0–100**.

| Signal | Δ score | Notes |
|--------|---------|-------|
| Open PR | → **0** (force keep) | Overrides other signals |
| Default / protected / current branch | → **0** | Hard keep |
| Merged PR exists | → **100** | Moves to Bucket B |
| Fully merged into default (git) | +35 | Even without PR metadata |
| Last commit > 180 days | +25 | |
| Last commit > 90 days | +15 | |
| Last commit < 14 days | −25 | Likely active |
| Closed PR, not merged, closed > 60 days | +20 | Abandoned |
| Closed PR, not merged, closed < 14 days | −15 | Might reopen |
| Unique commits ahead of default | −min(30, ahead) | Unique work |
| No PR ever associated | +10 | Orphan branch |
| Author = current user AND last commit < 30 days | −20 | User may still care |

Implement scoring in a single `score_branch()` function for maintainability.

## Script design: `scripts/branch_sum.sh`

### Usage

```text
Usage: scripts/branch_sum.sh [options]

  --remote <name>       Remote to audit (default: origin)
  --repo <owner/repo>   Override repo slug (default: from remote URL)
  --base <ref>          Integration base for merged/ahead checks (default: remote default branch)
  --keep-pattern <glob> Repeatable; branches matching glob are always kept
  --min-score <N>       Only show review branches with score >= N
  --json                Machine-readable JSON on stdout; suppress human sections
  --no-fetch            Skip git fetch --prune
  -h, --help            Show help
```

### Execution flow

1. **Prerequisites** — `git`; `gh` + `jq` required for full PR/protected metadata (warn and degrade if missing)
2. **Repo root** — `git rev-parse --show-toplevel`; `cd` there; `GIT_PAGER=cat`
3. **Resolve remote / slug** — reuse `parse_repo_slug()` from `pr_sum.sh` pattern
4. **Fetch** — `git fetch --prune {remote}` unless `--no-fetch`
5. **Load metadata**
   - Default branch (gh or symbolic-ref)
   - Protected branches (gh API; cache per run)
   - All remote heads: `git ls-remote --heads {remote}` or paginated `gh api`
   - PR index: `gh pr list --state all --limit 500 --json number,title,state,mergedAt,closedAt,headRefName,author,updatedAt`
6. **Per branch** — last commit date, SHA, ahead-of-base count, merged-into-base check, PR association (latest by `updatedAt`)
7. **Classify** — Bucket A / B / C + score + reason string
8. **Output** — sections below (or JSON)

### Output sections (human)

```text
=== Context ===
Repository, default branch, remote, fetch status, gh auth, branch counts

=== Summary ===
Keep: N | Safe remove: N | Review: N

=== Keep ===
branch    reason

=== Safe to remove (merged) ===
score  branch    merged PR    last commit

=== Review (rated) ===
score  branch    last commit  ahead  PR state/number  reasons

=== Suggested commands (dry-run) ===
# Remote delete examples (not executed):
git push origin --delete <branch> ...
# Local cleanup after remote delete:
git branch -d <branch> ...
```

### JSON shape (illustrative)

```json
{
  "repo": "owner/name",
  "default_branch": "main",
  "remote": "origin",
  "summary": { "keep": 3, "safe_remove": 18, "review": 26 },
  "keep": [{ "branch": "main", "reason": "default" }],
  "safe_remove": [{ "branch": "fix/typo", "score": 100, "merged_pr": 138 }],
  "review": [{
    "branch": "experiment/old-ui",
    "score": 82,
    "last_commit": "2024-03-01",
    "ahead": 12,
    "pr": { "number": 91, "state": "CLOSED", "merged": false },
    "reasons": ["age:180d +25", "closed-pr:45d +20", "ahead:12 -12"]
  }]
}
```

### Helpers to reuse from `pr_sum.sh`

| Helper | Purpose |
|--------|---------|
| `section()`, `die()`, `require_command()`, `usage()` | Shell structure |
| `parse_repo_slug()` | `owner/repo` from remote URL |
| `try_fetch()` | Prune fetch with warning on failure |
| `first_remote()`, `remote_repo_slug()` | Remote resolution |
| `gh auth` block | Same messaging as `pr_sum.sh` / `release_sum.sh` |

Extract shared helpers to `scripts/lib/git_github.sh` **only if** duplication exceeds ~80 lines; otherwise copy minimally to keep scope small.

## Skill design: `skills/branch-cleanup/SKILL.md`

### Frontmatter

```yaml
---
name: branch-cleanup
description: >-
  Audit GitHub branches: keep default/active/open PRs, flag merged branches
  for removal, score stale or closed-PR branches 0–100. Use when the user
  asks to clean up branches, prune stale remotes, or audit branch hygiene.
disable-model-invocation: true
---
```

### Procedure

1. Confirm git root and target repo (`--repo` / `--remote` if user specified)
2. Run `branch_sum.sh`:
   - Installed: `bash "${HOME}/.cursor/ai-toolkit/branch_sum.sh"`
   - Clone: `bash scripts/branch_sum.sh`
   - Shell tool: `required_permissions: ["full_network"]` for `gh`
3. Present report using fixed template (Summary, Keep, Safe remove, Review table, Suggested commands)
4. **Do not delete** unless user explicitly confirms scope (e.g. “all safe-remove”, “score ≥ 90”, or named branches)
5. On confirmation:
   - Remote: `git push {remote} --delete {branch}` (batch with user-approved list)
   - Local: `git branch -d` / `-D` only after remote delete or when user requests local-only
6. Re-run `branch_sum.sh` to verify

### Safety rules (must appear verbatim in skill)

- Never delete default, protected, or open-PR branches
- Never delete the user’s current branch without explicit warning
- Default to dry-run; merged-only suggestions still require confirmation
- For scores 30–89, list branch + PR link + commits ahead before asking
- Fork workflow: only delete on remotes the user owns (`origin`), not `upstream` parent

### Output template

```markdown
# Branch cleanup audit

## Summary
[Counts and repo context]

## Keep
| Branch | Reason |

## Safe to remove (merged)
| Branch | Merged PR | Last commit |

## Review (rated)
| Score | Branch | Last commit | Ahead | PR | Reasons |

## Suggested commands
[dry-run git commands — not executed]

## Verdict
[Awaiting confirmation / Ready to delete N branches / Nothing to do]
```

## Command design: `commands/branch_cleanup.md`

Mirror `commands/pr_ready.md`:

- **When to use** — “clean up branches”, “prune stale remotes”, “branch hygiene”
- **Steps** — load `@branch-cleanup` → run `branch_sum.sh` → report → confirm → delete
- **Output** — markdown audit; delete only on explicit user approval

## `install.sh` / README updates

1. **`install.sh`** — include `branch_sum.sh` in gh hint block (alongside `pr_sum.sh`, `release_sum.sh`)
2. **`README.md`** — document `branch_sum.sh` under bundled scripts; note gh + jq requirement for full features

No changes to `install-manifest.json` schema; new script is picked up with existing `scripts/` install.

## Edge cases

| Case | Handling |
|------|----------|
| Pagination (>500 PRs or >1000 branches) | Paginate `gh pr list` / GraphQL `refs`; document limits in `--help` |
| Branch renamed after PR | Match by latest PR `headRefName`; show PR number in output |
| Local-only branches | Separate “local only” section; no remote delete suggestion |
| Shallow clone | Warn like `pr_sum.sh`; merged-into-base checks may be incomplete |
| No `origin` remote | Use first remote or require `--remote` |
| Auth failure | Same errors as `release_sum.sh`; stop before scoring if PR data required |
| Monorepo / many stale bots | `--min-score` filters noise; `--keep-pattern` for release lines |

## GraphQL option (phase 2+)

For repos with many branches, replace N REST calls with one paginated query:

```graphql
query($owner: String!, $name: String!, $after: String) {
  repository(owner: $owner, name: $name) {
    defaultBranchRef { name }
    refs(refPrefix: "refs/heads/", first: 100, after: $after) {
      pageInfo { hasNextPage endCursor }
      nodes {
        name
        target {
          ... on Commit {
            committedDate
            oid
            author { user { login } }
          }
        }
        associatedPullRequests(first: 5, states: [OPEN, MERGED, CLOSED]) {
          nodes { number state merged closedAt title updatedAt }
        }
      }
    }
  }
}
```

Start with `gh pr list` + `git ls-remote` for v1; add GraphQL if performance becomes an issue.

## Implementation phases

### Phase 1 — Script core (MVP)

- [ ] Add `scripts/branch_sum.sh` with Context / Summary / Keep / Safe remove sections
- [ ] Default + open PR + merged PR detection
- [ ] `git merge-base --is-ancestor` for fully merged branches
- [ ] `--remote`, `--no-fetch`, `--help`
- [ ] Reuse `pr_sum.sh` shell patterns and gh auth messaging

**Acceptance:** Running on a real repo prints correct keep and safe-remove lists; no deletes.

### Phase 2 — Scoring and review bucket

- [ ] Implement `score_branch()` with rubric v1
- [ ] Review table with reason tokens
- [ ] `--min-score`, `--keep-pattern`
- [ ] Closed-not-merged PR signals

**Acceptance:** Ambiguous branches appear in Review with scores and explanations.

### Phase 3 — Agent surfaces

- [ ] `skills/branch-cleanup/SKILL.md`
- [ ] `commands/branch_cleanup.md`
- [ ] README + `install.sh` gh hint

**Acceptance:** `@branch-cleanup` / command runs script and produces templated report; refuses delete without confirmation.

### Phase 4 — Machine output and hardening

- [ ] `--json` output
- [ ] `--repo`, `--base` overrides
- [ ] Fork-aware remote selection (don’t suggest upstream deletes)
- [ ] Pagination for large repos
- [ ] Optional GraphQL bulk fetch

**Acceptance:** Agent can parse JSON; large repo smoke test completes without rate-limit failures.

### Phase 5 — Polish (optional)

- [ ] Extract shared bash helpers to `scripts/lib/git_github.sh` if duplication hurts maintenance
- [ ] `--local-only` mode for local branch audit without gh
- [ ] `--active-days N` to treat recently pushed branches as keep candidates
- [ ] Integration note in `docs/cursor.md` if needed

## Test plan

Manual verification (no automated bash tests in repo today):

1. **Repo with merged PR branches left on remote** — appear in Safe remove
2. **Open PR branch** — in Keep; score 0 if forced through rubric
3. **Protected default** — in Keep
4. **Closed unmerged PR, old** — Review with score ≥ 60
5. **Recent branch, commits ahead of main** — Review with low score
6. **`--no-fetch`** — runs offline against local refs; warns if stale
7. **No gh** — degrades gracefully (git-only merged check)
8. **Skill dry-run** — agent prints suggested `git push --delete` but does not run until user confirms

## Open questions

Resolve before or during Phase 2:

1. **“Active” definition** — open PR only, or also branches pushed within N days? *Proposal: open PR + current branch + optional `--active-days N` in Phase 5.*
2. **Delete scope default** — remote only, local only, or both? *Proposal: remote first; local `git branch -d` only after remote delete or explicit local confirmation.*
3. **Minimum gh scope** — read-only `repo` sufficient; document in README.

## References

- `scripts/pr_sum.sh` — section output, gh auth, fork/upstream patterns
- `scripts/release_sum.sh` — gh-required script, fetch semantics
- `skills/pr-ready/SKILL.md` — skill procedure + script invocation pattern
- `commands/pr_ready.md` — command entry point pattern
- `issues/issues_mage.md` — planning doc format for this repo
