# UI Parity Master Current Report

Date: 2026-06-21
Workspace: `/Users/fadhelalqaidoom/Documents/gorp`
Scope: GoERP `/web` UI parity with Odoo 19 Enterprise.
Accounting: excluded. Phase 2 only.
Reference input: `/Users/fadhelalqaidoom/Desktop/odoo/odoo19` inspected only as reference. No proprietary source/assets copied.

## Status

GoERP `/web` is usable for testing.

Current parity estimate: 45-55%.

Current slices fixed the highest visible form-header issue, added Odoo-style hash route restore for list/form navigation, made the bundled TypeScript/Odoo-like webclient own default `/web`, and wired the clean-room Settings renderer into `res.config.settings` window actions. Full Odoo parity is still incomplete because dialogs, advanced Settings behavior, systray, Apps install, and deeper view renderers are not complete.

## Implemented This Slice

- Fixed form control-panel layout so Save/Discard, breadcrumbs, title, pager, and form sheet do not overlap on desktop or mobile.
- Added a visible Odoo-style pager lane in record forms.
- Added `/web/static/frontend/` serving for compiled frontend dist assets.
- Injected the passive TypeScript webclient bootstrap script when `frontend/dist/apps/webclient/src/main.js` exists.
- Added `frontend/apps/webclient/src/main.ts` bootstrap entrypoint.
- Extended frontend tests to cover `frontend/apps/**`.
- Extended visual smoke to cover mobile technical form.
- Added form-header bounding-box assertions to visual smoke.
- Fixed visual smoke navigation reset with a per-scenario cache-busted `/web?smoke=N` URL.
- Integrated a small `res.users` read-mutation guard after local smoke exposed a concurrent derived-field map mutation crash during session-info reads.
- Added hash route writing/restoration for menu, action, model, list, kanban, and form record state.
- Added browser back/forward route restoration hooks.
- Improved `?ts_webclient=1` takeover to render the shared Odoo-like shell from session/menu data.
- Added `hash-route-desktop` and `ts-webclient-takeover` visual smoke coverage.
- Made the TypeScript webclient the default `/web` runtime, with the legacy inline shell available through `?legacy_webclient=1`.
- Added default `/web` visual smoke coverage for TS shell takeover and Settings action rendering.
- Wired TS app/menu clicks to load window actions into the shared renderer and write route hash state.
- Added action-stack route metadata, current-route snapshots, target `new` dialog route exclusion, target `main` stack clearing, and history-state stack payloads.
- Added search facet category/value metadata and Odoo-like multi-value facet rendering.
- Added a clean-room settings renderer foundation for Odoo-style `<app>`, `<block>`, `<setting>`, typed controls, and simple invisible-expression handling.
- Wired the Settings renderer into TS window actions with dirty state, Save/Discard buttons, `webSave` persistence, and discard rerender.
- Added route-stack active-id merge and nested dialog parent regressions.

## Changed Files

- `internal/http/server.go`
- `internal/http/server_test.go`
- `internal/record/record.go`
- `internal/record/record_test.go`
- `internal/runtime/bootstrap.go`
- `frontend/apps/webclient/src/main.ts`
- `frontend/apps/webclient/src/main.test.mjs`
- `frontend/packages/webclient/src/index.ts`
- `frontend/packages/webclient/src/index.test.mjs`
- `frontend/packages/webclient/src/control_panel/control_panel.ts`
- `frontend/packages/webclient/src/control_panel/control_panel.test.mjs`
- `frontend/packages/webclient/src/router/action_router.ts`
- `frontend/packages/webclient/src/router/action_router.test.mjs`
- `frontend/packages/webclient/src/search/search_model.ts`
- `frontend/packages/webclient/src/search/search_model.test.mjs`
- `frontend/packages/webclient/src/settings/settings_renderer.ts`
- `frontend/packages/webclient/src/settings/settings_renderer.test.mjs`
- `frontend/packages/webclient/src/services/action_stack.ts`
- `frontend/packages/webclient/src/services/action_stack.test.mjs`
- `frontend/packages/webclient/src/webclient/shell.ts`
- `frontend/packages/webclient/src/webclient/shell.test.mjs`
- `frontend/scripts/build.mjs`
- `frontend/scripts/test.mjs`
- `tools/web_visual_smoke/run.mjs`
- `tools/web_visual_smoke/run.test.mjs`
- `reports/agent_audit_backlog.md`
- `reports/progress_dashboard.html`
- `reports/ui_parity_master_current.md`
- `reports/web_visual_smoke/manifest.json`
- `reports/web_visual_smoke/technical-form-desktop.png`
- `reports/web_visual_smoke/technical-form-mobile.png`
- `reports/web_visual_smoke/technical-list-mobile.png`

