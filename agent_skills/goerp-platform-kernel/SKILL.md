---
name: goerp-platform-kernel
description: Build or audit GoERP base platform internals: ORM, registry, module metadata, runtime bootstrap, security, automation, scheduler, mail, AI, and deployment checks.
---

# GoERP Platform Kernel

Use this skill when touching the Go implementation of Odoo base behavior.

## Scope

Core files:

- `internal/record`: in-memory ORM recordsets, domains, computed reads, create/write/unlink hooks.
- `internal/base`: base model metadata, fixtures, ACLs, users, groups, technical menus.
- `internal/runtime/bootstrap.go`: OI runtime assembly, fixture loading, module rows, assets, actions, menus, views, security, workflow hooks.
- `internal/http/server.go`: Odoo-shaped HTTP routes and `/web/dataset/call_kw`.
- `internal/security`: ACL and record-rule engine.
- `internal/actions`, `internal/automation`, `internal/scheduler`, `internal/mail`, `internal/ai`: platform services.
- `migrations`: SQL parity for persistent schema.

## Nonnegotiable Constraints

- Do not implement accounting features in phase 1. Preserve the default accounting gate.
- Do not copy proprietary Odoo Enterprise or OI code/assets. Inspect source behavior only.
- Keep runtime modules installable and testable as Go modules/manifests.
- Use Odoo-shaped route names, model names, fields, actions, menus, ACLs, and record rules where possible.
- Keep edits scoped. Do not refactor unrelated platform layers during a parity slice.

## Recon Checklist

1. Find the source model/route/fixture in `/Users/fadhelalqaidoom/Desktop/odoo`.
2. Find local equivalents with `rg`.
3. Read existing tests before editing.
4. Identify model fields, view/action/menu fixtures, ACLs, record rules, HTTP behavior, and side effects.
5. Decide the smallest local surface that makes the requested Odoo behavior true.

## Implementation Pattern

1. Add or correct metadata in the owning package:
   - model fields in `internal/base` or `addons/*`.
   - fixtures in `data`, `views`, or `security`.
   - SQL migrations only when persistent schema parity is required.
2. Add runtime behavior in the narrow service:
   - record hooks for ORM side effects.
   - HTTP dispatchers for Odoo RPC methods.
   - service helpers for mail, workflow, scheduler, automation, or AI.
3. Add tests at the behavior boundary:
   - `internal/record` for ORM semantics.
   - `internal/http` for route/RPC shape.
   - `internal/runtime` for bootstrapped integration.
   - addon package tests for module-specific parity.
4. Update dashboard source after a completed slice:
   - edit `reports/agent_audit_backlog.md`.
   - run `go run ./tools/progress_dashboard --out reports/progress_dashboard.html`.

## Verification Commands

Use the narrowest command first:

```sh
go test ./internal/base ./internal/record ./internal/security
go test ./internal/http -run '<focused test names>'
go test ./internal/runtime -run '<focused test names>'
go test ./addons/oi_workflow ./addons/oi_workflow_advance ./addons/oi_delegation ./addons/oi_login_as
```

Before release or deploy:

```sh
go test ./...
```

Use `make ci` when the user asks for CI-level proof.

## Completion Evidence

Report exact files changed, tests run, and runtime evidence. For deployment, include commit, push, active release path, service status, and HTTP status checks.
