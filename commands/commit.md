Generate a Git commit message using only staged changes (the Git index).

Steps you must follow:
1. Run git diff --cached --name-only and git diff --cached --stat to see scope.
2. Run git diff --cached for the full patch. If there is nothing staged, say so and stop.
3. Run git commit and enter the generate message


Output format (exact structure):
<scope>(<type>): <imperative overview — 8 words or fewer>

  * [brief bullet: what changed or why]
  * [add more bullets only if needed]


Rules:
- type: one of feat, fix, build, chore, docs, style, refactor, perf, test, ci
- scope: optional — names the system, module, or shared component affected (e.g., auth, api, db, ui, config). Omit parentheses entirely if not used.
- fix vs feature: fix for bugs, regressions, broken behavior, or corrections; feature for new behavior, capabilities, or user-visible additions.
- Subject line: imperative mood, no trailing period, ≤8 words. Prefer a short product-style phrase over a full sentence.
- Body bullets: lines start with two spaces, then *, then text. Each bullet: one line when possible; about one short sentence (roughly 10–18 words). State behavior and key dependencies (including shared planner settings when relevant), not implementation detail unless the change is purely internal.
- Prefer at most two bullets unless the patch clearly needs more.
- Tone: use verbs like Add, Fix, and Align in the subject; bullets may name user-facing settings or parameters in plain language (e.g. Week starts, goal row height).
- Do not describe unstaged files or guess changes not shown in git diff --cached.
- Do not abbreviate words or phrases.
- Do not include blank lines between body bullets or after the last bullet.