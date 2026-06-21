# UI Parity Master Report

Date: 2026-06-21
Scope: GoERP web UI versus Odoo 19 web UI.
Accounting: excluded. Phase 2 only.
Reference input: `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo`.

## Current Status

| Area | Status | Evidence |
| --- | --- | --- |
| Odoo reference | Available for source/runtime inspection | `odoo-bin --version` returns `Odoo Server 19.0`. |
| GoERP local | Pass | Fresh local server on `127.0.0.1:8073`; `/web` returned `102537` bytes. |
| GoERP production | Pass, prior deployed build | `https://api.fadhelalqaidoomxyz.xyz/web` returned `200 OK` through nginx. |
| Shell identity | Pass | Title `Odoo`; `.o_web_client`, `.o_main_navbar`, `.o_action_manager` present. |
| App launcher | Partial pass | App tiles: Approvals, Delegation, Settings, Apps. Nested action search improved locally. |
| Settings | Improved locally | Settings now opens `settingsView`, `3` settings blocks, `14` setting boxes. |
| Technical menus | Pass | Technical categories and direct actions reachable. |
| List view | Partial pass | Server Actions list: `20` rows, pager `1-20 / 20`. |
| Kanban view | Partial pass | Ungrouped kanban renderer and cards present. |
| Form view | Improved locally | Form switch active, record form visible, one active form control panel. |
| Mobile | Partial pass | `390x844` verified by subagent: app tiles visible, menu toggle present, no horizontal overflow. |

## Implemented In This Slice

- Added an Odoo-like Settings surface in `/web`.
- Added settings blocks for Users & Companies, Technical, Apps, and AI.
- Wired settings actions to existing menu/action records instead of showing an empty generic list.
- Added recursive Technical menu/action discoverability from the app launcher search.
- Added direct-action menu metadata to menu payloads.
- Fixed form view state so Form becomes the active switch and the list control panel hides while the record form is open.
- Changed empty list state to use `o_view_nocontent`.

## Remaining Gaps

P0:

1. Replace production inline `/web` shell with the bundled TS/OWL-style webclient runtime.
2. Implement real Odoo action stack: route state, breadcrumbs, action container, nested dialogs, target `new/current/main`.
3. Implement full control panel search model: filters, group by, favorites, saved filters, suggestions.
4. Implement Settings form parity: real configuration fields, app sidebar, unsaved changes, save/discard semantics.
5. Implement form renderer parity: groups, notebooks, modifiers, onchanges, x2many editors, field widgets.

P1:

1. Complete list renderer: row selection actions, sorting, optional columns, inline edit.
2. Complete kanban renderer: grouped columns, card templates, quick create, drag/drop, progress bars, record menus.
3. Implement systray dropdowns: messages, activities, company switcher, user menu, debug menu.
4. Add dialog/notification service parity: modal stack, confirmation dialogs, export/import dialogs, toasts.
5. Replace initials-based app icons with provenance-safe generated or licensed icon assets.

P2:

1. Add screenshot regression suite for app launcher, Settings, Technical menus, list, kanban, form, dialogs, and mobile.
2. Add production smoke that verifies selector counts after each deploy.
3. Add Odoo reference comparison captures when an authenticated reference session is available.

## Recommended Agent Assignments

1. `runtime-shell-worker`: replace inline `/web` with TS webclient bundle; own `internal/http/server.go`, frontend build integration, web assets.
2. `search-control-worker`: implement search model/dropdowns/favorites; own `frontend/packages/webclient/src/search/**` and control panel tests.
3. `settings-worker`: implement real Settings form renderer and save/discard; own settings view files and `res.config.settings` UI contracts.
4. `renderer-worker`: finish list/form/kanban parity; own renderer code and focused view tests.
5. `systray-dialog-worker`: implement systray dropdowns, dialog service, notifications; own navbar/dialog files.
6. `visual-regression-worker`: create browser screenshot checks for local/prod/reference parity.

## Verification Run

Passed:

- `go test -timeout=10m ./internal/http -run 'TestWebAliasesAndAssets|TestWebclientLoadMenusOdooShape|TestActionLoadOdooShapeAndJSONRPC|TestActionLoadNormalizesWindowDomainContextForWebShell|TestWebShellActionMetadataNormalizesPythonLiterals|TestCallKWGetViewsOdooShape|TestCallKWGetViewsToolbarBindings' -count=1`
- Browser local `127.0.0.1:8073/web`: Settings panel rendered `3` blocks and `14` setting boxes.
- Browser local `127.0.0.1:8073/web`: Server Actions list -> Kanban -> Form rendered one active form control panel and active switch `Form`.
- `make ci`

Pending:

- Production deploy of this local UI slice.
