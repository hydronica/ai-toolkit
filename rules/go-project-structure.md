# Go project structure

Packages, `internal/`, and `cmd/`—structure that agents often get wrong. Exhaustive layout: [Organizing a Go module](https://go.dev/doc/modules/layout).

## Single importable package

```
project-root/
  go.mod
  modname.go
  modname_test.go
```

All files: `package modname`. Import: `example.com/modname`. Avoid extra directories for trivial libraries.

## Single command (one binary)

Put `main.go` at module root with `package main`. Use `cmd/` only when you have **two or more** binaries.

```
project-root/
  go.mod
  main.go
```

Install: `go install example.com/modname@latest`.

## Multiple packages and `internal/`

```
project-root/
  go.mod
  feature.go
  auth/
    auth.go
  internal/
    impl/
      impl.go
```

Code under `internal/` is importable only from within the parent tree of that `internal` directory—external modules cannot import it. Prefer `internal/` for implementation you may refactor; expose stable APIs at the module root or public subpackages.

## Multiple commands (2+ binaries)

```
project-root/
  go.mod
  internal/shared/...
  cmd/server/main.go    # package main
  cmd/cli/main.go       # package main
```

`go install example.com/modname/cmd/server@latest`

## Services (typical shape)

- Group by **feature** (e.g. `internal/users/…` with handler, service, store together).
- Use layer-first layout only when layers are genuinely shared and stable.
- Avoid deep paths like `internal/app/domain/user/service/impl/…`.

## Files within a package

Prefer **one primary `.go` file** (plus `_test.go`) until ~400+ lines or clearly distinct subsystems. Split when (a) hard to review, (b) a second subsystem appears, or (c) build boundaries require it (`//go:build`, `*_unix.go`, generated code).

Keep sentinels and small helpers in the primary file. Avoid `errors.go`, `types.go`, `paths.go`, and similar kind-splits until related symbols form a cluster. Simplicity applies to **file layout**, not just APIs.

## Anti-patterns

- `src/` at repo root (not Go convention).
- Using `pkg/` for first-party code unless you have a clear public-package boundary.
- Mixing `package main` and other package names in the same directory.
- Kind-split files or extra `.go` files created only to test unexported helpers (`go-testing.mdc`).

## Package names

Directory `internal/auth/token` → `package token` (matches last path segment). Callers write `token.Parse`, not `auth_token.Parse`.

## Modules

Default: **one module** per repo. Split modules only for clear release cadence, dependency isolation, or genuinely independent artifacts.
