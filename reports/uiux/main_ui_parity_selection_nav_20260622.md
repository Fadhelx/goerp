# UI Parity Selection/Nav Slice

Date: 2026-06-22
Scope: default `/web` UI parity, accounting excluded.
Reference policy: clean-room only. `/Users/fadhelalqaidoom/Desktop/odoo` used as reference input only.

## Implemented

- Navbar now swaps root apps for active app section menus, keeps the app brand in the topbar, and highlights nested Technical/Settings sections.
- Launcher now includes a dismissible database-registration notice and keeps compact Enterprise-style app spacing.
- Apps catalog sidebar/detail layout now remains visible in the TypeScript webclient.
- Control-panel view switcher and pager now render compact icon-style controls.
- Readonly selection fields render Odoo-style selection pills in forms.
- Editable selection fields render Odoo-style radio pills when choices are known.
- Server Action forms now include a clean-room header/meta band, Code/Help notebook, readonly code viewer, and code-editor-style textarea.
- `ir.actions.server.state` now exposes explicit selection metadata for runtime `fields_get` and visual form rendering.
- `default-technical-form-desktop` smoke now asserts Server Action band, notebook, code viewer/editor, state pills, and state radio editor.

## Evidence

- Canonical visual smoke: `reports/web_visual_smoke/manifest.json`
- Canonical screenshots: `reports/web_visual_smoke/`
- Focused Server Action evidence: `reports/uiux/kant_server_action_form_v1_20260622/`

## Verification

- `go test ./internal/base -run TestRegisterModels -count=1`
- `go test ./internal/http -run TestWebAliasesAndAssets -count=1`
- `pnpm -C frontend test -- index.test.mjs webclient/navbar/navbar.test.mjs webclient/shell.test.mjs home_menu/home_menu.test.mjs apps/webclient/src/main.test.mjs`
- `node --test tools/web_visual_smoke/run.test.mjs`
- `node tools/web_visual_smoke/run.mjs --base-url=http://127.0.0.1:8084 --out=reports/web_visual_smoke --timeout-ms=60000`

Full canonical visual smoke passed 26/26 scenarios.

## Remaining

- Server Action form still needs exact statusbar/action buttons, conditional fields, and source-level code editor behavior.
- Search custom filter/group/favorite flows remain shallow.
- Grouped kanban columns remain incomplete.
- Mobile shell needs closer Odoo mobile navigation and systray behavior.
- Chatter/activity composer depth remains incomplete.
- Apps install/upgrade confirmation and dependency wizard depth remains incomplete.
