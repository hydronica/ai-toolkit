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
```

## Installing the repo

clone and run `install.sh` or 

```
sh -c "$(curl -fsSL https://raw.githubusercontent.com/hydronica/ai-toolkit/main/install.sh)"
```

[`install.sh`](install.sh) installs toolkit assets **globally under your Cursor home layout** — one target per asset type:

- `${HOME}/.cursor/rules/ai-toolkit`
- `${HOME}/.cursor/commands/ai-toolkit`
- `${HOME}/.cursor/skills/ai-toolkit`
- `${HOME}/.cursor/agents/ai-toolkit`
- `${HOME}/.cursor/ai-toolkit` — executable helpers from this repo’s `scripts/` (e.g. `pr_sum.sh`). Add that directory to `PATH` if you want to run them by name; see the post-install hint from `install.sh`.

`install.sh` will try to symlink assets when run from a local clone, or copy them when installing from the remote tarball. Use `./install.sh --copy` or `./install.sh --link` to force a mode.

Each run replaces the existing `ai-toolkit` entry under each category and the bin directory (`rm -rf` then link or copy). Online installs need `curl` and `tar`.

### uninstall 

[`uninstall.sh`](uninstall.sh) removes those paths and `${HOME}/.cursor/ai-toolkit` (whether each is a symlink or a copied directory).

```
sh -c "$(curl -fsSL https://raw.githubusercontent.com/hydronica/ai-toolkit/main/uninstall.sh)"
```

### Importing rules inside Cursor (alternative)

You can also add rules **per project** from a GitHub repo through the Cursor UI: **Cursor Settings → Rules, Commands** and use **Remote rule (GitHub)**. Cursor syncs `.mdc` files into `.cursor/rules/imported/<repoName>/` as described in [Importing rules](https://cursor.com/docs/rules#importing-rules). That path is separate from the user-level `~/.cursor/rules/ai-toolkit` layout used by `install.sh`.

### Troubleshooting

- **`Operation not permitted` or symlink errors:** Run with `--copy`, or enable symlink support (e.g. Windows Developer Mode).
- **Missing directories / download errors:** The remote tarball must contain `rules`, `commands`, `skills`, `agents`, and `scripts`; a sparse upstream repo will fail validation until those folders exist.

## Using assets with Cursor

- **User-level install:** After `install.sh`, rules and related assets live under `~/.cursor/{rules,commands,skills,agents}/ai-toolkit`, with scripts at `~/.cursor/ai-toolkit`, as configured above.
- **Go rules:** `rules/go-standards.mdc` (hub for `*.go`) and `rules/go-testing.mdc` (`*_test.go`), with topic-specific `rules/go-*.md` companions for deeper reference.
- **Project rules:** You can also place rule files under a project’s `.cursor/rules/`. Prefer `.mdc` with frontmatter when you need `description`, `globs`, or `alwaysApply`. See [Rule anatomy](https://cursor.com/docs/rules#rule-anatomy).
- **AGENTS.md:** For simpler, repo-wide instructions without per-rule metadata, use `AGENTS.md` in the project root (or nested directories). See [AGENTS.md](https://cursor.com/docs/rules#agentsmd).
- **Precedence:** If you use Team Rules, remember order is **Team → Project → User** when guidance conflicts. See [Team Rules](https://cursor.com/docs/rules#team-rules).
