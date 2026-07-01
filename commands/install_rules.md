Install ai-toolkit Cursor rules into the **open project** at `.cursor/rules/ai-toolkit/`, filtered by org policy and detected language stacks.

## Steps

1. **Confirm the target project** is the workspace you intend (Git root of the open folder). If the user named a different directory, pass `--project <path>` to the script.

2. **Run the install script** from the project (or with `--project`):
   - Prefer: `bash "${HOME}/.cursor/ai-toolkit/install-rules.sh" --filter auto`
   - If that path is missing, instruct the user to run `install.sh` from the ai-toolkit repo first.
   - Use `--filter go` (or another stack from `rules/manifest.sh`) when the user asks for a specific language pack.
   - Use `--filter org` for org-wide rules only (`rule-attribution.mdc`).
   - Use `--filter all` to install every stack defined in the manifest.
   - Use `--dry-run` first when the user wants to preview what would be installed.
   - Add `--copy` to force a copy instead of symlinks (symlinks are the default when the source is local).

3. **Interpret the script output**
   - **Filter `auto`:** installs org rules plus stacks detected in the project (e.g. `go.mod` → Go rules). If no stack is detected, only org rules are installed — tell the user and offer `--filter go` (or another stack) if appropriate.
   - Report the installed file list and target path `.cursor/rules/ai-toolkit/`.
   - Remind the user to reload the workspace in Cursor and check **Customize → Rules**.

4. **Do not** install rules to `~/.cursor/rules/` — user-level rule files are not supported by Cursor. Project rules only.

## Manifest

Stack and org membership is defined in `rules/manifest.sh` in the ai-toolkit repository. When adding new language rules to the toolkit, update that manifest; do not hard-code file lists in this command.
