Audit remote and local Git branches for cleanup candidates. Report-first — delete only after explicit user confirmation.

## When to use

- Clean up stale remote branches after merged PRs
- Prune local branches that are fully merged or whose upstream is gone
- Audit branch hygiene before asking an agent to delete anything

## Inputs

- **Scope:** `both` (default), `remote`, or `local` — passed to `branch_sum.sh` as `--scope`
- **Remote:** optional audited remote (default `origin`); delete suggestions never target `upstream`
- **Base:** optional integration ref for merged/ahead checks; passed as `--base`
- **Keep patterns:** optional globs (for example `release/*`); passed as repeatable `--keep-pattern`
- **Min score:** optional floor for review rows; passed as `--min-score`

## Steps

1. **Load the skill** — follow [`@branch-cleanup`](../skills/branch-cleanup/SKILL.md) for the full workflow, safety rules, score interpretation, and output template.

2. **Collect context** with `branch_sum.sh` from the **repository you are auditing** (usually the workspace root):
   - Prefer: `bash "${HOME}/.cursor/ai-toolkit/branch_sum.sh"` (installed by this repo’s `install.sh`, which copies or links `scripts/` to `~/.cursor/ai-toolkit/`).
   - From a local ai-toolkit clone before install: `bash scripts/branch_sum.sh`
   - If that path is missing, instruct the user to run `install.sh` from the ai-toolkit repo.
   - Use `required_permissions: ["full_network"]` for `gh` and fetch.
   - Pass `--scope`, `--remote`, `--repo`, `--base`, `--keep-pattern`, `--min-score`, or `--no-fetch` when the user asks.

3. **Report**
   - Present remote and local Keep / Safe remove / Review sections (omit unused scope when filtered).
   - Include dry-run suggested commands; do not execute them yet.
   - Emit the skill output template, including Verdict.

4. **Delete only on explicit approval**
   - Confirm remote deletes and local deletes separately unless the user names both.
   - Remote: `git push {remote} --delete {branch}` for approved names only.
   - Local: `git branch -d` for merged candidates; `git branch -D` only when the user names unmerged branches.
   - Never delete default, protected, open-PR, current-checkout, or worktree-checkout branches.
   - Re-run `branch_sum.sh` after deletes to verify.

## Output

Markdown audit with:

- Summary (remote and local counts, linkage)
- Keep / Safe remove / Review tables per scope
- Suggested dry-run commands
- Verdict (`Awaiting confirmation`, ready-to-delete counts, or nothing to do)

Report-only until the user confirms deletions.
