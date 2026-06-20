# Plan 002: Build Base Kernel, Registry, ORM, Modules, Data Loader

> Executor instructions: implement the base runtime kernel in Go. Keep it small but structurally correct.
>
> Drift check: run `git diff --stat -- go.mod cmd internal migrations testdata`. If in-scope files changed since Plan 001, inspect before editing.

## Status

- Priority: P1
- Effort: L
- Risk: HIGH
- Depends on: `plans/001-bootstrap-go-repo.md`
- Category: architecture
- Planned at: no commits, 2026-06-16

## Why This Matters

Odoo compatibility depends on data-driven modules, a per-database registry, model metadata, fields, recordsets, environments, domains, transactions, and XML/CSV data loading. Odoo 19 docs describe recordsets as ordered collections of records and data files as the mechanism for UI, security, reports, and base data. The local Odoo 19 source confirms the same surface in `base`, `web`, `account`, `mail`, and `base_automation`.

## Current State

Reference paths:
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo/addons/base`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo/orm/registry.py`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo/orm/model_classes.py`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo/models.py`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo/environments.py`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo/fields.py`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo/fields_relational.py`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo/modules`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo/tools/convert.py`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo/addons/base/models`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo/addons/base/data`

Local Odoo 19 `base` inventory includes:
- data: `base_data.sql`, `ir_cron_data.xml`, `ir_module_category_data.xml`, `ir_module_module.xml`, countries, currencies, languages, companies, users
- models: `ir_model.py`, `ir_fields.py`, `ir_module.py`, `ir_actions.py`, `ir_ui_view.py`, `ir_ui_menu.py`, `ir_cron.py`, `ir_config_parameter.py`, `ir_attachment.py`, `res_users.py`, `res_company.py`, `res_partner.py`

Core implementation implications from local source:
- registry is per database and assembled from module definitions
- `_inherit` and `_inherits` require explicit metadata composition in Go
- XML/CSV/SQL data loading must be deterministic and preserve external IDs through `ir.model.data`
- lifecycle hooks are required for automation and mail; do not bolt callbacks onto CRUD later

## Commands You Will Need

| Purpose | Command | Expected on success |
|---------|---------|---------------------|
| Tests | `go test ./...` | exit 0 |
| Race tests | `go test -race ./...` | exit 0 |
| Vet | `go vet ./...` | exit 0 |
| Format | `test -z "$(gofmt -l .)"` | exit 0 |
| Module tests | `go test ./internal/module ./internal/registry ./internal/model ./internal/domain ./internal/data` | exit 0 |

## Scope

In scope:
- `internal/db`
- `internal/migrate`
- `internal/registry`
- `internal/module`
- `internal/model`
- `internal/field`
- `internal/record`
- `internal/domain`
- `internal/data`
- `internal/base`
- `migrations`
- `testdata/odoo19/base`

Out of scope:
- HTTP controllers
- web client
- security enforcement beyond placeholders
- accounting logic
- AI logic
- OI modules

## Git Workflow

- Branch: `advisor/002-base-kernel-orm-modules`
- Commit style: `kernel: add registry and ORM metadata`

## Steps

### Step 1: Add database and migration primitives

Implement:
- PostgreSQL-only adapter.
- Transaction wrapper.
- Savepoints.
- Migration table.
- Ordered migration execution.

Initial tables:
- `ir_module_module`
- `ir_model`
- `ir_model_fields`
- `ir_model_data`
- `ir_config_parameter`
- `ir_cron`
- `ir_cron_trigger`
- `ir_cron_progress`

Verify: `go test ./internal/db ./internal/migrate` exits 0.

### Step 2: Add module manifest and dependency graph

Define `module.Manifest` with fields equivalent to Odoo manifest essentials:
- `Name`
- `TechnicalName`
- `Version`
- `Category`
- `Depends`
- `Data`
- `Demo`
- `Assets`
- `Installable`
- `AutoInstall`
- `Application`

Support manifest format in YAML or JSON for Go modules. Store source compatibility metadata for imported Odoo/OI modules.

Verify: `go test ./internal/module -run TestManifest` exits 0.

### Step 3: Add registry

Implement per-database registry:
- module graph
- model metadata
- field metadata
- external IDs
- cache invalidation
- installed/upgrade states
- lifecycle hook registry for create/write/unlink/archive/unarchive/message/time triggers

Verify: `go test ./internal/registry -run TestRegistry` exits 0.

### Step 4: Add model and field metadata

Implement:
- model names such as `res.users`, `ir.model`, `ir.actions.act_window`
- field kinds: bool, int, float, decimal, char, text, date, datetime, selection, binary, many2one, one2many, many2many, computed, related
- metadata flags: required, readonly, index, translate, groups, company dependent, store

Verify: `go test ./internal/model ./internal/field` exits 0.

### Step 5: Add recordset and environment API

Implement:
- `Env`
- `ModelSet`
- `RecordSet`
- immutable recordset IDs
- cache and recompute queue placeholders
- `Browse`
- `Search`
- `Create`
- `Write`
- `Unlink`
- `Read`
- `Mapped`
- `Filtered`
- context/user/company fields as placeholders until Plan 003

Do not expose raw SQL to module code except through a controlled internal interface.

Verify: `go test ./internal/record -run TestRecordSet` exits 0.

### Step 6: Add domain parser and SQL compiler

Implement structured domain AST:
- logical: AND, OR, NOT
- operators: `=`, `!=`, `in`, `not in`, `<`, `<=`, `>`, `>=`, `like`, `ilike`, `child_of`
- parameterized SQL only
- invalid symbol rejection

Verify: `go test ./internal/domain -run TestDomain` exits 0.

### Step 7: Add XML/CSV data loader

Support first:
- XML root `odoo`
- record create/update by external ID
- field scalar values
- references
- noupdate flag
- CSV load for access rules and seed records

Create fixtures in `testdata/odoo19/base` based on minimal original records, not copied bulk data.

Verify: `go test ./internal/data -run TestLoad` exits 0.

## Test Plan

- Unit tests for manifest parsing.
- Unit tests for module dependency sorting and cycle detection.
- Unit tests for every domain operator.
- Integration tests for data import with external IDs.
- Transaction rollback tests.

## Done Criteria

- `go test ./...` exits 0.
- `go test -race ./...` exits 0.
- Base registry can install a minimal `base` module fixture.
- Domain compiler uses parameterized queries only.
- No code copied from local Odoo source.

## STOP Conditions

- Implementing Odoo Python dynamic inheritance in Go requires source copying or unbounded runtime eval.
- PostgreSQL is unavailable for integration tests and no test container strategy exists.
- Module format cannot be decided from Plan 001 conventions.

## Maintenance Notes

Do not optimize prefetch/cache before security and record rules exist. Keep APIs explicit.