## Local Evidence

GoERP local:
- URL: `http://127.0.0.1:8069/web`
- Visual smoke: 11/11 passed.
- Manifest: `reports/web_visual_smoke/manifest.json`

Screenshots:
- `reports/web_visual_smoke/launcher-desktop.png`
- `reports/web_visual_smoke/settings-desktop.png`
- `reports/web_visual_smoke/default-webclient-takeover.png`
- `reports/web_visual_smoke/default-webclient-action-desktop.png`
- `reports/web_visual_smoke/technical-list-desktop.png`
- `reports/web_visual_smoke/hash-route-desktop.png`
- `reports/web_visual_smoke/technical-form-desktop.png`
- `reports/web_visual_smoke/search-menu-desktop.png`
- `reports/web_visual_smoke/launcher-mobile.png`
- `reports/web_visual_smoke/technical-list-mobile.png`
- `reports/web_visual_smoke/technical-form-mobile.png`

Smoke assertions:
- Launcher desktop: 4 app tiles, 5 systray entries.
- Settings desktop: 3 settings blocks, 14 settings boxes.
- Default TS takeover desktop: Odoo shell, navbar, action manager, and app launcher render from loaded session/menu data on plain `/web`.
- Default TS action desktop: Settings action opens in the shared renderer with title `Settings`, action hash, control panel, `.o_settings_container`, and disabled Save/Discard buttons.
- Technical list desktop: Server Actions, 20 rows.
- Hash route desktop: Server Actions writes `#action`, `model`, `view_type`, and `menu_id`, then reloads back into the list.
- Technical form desktop: 6 fields, no header overlap.
- Search menu desktop: 3 filters, 4 group-by entries, 2 favorites.
- Launcher mobile: 4 app tiles, menu toggle, 0 px horizontal overflow.
- Technical list mobile: 20 cards, 0 px horizontal overflow.
- Technical form mobile: 6 fields, no header overlap, 0 px horizontal overflow.

## Tests Run

- `go test -timeout=10m ./internal/http -run 'Test(WebAliasesAndAssets|FrontendDistAssetAndBootstrapScript|AssetDebugFileServesBundleMember)$'`
- `go test -timeout=10m ./internal/http -run 'Test(WebAliasesAndAssets|FrontendDistAssetAndBootstrapScript|AssetDebugFileServesBundleMember|WebRoutes|WebclientLoadMenusOdooShape|ActionLoadOdooShapeAndJSONRPC|ActionLoadNormalizesWindowDomainContextForWebShell|CallKWGetViewsOdooShape)$'`
- `pnpm -C frontend test packages/webclient/src/index.test.mjs packages/webclient/src/settings/settings_renderer.test.mjs packages/webclient/src/services/action_stack.test.mjs packages/webclient/src/router/action_router.test.mjs apps/webclient/src/main.test.mjs`
- `pnpm -C frontend build`
- `node --test tools/web_visual_smoke/run.test.mjs`
- `node tools/web_visual_smoke/run.mjs --base-url=http://127.0.0.1:8069 --out=tmp/verification/settings_renderer_local --scenario=default-webclient-action-desktop --timeout-ms=30000`
- `node tools/web_visual_smoke/run.mjs --base-url=http://127.0.0.1:8069 --out=reports/web_visual_smoke --timeout-ms=30000`
- `make ci`

## P0 Mismatches

1. Action stack and dialogs are incomplete.
   - Improved: route-stack metadata, active-id merge, nested dialog parents, current-route snapshots, target `new` route exclusion, target `main` stack clearing, and history-state stack payloads.
   - Required: render target `new` modals, implement stale-panel cleanup, and support deeper breadcrumb navigation.

2. Settings is not full Odoo Settings.
   - Improved: Odoo-style app/block/setting parsing, typed controls, dirty Save/Discard state, save event/persistence path, and discard rerender.
   - Required: Settings search, module install links, confirmation flows, company/user scoped controls, richer invisible/modifier expressions, and exact settings navigation polish.

3. List/form renderers are partial.
   - Required: row selection/action menus/sort/grouping/edit gates; form buttons/statusbars/notebooks/modifiers/onchange/x2many/chatter.

4. Systray/mobile parity is partial.
   - Required: user/company/debug/mail/activity dropdowns, mobile burger/back navigation, responsive action state.

5. Apps catalog parity is partial.
   - Required: app catalog metadata, module install/update states, categories, provenance-safe icons, and post-install refresh.

## Implementation Status

Complete for this slice.

Not complete for full UI parity.

Next implementation target: implement target `new` modal rendering and Settings search/company-scope behavior in the live TS action manager.
