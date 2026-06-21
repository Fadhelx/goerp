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

## Production Release Packaging

Do not deploy `gorpd` as a binary-only artifact. Runtime bootstrap resolves the release root, then reads fixture and asset files from `internal/base` and `addons`.

Build releases as source trees:

```sh
release="gorp-$(git rev-parse --short HEAD)"
rm -rf "dist/$release"
mkdir -p dist
git archive --format=tar --prefix="$release/" HEAD | tar -C dist -xf -
go build -o "dist/$release/gorpd" ./cmd/gorpd
test -f "dist/$release/go.mod"
test -f "dist/$release/internal/base/data/res_bank.xml"
test -f "dist/$release/addons/oi_workflow/data/ir_cron.xml"
(cd "dist/$release" && ./gorpd modules >/dev/null)
tar -C dist -czf "dist/$release.tar.gz" "$release"
```

Deploy the unpacked release directory. Start `./gorpd serve` from inside that directory, or make the service working directory point at that directory.

## Agent Guidance

Repo-local agent skills live in `agent_skills/`.

Eve-ready agent blueprints live in `agents/eve/`. They follow Eve's filesystem-first layout with `agent/instructions.md`, optional `agent.ts`, `skills/`, and `subagents/`.

Use these files to start future platform, OI parity, and UI verification agents without rediscovering repository structure.
