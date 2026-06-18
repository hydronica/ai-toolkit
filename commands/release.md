Generate a GitHub release for the current repository using semantic versioning.

## Steps

1. **Gather release context** with `release_sum.sh`. Run it from the repository you are releasing:
   - Prefer: `bash "${HOME}/.cursor/ai-toolkit/release_sum.sh"`
   - If that path is missing, instruct the user to run `install.sh` from the ai-toolkit repo.
   - **IMPORTANT:** Use `required_permissions: ["full_network"]` when running via Shell tool (needed for `gh auth status`).
   - Pass `--pre-release <suffix>` only if the user explicitly requested a pre-release (e.g. `rc.1`, `beta.1`).
   - If the script exits with "HEAD is already tagged — nothing to release", stop and report this to the user.

2. **Review and validate the suggested bump** by reading the full commit list (subjects and bodies):
   - The script derives the bump level from conventional commit prefixes on the subject line only.
   - You must also read commit bodies for intent signals: words like "feature", "feat", "new", "add support for", "breaking", "remove", "deprecate" in body bullets may indicate a higher bump than the script suggested.
   - If the commit bodies suggest a higher bump than the script's suggestion, recommend the higher level and explain why.
   - If subjects are non-conventional but bodies are clear, use your judgment and note the discrepancy.

3. **Present the release summary** and wait for user confirmation before proceeding:
   - Suggested tag (and your validated bump level if it differs from the script)
   - Breaking changes, if any, listed explicitly
   - Full commit list since the last tag
   - Note if this is the first release (no prior tags → defaulting to `v0.1.0`)

4. **Confirm the tag** — the user may accept the suggested tag or provide an override.

5. **Create a draft release**:
   ```
   gh release create <tag> --generate-notes --draft --title "<tag>"
   ```
   - **IMPORTANT:** Use `required_permissions: ["full_network"]` when running via Shell tool.
   - Always use `--draft`; never publish directly.

6. **Show the draft URL** and ask the user to review it on GitHub before publishing.
   To publish after review:
   ```
   gh release edit <tag> --draft=false
   ```

---

## Rules

- **Draft first, always.** Never run `gh release create` without `--draft`.
- **No pre-release suffix** unless the user explicitly asks for one.
- **First release default.** If there are no prior tags, suggest `v0.1.0` unless the user specifies otherwise.
- **Nothing to release.** If HEAD is already tagged, stop immediately and tell the user.
- **Do not invent commits.** Only describe what appears in the `release_sum.sh` output.
- **Breaking changes must be surfaced.** If the bump is `major`, call out each breaking change explicitly before asking the user to confirm.
- **gh auth required.** If the script reports authentication failure, tell the user to run `gh auth login` and stop.
