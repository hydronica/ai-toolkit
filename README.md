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

[`install.sh`](install.sh) installs toolkit assets **globally under your Cursor home layout**:

- `${HOME}/.cursor/commands/ai-toolkit`
- `${HOME}/.cursor/skills/ai-toolkit`
- `${HOME}/.cursor/agents/ai-toolkit`
- `${HOME}/.cursor/ai-toolkit` — executable helpers from this repo’s `scripts/` (e.g. `install-rules.sh`, `pr_sum.sh`). Add that directory to `PATH` if you want to run them by name; see the post-install hint from `install.sh`.

`install.sh` will try to symlink assets when run from a local clone, or copy them when installing from the remote tarball. Use `./install.sh --copy` or `./install.sh --link` to force a mode.

Each run replaces the existing `ai-toolkit` entry under each category and the bin directory (`rm -rf` then link or copy). Online installs need `curl` and `tar`.

**Rules are not installed globally** — Cursor does not reliably load user-level rule files. Install rules per project instead (see below).

### uninstall 

[`uninstall.sh`](uninstall.sh) removes those paths and `${HOME}/.cursor/ai-toolkit` (whether each is a symlink or a copied directory). Project-level rules under `.cursor/rules/ai-toolkit/` in your repos are not removed.

```
sh -c "$(curl -fsSL https://raw.githubusercontent.com/hydronica/ai-toolkit/main/uninstall.sh)"
```

### Installing rules in a project

Rules must live in each project’s `.cursor/rules/` tree. Use one of:

- **Command:** invoke **`install_rules`** in Agent chat (see [`commands/install_rules.md`](commands/install_rules.md)).
- **Script:** `~/.cursor/ai-toolkit/install-rules.sh --filter auto` from the project directory (detects stacks from [`rules/manifest.sh`](rules/manifest.sh)).
- **Cursor UI:** **Settings → Rules → Remote rule (GitHub)** syncs into `.cursor/rules/imported/<repoName>/` ([Importing rules](https://cursor.com/docs/rules#importing-rules)).

See [`docs/cursor.md`](docs/cursor.md) for Cursor setup, rule discovery limits, and troubleshooting.

### Suggested user rules

After you install project rules, add this **user rule** so the agent reports which ai-toolkit rules it applied. That makes it easy to see during prompts whether project rules (e.g. `go-standards.mdc`) actually attached — if the table is wrong or empty, something is misconfigured.

**How to add manually**

1. Open **Cursor → Settings → Rules → User Rules**.
2. Paste the block below (merge with any existing user rules).
3. Save. User rules sync to other machines on the same Cursor account ([`docs/cursor.md`](docs/cursor.md)).

**Suggested text**

```markdown
After creating or modifying files, end your response with **Rules applied** (last section). Skip when no files changed.

List ai-toolkit `rules/` filenames you followed: infer from each rule’s frontmatter **globs**, cross-references in rule bodies (e.g. hub rules citing companions), and any `@`-mentioned rules. If none: `None (ai-toolkit rules)`.

## Rules applied

| File Pattern | Rules |
|------|-------|
| `*.go` | `go-standards.mdc` |
| `*_test.go` | `go-standards.mdc`, `go-testing.mdc` |

One row per changed file; alphabetize rules; collapse paths that share the same set.
```
### Troubleshooting

- **`Operation not permitted` or symlink errors:** Run with `--copy`, or enable symlink support (e.g. Windows Developer Mode).
- **Missing directories / download errors:** The remote tarball must contain `commands`, `skills`, `agents`, and `scripts`; a sparse upstream repo will fail validation until those folders exist.

## Using assets with Cursor

- **User-level install:** After `install.sh`, commands, skills, and agents live under `~/.cursor/{commands,skills,agents}/ai-toolkit`, with scripts at `~/.cursor/ai-toolkit`.
- **Project rules:** Run `install-rules.sh` or the **`install_rules`** command to link filtered rules into `<project>/.cursor/rules/ai-toolkit/`. Org rules (e.g. `rule-attribution.mdc`) always install; language stacks (Go, etc.) are selected by `--filter auto` or explicitly.
- **Go rules:** `go-standards.mdc`, `go-testing.mdc`, and companion `go-project-structure.md` — installed when Go is detected or `--filter go` is used.
- **Skills:** `skills/red-green-bug-fix/` — replication contract, RED/GREEN bug fix; invoke **`@red-green-bug-fix`** (not auto-attached).
- **AGENTS.md:** For simpler, repo-wide instructions without per-rule metadata, use `AGENTS.md` in the project root (or nested directories). See [AGENTS.md](https://cursor.com/docs/rules#agentsmd).
- **Precedence:** If you use Team Rules, remember order is **Team → Project → User** when guidance conflicts. See [Team Rules](https://cursor.com/docs/rules#team-rules).
