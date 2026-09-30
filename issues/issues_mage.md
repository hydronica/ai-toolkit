# Install & build tooling: Mage vs `toolkit` CLI

Planning document for replacing or complementing today's `install.sh` + `Makefile` stack.

## Problem statement

`install.sh` (~1,150 lines of bash) currently handles:

- Installing Cursor assets (commands, skills, agents) via link or copy
- Installing `scripts/`, `rules-source/`, and managed binaries (`cuse`, `db-query`)
- Smart updates via `install-manifest.json`, link-mode repair, and rule sync
- Online installs: resolve release/tag, download tarball or release binaries, platform detection
- Legacy migration (symlinked `~/.cursor/ai-toolkit` → `scripts/`, self-symlink cleanup)
- Optional checks (`--check-attribution`, `gh` auth hints after install)

`scripts/install-rules.sh` (~780 lines) handles per-project rule install, registry, sync, and purge.

Pain points motivating change:

- **Soft dependencies** — `jq` for manifest/JSON and CLI attribution; `sqlite3` for IDE attribution paths in `cuse`; install degrades without `jq` but smart updates and attribution are weaker
- **Fragile JSON and platform edge cases** in bash (grep/sed fallbacks when `jq` is missing)
- **Build logic split** across `Makefile`, `install.sh` (`make` invocations), and per-tool GoReleaser configs (`cmd/cuse`, `cmd/db-query`)
- **Hard to unit-test** install behavior; changes require manual matrix testing (link/copy, local/online, macOS/Linux/Git Bash)
- **Growing surface area** — each new flag or preserved path increases bash complexity

## Non-negotiable constraints

Any option must preserve:

1. **`install.sh` stays as the public bootstrap entrypoint** — including:
   ```bash
   sh -c "$(curl -fsSL https://raw.githubusercontent.com/hydronica/ai-toolkit/main/install.sh)"
   ```
2. **Full backwards compatibility** with existing installs under `~/.cursor/`:
   - `~/.cursor/{commands,skills,agents}/ai-toolkit`
   - `~/.cursor/ai-toolkit/` (scripts, `rules-source/`, `projects.registry`, `install-manifest.json`, binaries, optional `.env`)
   - Current flags: `--link`, `--copy`, `--check`, `--force`, `--sync-rules`, `--no-sync-rules`, `--check-attribution`
3. **Cross-platform support**: macOS, Linux, and Windows via Git Bash / WSL / MSYS (as today)
4. **End users must not need Go, Mage, or Make** for curl-based installs
5. **Local clone / contributor workflow** must keep link-mode ergonomics (edit source → `make` rebuild → binaries on PATH update immediately when symlinked)

## Current architecture (baseline)

| Component | Role |
|-----------|------|
| `install.sh` | User-facing installer; assets, binaries, manifest, repair, sync, online fetch |
| `Makefile` | Dev build/test for `cuse` and `db-query` → `scripts/` |
| GoReleaser | Per-tool release binaries (`cmd/cuse`, `cmd/db-query`) |
| `scripts/install-rules.sh` | Project rule install/sync (invoked by `install.sh`) |
| `uninstall.sh` | Global teardown; optional `--purge-project-rules` |

**Install layout (unchanged contract):**

```text
~/.cursor/commands/ai-toolkit/     → repo commands/ (link or copy)
~/.cursor/skills/ai-toolkit/       → repo skills/
~/.cursor/agents/ai-toolkit/       → repo agents/
~/.cursor/ai-toolkit/              → scripts + rules-source + state + binaries
```

---

## Option A: Mage for dev build + setup; shipped binary for users

