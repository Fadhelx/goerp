---
name: goerp-oi-parity
description: Deliver clean-room OI module parity for `oi_workflow`, `oi_workflow_advance`, `oi_delegation`, and `oi_login_as`.
---

# GoERP OI Parity

Use this skill for OI app parity slices.

## Source Inputs

Reference only:

- `/Users/fadhelalqaidoom/Desktop/odoo/odoo18/odoo18-addons`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo17/odoo17-addons`
- any OI reference path provided by the user.

Do not copy proprietary source code or assets. Use behavior, model names, field names, action/menu shape, and testable semantics as reference.

## Local Modules

- `addons/oi_base`
- `addons/oi_workflow`
- `addons/oi_workflow_advance`
- `addons/oi_delegation`
- `addons/oi_login_as`
- shared runtime hooks in `internal/workflow`, `internal/delegation`, `internal/impersonation`, `internal/http`, and `internal/runtime`.

## Delivery Loop

1. Pick one bounded OI behavior.
2. Inspect source model, views, security, data, and JS assets.
3. Inventory local coverage with `rg`.
4. Implement only the missing local behavior.
5. Add focused tests:
   - addon tests for manifest/data/security coverage.
   - `internal/workflow` or `internal/delegation` tests for service behavior.
   - `internal/http` tests for routes/RPC.
   - `internal/runtime` tests for bootstrapped integration.
6. Update dashboard.

## Common Parity Surfaces

- approval state transitions and workflow nodes.
- approval buttons and server actions.
- delegation record-rule expansion and revocation.
- login-as systray/debug routes and security groups.
- view/action/menu metadata.
- mail thread/activity integration.
- assets in `web.assets_backend`.

## Tests

```sh
go test ./addons/oi_base ./addons/oi_workflow ./addons/oi_workflow_advance ./addons/oi_delegation ./addons/oi_login_as
go test ./internal/workflow ./internal/delegation ./internal/impersonation
go test ./internal/http -run '(Workflow|Delegation|LoginAs|Approval)'
go test ./internal/runtime -run '(Bootstrap|Workflow|Delegation|LoginAs|Approval)'
```

Run broader tests before release:

```sh
go test ./...
```
