# Contributing to AI-Toolkit

This document is for **human contributors** and for **AI assistants** asked to add or update tools in this repository. Keep changes focused, reusable, and easy to adopt in other repos.

Canonical product behavior for Cursor rules is described in the [Cursor Rules documentation](https://cursor.com/docs/rules). When writing rules, follow [Best practices](https://cursor.com/docs/rules#best-practices): short, actionable, scoped, composable, and reference real files instead of pasting large guides.

**Consumers:** To wire `rules/` into a project, run [`scripts/install-rules.sh`](scripts/install-rules.sh) or use the **`install_rules`** command (see [README.md](README.md)); do not copy rule files by hand unless you have a one-off exception.

For Cursor install paths, rule behavior, and known limitations, see [`docs/cursor.md`](docs/cursor.md). Use the [`docs/`](docs/) folder for additional platform-specific notes when authoring or troubleshooting assets.

## Human workflow

1. **Branch or fork** from the default branch; open a pull request with a clear summary.
2. **Place assets** in the folder that matches their type (see [readme.md](readme.md) layout). If you introduce a new top-level category, update the README layout section in the same change.
3. **Name files** predictably: use `kebab-case` or consistent prefixes (for example `rules/typescript-strict.mdc`).
4. **One concern per file** when possible; split large guidance into multiple rules or prompts.
5. **Do not commit secrets**, API keys, or internal-only URLs unless this repo is strictly private and policy allows it.

## When to use which format

| Asset | Typical location | Notes |
|--------|------------------|--------|
| Cursor rule (simple) | `rules/*.md` | Plain markdown; often manual `@`-mention or settings-driven. |
| Cursor rule (scoped) | `rules/*.mdc` | Use YAML frontmatter for `description`, `globs`, `alwaysApply`. |
| Repo instructions (consumer) | Consumer’s `AGENTS.md` | Plain markdown; optional nested files per directory in the **consumer** repo. |
| Command / slash-style text | `commands/` | Document intent, inputs, and expected outputs. |
| Skill-style instructions | `skills/` | Self-contained “when to use + steps + pitfalls”; can mirror Cursor `SKILL.md` patterns. |
| Prompt templates | `prompts/` | User- or agent-facing; include variables in a consistent notation (for example `{{topic}}`). |

## Cursor rule frontmatter (`.mdc`)

Behavior is controlled by `alwaysApply`, `description`, and `globs` as described under [Rule anatomy](https://cursor.com/docs/rules#rule-anatomy):

| `alwaysApply` | `description` | `globs` | Effect (summary) |
|---------------|---------------|---------|-------------------|
| `true` | ignored | ignored | Always included. |
| `false` | — | set | Attached when a matching file is in context. |
| `false` | set | — | Agent may include when the description matches the task. |
| `false` | — | — | Typically only when `@`-mentioned. |

**Guidelines:**

- Prefer **narrow globs** over `alwaysApply: true` unless the rule truly applies to every conversation.
- Write **`description`** like a precise product requirement: what situation triggers this rule.
- Keep the rule body **under a few hundred lines**; split into multiple rules if it grows.

### Adding a language stack (`rules/manifest.sh`)

When you add rules for a new language or toolchain:

1. Add `.mdc` (and optional companion `.md`) files under `rules/`.
2. Edit [`rules/manifest.sh`](rules/manifest.sh):
   - Append the stack name to `STACK_NAMES`.
   - Define `STACK_<name>_DETECT=(…)` — project markers (e.g. `go.mod`).
   - Define `STACK_<name>_RULES=(…)` — rule filenames to install with that stack.
3. Org-wide rules belong in `ORG_RULES` only.

### Example: file-scoped rule

```markdown
---
description: Conventions for React components under src/components
globs: src/components/**/*.tsx
alwaysApply: false
---

- Use named exports.
- Co-locate styles with the component.
```

### Example: always-on policy (use sparingly)

```markdown
---
alwaysApply: true
---

- Do not edit generated output under dist/ or build/.
```

## Authoring commands (`commands/`)

Each command should be easy to copy into Cursor or another tool:

- **Title** and **one-line purpose**
- **When to use** (bullets)
- **Inputs** (what the user or agent must provide)
- **Steps** (ordered)
- **Output shape** (for example: “markdown table”, “unified diff only”, “JSON with keys …”)
- **Edge cases** (only if common)

Avoid documenting every CLI the model already knows; focus on **your team’s** defaults and guardrails.

## Authoring skills (`skills/`)

Skills are longer-lived instructions than one-off prompts:

- Start with **scope**: codebase area, role, or task type.
- Separate **policy** (must/never) from **procedure** (steps).
- Link or `@`-reference **canonical examples** in a consumer repo when the skill is meant to be copied there; in this toolkit, use `examples/` or short illustrative snippets.
- List **failure modes** (“if tests fail, do X”).

## Authoring prompts (`prompts/`)

- State the **audience** (user vs agent) and **success criteria**.
- Use a consistent placeholder syntax and document it once at the top of the file.
- Prefer one prompt per file unless variants are tightly related.

## AI generation checklist (before proposing a change)

Use this as an explicit self-review:

1. **Scoped:** Does this apply to a clear situation or path glob? Avoid “everything” rules.
2. **Actionable:** Are instructions imperative and testable (“use X”, “run Y”), not vague values?
3. **Short:** Could this be split into two composable files without losing clarity?
4. **Non-duplicative:** Does it repeat linter/docs content? If yes, reference or delete.
5. **Stable:** Does it reference real paths or `@` files that exist in a typical consumer layout?
6. **Safe:** No secrets; no instructions that encourage disabling security or exfiltrating data.
7. **Discoverable:** Filename and `description` (if `.mdc`) make the trigger obvious.

## References

- [Cursor setup and behavior](docs/cursor.md) — ai-toolkit notes on install paths, rules, and quirks
- [Cursor Rules](https://cursor.com/docs/rules)
- [Best practices](https://cursor.com/docs/rules#best-practices)
- [AGENTS.md](https://cursor.com/docs/rules#agentsmd)
- [Importing rules](https://cursor.com/docs/rules#importing-rules)
