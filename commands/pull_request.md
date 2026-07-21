Generate a pull request for the current Git branch. Use a **predefined repository or organization PR template** when `pr_sum.sh` reports one; otherwise use the **# PR Template** section at the bottom as the PR body (fill checkboxes and **### Summary** from context).

## Steps

1. **Collect context** with `pr_sum.sh` (summarizes the current branch against the integration base, **checks gh auth**, **verifies branch push status**, and **detects PR templates**). Run it from the **repository you are opening the PR for** (the project worktree, usually the workspace root):
   - Prefer: `bash "${HOME}/.cursor/ai-toolkit/pr_sum.sh"` (installed by this repo’s `install.sh`, which copies or links `scripts/` to `~/.cursor/ai-toolkit/`).
   - If that path is missing instruct the user to run 'install.sh' from the ai-toolkit repo
   - **IMPORTANT:** When running this script via Shell tool, use `required_permissions: ["full_network"]` because the script needs network access to check GitHub authentication via `gh auth status`

2. **Decide with the user before opening a PR**
   - If the summary indicates merge/rebase conflicts or that the branch is far behind the base, **stop and ask** how they want to proceed (rebase, merge, or abort).
   - Fill **### Summary** (or the repo/org template’s summary section) from commit messages and changed paths; keep checklist items honest.
   - If changed files do not match the commit messages, say so and offer to base the summary on a full diff if they want.

3. **Create the PR** only when step 2 needs no further user decision (or the user explicitly said to continue despite unpushed commits or similar).
   - Ensure GitHub CLI works: if `gh auth status` is not successful, tell the user to run `gh auth login` instead of creating the PR.
   - **PR body source (from the `PR template` section of `pr_sum.sh`):**
     - **Predefined template found:** fill that template (local path, or fetch the org default first if the script shows a fetch command), then run:
       `gh pr create --title "<concise title, at most five words>" <template flag from pr_sum.sh>`
       Use `--template <path>` with the repo’s template file. Do **not** use the ai-toolkit **# PR Template** below.
     - **No predefined template:** write the completed **# PR Template** block (from this file) to a temporary file and run:
       `gh pr create --title "<concise title, at most five words>" --body-file <that-file>`
   - Use `--draft` when the change set is incomplete or the user asked for a draft. Use `--base <branch>` when the target branch is not the repository default.
   - **IMPORTANT:** Use `required_permissions: ["full_network"]` when running `gh pr create` via Shell tool

---

# PR Template

Use this section **only when `pr_sum.sh` reports no repository or organization PR template**.

### Quality check

- [ ] Documentation included
- [ ] Test coverage

### Summary

<generated paragraphs and/or bullets>