Replace **Makefile** and the **heavy logic inside `install.sh`** with [Mage](https://magefile.org/) for contributors, while end users run a **prebuilt Go binary** (not Mage).

### Shape

```text
End user:
  curl install.sh  →  detect OS/arch  →  download release binary  →  exec installer

Contributor (clone):
  mage setup       →  link assets, build binaries, write manifest
  mage build       →  replaces make cuse / make db-query
  mage test        →  replaces make test
  mage release     →  wraps GoReleaser (optional)

install.sh (thin):
  ~30–80 lines: platform detect, download, checksum verify, exec <binary> install [flags]
  Delegates all real work to the release binary; preserves CLI flags for backwards compat.
```

### Mage targets (illustrative)

| Target | Replaces |
|--------|----------|
| `mage Setup` | `./install.sh` from local clone (link mode) |
| `mage Build` | `make build` |
| `mage Test` | `make test` |
| `mage Release` | manual / CI GoReleaser invocation |
| `mage SyncRules` | `install-rules.sh --sync-all` (optional convenience) |

Magefile lives in-repo; contributors run `go run github.com/magefile/mage@latest` or install Mage once. **Users never run Mage.**

### What the shipped binary is

- Single artifact per platform (e.g. `ai-toolkit-installer` or `toolkit`) published beside `cuse` / `db-query`, **or** one unified `toolkit` binary with subcommands (`install`, `rules`, `attribution`)
- Implements: plan/install/repair, manifest read/write, asset link/copy, binary ensure, rule sync hook
- **Does not** require bash beyond the bootstrap stub

### Pros

- Contributors get typed, testable Go for install logic
- Mage is a thin orchestration layer familiar to Go repos
- Curl UX unchanged if stub delegates correctly

### Cons

- Two build stories (Mage for dev + GoReleaser for ship) unless Mage only wraps `go build`
- New dependency for contributors (Mage)
- Migration is a large cutover unless phased behind feature flags inside the binary

---

## Option B: `toolkit` CLI only (no Mage)

Add **`cmd/toolkit`** (name TBD) as the sole non-bash implementation: install, rules, attribution, and contributor `build` subcommands.

### Shape

```text
End user:
  curl install.sh  →  download toolkit  →  toolkit install [flags]

Contributor:
  go run ./cmd/toolkit setup
  go run ./cmd/toolkit build
  go test ./cmd/toolkit/...
```

Retire **Makefile** for install-related targets; optionally keep Makefile as a one-line forwarder (`make build` → `go run ./cmd/toolkit build`) during transition.

### Pros

- One language, one test suite, no Mage dependency
- Subcommands map cleanly to today’s flags and `install-rules.sh` behaviors
- Easier to embed version/commit and structured `--check` JSON output

### Cons

- Largest upfront port (`install.sh` + much of `install-rules.sh`)
- Contributors must use Go for setup unless stub shell scripts remain
- GoReleaser/release matrix grows if `toolkit` is another published binary

---

## Option C: Incremental extraction (hybrid)

Keep **`install.sh` as orchestrator**; move **isolated subsystems** into Go libraries/binaries over time.

### Suggested slices (order)

1. **Manifest + plan** — `toolkit plan` / `toolkit manifest write` (jq-free JSON in Go)
2. **Binary ensure** — download/verify/symlink `cuse` and `db-query` (already partially isolated in `ensure_binaries`)
3. **Attribution** — merge CLI + IDE checks from bash into `cuse` or `toolkit attribution`
4. **Rules** — optional later: `toolkit rules install|sync` wrapping or replacing `install-rules.sh`

`install.sh` shrinks by calling `toolkit` subprocesses; behavior and flags stay stable.

### Pros

- Lowest risk per PR; easy rollback
- No Mage; optional Makefile until build moves to `toolkit build`

### Cons

- Long period of bash + Go co-ownership
- Subprocess boundaries and error messages need discipline

---

## Option D: Status quo + hardening

No Mage, no new CLI. Invest in:

- Documented manual test matrix in `CONTRIBUTING.md`
- Shellcheck / bats tests for critical `install.sh` paths
- Stronger `jq` messaging (already improved)
- Extract **pure functions** into `scripts/lib/install_common.sh` shared with `install-rules.sh`

### Pros

- No migration cost; fits small team bandwidth

### Cons

- Does not fix testability or JSON fragility at the root
- Line count likely keeps growing

---

## Comparison matrix

| Criterion | A (Mage + binary) | B (`toolkit` only) | C (incremental) | D (harden bash) |
|-----------|-------------------|--------------------|-----------------|-----------------|
| End-user curl UX | ✓ (thin stub) | ✓ | ✓ (until cutover) | ✓ |
| Contributor ergonomics | Mage targets | `go run` subcommands | Mixed | `make` + bash |
| Unit testability | High (binary) | High | Medium → High | Low |
| Migration risk | Medium–High | High | Low per step | None |
| New deps for contributors | Mage | None | None | None |
| Time to first value | Medium | Long | **Short** | Short |

---

## Recommendation (draft)

**Prefer Option C first**, with a clear end state compatible with **Option B**:

1. Introduce `cmd/toolkit` with `plan` + `manifest` + `install` delegating from a shrinking `install.sh` (Phase 1–2).
2. Port `ensure_binaries` and attribution checks into Go (Phase 3).
3. Evaluate whether **Makefile** should become `toolkit build` only; **defer Mage** unless contributors explicitly want Mage targets over `go run`.
4. Keep **`install-rules.sh`** in bash until rule logic is well-specified in tests; then optional `toolkit rules` subcommand (Phase 4+).

Revisit **Option A** only if the team wants Mage-style task discovery (`mage -l`) without growing `toolkit` subcommand surface.

---

## Implementation phases (Option C → B)

### Phase 0 — Baseline

- [ ] Document current manual test matrix (link/copy, local/online, `--check`, `--force`, migration from legacy symlink layout)
- [ ] Add shellcheck job for `install.sh` / `install-rules.sh` (non-blocking or blocking per team preference)

### Phase 1 — Plan + manifest in Go

- [ ] `cmd/toolkit` with `toolkit plan` mirroring `install.sh --check` output
- [ ] `toolkit manifest read|write` using same JSON schema as `install-manifest.json`
- [ ] `install.sh` calls `toolkit plan` when binary present; bash fallback when not

**Acceptance:** `--check` identical semantics with and without Go binary on PATH during dev.

### Phase 2 — Install core

- [ ] `toolkit install` implements asset link/copy + rules-source + manifest write
- [ ] Thin `install.sh` downloads release `toolkit` for online users; local clone uses `go run` or built `scripts/toolkit`

**Acceptance:** Fresh curl install and local `./install.sh` produce same tree as today.

### Phase 3 — Binaries + attribution

- [ ] Move `ensure_binaries` + version matching into Go
- [ ] `--check-attribution` via `toolkit attribution` (sqlite/jq dependencies documented)

### Phase 4 — Rules (optional)

- [ ] Spec for registry format and filters (`rules/manifest.sh` behavior)
- [ ] `toolkit rules install|sync-all|list` or keep bash wrapper calling Go for JSON only

### Phase 5 — Retire bash

- [ ] `install.sh` ≤ 80 lines stub only
- [ ] Deprecate duplicated logic in `install-rules.sh` or replace entirely

---

## Open questions

1. **Binary naming** — `toolkit`, `ai-toolkit`, or `atk`? Align with PATH story (`~/.cursor/ai-toolkit/` already named).
2. **Single vs multiple release artifacts** — one fat `toolkit` vs separate installer + `cuse` + `db-query`.
3. **Windows** — ship `toolkit.exe` in GoReleaser matrix; stub detects MSYS vs native.
4. **Manifest schema versioning** — bump `version` field in JSON when Go takes over writes?
5. **install-rules.sh** — port vs wrap: registry and `--filter auto` detection are the hardest parts.

---

## Test plan (any option that changes install)

| Scenario | Expect |
|----------|--------|
| Curl install (copy mode) | Assets under `~/.cursor/`, manifest written when `jq`/Go available |
| Local clone link mode | Symlinks to repo; `make cuse` updates `~/.cursor/ai-toolkit/cuse` |
| Re-run install unchanged | Skip assets (`--check` shows skip) |
| `--force` | Full reinstall |
| Legacy `~/.cursor/ai-toolkit` → `scripts/` symlink | Migration preserves registry, manifest, `.env` |
| `--no-sync-rules` / `--sync-rules` | Registry sync behavior unchanged |
| Online install without Go | Binaries downloaded from GitHub release |
| `uninstall.sh` | Removes global paths; optional purge |

---

## References

- `install.sh` — installer implementation and manifest schema
- `scripts/install-rules.sh` — project rules and registry
- `Makefile` — contributor builds
- `README.md` — user-facing install and smart-update docs
- Other `issues/*.md` planning docs in this repo (same section structure: problem → options → phases)
