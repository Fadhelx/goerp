---
description: Map GoERP runtime ownership and verification commands.
---

# GoERP Runtime Map

Core ownership:

- `internal/base`: base model metadata, fixtures, technical menus, ACLs.
- `internal/record`: ORM, recordsets, domains, hooks, read/write/create/unlink.
- `internal/runtime/bootstrap.go`: bootstrapped app assembly.
- `internal/http/server.go`: Odoo-like HTTP, RPC, web client.
- `internal/security`: ACL and record rules.
- `internal/actions`, `internal/automation`, `internal/scheduler`: automation and server actions.
- `internal/mail`: mail, chatter, SMS, WhatsApp, fetchmail, digest.
- `addons/oi_*`: OI app manifests, data, security, and clean-room behavior.

Verification:

```sh
go test ./internal/base ./internal/record ./internal/security
go test ./internal/http
go test ./internal/runtime
go test ./addons/oi_base ./addons/oi_workflow ./addons/oi_workflow_advance ./addons/oi_delegation ./addons/oi_login_as
```

Run `go test ./...` before release claims.
