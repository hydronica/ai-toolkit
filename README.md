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
- `${HOME}/.cursor/ai-toolkit` — scripts, canonical `rules-source/`, `projects.registry`, and `install-manifest.json` (e.g. `install-rules.sh`). Add that directory to `PATH` if you want to run scripts by name; see the post-install hint from `install.sh`.

Other bundled scripts include `review_sum.sh` (PR-ready context; no `gh` required). Some scripts require the [GitHub CLI](https://cli.github.com/) (`gh`) with `gh auth login` completed — notably `pr_sum.sh`, `release_sum.sh`, and `branch_sum.sh` (full PR/protected metadata; git-only mode still reports remote and local branches). `install.sh` checks for `gh` after install and prints setup instructions if it is missing or not authenticated.

`install.sh` will try to symlink assets when run from a local clone, or copy them when installing from the remote tarball. Use `./install.sh --copy` or `./install.sh --link` to force a mode.

**Smart updates:** Re-running `install.sh` compares the current source (`main` commit for online installs, local `git` HEAD for clones) to `~/.cursor/ai-toolkit/install-manifest.json`. When nothing changed, asset reinstall and rule sync are skipped. Use `./install.sh --check` to preview the plan, `--force` to reinstall anyway, and `--sync-rules` to refresh registered project rules when assets are unchanged. Smart updates and `./install.sh --check-attribution` use [jq](https://jqlang.org/) when available (`brew install jq`, `sudo apt install jq`, or `winget install jqlang.jq`); install still completes without jq, but the install manifest may not be recorded and attribution status is reported as unknown.

From a local clone with Go installed, `install.sh` builds `cuse` and `db-query` via `make`. In **link** mode (default for local clones), those binaries are symlinked from `scripts/` into `~/.cursor/ai-toolkit/`, so `make cuse` updates the binary on your `PATH` immediately. Copy mode and online installs copy or download release binaries instead.

Pass `--no-sync-rules` to skip refreshing registered project rules when assets do change.

## Dependencies

What you need beyond a normal Cursor install. Optional pieces are called out in install output when they matter.

| Area | Required | Optional |
|------|----------|----------|
| **Core install** (`install.sh`) | Shell, `curl` or a local clone | [jq](https://jqlang.org/) (install manifest and `--check-attribution`); [GitHub CLI](https://cli.github.com/) (`gh auth login`) for `pr_sum.sh`, `release_sum.sh`, and `branch_sum.sh`; [Go](https://go.dev/) to build `cuse` and `db-query` from a local clone |
| **Project rules** (`install-rules.sh`) | Shell | None |
| **Capture HTML** (`@capture-html`) | [Chrome DevTools MCP](#chrome-devtools-mcp) with `--workspace=${userHome}/Downloads`; host `base64` to decode the stitched PNG | Node.js + Chrome; absolute `npx` path when using nvm (see [setup](#chrome-devtools-mcp)) |
| **Draw.io diagrams** (`drawio.mdc`) | [Draw.io Integration](https://marketplace.visualstudio.com/items?itemName=hediet.vscode-drawio) (or Cursor equivalent) to edit committed `.drawio.svg` / `.drawio.png` | [draw.io MCP](#drawio-mcp) when the agent should draft or iterate diagrams in chat |
| **Branch cleanup / PR scripts** | Git | `gh` (+ `jq` for full `branch_sum.sh` metadata) |

Skills and commands should keep new dependencies rare; see [CONTRIBUTING.md](CONTRIBUTING.md) (authoring skills). MCP-backed workflows are listed in **[MCP servers](#mcp-servers)**.

## MCP servers

Some skills and rules need [MCP](https://cursor.com/docs/mcp) tools. Add each server under **`mcpServers`** in `~/.cursor/mcp.json` or `.cursor/mcp.json`, merge multiple blocks into one file, then enable them in **Cursor Settings → MCP**. Use **`${userHome}`** in paths ([config interpolation](https://cursor.com/docs/mcp)) instead of hard-coded usernames.

| Toolkit asset | MCP |
|---------------|-----|
| `@capture-html` | [Chrome DevTools](#chrome-devtools-mcp) |
| `drawio.mdc` | [draw.io](#drawio-mcp) (optional) |
| `db-query` | [db-query](#db-query-mcp-optional) |

### Chrome DevTools MCP

**For:** [`@capture-html`](skills/capture-html/SKILL.md) — browser navigation, screenshots, in-page stitch. **Requires:** [Node.js](https://nodejs.org/) LTS, [Chrome](https://www.google.com/chrome/), and [chrome-devtools-mcp](https://github.com/ChromeDevTools/chrome-devtools-mcp).

1. Run `command -v npx` and note the path (nvm/fnm users need this — Cursor often does not load your shell `PATH`). 
2. Merge the block below into `mcp.json`. Set **`command`** and the Node segment in **`env.PATH`** to that `npx` path; align `v22.23.4` with your version under `${userHome}/.nvm/versions/node/`.
3. **`--workspace=${userHome}/Downloads`** — required so `take_screenshot` can write where the skill saves PNGs.
4. **Extensions (optional):** keep `--categoryExtensions`, `--ignore-default-chrome-arg=--disable-extensions`
   
```json
{
  "mcpServers": {
    "chrome-devtools": {
      "command": "${userHome}/.nvm/versions/node/v22.23.4/bin/npx",
      "args": [
        "-y",
        "chrome-devtools-mcp@latest",
        "--workspace=${userHome}/Downloads",
        "--categoryExtensions=true",
        "--ignore-default-chrome-arg=--disable-extensions"
      ],
      "env": {
        "PATH": "${userHome}/.nvm/versions/node/v22.23.4/bin:/usr/local/bin:/usr/bin:/bin"
      }
    }
  }
}
```

If MCP stays red but `npx -y chrome-devtools-mcp@latest --version` works in a terminal, fix **`command`** / **`env.PATH`**. Screenshot path errors mean missing **`--workspace`**. Restart MCP after changing extension flags. More: [configuration](https://github.com/ChromeDevTools/chrome-devtools-mcp/blob/main/docs/configuration.md).

### draw.io MCP

**For:** optional agent diagram drafts with [`drawio.mdc`](rules/drawio.mdc) (always installed with org rules via `install-rules.sh`). Committed files still use embedded XML in `.drawio.svg` / `.drawio.png`; use the [Draw.io Integration](https://marketplace.visualstudio.com/items?itemName=hediet.vscode-drawio) extension to edit them.

**Hosted (Cursor 2.6+, inline in chat)** — diagram data goes to `mcp.draw.io` ([privacy](https://www.drawio.com/docs/manual/generate/drawio-mcp-server/)):

```json
{
  "mcpServers": {
    "drawio": {
      "url": "https://mcp.draw.io/mcp"
    }
  }
}
```


### db-query MCP (optional)

**For:** read-only queries via [`cmd/db-query`](cmd/db-query/README.md) after `install.sh` (with Go) or `make db-query`. Place `config.toml` beside the binary (`config.example.toml`).

```json
{
  "mcpServers": {
    "db-query": {
      "command": "${userHome}/.cursor/ai-toolkit/db-query",
      "args": ["-mcp"]
    }
  }
}
```

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

End every response with **Rules applied** (last section).

Before planning code changes, read matching project rules per [`AGENTS.md`](AGENTS.md) and let the plan follow those conventions, not only issue docs.

List only rules that were in context (globs, alwaysApply, description match, `@` mention, or opened because `AGENTS.md` required them). Do not guess rule names.

## Rules applied

| File / Pattern | Rules |
|------|-------|
| `path/or/pattern` | `rule-name` |

Use `— | None` for read-only turns with no matching project rules; `— | No Rules` if none loaded. On plans, use planned paths or globs when you consulted rules for that scope. Otherwise one row per changed file or planned pattern; collapse paths with the same rule set; alphabetize rule names in each cell.
```



### Troubleshooting

- `Operation not permitted` **or symlink errors:** Run with `--copy`, or enable symlink support (e.g. Windows Developer Mode).
- **Missing directories / download errors:** The remote tarball must contain `commands`, `skills`, `agents`, and `scripts`; a sparse upstream repo will fail validation until those folders exist.
- **`pr_sum.sh` / `release_sum.sh` / `branch_sum.sh` failures:** Install [GitHub CLI](https://cli.github.com/) and run `gh auth login`. For full `branch_sum.sh` PR/protected metadata, also install [jq](https://jqlang.org/).



## Using assets with Cursor

- **User-level install:** After `install.sh`, commands, skills, and agents live under `~/.cursor/{commands,skills,agents}/ai-toolkit`, with scripts and `rules-source/` at `~/.cursor/ai-toolkit`.
- **Project rules:** Run `install-rules.sh` or the `install_rules` command to install filtered rules into `<project>/.cursor/rules/ai-toolkit/`. Projects are registered automatically for sync on the next `install.sh` run. Use `--no-register` to opt out.
- **Go rules:** `go-standards.mdc`, `go-testing.mdc`, and companion `go-project-structure.md` — installed when Go is detected or `--filter go` is used.
- **Bruno rules:** `bruno.mdc` — installed when a Bruno collection is detected (`bruno/`, `bruno.json`, or `collection.bru`) or `--filter bruno` is used.
- **Draw.io:** `drawio.mdc` — always installed with org rules (so first diagrams are covered); attaches via `**/*.drawio*` globs. Optional **[draw.io MCP](#drawio-mcp)** for agent-authored diagrams; use the Draw.io VS Code extension for edits on committed files.
- **Skills:** `skills/red-green-bug-fix/` — replication contract, RED/GREEN bug fix; invoke `@red-green-bug-fix` (not auto-attached). `skills/branch-cleanup/` — remote/local branch audit via `branch_sum.sh`; invoke `@branch-cleanup` or `/ai-toolkit/branch_cleanup` (deletes only after confirmation).
- **PR ready:** `commands/pr_ready.md` + `skills/pr-ready/` — check changes against `CONTRIBUTING.md`, `AGENTS.md`, and project rules before opening a PR; invoke `/ai-toolkit/pr_ready` or `@pr-ready`. For best results, install project rules with `install_rules` first.
- **Capture HTML:** `skills/capture-html/` — capture a whole web page as one PNG via the Chrome DevTools MCP; stitches viewport sections in the browser when lazy panels (for example Grafana) would be blank in a full-page capture. Invoke `@capture-html`. See **[MCP servers → Chrome DevTools](#chrome-devtools-mcp)** and **Dependencies**.
- **AGENTS.md:** For simpler, repo-wide instructions without per-rule metadata, use `AGENTS.md` in the project root (or nested directories). See [AGENTS.md](https://cursor.com/docs/rules#agentsmd).
- **Precedence:** If you use Team Rules, remember order is **Team → Project → User** when guidance conflicts. See [Team Rules](https://cursor.com/docs/rules#team-rules).

