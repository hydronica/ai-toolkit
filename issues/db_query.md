# db-query: secure credentials, settings UI, and agent skill

## Goal

1. An agent skill that lets AI use `db-query` through the CLI or MCP, with instructions for adding new databases.
2. Database credentials stored securely, so the AI and other users never see them in config files, CLI history, or chat.
3. A local UI where users enter connection details and test them before saving.

## Decisions

| Topic | Decision | Source |
|-------|----------|--------|
| Credential store | KeePass `.kdbx` file, read and written with `github.com/tobischo/gokeepasslib/v3` (pure Go) | User choice: kdbx, works on macOS, Windows, and Linux |
| Master password | Stored in the OS credential store via `github.com/zalando/go-keyring` (service `db-query`) | User choice |
| Where connections live | All connection details, including host and user, go into the kdbx. No plain config file remains | User choice: "all in kdbx" |
| Plaintext TOML | Removed at the switch. Old configs get a one-time import, then a clear error | User choice |
| Agent guard | Skill policy only, no `beforeShellExecution` hook | User choice: skill only |
| Scope | Phased. UI first, then switch the readers to the vault | User choice |
| Interim storage | The UI writes to the existing TOML until the switch | User choice: TOML first |
| Interim TOML location | Beside the binary, which is inside this workspace (`scripts/`) | User choice, with the risk noted below |

### Accepted risk

On macOS, `go-keyring` reads the Keychain through `/usr/bin/security`. The Keychain trusts that tool without a prompt, so a shell command the agent runs could read the master password and open the kdbx. The kdbx is encrypted at rest, so reading the file alone is not enough. The skill policy is the only guard for now. Native per-OS backends (cgo on macOS) would tie access to `db-query` itself. That option was declined for this scope and can be revisited.

### Interim risk

Until the switch, the UI writes passwords into the TOML beside the binary. That file is inside this workspace, so the agent's Read tool can open it. Nothing in the repo holds a plaintext password today.

## Current state

### Done (Phase 1 code)

- [x] `Config.Validate` renamed to `CheckDatabases`. `go-config` runs `Validate` during `Load`, which would block the UI from opening a config that needs fixing. The CLI and MCP paths call `CheckDatabases` explicitly.
- [x] `internal/config/file.go`: `ReadFile`, `WriteFile` (temp file, rename, mode 0600), `Entries`, `SetEntries`, `DecodeEntry`, `FieldNames`.
- [x] `internal/config/file_test.go`: field names, file round trip, and mode check.
- [x] `internal/webui/webui.go`: local server on `127.0.0.1:0`, per-run token, Host check against DNS rebinding, and CSP headers. Routes:
  - `GET /` (token in query)
  - `GET /api/types`
  - `GET /api/databases`
  - `POST /api/databases` (add or update)
  - `DELETE /api/databases/{name}`
  - `POST /api/test` (never writes)
- [x] Secrets (`password`, `connection`, `uri`) are never returned to the browser. A blank secret on save keeps the saved value. Driver errors are redacted before they reach the page.
- [x] `internal/webui/index.html`: single page, no CDN, DOM APIs only (no `innerHTML`).
- [x] `internal/webui/webui_test.go`: merge rules, auth gates, save and delete through the real routes, Test does not write, redaction.
- [x] `main.go`: `-ui` flag.
- [x] `go mod tidy`: `github.com/hydronica/toml` is now a direct dependency.
- [x] `go vet ./...` is clean. `go test -race ./...` passes for `config`, `db`, and `webui`.

### Not done

- [ ] Manual check of the page in a browser.
- [ ] The kdbx vault and the master password in `go-keyring`.
- [ ] Switch: CLI and MCP read the vault, and the `password` field is removed from the TOML path.
- [ ] One-time import from the existing TOML.
- [ ] Skill: `skills/db-query/SKILL.md`.
- [ ] Docs: `cmd/db-query/README.md` and the root README. The root README says `config.toml`, but the code reads `.conf.toml`. Fix both to match the new layout.
- [ ] Windows and Linux verification. Only macOS is available here.
- [ ] `scripts/db-query` is stale. Rebuild with `make db-query`.

## Phased plan

### Phase 1: settings UI on the TOML (code done, verify)

Goal: a working UI now, so users stop editing TOML by hand.

