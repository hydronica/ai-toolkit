---
name: local-pr-review
description: >-
  Local PR-style code review against CONTRIBUTING.md, AGENTS.md, and project
  Cursor rules. Use when the user runs /review, @local-pr-review, or asks for
  a local pull request review before opening a PR.
disable-model-invocation: true
---

# Local PR review

**Load:** Invoke **`@local-pr-review`** or **`/review`** before opening a PR. Report-only — do not fix code unless the user asks.

Mimics a pull request review locally by checking changes against `CONTRIBUTING.md`, `AGENTS.md`, and installed project rules (for example `go-standards`, `go-testing`, `go-project-structure`).

## Procedure

1. **Confirm target repo** — workspace Git root. Stop if not inside a repository.
2. **Choose diff scope**
   - Default: **branch changes** — committed range vs integration base plus staged and unstaged working tree.
   - **Uncommitted only** when the user asks to review dirty/local/working-tree changes.
3. **Collect context** — run `review_sum.sh` from the repository under review:
   - Prefer: `bash "${HOME}/.cursor/ai-toolkit/review_sum.sh"`
   - From a clone of this repo before install: `bash scripts/review_sum.sh`
   - If the script is missing, tell the user to run `install.sh` from ai-toolkit.
   - Pass `--scope uncommitted` for uncommitted-only review.
   - Pass `--base <ref>` when the user names a specific integration branch.
4. **Stop on empty diff** — if `review_sum.sh` reports zero changed files, say there is nothing to review and stop.
5. **Read policy docs** — read every path listed under **Policy documents** in full (not just the paths).
6. **Read applicable rules** — read every file under **read for review** in full. When a hub rule references a companion file (for example `go-project-structure.md`), read that too.
7. **Review the diff** — read changed files and enough surrounding context to judge correctness. Map each finding to a **source** (rule name, `CONTRIBUTING.md` section, or `AGENTS.md` heading).
8. **Produce the report** using the output template below.

## Review dimensions

Address each dimension explicitly in **Compliance** and **Findings**:

| Dimension | Sources |
|-----------|---------|
| Process / contribution | `CONTRIBUTING.md` |
| Repo-specific agent policy | `AGENTS.md` (root and nested along changed paths) |
| Go production code | `go-standards.mdc`, `go-project-structure.md` |
| Go tests | `go-testing.mdc`; for regressions, `@red-green-bug-fix` expectations |
| Always-on behavior | `agent-behavior.mdc` and other `alwaysApply` rules |

## Severity rubric

- **Blocker** — must fix before merge (rule violation, missing tests for behavior change, `CONTRIBUTING.md` breach)
- **Suggestion** — should consider (style, naming, test placement, clearer structure)
- **Nit** — optional polish

## Output template

Use this structure exactly:

```markdown
# Local PR Review

## Summary
[1–3 sentences: what changed, overall readiness]

## Compliance
| Check | Status | Notes |
|-------|--------|-------|
| CONTRIBUTING.md | pass/fail/n-a | … |
| AGENTS.md | pass/fail/n-a | … |
| Rules applied | pass/fail/n-a | list rule names checked |

## Findings
| Severity | Location | Source | Finding |
|----------|----------|--------|---------|
| Blocker | path:line | go-testing | … |

## Test plan
[Commands or manual checks; suggest `go test ./path` when Go packages changed]

## Verdict
[Approve / Approve with nits / Request changes — include blocker count]
```

Sort findings by severity (Blocker first, then Suggestion, then Nit).

## Edge cases

- **Rules not installed** — `review_sum.sh` may list `~/.cursor/ai-toolkit/rules-source/` as fallback. Warn that project rules may be missing; suggest `install_rules`. Still review against the listed fallback files.
- **Nested `AGENTS.md`** — include every `AGENTS.md` discovered along changed paths.
- **Bug/security depth** — this skill checks standards and policy, not exhaustive bug hunting. If the user wants deeper passes, point them to Bugbot (`/review-bugbot`) or Security Review (`/review-security`).
- **No auto-fix** — do not change code or rerun review unless the user explicitly asks.

## Examples

**Branch review (default):**

```bash
bash "${HOME}/.cursor/ai-toolkit/review_sum.sh"
```

**Uncommitted changes only:**

```bash
bash "${HOME}/.cursor/ai-toolkit/review_sum.sh" --scope uncommitted
```

**Custom base:**

```bash
bash "${HOME}/.cursor/ai-toolkit/review_sum.sh" --base origin/develop
```
