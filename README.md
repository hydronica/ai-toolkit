# AI-Toolkit

Versioned, shareable AI assets for [Cursor](https://cursor.com) and Claude-based workflows. Use this repository to distribute rules, commands, skills, prompts, and examples across teams and codebases with consistent quality.

## Repository layout (template)

Organize tools for consumption via the install/uninstall scripts (Cursor integration) and for copy/paste of prompts and skills where needed:

```text
  rules/                    # Cursor project rules (.md / .mdc)
  commands/                 # Reusable command definitions or snippets
  skills/                   # Agent skill descriptions (e.g. SKILL.md-style)
  agents/                   # Agent definitions consumed by Cursor
  prompts/                  # Standalone prompt templates
  examples/                 # Minimal example projects or usage demos
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

install.sh will try and symlink the commands when run from a local repo or copy the files if run remotely. This behavior can also be specified with `./install.sh --copy` and `./install.sh --link` commands 

Each run replaces the existing `ai-toolkit` entry under each category (`rm -rf` then link or copy). Online installs need `curl` and `tar`.

### uninstall 

[`uninstall.sh`](uninstall.sh) removes those four paths (whether each is a symlink or a copied directory).

```
sh -c "$(curl -fsSL https://raw.githubusercontent.com/hydronica/ai-toolkit/main/uninstall.sh)"
```

### Importing rules inside Cursor (alternative)

You can also add rules **per project** from a GitHub repo through the Cursor UI: **Cursor Settings → Rules, Commands** and use **Remote rule (GitHub)**. Cursor syncs `.mdc` files into `.cursor/rules/imported/<repoName>/` as described in [Importing rules](https://cursor.com/docs/rules#importing-rules). That path is separate from the user-level `~/.cursor/rules/ai-toolkit` layout used by `install.sh`.

### Troubleshooting

- **`Operation not permitted` or symlink errors:** Run with `--copy`, or enable symlink support (e.g. Windows Developer Mode).
- **Missing directories / download errors:** The remote tarball must contain `rules`, `commands`, `skills`, and `agents`; a sparse upstream repo will fail validation until those folders exist.

## Using assets with Cursor

- **User-level install:** After `install.sh`, rules and related assets live under `~/.cursor/{rules,commands,skills,agents}/ai-toolkit` as configured above.
- **Project rules:** You can also place rule files under a project’s `.cursor/rules/`. Prefer `.mdc` with frontmatter when you need `description`, `globs`, or `alwaysApply`. See [Rule anatomy](https://cursor.com/docs/rules#rule-anatomy).
- **AGENTS.md:** For simpler, repo-wide instructions without per-rule metadata, use `AGENTS.md` in the project root (or nested directories). See [AGENTS.md](https://cursor.com/docs/rules#agentsmd).
- **Precedence:** If you use Team Rules, remember order is **Team → Project → User** when guidance conflicts. See [Team Rules](https://cursor.com/docs/rules#team-rules).
