# UI Parity Master Current Plan

Date: 2026-06-21
Workspace: `/Users/fadhelalqaidoom/Documents/gorp`
Scope: `/web` UI parity with Odoo 19 Enterprise.
Accounting: excluded. Phase 2 only.
Reference input: `/Users/fadhelalqaidoom/Desktop/odoo` used only as reference.

## Current Result

GoERP `/web` is usable for first testing, but it is not yet 100% Odoo Enterprise UI parity.

Current parity estimate: 35-45%.

Reason: the visible `/web` route still behaves like a static server shell. The frontend TS webclient now has stronger Odoo-compatible pieces, but full parity requires making that runtime own `/web` actions, routing, dialogs, settings, views, and services.

## Evidence Created

Local server:
- `http://127.0.0.1:8075/web`
- Health: `{"status":"ok"}`

Visual smoke:
- Passed 7/7 scenarios.
- Report: `reports/uiux/master_current_smoke/manifest.json`

Screenshots:
- `reports/uiux/master_current_smoke/launcher-desktop.png`
- `reports/uiux/master_current_smoke/settings-desktop.png`
- `reports/uiux/master_current_smoke/technical-list-desktop.png`
- `reports/uiux/master_current_smoke/technical-form-desktop.png`
- `reports/uiux/master_current_smoke/search-menu-desktop.png`
- `reports/uiux/master_current_smoke/launcher-mobile.png`
- `reports/uiux/master_current_smoke/technical-list-mobile.png`

Subagent reports:
- Reference audit: `reports/uiux/subagents/reference-audit/odoo19_web_reference_audit.md`
- Current runtime audit: `reports/uiux/subagents/current-audit/current-ui-runtime-audit.md`

Additional subagent screenshots:
- `reports/uiux/subagents/current-audit/`

## Verified Working Now

Smoke assertions:
- Launcher desktop: 4 app tiles, 5 systray entries.
- Settings desktop: 3 setting blocks, 14 setting boxes.
- Technical list desktop: Server Actions, 20 rows.
- Technical form desktop: Process Workflow Escalation, 6 fields.
- Search menu desktop: 3 filters, 4 group by entries, 2 favorites.
- Launcher mobile: 4 app tiles, mobile menu toggle, 0 px horizontal overflow.
- Technical list mobile: 20 cards, 0 px horizontal overflow.

Checks:
- `pnpm -C frontend test`: passed.
- `make ci`: passed.

## Implementation Begun

Frontend changes present in the working tree:
- `frontend/packages/webclient/src/home_menu/home_menu.ts`
- `frontend/packages/webclient/src/home_menu/home_menu.test.mjs`
- `frontend/packages/webclient/src/webclient/navbar/navbar.ts`
- `frontend/packages/webclient/src/webclient/navbar/navbar.test.mjs`
- `frontend/packages/webclient/src/index.ts`
- `frontend/packages/webclient/src/index.test.mjs`
- `frontend/packages/webclient/src/search/search_model.ts`
- `frontend/packages/webclient/src/search/search_model.test.mjs`
- `frontend/packages/webclient/src/search/search_arch_parser.test.mjs`

Implemented:
- Added Odoo-compatible home menu hooks: `data-view="apps"`, `o_home_menu`, `o_apps`.
- Added app/nav metadata: `data-menu-id`, titles, `aria-label`, `aria-current`.
- Marked launcher active when no app is active.
- Hid zero systray counters.
- Wired parsed search arch/default facets into TS window action execution.
- Applied default search domain/context/group-by to TS `web_search_read`.
- Rendered TS window actions with an Odoo-compatible control panel instead of a standalone title.

Report/dashboard files changed:
- `reports/agent_audit_backlog.md`
- `reports/progress_dashboard.html`
- `reports/ui_parity_master_current.md`

## P0 Gaps

1. Replace inline `/web` server shell with bundled TS webclient runtime.
   - Impact: current user-facing shell cannot reach full Odoo action/service behavior.
   - Owner: runtime shell worker.
   - Acceptance: `/web` bootstraps frontend bundle; route state supports action, model, view type, id, menu id, debug, and company ids.

2. Fix form control panel and breadcrumb overlap.
   - Evidence: `reports/uiux/subagents/current-audit/technical-form-desktop.png`
   - Owner: action/control-panel worker.
   - Acceptance: Save/Discard, breadcrumbs, title, pager, and view controls never overlap on desktop or mobile.

3. Implement real action service and action stack.
   - Owner: action stack worker.
   - Acceptance: menu opens action, list opens form, breadcrumbs return correctly, dialogs handle target `new`, no stale panels remain.

4. Implement real Settings app parity.
   - Owner: settings worker.
   - Acceptance: General Settings, Users & Companies, Technical, and Translations render as settings apps with typed controls, sections, dirty save/discard, and search.

5. Implement Apps catalog parity.
   - Owner: apps catalog worker.
   - Acceptance: app cards show product names, provenance-safe icons, descriptions, categories, install/update state, activation flow, and module info.

## P1 Gaps

1. Complete list renderer.
   - Add row checkboxes, select all, action menu, sortable headers, optional columns, grouped rows, inline edit gates, and keyboard behavior.

2. Complete form renderer.
   - Add header buttons, smart buttons, statusbars, sheets, groups, notebooks, modifiers, onchanges, x2many widgets, image/binary widgets, code editor, and chatter layout.

3. Complete search/control panel.
   - Add full search arch behavior, saved favorites persistence, custom filters/groups, date intervals, suggestions, action/cog menus, and mobile layout.

4. Complete navbar/systray/mobile behavior.
   - Add messages, activities, user menu, company switcher, debug menu, mobile burger/back navigation, and responsive systray state.

5. Complete Enterprise launcher theme.
   - Add provenance-safe generated/licensed icon set, Enterprise dark launcher spacing, search behavior, keyboard navigation, and app deduplication.

## P2 Gaps

1. Add automated Odoo reference comparison screenshots.
2. Add production selector smoke for `/web`.
3. Add normal-user browser workflow fixtures.
4. Add stable selector contract on top of Odoo-compatible classes.
5. Tokenize Enterprise dark/light theme across navbar, home menu, settings, list, form, search, dropdowns, buttons, and mobile.

## Next Assignment Queue

1. `runtime-shell-worker`: make TS webclient own `/web`; keep server as API/runtime provider.
2. `action-stack-worker`: route restore, action stack, target modes, breadcrumbs, dialog target.
3. `settings-worker`: real settings parser/controller and typed settings fields.
4. `list-form-worker`: list/form renderer parity and form header overlap fix.
5. `apps-catalog-worker`: module catalog UX and safe install/update flow.
6. `systray-mobile-worker`: user/company/debug/mail/activity dropdowns and mobile navigation.
7. `visual-regression-worker`: local, production, and reference screenshot checks.

## Notes

- No accounting module work was done.
- No proprietary Odoo Enterprise or OI source/assets were copied.
- Existing untracked production smoke folder remains: `reports/verification/web_visual_smoke_prod/`.
