# Kant UI Parity Backlog

Date: 2026-06-22
Workspace: `/Users/fadhelalqaidoom/Documents/gorp`
Scope: frontend/UI only. Accounting excluded.
Reference input: `/Users/fadhelalqaidoom/Desktop/odoo` and stored Odoo reference screenshots.

## Current Evidence

- GoERP current smoke: `reports/uiux/kant_goal_current_20260622/`
- Launcher compact fix smoke: `reports/uiux/kant_launcher_compact_20260622/`
- Odoo reference screenshots:
  - `reports/uiux/screenshots/odoo-desktop-apps.png`
  - `reports/uiux/screenshots/odoo-desktop-settings-debug.png`
  - `reports/uiux/screenshots/odoo-desktop-server-actions-list.png`
  - `reports/uiux/screenshots/odoo-desktop-server-action-form.png`
  - `reports/uiux/screenshots/odoo-mobile-home.png`

Local Odoo source exists at `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo-bin`.
No local Odoo reference server was listening on `:8069` or `:8072` during this pass.

## Implemented In This Pass

P0 launcher visual parity:
- Idle Apps launcher no longer shows the `Settings / Technical` path under the Apps tile.
- App launcher tiles now render as compact Odoo-like icon buttons instead of large bordered cards.
- Smoke now asserts launcher tile geometry and transparent card styling.

Changed ownership:
- `frontend/packages/webclient/src/home_menu/home_menu.ts`
- `frontend/packages/webclient/src/home_menu/home_menu.test.mjs`
- `internal/http/server.go`
- `tools/web_visual_smoke/run.mjs`
- `reports/uiux/kant_launcher_compact_20260622/*`

Verification:
- `pnpm -C frontend test -- home_menu.test.mjs index.test.mjs`: passed.
- `pnpm -C frontend build`: passed.
- `node --test tools/web_visual_smoke/run.test.mjs`: passed.
- `go test ./internal/http -run TestWebAliasesAndAssets -count=1`: passed.
- `node tools/web_visual_smoke/run.mjs --base-url=http://127.0.0.1:8079 --out=reports/uiux/kant_launcher_compact_20260622 --scenario=default-webclient-takeover --timeout-ms=60000`: passed.

DOM evidence:
- `app_card_width_px`: 92
- `app_card_height_px`: 96
- `app_card_bg`: `rgba(0, 0, 0, 0)`
- `app_card_border_color`: `rgba(0, 0, 0, 0)`
- `app_icon_width_px`: 54
- `app_icon_height_px`: 54

## Backlog With Ownership

P0. Launcher reference message and onboarding alert
- Gap: Odoo reference shows a top notification banner on fresh databases. GoERP launcher lacks this system notification area.
- Owner files: `frontend/packages/webclient/src/home_menu/**`, `frontend/packages/webclient/src/webclient/**`, `internal/http/server.go`.
- Evidence target: launcher screenshot with dismissible banner and DOM role/state.

P0. Form relation widgets
- Gap: readonly many2one and readonly x2many display improved, but editable many2one search, many2many tag editor, one2many list editor, create/edit dialogs, and access-error states remain incomplete.
- Owner files: `frontend/packages/webclient/src/index.ts`, `frontend/packages/webclient/src/index.test.mjs`, `tools/web_visual_smoke/run.mjs`.
- Evidence target: record open/edit flow with relation search, tag add/remove, save payload, and dialog screenshot.

P0. Apps install flow depth
- Gap: Apps list and lifecycle smoke pass, but Odoo-style app detail, dependency confirmation, upgrade/uninstall wizards, app icons, and installed-state transitions are still shallow.
- Owner files: `frontend/packages/webclient/src/**`, minimal backend only if lifecycle endpoint data is missing.
- Evidence target: Apps catalog screenshot and DOM checks for install/cancel/upgrade/uninstall flows.

P1. Search control panel parity
- Gap: filters/group-by/favorites exist, but saved favorite creation, advanced search domains, and date/group menus are incomplete.
- Owner files: `frontend/packages/webclient/src/index.ts`, `tools/web_visual_smoke/run.mjs`.
- Evidence target: saved favorite UI, active facets, persisted favorite DOM state.

P1. List and form density polish
- Gap: list/form renderers pass current smoke but still differ from Odoo in field widget density, button box, statusbar behavior, chatter, and form sidebar/chatter placement.
- Owner files: `frontend/packages/webclient/src/index.ts`, `internal/http/server.go`.
- Evidence target: Server Actions form and contact-style form screenshots against reference.

P1. Kanban parity
- Gap: kanban cards exist only in limited form; grouping, quick create, drag/drop, progress/state styling, and empty groups are incomplete.
- Owner files: `frontend/packages/webclient/src/index.ts`, focused tests under matching `*.test.mjs`.
- Evidence target: CRM-style kanban screenshot and DOM checks.

P1. Mobile parity
- Gap: mobile smoke passes no-overflow and basic flows, but Odoo mobile action drawer, search overlay, breadcrumbs, and view switch behavior remain incomplete.
- Owner files: `frontend/packages/webclient/src/**`, `internal/http/server.go`.
- Evidence target: mobile launcher/list/form screenshots with drawer/search interactions.

P2. Systray services
- Gap: systray dropdowns open/close, but mail/activity/company/debug/user services are mostly static.
- Owner files: `frontend/packages/webclient/src/webclient/**`, `frontend/packages/webclient/src/index.ts`.
- Evidence target: dropdown screenshots with real action targets and no static dead entries.

## External Requests

- Main agent should run full `make ci` before merge/deploy.
- Do not deploy from this UI worker goal.
