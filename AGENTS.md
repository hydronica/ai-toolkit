# Agent instructions

This repo authors Cursor rules under [`rules/`](rules/). Treat `rules/` as the source of truth — read the matching rule **before** changing code. `.cursor/rules/ai-toolkit/*.mdc` is a synced copy; if both exist, follow whichever you read and keep them consistent.

Always read [`rules/agent-behavior.mdc`](rules/agent-behavior.mdc) first: rules override surrounding code (match local style only where rules are silent), and ambiguity gets a question, not a guess.

## Which rule applies where

| When you touch… | Read |
|---|---|
| Any `**/*.go` (production) | [`go-standards.mdc`](rules/go-standards.mdc) |
| Any `**/*_test.go`, or a bug fix / regression | [`go-standards.mdc`](rules/go-standards.mdc) **and** [`go-testing.mdc`](rules/go-testing.mdc) |
| `cmd/`, `internal/`, new binaries/modules, file splits | add [`go-project-structure.md`](rules/go-project-structure.md) |
| Files under `rules/` | [`CONTRIBUTING.md`](CONTRIBUTING.md) + [`rules/manifest.json`](rules/manifest.json) |
| Commands, skills, scripts, docs | [`CONTRIBUTING.md`](CONTRIBUTING.md) |

## Go gotcha: globs don't cover bug fixes

`go-testing.mdc` only globs `**/*_test.go`, so it won't auto-attach when a bug-fix task starts on a production `.go` file. **Read it anyway** before writing or changing tests. `go-project-structure.md` has no frontmatter at all — Cursor never auto-attaches it, so open it explicitly for layout work.

Prefer extending an existing `Test…`/`trial` table with a golden `Expected` row over new helpers, nested `t.Run` loops, or parallel harnesses. Use the `@red-green-bug-fix` skill for regressions.

Go code lives under `cmd/` (`cuse`, `db-query`, `toolkit`). Stack membership is defined in [`rules/manifest.json`](rules/manifest.json); keep [`rules/manifest.sh`](rules/manifest.sh) in sync until the toolkit CLI cutover finishes.

## Precedence

`agent-behavior.mdc` and stack rules → this `AGENTS.md` → `CONTRIBUTING.md` → local patterns where those are silent. Don't weaken a rule to shrink a diff.

## After edits

If rule attribution is requested, list the `rules/` files you actually read and applied. If you changed Go without reading the matching rule(s), stop and read them before finishing.
