Generate a Git commit message using only staged changes (the Git index).

Steps you must follow:
1. Run git diff --cached --name-only and git diff --cached --stat to see scope.
2. Run git diff --cached for the full patch. If there is nothing staged, say so and stop.
3. Run git commit and enter the generate message


Output format (exact structure):
<type>(<scope>): <imperative overview — 8 words or fewer>

  * [brief bullet: what changed or why]
  * [add more bullets only if needed]


Rules:
- Types follow [Conventional Commits](https://www.conventionalcommits.org/) as used by [commitizen-go](https://github.com/lintingzhen/commitizen-go) (`git cz`). Pick one:
  - `feat` — a new feature or user-visible capability
  - `fix` — a bug fix, regression, or incorrect behavior
  - `docs` — documentation only
  - `style` — formatting, whitespace, or lint fixes; no logic change
  - `refactor` — code change that is not a fix, feat, or perf improvement
  - `perf` — performance improvement only
  - `test` — adding or correcting tests only
  - `build` — build system, packaging, or install tooling (e.g. `install.sh`, `Makefile`, dependencies)
  - `ci` — continuous integration configuration only (e.g. GitHub Actions)
  - `chore` — other maintenance (scripts, config, tooling) not covered above
- scope: optional — names the system, module, or shared component affected (e.g., install, rules, commit). Omit parentheses entirely if not used.
- feat vs fix vs chore: `feat` for new behavior others consume; `fix` for broken behavior; `chore` for housekeeping that is not docs, build, ci, or test.
- Subject line: imperative mood, no trailing period, ≤8 words. Prefer a short product-style phrase over a full sentence.
- Body bullets: lines start with two spaces, then *, then text. Each bullet: one line when possible; about one short sentence (roughly 10–18 words). State behavior and key dependencies (including shared planner settings when relevant), not implementation detail unless the change is purely internal.
- Prefer at most two bullets unless the patch clearly needs more.
- Tone: use verbs like Add, Fix, and Align in the subject; bullets may name user-facing settings or parameters in plain language (e.g. Week starts, goal row height).
- Do not describe unstaged files or guess changes not shown in git diff --cached.
- Do not abbreviate words or phrases.
- Do not include blank lines between body bullets or after the last bullet.