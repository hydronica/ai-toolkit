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
| Rules | *(not installed by `install.sh`)* | **Project-scoped only** — see below |

Skills and commands work globally after `install.sh`. **Rules do not** — install them per project with `install-rules.sh` or the **`install_rules`** command.

### Where Cursor expects rules

Cursor supports three rule sources ([Rules docs](https://cursor.com/docs/rules)):

| Type | Location | Format |
|------|----------|--------|
| **Project rules** | `<project>/.cursor/rules/` | `.mdc` with YAML frontmatter |
| **User rules** | **Settings → Rules → User Rules** (UI) | Plain text only — no `.mdc`, no globs |
| **Imported rules** | `<project>/.cursor/rules/imported/<repoName>/` | `.mdc`, synced via **Remote rule (GitHub)** in Cursor Settings |

**User-level file rules are not officially supported.** Cursor staff have confirmed that global loading of `.mdc` from `~/.cursor/rules` is [not supported yet](https://forum.cursor.com/t/user-rules-are-not-recognized-from-folder-cursor-rules/144739). Use project rules instead.

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

- **ORG_RULES** — always installed (e.g. `rule-attribution.mdc`)
- **STACK_NAMES** plus **STACK_\<name\>_DETECT** and **STACK_\<name\>_RULES** per language pack

Add new language packs by extending `manifest.sh` and adding rule files under `rules/`.

**Alternative — Remote rule (GitHub) import**

Cursor Settings → Rules → **Remote rule (GitHub)** syncs into `<project>/.cursor/rules/imported/<repoName>/`.

**Alternative — User Rules UI**

Plain-text global preferences in Settings → Rules → User Rules. No `globs` or `alwaysApply`.

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

**Agents Window (Glass layout)**

- Open a **project folder as a workspace** before chatting. Without a workspace, the agent may start from your home directory and will not load project rules from `.cursor/rules/`.
- File-backed rules and Settings UI visibility have [known bugs](https://forum.cursor.com/t/cursor-settings-don-t-list-or-reflect-active-user-rules-and-plugins/156864) in the Agents Window; project-scoped rules in an open workspace are the most reliable path.

**Rules vs other AI features**

- Rules apply to **Agent (Chat)** only — not Cursor Tab, Inline Edit (Cmd/Ctrl+K), or other features ([FAQ](https://cursor.com/docs/rules)).

**Precedence when guidance conflicts**

Team Rules → Project Rules → User Rules ([Team Rules](https://cursor.com/docs/rules#team-rules)).

### Verifying rules are active

In a project where rules are installed under `.cursor/rules/ai-toolkit/`:

1. Open **Customize → Rules** — project rules should list the `.mdc` files and their types.
2. In Agent chat with a matching file in context, scoped rules (e.g. `globs: "**/*.go"`) should attach.
3. Rules with `alwaysApply: true` (e.g. `rule-attribution.mdc`) should govern every Agent session in that project.
4. In the Agents Window, confirm the **project folder is open as a workspace** before testing.

### References

- [Cursor Rules](https://cursor.com/docs/rules)
- [Importing rules](https://cursor.com/docs/rules#importing-rules)
- [Rule anatomy](https://cursor.com/docs/rules#rule-anatomy)
- [User rules not recognized from ~/.cursor/rules](https://forum.cursor.com/t/user-rules-are-not-recognized-from-folder-cursor-rules/144739)
- [Feature request: global ~/.cursor/rules .mdc support](https://forum.cursor.com/t/support-for-cursor-rules-for-global-mdc-rules/144819)

## Claude

*(Documentation for Claude-based workflows will be added here.)*
