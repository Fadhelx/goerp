# UI Parity Master Report

Date: 2026-06-21
Scope: `/web` visual and behavior parity with Odoo 19 Enterprise for normal users.
Accounting: excluded for phase 2.
Reference source: `/Users/fadhelalqaidoom/Desktop/odoo/odoo19` only. No proprietary Odoo/OI code or assets copied.

## Result

GoERP `/web` is usable for testing and now looks materially closer to Odoo Enterprise, but it is not yet 100% indistinguishable.

Current parity estimate:
- Shell and launcher: 60-65%.
- Settings and technical list/form: 55-60%.
- Full Enterprise behavior parity: 40-50%.

## Reference Status

Local Odoo 19 source exists:
- Community branch: `19.0` at `022d7b613da`.
- Enterprise branch: `19.0` at `186615d6`.

Local Odoo reference instance was not runnable in the current machine runtime:
- System Python is `3.9.6`, below Odoo 19 requirements.
- Framework Python 3.11/3.12 hung during import probes.
- Docker fallback had no local image; registry probe timed out.

Existing Odoo screenshots were used as visual reference:
- `reports/uiux/screenshots/odoo-desktop-apps.png`
- `reports/uiux/screenshots/odoo-desktop-server-actions-list.png`
- `reports/uiux/screenshots/odoo-desktop-server-action-form.png`
- `reports/uiux/screenshots/odoo-desktop-settings-debug.png`
- `reports/uiux/screenshots/odoo-mobile-home.png`

## Implemented This Slice

- App launcher dark Enterprise-style full-height viewport.
- Broken app icon images removed; raw valid base64 icons become data URLs, invalid/missing icons fall back to generated initials.
- Apps catalog card resolves to real `Settings / Technical / Apps` menu when present.
- Launcher aliases added for `install`, `modules`, `developer`, and `configuration`.
- Navbar active app state updates from launcher cards and top nav.
- Descendant technical/app actions keep the root app active.
- View switcher and pager changed to icon-only controls with labels/titles.
- Dialog shell gained `aria-labelledby`, focus target, and Escape close handling.
- List renderer gained Odoo-like classes and mobile cards.
- Form renderer gained Odoo-like sheet/body classes, title block, statusbar placement, and readonly field widget classes.
- Release root detection now checks executable directory before compile-time source paths, fixing production release verification from extracted archives.
- Normal internal users created in `res.users` can authenticate without restarting the server, and their hydrated security context carries company, partner, and group metadata.
- The app launcher no longer exposes the Apps catalog unless the authenticated menu payload includes an accessible Apps menu.
- Normal-user visual smoke now proves Approvals is visible while Apps, Delegation, Settings, and Technical are hidden.
- The default TypeScript `/web` Apps catalog now renders an Odoo-shaped module catalog from `/web/session/modules`, preserves the active catalog search across lifecycle actions, and covers install, upgrade, uninstall, cancel install, cancel upgrade, cancel uninstall, and restore smoke for the `ai` module.
- Direct `ir.module.module` lifecycle RPC calls now require admin/system-user context.
- Default mobile `/web` smoke now opens Server Actions, renders mobile list cards, opens a form, verifies breadcrumbs/form sheet visibility, checks hash routing, and proves no horizontal overflow.
- The TypeScript systray now opens message, activity, company, debug, and user dropdowns, closes on Escape/outside click, and stays out of the action manager state.
- The default TypeScript systray now fetches live `/mail/data` Store metadata during bootstrap and renders backend inbox, starred, and activity counters, activity group rows, current-company state, Odoo-like systray order, and typed menu actions.

## Evidence

Local app:
- URL: `http://127.0.0.1:8069/web`
- Visual smoke: 23/23 passed.
- Manifest: `reports/uiux/ui_parity_master_20260621_live/manifest.json`
- Apps lifecycle manifest: `reports/uiux/ui_parity_master_20260621_live_apps_lifecycle/manifest.json`
- Apps cancel-state manifest: `reports/uiux/ui_parity_master_20260621_live_apps_cancel/manifest.json`
- Mobile flow manifest: `reports/uiux/ui_parity_master_20260621_live_mobile_flow/manifest.json`
- Systray manifest: `reports/uiux/ui_parity_master_20260621_live_systray/manifest.json`

