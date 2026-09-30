---
name: branch-cleanup
description: >-
  Audit GitHub branches: keep default/active/open PRs, flag merged branches
  for removal, score stale or closed-PR branches 0–100. Reports remote and
  local scopes. Use when the user asks to clean up branches, prune stale
  remotes, or audit branch hygiene.
disable-model-invocation: true
---

# Branch cleanup

**Load:** Invoke **`/ai-toolkit/branch_cleanup`** or **`@branch-cleanup`**. Report-first — do not delete branches unless the user explicitly confirms the scope.

Audits remote and local branches into Keep / Safe remove / Review (scored 0–100). Higher review scores mean a stronger case for removal.

## Procedure

1. **Confirm target repo** — workspace Git root. Stop if not inside a repository.
2. **Run `branch_sum.sh`** from the repository under audit:
   - Prefer: `bash "${HOME}/.cursor/ai-toolkit/branch_sum.sh"`
   - From a clone of this repo before install: `bash scripts/branch_sum.sh`
   - If the script is missing, tell the user to run `install.sh` from ai-toolkit.
   - Shell tool: use `required_permissions: ["full_network"]` so `gh` and fetch work.
   - Pass `--remote`, `--repo`, `--base`, `--scope`, `--keep-pattern`, or `--min-score` when the user specifies them.
   - Default scope is **both** (remote and local).
3. **Present the report** using the output template below (Summary, Keep, Safe remove, Review, Suggested commands).
4. **Do not delete** unless the user explicitly confirms scope (for example “all safe-remove”, “score ≥ 90”, or named branches).
5. **On confirmation:**
   - Confirm **remote** deletes and **local** deletes separately unless the user names both in one approval.
   - Remote: `git push {remote} --delete {branch}` for the user-approved list only (never `upstream`).
   - Local: `git branch -d {branch}` for merged safe-remove candidates. Use `git branch -D` only when the user names unmerged branches.
   - Never delete the current checkout or a branch checked out in another worktree.
6. **Re-run** `branch_sum.sh` to verify.

## Safety rules (must follow)

- Never delete default, protected, or open-PR branches
- Never delete the user’s current branch without explicit warning
- Default to dry-run; merged-only suggestions still require confirmation
- For scores 30–89, list branch + PR link + commits ahead before asking
- Fork workflow: only delete on remotes the user owns (`origin`), not `upstream` parent
- Confirm remote deletes and local deletes separately unless the user names both
- Local merged suggestions use `git branch -d`; `git branch -D` only when the user names unmerged branches

## Score interpretation

| Range | Meaning |
|-------|---------|
| 90–100 | Recommend delete (still requires user confirmation) |
| 60–89 | Probably stale; show PR link / commit summary before asking |
| 30–59 | Unclear; default to keep unless user opts in |
| 0–29 | Likely keep |

## Output template

```markdown
# Branch cleanup audit

## Summary
[Counts and repo context — remote and local]

## Remote: Keep
| Branch | Reason |

## Remote: Safe to remove (merged)
| Branch | Merged PR | Last commit |

## Remote: Review (rated)
| Score | Branch | Last commit | Ahead | PR | Reasons |

## Local: Keep
| Branch | Reason |

## Local: Safe to remove (merged)
| Branch | Merged PR | Last commit |

## Local: Review (rated)
| Score | Branch | Last commit | Ahead | PR | Reasons |

## Suggested commands
[dry-run git commands — not executed]

## Verdict
[Awaiting confirmation / Ready to delete N remote and M local branches / Nothing to do]
```

When `--scope remote` or `--scope local` was used, omit the unused sections.

## Examples

**Full audit (default):**

```bash
bash "${HOME}/.cursor/ai-toolkit/branch_sum.sh"
```

**Remote only:**

```bash
bash "${HOME}/.cursor/ai-toolkit/branch_sum.sh" --scope remote
```

**Local only, no fetch:**

```bash
bash "${HOME}/.cursor/ai-toolkit/branch_sum.sh" --scope local --no-fetch
```

**JSON for structured follow-up:**

```bash
bash "${HOME}/.cursor/ai-toolkit/branch_sum.sh" --json
```
