Install ai-toolkit Cursor rules into the **open project** at `.cursor/rules/ai-toolkit/`, filtered by detected language stacks (and org rules when defined in the manifest). Registers the project for automatic sync on future `install.sh` runs unless `--no-register` is passed.

## Steps

1. **Confirm the target project** is the workspace you intend (Git root of the open folder). If the user named a different directory, pass `--project <path>` to the script.

2. **Run the install script** from the project (or with `--project`):
   - Prefer: `bash "${HOME}/.cursor/ai-toolkit/install-rules.sh" --filter auto`
   - If that path is missing, instruct the user to run `install.sh` from the ai-toolkit repo first.
   - Use `--filter go` (or another stack from `rules/manifest.sh`) when the user asks for a specific language pack, or when `--filter auto` detects no stacks.
   - Use `--filter org` for org-wide rules only.
   - Use `--filter all` to install every stack defined in the manifest.
   - Use `--dry-run` first when the user wants to preview what would be installed.
   - Add `--copy` to force a copy instead of symlinks (symlinks are the default when the source is local/canonical).
   - Add `--no-register` when the user does not want the project tracked for automatic sync.

3. **Registry and multi-project commands** (when the user asks to manage or sync across projects):
   - **List:** `install-rules.sh --list`
   - **Sync all registered:** `install-rules.sh --sync-all` (also runs automatically after `install.sh` unless `--no-sync-rules`)
   - **Unregister:** `install-rules.sh --unregister --project <path>` (rules on disk unchanged)
   - **Purge one project:** `install-rules.sh --purge --project <path>`
   - **Purge all registered:** `install-rules.sh --purge-all` (or `uninstall.sh --purge-project-rules` before global uninstall)

4. **Interpret the script output**
   - **Filter `auto`:** installs org rules (if any) plus stacks detected in the project (e.g. `go.mod` → Go rules). If no stack is detected and there are no org rules, the script exits with an error — tell the user and rerun with `--filter go` (or another stack) if appropriate.
   - Report the installed file list and target path `.cursor/rules/ai-toolkit/`.
   - Remind the user to reload the workspace in Cursor and check **Customize → Rules**.

5. **Do not** install rules to `~/.cursor/rules/` — user-level rule files are not supported by Cursor. Project rules only.

## Registry

Stack and org membership is defined in `rules/manifest.sh` in the ai-toolkit repository. Registered projects are stored at `~/.cursor/ai-toolkit/projects.registry`:

```text
# columns: absolute_path|filter|mode
/path/to/project|auto|link
```

When adding new language rules to the toolkit, update `manifest.sh`; do not hard-code file lists in this command.
