# AI-Toolkit documentation

Platform-specific notes for consuming assets from this repository.

## Cursor

### How asset types are discovered

`install.sh` installs commands, skills, agents, and scripts under your Cursor home directory:

| Asset | Install path | Cursor support |
|-------|--------------|----------------|
| Skills | `~/.cursor/skills/ai-toolkit/` | Supported — Cursor scans user-level skill directories |
| Commands | `~/.cursor/commands/ai-toolkit/` | Supported — Cursor scans user-level command directories |
| Agents | `~/.cursor/agents/ai-toolkit/` | Supported at user level |
| Scripts | `~/.cursor/ai-toolkit/` | Shell helpers (e.g. `install-rules.sh`) |
| User rules | `~/.cursor/rules/user-ai-toolkit.txt` | **Not loaded by Cursor** — see [Where user rules are stored](#where-user-rules-are-stored) |
| Project rules | *(per project)* | **Project-scoped** `.mdc` — see below |

Skills and commands work globally after `install.sh`. **Project rules** are installed per repo with `install-rules.sh` or the **`install_rules`** command.

### Where user rules are stored

Cursor’s **supported** user-rule path is **Settings → Rules → User Rules** (plain text in the UI). That content is **not** a normal file under `~/.cursor/rules/` and does **not** appear in **Customize → Rules** the way project `.mdc` rules do.

**Cloud sync:** User rules entered in **Settings → Rules → User Rules** sync to your Cursor account and appear on other machines signed into the same account (verified Jul 2026). The rule **body** is not stored in plaintext locally; only UI metadata is cached on disk (see below).

Local investigation on macOS (Jul 2026) found:

| What | Where | Notes |
|------|--------|--------|
| **Rule body text (User Rules UI)** | *Cursor cloud (account sync)* | Syncs across machines on the same account; **not** found as editable plaintext under `~/.cursor/` |
| **Rule metadata (local UI cache)** | `~/Library/Application Support/Cursor/User/workspaceStorage/<workspace-id>/state.vscdb` | SQLite `ItemTable`, key `workbench.customize.primitiveSourceSnapshot.rules.v3` — descriptors only (name, `scope: "user"`, etc.) |
| **Global Cursor state** | `~/Library/Application Support/Cursor/User/globalStorage/state.vscdb` | Large `applicationUser` JSON blob — models, composer prefs, etc.; **no user rule text** observed |
| **Editor settings** | `~/Library/Application Support/Cursor/User/settings.json` | No user rules |
| **Files in `~/.cursor/rules/`** | e.g. `user-ai-toolkit.txt` | **Ignored** by Cursor’s rules UI and agent rule loader |

**Workspace SQLite example** — user-scoped rule descriptor (metadata only):

```bash
sqlite3 "$HOME/Library/Application Support/Cursor/User/workspaceStorage/<workspace-id>/state.vscdb" \
  "SELECT value FROM ItemTable WHERE key = 'workbench.customize.primitiveSourceSnapshot.rules.v3';"
```

Example value (truncated):

```json
{
  "descriptors": [{
    "kind": "rule",
    "name": "Home workspace guidance",
    "scope": "user",
    "sourceLabel": "User",
    "filePath": "Home workspace guidance",
    "isWorkspaceViewable": false
  }]
}
```

`filePath` here is a **display id**, not a path on disk. Project rules in the same key use real paths (e.g. `/path/to/project/.cursor/rules/ai-toolkit/go-standards.mdc`) because the `.mdc` file exists in the repo.

To find `<workspace-id>` for a folder, read `workspace.json` in each `workspaceStorage/*` subdirectory and match your project URI.

**Implication for ai-toolkit:** `install.sh` copies [`rules/user-ai-toolkit.txt`](rules/user-ai-toolkit.txt) to `~/.cursor/rules/user-ai-toolkit.txt` as a **versioned source-of-truth** for your team or repo. To activate in Cursor, paste (or merge) into **Settings → User Rules** on each account — or rely on cloud sync after the first paste. Cursor does not load the installed file as a rule on its own.

### Where Cursor expects rules

Cursor supports three rule sources ([Rules docs](https://cursor.com/docs/rules)):

| Type | Location | Format |
|------|----------|--------|
| **Project rules** | `<project>/.cursor/rules/` | `.mdc` with YAML frontmatter |
| **User rules** | **Settings → Rules → User Rules** (UI) | Plain text only — no `.mdc`, no globs; **synced via Cursor account** |
| **Imported rules** | `<project>/.cursor/rules/imported/<repoName>/` | `.mdc`, synced via **Remote rule (GitHub)** in Cursor Settings |

**User-level file rules are not officially supported.** Cursor staff have confirmed that global loading of `.mdc` from `~/.cursor/rules` is [not supported yet](https://forum.cursor.com/t/user-rules-are-not-recognized-from-folder-cursor-rules/144739). Plain `.txt` or `.md` files in that directory are likewise **not** picked up. Use **Settings → User Rules** or project `.mdc` rules instead.

### Installing ai-toolkit user rules

`install.sh` copies or symlinks [`rules/user-ai-toolkit.txt`](rules/user-ai-toolkit.txt) to `~/.cursor/rules/user-ai-toolkit.txt`. Expand that file for org-wide preferences (e.g. rule attribution). `uninstall.sh` removes it.

**To apply the content in Cursor today:** paste (or merge) from `~/.cursor/rules/user-ai-toolkit.txt` into **Settings → Rules → User Rules** on one machine. After that, cloud sync should propagate to other machines on the same account. The file install alone does not register a rule.

### Installing ai-toolkit rules in a project

**Recommended — script or command**

```bash
# From your project directory (auto-detect stacks from rules/manifest.sh):
~/.cursor/ai-toolkit/install-rules.sh --filter auto

# Explicit Go stack:
~/.cursor/ai-toolkit/install-rules.sh --filter go

# Preview only:
~/.cursor/ai-toolkit/install-rules.sh --filter auto --dry-run
```

Or invoke the **`install_rules`** command in Agent chat ([`commands/install_rules.md`](commands/install_rules.md)).

Rules are installed to `<project>/.cursor/rules/ai-toolkit/`. Which files are included is controlled by [`rules/manifest.sh`](rules/manifest.sh):

- **STACK_NAMES** plus **STACK_\<name\>_DETECT** and **STACK_\<name\>_RULES** per language pack

Add new language packs by extending `manifest.sh` and adding rule files under `rules/`. User-level rules belong in `rules/user-ai-toolkit.txt` and are installed by `install.sh`, not `install-rules.sh`.

**Alternative — Remote rule (GitHub) import**

Cursor Settings → Rules → **Remote rule (GitHub)** syncs into `<project>/.cursor/rules/imported/<repoName>/`.

**User rules (Settings UI)**

Plain-text global preferences in **Settings → Rules → User Rules**. No `globs` or `alwaysApply`. **Body text is account-synced in the cloud**; local `state.vscdb` caches descriptors only (see [Where user rules are stored](#where-user-rules-are-stored)).

### Rule file format

- **`.mdc` only** — Project rules must use the `.mdc` extension with valid YAML frontmatter (`---` opening and closing markers). Plain `.md` files in `.cursor/rules/` are **ignored** by Cursor's rules system (no frontmatter to specify `description`, `globs`, or `alwaysApply`).
- **Companion `.md` files** — Files such as `rules/go-project-structure.md` are reference material linked from hub `.mdc` rules; they are copied with the Go stack but are not loaded as rules on their own.
- **Nested folders inside one `.cursor/rules/` directory** — Subfolders such as `.cursor/rules/ai-toolkit/*.mdc` may be unreliable. Cursor discovers rules by scanning `.cursor/rules/`; nested paths within a single rules folder [often fail to trigger](https://forum.cursor.com/t/why-don-t-nested-cursor-rules-directories-work-for-mdc-rules/100859). This toolkit uses `.cursor/rules/ai-toolkit/` as an experiment; if rules do not appear, try a flat layout.

Frontmatter controls when a rule applies ([Rule anatomy](https://cursor.com/docs/rules#rule-anatomy)):

| `alwaysApply` | `description` | `globs` | Behavior |
|---------------|---------------|---------|----------|
| `true` | — | — | Always included |
| `false` | — | set | Attached when a matching file is in context |
| `false` | set | — | Agent may include when description matches the task |
| `false` | — | — | Manual `@`-mention only |

### Known issues and quirks

**`~/.cursor/rules/` files (including ai-toolkit install)**

- Plain `.txt`, `.md`, and `.mdc` files in `~/.cursor/rules/` are **not** loaded as user rules and **do not** appear in **Customize → Rules**.
- Project rules must live under `<project>/.cursor/rules/` as `.mdc` with YAML frontmatter.

**Agents Window (Glass layout)**

- Open a **project folder as a workspace** before chatting. Without a workspace, the agent may start from your home directory and will not load project rules from `.cursor/rules/`.
- File-backed rules and Settings UI visibility have [known bugs](https://forum.cursor.com/t/cursor-settings-don-t-list-or-reflect-active-user-rules-and-plugins/156864) in the Agents Window; project-scoped rules in an open workspace are the most reliable path.

**Rules vs other AI features**

- Rules apply to **Agent (Chat)** only — not Cursor Tab, Inline Edit (Cmd/Ctrl+K), or other features ([FAQ](https://cursor.com/docs/rules)).

**Precedence when guidance conflicts**

Team Rules → Project Rules → User Rules ([Team Rules](https://cursor.com/docs/rules#team-rules)).

### Verifying rules are active

**Project rules** (under `<project>/.cursor/rules/ai-toolkit/`):

1. Open **Customize → Rules** — project `.mdc` files should list with their types.
2. In Agent chat with a matching file in context, scoped rules (e.g. `globs: "**/*.go"`) should attach.
3. In the Agents Window, confirm the **project folder is open as a workspace** before testing.

**User rules** (Settings → User Rules):

1. Confirm text in **Settings → Rules → User Rules** on each machine (or sign into the same Cursor account and wait for cloud sync).
2. Optional: inspect workspace `state.vscdb` for a `scope: "user"` descriptor under `workbench.customize.primitiveSourceSnapshot.rules.v3` — confirms local UI cache only, not the full rule body.
3. `~/.cursor/rules/user-ai-toolkit.txt` from `install.sh` is for **distribution** only; paste into User Rules to activate and sync.

### References

- [Cursor Rules](https://cursor.com/docs/rules)
- [Importing rules](https://cursor.com/docs/rules#importing-rules)
- [Rule anatomy](https://cursor.com/docs/rules#rule-anatomy)
- [User rules not recognized from ~/.cursor/rules](https://forum.cursor.com/t/user-rules-are-not-recognized-from-folder-cursor-rules/144739)
- [Feature request: global ~/.cursor/rules .mdc support](https://forum.cursor.com/t/support-for-cursor-rules-for-global-mdc-rules/144819)

## Claude

*(Documentation for Claude-based workflows will be added here.)*
