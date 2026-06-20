# Plan 001: Bootstrap Go ERP Repository

> Executor instructions: create the initial Go/TypeScript monorepo scaffold only. Do not implement business modules in this plan.
>
> Drift check: repository has no commits. Run `git status --short --branch` first. If tracked source files already exist outside `plans/`, stop and report before scaffolding.

## Status

- Priority: P1
- Effort: M
- Risk: LOW
- Depends on: none
- Category: dx
- Planned at: no commits, 2026-06-16

## Why This Matters

The target repository is empty. Every later plan needs a deterministic build, test, lint, run, module, and parity-audit baseline.

## Current State

- `/Users/fadhelalqaidoom/Documents/gorp` contains only `.git` and `plans/`.
- Source inventory exists outside the repo:
  - `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo`
  - `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise`
  - `/Users/fadhelalqaidoom/Desktop/odoo/odoo18/odoo18-addons`
  - `/Users/fadhelalqaidoom/Desktop/odoo/odoo17/odoo17-addons`

## Commands You Will Need

| Purpose | Command | Expected on success |
|---------|---------|---------------------|
| Inspect | `git status --short --branch` | no unexpected tracked files |
| Go test | `go test ./...` | exit 0 |
| Go vet | `go vet ./...` | exit 0 |
| Format check | `test -z "$(gofmt -l .)"` | exit 0 |
| Frontend install | `pnpm -C frontend install` | exit 0 |
| Frontend typecheck | `pnpm -C frontend typecheck` | exit 0 |
| Frontend tests | `pnpm -C frontend test` | exit 0 |
| Frontend build | `pnpm -C frontend build` | exit 0 |

## Scope

In scope:
- `go.mod`
- `Makefile`
- `README.md`
- `AGENTS.md`
- `.gitignore`
- `.env.example`
- `cmd/gorpd/main.go`
- `internal/config`
- `internal/cli`
- `internal/logging`
- `internal/testutil`
- `frontend/package.json`
- `frontend/pnpm-workspace.yaml`
- `frontend/tsconfig.json`
- `frontend/vitest.config.ts`
- `.github/workflows/ci.yml`

Out of scope:
- ORM implementation
- security implementation
- accounting implementation
- AI implementation
- OI modules
- copying files from `/Users/fadhelalqaidoom/Desktop/odoo`

## Git Workflow

- Branch: `advisor/001-bootstrap-go-repo`
- Commit style: `bootstrap: create Go ERP scaffold`
- Do not push unless instructed.

## Steps

### Step 1: Create Go module and command

Use module path `gorp` unless a remote path is known.

Create:
- `go.mod` with Go 1.24 or the locally installed stable version if `go version` reports older.
- `cmd/gorpd/main.go` with a minimal CLI entrypoint.
- `internal/config/config.go` with env-based config loading.
- `internal/logging/logging.go` using Go standard `log/slog`.

Verify: `go test ./...` exits 0.

### Step 2: Create Makefile gates

Add targets:
- `make test`
- `make vet`
- `make fmt-check`
- `make frontend-install`
- `make frontend-test`
- `make frontend-build`
- `make ci`

Verify: `make ci` exits 0. If frontend has no packages yet, the target may print `frontend skipped` and exit 0.

### Step 3: Create frontend workspace

Create a minimal pnpm workspace:
- `frontend/package.json`
- `frontend/pnpm-workspace.yaml`
- `frontend/tsconfig.json`
- `frontend/vitest.config.ts`
- `frontend/packages/.gitkeep`

Scripts:
- `typecheck`
- `test`
- `build`
- `lint`

Use placeholder scripts only if they execute and prove the workspace shape.

Verify: `pnpm -C frontend install && pnpm -C frontend test && pnpm -C frontend build` exits 0.

### Step 4: Document local source inputs

Update `README.md` with:
- project purpose
- local source inventory paths
- clean-room rule
- setup commands
- verification commands

Create `AGENTS.md` with:
- direct factual style
- no destructive git commands
- source files under `/Users/fadhelalqaidoom/Desktop/odoo` are reference inputs
- no source-copying from proprietary Odoo Enterprise/OI modules without license decision

Verify: `sed -n '1,200p' README.md AGENTS.md` shows the source paths and commands.

### Step 5: Add CI

Create `.github/workflows/ci.yml` running:
- Go test
- Go vet
- gofmt check
- frontend install/test/build if `frontend/package.json` exists

Verify: `make ci` exits 0.

## Test Plan

- `go test ./...`
- `go vet ./...`
- `test -z "$(gofmt -l .)"`
- `pnpm -C frontend test`
- `pnpm -C frontend build`

## Done Criteria

- `make ci` exits 0.
- `README.md` contains setup and source paths.
- `AGENTS.md` exists.
- No feature modules are implemented.
- `git status --short` shows only expected bootstrap files and `plans/` changes.

## STOP Conditions

- Existing source files appear outside `plans/` before starting.
- `go version` is unavailable.
- `pnpm` is unavailable and cannot be replaced with an agreed package manager.
- The operator requires direct source copying from Odoo Enterprise/OI modules without a license decision.

## Maintenance Notes

All later plans assume `make ci` is the main verification gate.