Key screenshots:
- `reports/uiux/ui_parity_master_20260621_live/default-webclient-takeover.png`
- `reports/uiux/ui_parity_master_20260621_live/default-systray-dropdowns-desktop.png`
- `reports/uiux/ui_parity_master_20260621_live/default-webclient-action-desktop.png`
- `reports/uiux/ui_parity_master_20260621_live/default-technical-search-desktop.png`
- `reports/uiux/ui_parity_master_20260621_live/default-technical-form-desktop.png`
- `reports/uiux/ui_parity_master_20260621_live/default-webclient-mobile.png`
- `reports/uiux/ui_parity_master_20260621_live/default-mobile-server-actions-flow.png`
- `reports/uiux/ui_parity_master_20260621_live/normal-user-launcher-desktop.png`
- `reports/uiux/ui_parity_master_20260621_live/default-apps-install-desktop.png`
- `reports/uiux/ui_parity_master_20260621_live/default-apps-lifecycle-cancel-desktop.png`

Renderer worker evidence:
- `reports/verification/renderer_dialog_control_panel_20260621/manifest.json`

## Remaining Gaps

P0:
- Launcher still lacks real Odoo app icon artwork, enterprise alert panel behavior, exact spacing, and top-right systray-only home layout.
- Control panel lacks search autocomplete, custom filter/group dialogs, saved `ir.filters`, and full mobile search panel.
- Action manager lacks complete breadcrumb stack, browser back/forward parity, nested actions, full `target="new"` lifecycle, and client action coverage.
- List renderer lacks grouped rows, column sorting/resizing, optional columns, select-all/bulk action states, aggregates, and inline edit.
- Form renderer lacks full arch layout support: groups, notebooks, smart buttons, relation widgets, onchange, x2many editors, modifiers, and dirty guards.

P1:
- Apps install flow has bounded method-backed install, upgrade, uninstall, cancel-state, and restore smoke coverage; it still needs Odoo-like categories, filters, module detail cards, and confirmation/wizard behavior.
- Dialogs need footer action mapping, stacked inactive state, confirmation dialogs, backdrop policy, draggable desktop, and mobile fullscreen/bottom-sheet behavior.
- Systray now uses real Store counters and activity groups; remaining work is full Discuss inbox/channels UI, activity edit/mark-done popovers, persistent company switching, and mobile burger relocation.
- Kanban needs grouped columns, quick create, drag/drop, folded groups, progress bars, and load-more.
- Mobile shell is usable without overflow, but still desktop-shaped.

P2:
- Pivot, graph, calendar, activity, cohort, gantt, and dashboard views are not implemented.
- OWL compatibility exports are still shallow for many field widgets/services.
- Visual regression suite still needs dialog and mobile search-panel scenarios.

## Bounded Implementation Tasks

1. Implement full action stack and breadcrumb lifecycle.
2. Implement search autocomplete, custom filters/groups, and saved filter persistence.
3. Implement list grouping/sorting/optional-columns/select-all.
4. Implement form notebooks/groups/smart-buttons/x2many/onchange/dirty guards.
5. Expand Apps catalog to categories, filters, module detail behavior, and confirmation/wizard flows.
6. Expand systray from live Store counters/actions into full Discuss, activity popover, company switch, and mobile behavior.
7. Implement kanban grouped columns and quick-create.
8. Add production smoke after every deploy for `/web`, Settings, Server Actions, view switch, search filter, normal user, and mobile.

## Agents

- Reference agent: found Odoo 19 source, could not run local Odoo due Python/Docker blockers, returned reference checklist.
- Audit agent: confirmed `/web` is partially implemented and identified stale `frontend/dist` risk.
- Navigation/settings worker: implemented app catalog routing/search aliases/navbar active state.
- Renderer/dialog worker: implemented view/pager icons, dialog accessibility, list mobile cards, and form sheet polish.
