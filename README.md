# Gorp

Go ERP platform targeting Odoo 19-compatible base behavior.

## Scope

Target modules:
- base ORM, registry, modules, views, actions, menus
- users, groups, ACLs, record rules
- automation, scheduled actions, mail, activities
- OWL-compatible web client
- standard and enterprise-like themes
- `account` accounting
- AI
- installable OI apps: `oi_base`, `oi_workflow`, `oi_workflow_advance`, `oi_delegation`, `oi_login_as`

## Source Inputs

Reference trees:
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo18/odoo18-addons`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo17/odoo17-addons`

Use these trees for feature inventory and behavior tests. Do not copy proprietary Enterprise or OI source/assets without a written license/provenance decision.

## Setup

Required:
- Go 1.23+
- Node 22+
- pnpm

Commands:

```sh
pnpm -C frontend install
make ci
```

## Inventory

Generate source inventory:

```sh
go run ./tools/source_inventory --config inventory/sources.yaml --out reports/source_inventory.json
```

Check parity coverage:

```sh
go run ./tools/parity_check --inventory reports/source_inventory.json --coverage reports/parity.yaml
```

## Development

Run the server shell:

```sh
go run ./cmd/gorpd
```
