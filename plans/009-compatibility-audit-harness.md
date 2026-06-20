# Plan 009: Build Compatibility Audit Harness And Source-Inventory Gates

> Executor instructions: create machine-checkable inventory and parity tracking. This plan may run in parallel after Plan 001.
>
> Drift check: run `git diff --stat -- tools inventory reports testdata`.

## Status

- Priority: P1
- Effort: M
- Risk: LOW
- Depends on: `plans/001-bootstrap-go-repo.md`
- Category: tests
- Planned at: no commits, 2026-06-16
- Status: DONE
- Completed at: 2026-06-16
- Verification: `go run ./tools/source_inventory --config inventory/sources.yaml --out reports/source_inventory.json`; `go run ./tools/parity_check --inventory reports/source_inventory.json --coverage reports/parity.yaml --rewrite`; `go run ./tools/parity_check --inventory reports/source_inventory.json --coverage reports/parity.yaml`; `go test ./tools/...`
- Inventory report: `reports/source_inventory.json`
- Parity report: `reports/parity.yaml`

## Why This Matters

The source tree is 26G with 291515 files. Manual claims of "everything read" are not useful. The project needs inventory manifests and coverage gates that show exactly which source files were inspected and how each feature maps to the Go implementation.

## Current State

Source roots:
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo18/odoo18-addons`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo17/odoo17-addons`

Key modules:
- Odoo 19 base: `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo/addons/base`
- Odoo 19 web: `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/web`
- Odoo 19 account: `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/account`
- Odoo 19 mail: `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/mail`
- Odoo 19 automation: `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/base_automation`
- Odoo 19 AI: `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise/ai`
- OI 18 modules: `oi_base`, `oi_workflow`, `oi_workflow_advance`, `oi_delegation`, `oi_login_as`

## Commands You Will Need

| Purpose | Command | Expected on success |
|---------|---------|---------------------|
| Inventory | `go run ./tools/source_inventory --config inventory/sources.yaml --out reports/source_inventory.json` | exit 0 |
| Coverage | `go run ./tools/parity_check --inventory reports/source_inventory.json --coverage reports/parity.yaml` | exit 0 |
| Tests | `go test ./tools/...` | exit 0 |

## Scope

In scope:
- `tools/source_inventory`
- `tools/parity_check`
- `inventory/sources.yaml`
- `reports/source_inventory.json`
- `reports/parity.yaml`
- `reports/README.md`

Out of scope:
- copying source files
- embedding proprietary code snippets
- full semantic diff engine

## Steps

### Step 1: Create source inventory config

Create `inventory/sources.yaml` with modules:
- base
- web
- account
- mail
- base_automation
- ai
- oi_base
- oi_workflow
- oi_workflow_advance
- oi_delegation
- oi_login_as

For each:
- source root
- version
- license from manifest
- include extensions: `.py`, `.xml`, `.csv`, `.js`, `.scss`, `.json`, `.sql`
- exclude `__pycache__`, `.git`, static description images unless asset inventory is explicitly needed

Verify: `go test ./tools/source_inventory -run TestConfig` exits 0.

### Step 2: Implement source inventory tool

Output JSON records:
- module
- path
- relative path
- extension
- line count
- sha256
- kind: model/view/security/data/controller/test/static
- manifest dependency data

Do not output file contents.

Verify: `go run ./tools/source_inventory --config inventory/sources.yaml --out reports/source_inventory.json` exits 0.

### Step 3: Implement parity coverage file

Create `reports/parity.yaml` with records:
- source module
- source file
- feature group
- target package/file
- status: pending | implemented | intentionally_omitted | blocked
- reason
- verification command

Seed it from the files already identified in plans.

Verify: `go run ./tools/parity_check --inventory reports/source_inventory.json --coverage reports/parity.yaml` exits 0.

### Step 4: Add coverage gate

Rules:
- P1 modules cannot have unmapped source files.
- OI modules must map every `.py`, `.xml`, `.csv`, `.js`, `.scss` file.
- Omitted files require explicit reason.
- Static description media may be omitted by policy.

Verify: `go test ./tools/parity_check` exits 0.

### Step 5: Add report docs

Create `reports/README.md` explaining:
- how to regenerate inventory
- how to update parity statuses
- how to review blocked files
- how to add new modules

Verify: `sed -n '1,200p' reports/README.md` shows commands and status meanings.

## Test Plan

- Inventory config parser tests.
- File classification tests.
- Hash stability tests.
- Parity missing-file detection tests.
- Omitted-file reason tests.

## Done Criteria

- Inventory JSON generated.
- Parity YAML generated.
- Coverage gate fails on unmapped P1 source files.
- No source contents are copied into reports.

## STOP Conditions

- Source roots are missing.
- Inventory requires reading secret files or `.env`; exclude and report path only.
- User requires publishing proprietary source hashes externally.

## Maintenance Notes

Run this after every feature-parity plan. Treat it as the proof that source coverage is complete.
