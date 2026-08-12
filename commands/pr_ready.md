Check whether branch changes are ready to open a PR against project standards.

## When to use

- Before opening a PR on the current branch
- After finishing a feature branch and wanting a standards pass
- To review uncommitted (staged or unstaged) work only

## Inputs

- **Scope:** branch changes (default) or uncommitted only
- **Base:** optional integration branch (for example `origin/main`); passed to `review_sum.sh` as `--base`
- **Focus:** optional user message narrowing what to emphasize

## Steps

1. **Load the skill** — follow [`@pr-ready`](../skills/pr-ready/SKILL.md) for the full workflow, rubric, and output template.

2. **Collect context** with `review_sum.sh` from the **repository you are reviewing** (usually the workspace root):
   - Prefer: `bash "${HOME}/.cursor/ai-toolkit/review_sum.sh"` (installed by this repo’s `install.sh`, which copies or links `scripts/` to `~/.cursor/ai-toolkit/`).
   - From a local ai-toolkit clone before install: `bash scripts/review_sum.sh`
   - If that path is missing, instruct the user to run `install.sh` from the ai-toolkit repo.
   - Use `--scope uncommitted` when the user wants dirty-tree-only review.
   - Use `--base <ref>` when the integration branch is not the repository default.

3. **Review and report**
   - If changed file count is zero, say there is nothing to review and stop.
   - Read discovered `CONTRIBUTING.md`, `AGENTS.md`, and applicable rules in full.
   - Review the diff; map findings to their source (rule, contributing section, or agents heading).
   - Emit the report using the skill output template (compliance table, findings table, test plan, verdict).

## Output

Markdown report with:

- Summary
- Compliance table (`CONTRIBUTING.md`, `AGENTS.md`, rules applied)
- Findings table (Severity, Location, Source, Finding)
- Test plan
- Verdict (`Approve`, `Approve with nits`, or `Request changes`)

Report-only — do not fix findings unless the user asks.

## Edge cases

- No project rules under `.cursor/rules/` — review against `rules-source` fallback and suggest `install_rules`.
- For deep bug or security review, point the user to Cursor's `/review` (Bugbot or Security Review) instead of duplicating those passes.