1. Run `make db-query` and `go test -race ./...` from `cmd/db-query`.
2. Run `scripts/db-query -ui` in your own terminal, not through the agent. The token prints to the terminal.
3. Check in a browser: add, edit, rename, delete, Test with a good and a bad connection, and a blank password on edit.
4. Confirm that Test leaves the file unchanged.

Exit when: the browser checks pass on macOS.

### Phase 2: kdbx vault and the switch

Goal: passwords and connection details live only in the kdbx. The TOML stops being read.

1. **Vault package** (`internal/vault`):
   - Create or open the kdbx at `~/.cursor/config/db-query/databases.kdbx`.
   - One KeePass entry per database. Title is the database name. The username, password, and URL fields hold the connection details, and extra fields hold the rest.
   - Master password comes from `go-keyring` (service `db-query`, account = kdbx path). On first use, generate a random master password and store it in the keyring.
   - Return the same `config.Database` values the CLI already uses.
2. **Keyring fallback.** Headless Linux and CI have no Secret Service. Decide the fallback. Options: an env var, or a prompt in an interactive shell. Open question below.
3. **Readers.** The CLI and MCP load from the vault. Both paths go through one loader.
4. **UI writes to the vault.** Replace the TOML writes in `webui` with the vault. The API shape stays the same.
5. **Import.** `db-query -import-toml <path>` copies entries from the old TOML into the vault, then renames the old file to `*.imported`. Refuse to run twice.
6. **Remove** the `password` field and the TOML loader from the normal path. An old config that still has a `password` field gets a migration error that points to `-import-toml`.
7. Tests: vault round trip with a temp kdbx, master password from a fake keyring, import of each database type, and the migration error.

Exit when: CLI, MCP, and UI all use the vault, and no plaintext password exists outside the kdbx.

### Phase 3: skill and docs

1. `skills/db-query/SKILL.md`, installed through the existing `skills/` symlink (`~/.cursor/skills/ai-toolkit/db-query/`). Cover:
   - When to use the CLI and when to use MCP.
   - Hard policy: never open the kdbx, never read the keyring or `security` output, never read or print secrets, never run `-ui` (the token would show in the agent's output), and never ask the user to paste passwords in chat.
   - Steps for adding a database: send the user to the settings page, then run `ping_database` or `-ping` to verify.
   - Troubleshooting: missing master password, locked keyring, and unreachable hosts.
2. Update `cmd/db-query/README.md`: config location, `-ui`, `-import-toml`, and the MCP setup.
3. Update the root README: the `db-query MCP` section and the dependencies table.
4. Follow `CONTRIBUTING.md` for skill authoring: keep it short, state policy separately from procedure, and list failure modes.

### Phase 4: verification and hardening

1. Test on Windows (Credential Manager) and Linux (Secret Service). Confirm the headless fallback works.
2. Cross-compile with `.goreleaser.yaml` for linux, darwin, and windows on amd64 and arm64.
3. Manual browser test of the UI, including a rebinding attempt against the `Host` check.
4. Review the token and redaction paths once more.
5. Rebuild `scripts/db-query` and `scripts/cuse` with `make build`.

## Open questions

1. **Headless fallback.** When no keyring is available (CI, SSH, headless Linux), should the master password come from an env var, a prompt, or fail?
2. **Final kdbx location.** The plan uses `~/.cursor/config/db-query/databases.kdbx`. Confirm this path.
3. **Keyring account name.** The plan uses the kdbx path. Confirm, or pick a fixed account name so the entry survives a path move.
4. **Interim TOML.** Confirm that the TOML stays beside the binary until the switch, or move it now to `~/.cursor/config/db-query/` as a smaller interim risk.

## Known gaps and notes

- The sandboxed agent can read anything the user can read. The guard is policy, not enforcement.
- Not yet verified on this machine: whether macOS prompts before the `security` CLI reads an item that `go-keyring` created. If it does not prompt, agent shell calls can read the master password silently. Test this in Phase 2 before relying on the skill policy alone.
- Saving through the UI rewrites the TOML, which drops comments. The file is replaced atomically with mode 0600.
- The kdbx master password is the only secret that must survive a machine change. If it is lost, the vault is unrecoverable. The plan should include a note on backing up the keyring entry.
