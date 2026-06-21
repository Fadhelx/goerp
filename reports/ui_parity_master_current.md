# UI Parity Master Current Report

Date: 2026-06-21
Workspace: `/Users/fadhelalqaidoom/Documents/gorp`
Scope: GoERP `/web` UI parity with Odoo 19 Enterprise.
Accounting: excluded. Phase 2 only.
Reference input: `/Users/fadhelalqaidoom/Desktop/odoo/odoo19` inspected only as reference. No proprietary source/assets copied.

## Status

GoERP `/web` is usable for testing.

Current parity estimate: 40-50%.

Current slices fixed the highest visible form-header issue, added a passive frontend bootstrap path, and added Odoo-style hash route restore for list/form navigation. Full Odoo parity is still incomplete because the default `/web` runtime still uses the inline Go shell.

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

## Changed Files

- `internal/http/server.go`
- `internal/http/server_test.go`
- `internal/record/record.go`
- `internal/record/record_test.go`
- `internal/runtime/bootstrap.go`
- `frontend/apps/webclient/src/main.ts`
- `frontend/apps/webclient/src/main.test.mjs`
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
- URL: `http://127.0.0.1:8073/web`
- Visual smoke: 10/10 passed.
- Manifest: `reports/web_visual_smoke/manifest.json`

Screenshots:
- `reports/web_visual_smoke/launcher-desktop.png`
- `reports/web_visual_smoke/settings-desktop.png`
- `reports/web_visual_smoke/ts-webclient-takeover.png`
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
- TS takeover desktop: Odoo shell, navbar, action manager, and app launcher render from loaded session/menu data.
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
- `pnpm -C frontend test apps/webclient/src/main.test.mjs packages/webclient/src/index.test.mjs`
- `pnpm -C frontend build`
- `node --test tools/web_visual_smoke/run.test.mjs`
- `node tools/web_visual_smoke/run.mjs --base-url=http://127.0.0.1:8073 --out=tmp/verification/local_runtime_slice_smoke --timeout-ms=30000`
- `node tools/web_visual_smoke/run.mjs --base-url=http://127.0.0.1:8073 --timeout-ms=30000`
- `make ci`

## P0 Mismatches

1. Default `/web` still uses the inline Go shell.
   - Required: bundled TS/OWL runtime owns shell, action manager, services, menus, dialogs, and routing.

2. Action stack is incomplete.
   - Improved: basic hash state now covers menu/action/model/list/kanban/form record restore.
   - Required: dialog target `new`, deeper breadcrumb stack behavior, multi-action stack history, and stale-panel cleanup.

3. Settings is not full Odoo Settings.
   - Required: typed settings fields, dirty save/discard, module sections, Technical settings depth, search, and company/user scoped controls.

4. List/form renderers are partial.
   - Required: row selection/action menus/sort/grouping/edit gates; form buttons/statusbars/notebooks/modifiers/onchange/x2many/chatter.

5. Systray/mobile parity is partial.
   - Required: user/company/debug/mail/activity dropdowns, mobile burger/back navigation, responsive action state.

6. Apps catalog parity is partial.
   - Required: app catalog metadata, module install/update states, categories, provenance-safe icons, and post-install refresh.

## Implementation Status

Complete for this slice.

Not complete for full UI parity.

Next implementation target: make the bundled TS/OWL webclient the default `/web` owner while preserving current passing visual smoke.
