# AI-Toolkit

Versioned, shareable AI assets for [Cursor](https://cursor.com) and Claude-based workflows. Use this repository to distribute rules, commands, skills, prompts, and scripts across teams and codebases with consistent quality.

## Repository layout (template)

Organize tools for consumption via the install/uninstall scripts (Cursor integration) and for copy/paste of prompts and skills where needed:

```text
  rules/                    # Cursor project rules (.md / .mdc)
  commands/                 # Reusable command definitions or snippets
  skills/                   # Agent skill descriptions (e.g. SKILL.md-style)
  agents/                   # Agent definitions consumed by Cursor
  scripts/                  # Shell helpers (installed to ~/.cursor/ai-toolkit)
  docs/                     # Platform notes (e.g. Cursor setup and behavior)
```



## Installing the repo

clone and run `install.sh` or 

```
sh -c "$(curl -fsSL https://raw.githubusercontent.com/hydronica/ai-toolkit/main/install.sh)"
```

`[install.sh](install.sh)` installs toolkit assets **globally under your Cursor home layout**:

- `${HOME}/.cursor/commands/ai-toolkit`
- `${HOME}/.cursor/skills/ai-toolkit`
- `${HOME}/.cursor/agents/ai-toolkit`
- `${HOME}/.cursor/ai-toolkit` — scripts, canonical `rules-source/`, and `projects.registry` (e.g. `install-rules.sh`). Add that directory to `PATH` if you want to run scripts by name; see the post-install hint from `install.sh`.

Some scripts require the [GitHub CLI](https://cli.github.com/) (`gh`) with `gh auth login` completed — notably `pr_sum.sh` and `release_sum.sh`. `install.sh` checks for `gh` after install and prints setup instructions if it is missing or not authenticated.

`install.sh` will try to symlink assets when run from a local clone, or copy them when installing from the remote tarball. Use `./install.sh --copy` or `./install.sh --link` to force a mode. Pass `--no-sync-rules` to skip refreshing registered project rules.

Each run replaces the existing `ai-toolkit` entry under each category and refreshes scripts and `rules-source/` under `${HOME}/.cursor/ai-toolkit/` while preserving `projects.registry` and local config (e.g. `cuse`’s `.env`). Online installs need `curl` and `tar`.

Re-running `install.sh` updates `rules-source/` and **syncs all registered projects** by default (copy-mode projects get fresh rule files; link-mode projects get reconciled symlinks).

### uninstall

`[uninstall.sh](uninstall.sh)` removes global toolkit paths. By default it does **not** remove project rules under `.cursor/rules/ai-toolkit/`. Use `uninstall.sh --purge-project-rules` to remove rules from all registered projects first.

```
sh -c "$(curl -fsSL https://raw.githubusercontent.com/hydronica/ai-toolkit/main/uninstall.sh)"
```



### Installing rules in a project

Rules must live in each project’s `.cursor/rules/` tree. The first install **registers** the project in `~/.cursor/ai-toolkit/projects.registry` so future `install.sh` runs can sync it.

Use one of:

- **Command:** invoke `install_rules` in Agent chat (see `[commands/install_rules.md](commands/install_rules.md)`).
- **Script:** `~/.cursor/ai-toolkit/install-rules.sh --filter auto` from the project directory (detects stacks from `[rules/manifest.sh](rules/manifest.sh)`).
- **Sync all registered projects:** `~/.cursor/ai-toolkit/install-rules.sh --sync-all`
- **List registered projects:** `~/.cursor/ai-toolkit/install-rules.sh --list`
- **Cursor UI:** **Settings → Rules → Remote rule (GitHub)** syncs into `.cursor/rules/imported/<repoName>/` ([Importing rules](https://cursor.com/docs/rules#importing-rules)).

**Registry format** (`~/.cursor/ai-toolkit/projects.registry`):

```text
# columns: absolute_path|filter|mode
/Users/jane/code/api-service|auto|link
/Users/jane/code/legacy-app|go|copy
```

See `[docs/cursor.md](docs/cursor.md)` for Cursor setup, rule discovery limits, and troubleshooting.

### Suggested user rules

After you install project rules, add this **user rule** so the agent reports which project rules actually guided its work. That makes it easy to see during prompts whether rules attached — if the table is empty or shows “none matched,” something may be misconfigured.

**How to add manually**

1. Open **Cursor → Settings → Rules → User Rules**.
2. Paste the block below (merge with any existing user rules).
3. Save. User rules sync to other machines on the same Cursor account (`[docs/cursor.md](docs/cursor.md)`).

**Suggested text**

```markdown
Rule Attribution: 
After creating or modifying files, end your response with **Rules applied** (last section). Skip when no files changed.

Summarize which Cursor **project rules** guided your edits. List only rules that were actually in context (globs, alwaysApply, description match, or explicit `@` mention). Do not guess rule names.

## Rules applied

| File / Pattern | Rules |
|------|-------|
| `path/or/pattern` | `rule-name` |

When no per-file table applies, use a single row with `—` in the first column:

- `— | No Rules` — no project rules were loaded
- `— | None` — project rules exist but none applied to the changed files

Otherwise: one row per changed file; collapse paths that share the same rule set; alphabetize rule names within each cell.
```



### Troubleshooting

- `Operation not permitted` **or symlink errors:** Run with `--copy`, or enable symlink support (e.g. Windows Developer Mode).
- **Missing directories / download errors:** The remote tarball must contain `commands`, `skills`, `agents`, and `scripts`; a sparse upstream repo will fail validation until those folders exist.
- **`pr_sum.sh` / `release_sum.sh` failures:** Install [GitHub CLI](https://cli.github.com/) and run `gh auth login`.



## Using assets with Cursor

- **User-level install:** After `install.sh`, commands, skills, and agents live under `~/.cursor/{commands,skills,agents}/ai-toolkit`, with scripts and `rules-source/` at `~/.cursor/ai-toolkit`.
- **Project rules:** Run `install-rules.sh` or the `install_rules` command to install filtered rules into `<project>/.cursor/rules/ai-toolkit/`. Projects are registered automatically for sync on the next `install.sh` run. Use `--no-register` to opt out.
- **Go rules:** `go-standards.mdc`, `go-testing.mdc`, and companion `go-project-structure.md` — installed when Go is detected or `--filter go` is used.
- **Skills:** `skills/red-green-bug-fix/` — replication contract, RED/GREEN bug fix; invoke `@red-green-bug-fix` (not auto-attached).
- **AGENTS.md:** For simpler, repo-wide instructions without per-rule metadata, use `AGENTS.md` in the project root (or nested directories). See [AGENTS.md](https://cursor.com/docs/rules#agentsmd).
- **Precedence:** If you use Team Rules, remember order is **Team → Project → User** when guidance conflicts. See [Team Rules](https://cursor.com/docs/rules#team-rules).

