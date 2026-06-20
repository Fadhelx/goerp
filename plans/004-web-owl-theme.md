# Plan 004: Build Views, Actions, Menus, Web Controllers, OWL-Compatible Web Client, Themes

> Executor instructions: implement metadata-backed web foundations and a clean-room OWL-compatible frontend.
>
> Drift check: run `git diff --stat -- internal/http internal/meta internal/assets frontend`.

## Status

- Priority: P1
- Effort: L
- Risk: HIGH
- Depends on: `plans/002-base-kernel-orm-modules.md`, `plans/003-security-users-acl-rules.md`
- Category: architecture
- Planned at: no commits, 2026-06-16

## Why This Matters

The base ERP needs views, actions, menus, controllers, assets, and a web client before users can install and operate modules. Odoo 19 uses Owl components, QWeb templates, registries, services, hooks, and asset bundles. The implementation must be clean-room and original.

## Current State

Reference paths:
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/web`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/web/controllers`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/web/models`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/web/static/src`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo/addons/base/models/ir_ui_view.py`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo/addons/base/models/ir_actions.py`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo/addons/base/models/ir_ui_menu.py`

Public docs confirm Owl is a declarative component framework using QWeb templates and Odoo-specific directives.

## Commands You Will Need

| Purpose | Command | Expected on success |
|---------|---------|---------------------|
| Go tests | `go test ./internal/http ./internal/meta ./internal/assets` | exit 0 |
| Frontend install | `pnpm -C frontend install` | exit 0 |
| Frontend typecheck | `pnpm -C frontend typecheck` | exit 0 |
| Frontend tests | `pnpm -C frontend test` | exit 0 |
| Frontend build | `pnpm -C frontend build` | exit 0 |
| Browser tests | `pnpm -C frontend test:e2e` | exit 0 |

## Scope

In scope:
- `internal/http`
- `internal/meta/view`
- `internal/meta/action`
- `internal/meta/menu`
- `internal/assets`
- `frontend/packages/owl-compat`
- `frontend/packages/template-compiler`
- `frontend/packages/qweb-runtime`
- `frontend/packages/webclient`
- `frontend/packages/theme-tokens`
- `frontend/themes/standard`
- `frontend/themes/enterprise-like`
- `frontend/apps/dev-shell`

Out of scope:
- exact Odoo Enterprise theme copying
- Odoo logos/assets/icons
- website builder
- POS
- mobile native apps

## Steps

### Step 1: Implement web controllers

Add routes equivalent in role to:
- session info
- login/logout
- dataset model RPC
- action load
- view load
- menu load
- binary attachment
- asset bundle serving

All model RPC calls must use Env from Plan 003.

Verify: `go test ./internal/http -run TestWebRoutes` exits 0.

### Step 2: Implement view/action/menu metadata

Models:
- `ir.ui.view`
- `ir.ui.menu`
- `ir.actions.act_window`
- `ir.actions.server`
- `ir.actions.report`
- `ir.actions.client`

Support:
- XML arch storage
- view inheritance hooks
- menu hierarchy
- action domains/context
- security filtering

Verify: `go test ./internal/meta/...` exits 0.

### Step 3: Implement asset bundles

Bundle names:
- `web.assets_common`
- `web.assets_backend`
- `web.assets_frontend`
- `web.assets_unit_tests`

Operations:
- append
- prepend
- include
- before
- after
- remove
- replace

Output:
- hashed JS/CSS/XML
- manifest JSON
- source maps

Verify: `go test ./internal/assets -run TestBundleOrder` exits 0.

### Step 4: Implement OWL-compatible package

In `frontend/packages/owl-compat`, implement:
- `Component`
- `xml`
- `useState`
- `useRef`
- `useEnv`
- `useService`
- lifecycle hooks
- microtask scheduler
- class components with `static template`, `static props`, `setup()`, `this.env`

Verify: `pnpm -C frontend test -- owl-compat` exits 0.

### Step 5: Implement QWeb/template compiler

Support first:
- `t-name`
- `t-if`
- `t-elif`
- `t-else`
- `t-foreach`
- `t-as`
- `t-key`
- `t-esc`
- `t-out`
- `t-att`
- `t-attf`
- `t-on`
- `t-model`
- slots

Use an expression sandbox. Do not use raw `eval`.

Verify: `pnpm -C frontend test -- template-compiler qweb-runtime` exits 0.

### Step 6: Implement webclient shell and registries

Registries:
- actions
- fields
- views
- services
- main_components

Services:
- rpc
- orm
- action
- notification
- dialog
- user
- router
- bus

Verify: `pnpm -C frontend test -- webclient` exits 0.

### Step 7: Implement themes

Create:
- `frontend/packages/theme-tokens`
- `frontend/themes/standard`
- `frontend/themes/enterprise-like`

Rules:
- original design tokens
- no Odoo logos
- no copied Odoo icons
- no exact Odoo Enterprise colors/screenshots/layouts
- support light/dark, compact/comfortable, mobile, RTL, high contrast

Verify: `pnpm -C frontend build` exits 0 and Playwright screenshot tests pass.

## Test Plan

- Go route tests.
- Menu/action/view security tests.
- Asset ordering tests.
- Template escaping tests.
- OWL lifecycle tests.
- Browser tests for login shell, menu load, action open, form/list view render.

## Done Criteria

- `go test ./...` exits 0.
- `pnpm -C frontend typecheck && pnpm -C frontend test && pnpm -C frontend build` exits 0.
- No proprietary image/theme assets copied.
- Browser test opens dev shell and renders a nonblank app.

## STOP Conditions

- Template compiler requires arbitrary JS eval.
- Theme request requires exact proprietary Enterprise visual clone.
- View inheritance needs source-copying from Odoo web internals.

## Maintenance Notes

Build compatibility fixtures before expanding widget coverage.
