# UI Parity Master Goal Report

Date: 2026-06-21
Workspace: `/Users/fadhelalqaidoom/Documents/gorp`
Scope: GoERP `/web` vs Odoo 19 UI, accounting excluded.
Reference input: `/Users/fadhelalqaidoom/Desktop/odoo` only. No Odoo Enterprise/OI source or assets copied.

## Result

GoERP `/web` works for testing, but it is not 100% Odoo Enterprise UI parity.

Current state:
- Local GoERP verified at `http://127.0.0.1:8073/web`.
- Browser smoke passed 13/13 scenarios after the follow-up Technical search slice.
- Default TypeScript webclient is active for launcher, Settings action, and mobile launcher checks.
- Technical list smoke now has a default TypeScript launcher-search path for Server Actions. Technical form/search-menu smoke still uses `?legacy_webclient=1`.
- Odoo-like selectors exist, but Enterprise behavior and visual density are incomplete.

Parity estimate:
- Functional shell parity: 45-55%.
- Enterprise visual/interaction parity: lower, because core Odoo behaviors are still partial.

## Evidence

Latest local smoke command:
- `node tools/web_visual_smoke/run.mjs --base-url=http://127.0.0.1:8073 --out=reports/web_visual_smoke --timeout-ms=60000`

Smoke results:
- 13/13 passed.
- Launcher: 4 app tiles, 5 systray entries.
- Default TS Settings action: `#action=3&model=res.config.settings&view_type=form&menu_id=1`.
- Default TS Server Actions search/open: `#action=7&model=ir.actions.server&view_type=list&menu_id=8`, 20 rows.
- Settings renderer: 1 settings container, 12 visible settings, Save/Discard disabled before edits.
- Technical list: 20 rows.
- Mobile: 0 px horizontal overflow.

Screenshots:
- `reports/web_visual_smoke/default-webclient-takeover.png`
- `reports/web_visual_smoke/default-webclient-action-desktop.png`
- `reports/web_visual_smoke/default-technical-search-desktop.png`
- `reports/web_visual_smoke/technical-list-desktop.png`
- `reports/web_visual_smoke/technical-form-desktop.png`
- `reports/uiux/screenshots/odoo-desktop-apps.png`
- `reports/uiux/screenshots/odoo-desktop-settings-debug.png`
- `reports/uiux/screenshots/odoo-desktop-server-actions-list.png`
- `reports/uiux/screenshots/odoo-desktop-server-action-form.png`

Agents:
- `019eeaa7-e1bc-78d3-a8be-fa9bdd12fb99`: Odoo 19 reference UI checklist.
- `019eeaa8-027b-7651-a88a-8593b4394d5d`: current GoERP UI implementation audit.
- `019eeaa8-6835-7e01-b78f-6c6257d294fe`: live browser verifier timed out and was shut down; local browser smoke above is the verification evidence.

## P0 Gaps

| Priority | Gap | Exact mismatch | Likely files/components | Assignment |
| --- | --- | --- | --- | --- |
| P0 | Enterprise launcher/theme | GoERP launcher has purple navbar, generated letter icons, dark top band, and empty gray lower viewport. Odoo Enterprise launcher is full-screen dark, uses app icons, alert/sysadmin panels, and tighter app-grid behavior. | `frontend/packages/webclient/src/home_menu/home_menu.ts`, `frontend/packages/webclient/src/home_menu/app_metadata.ts`, `frontend/packages/webclient/src/webclient/shell.ts`, `frontend/themes/enterprise-like`, `internal/http/server.go` CSS | `shell-theme-worker` |
| P0 | Action runtime | Default TS runtime opens Settings and Server Actions via launcher search, but technical form/search-menu smoke still uses legacy shell. View switch, search, pager, route restore, breadcrumbs, client actions, and modal stack are incomplete. | `frontend/apps/webclient/src/main.ts`, `frontend/packages/webclient/src/index.ts`, `frontend/packages/webclient/src/services/action_stack.ts`, `frontend/packages/webclient/src/router/action_router.ts`, `tools/web_visual_smoke/run.mjs` | `action-runtime-worker` |
| P0 | Control panel/search | Current search has basic facets/menu. Odoo requires facet editing/removal, autocomplete, filters, group by, favorites, custom filter/group dialogs, saved `ir.filters`, search panel, mobile search dropdown, pager next/prev fetch behavior. | `frontend/packages/webclient/src/control_panel/control_panel.ts`, `frontend/packages/webclient/src/search/search_model.ts`, `frontend/packages/webclient/src/search/search_arch_parser.ts`, `frontend/packages/webclient/src/index.ts` | `search-control-worker` |
| P0 | List renderer | GoERP list is a basic table. Odoo list has selectors, select-all, bulk actions, sortable/resizable columns, optional columns gear, grouped rows/pagers, footer aggregates, inline edit gates, decorations, keyboard flow. | `frontend/packages/webclient/src/index.ts`, `internal/http/server.go` legacy list renderer | `list-renderer-worker` |
| P0 | Form renderer | GoERP TS/default form is still flat. Odoo form requires sheet/nosheet layout, header buttons, smart buttons, statusbar, notebooks, groups, modifiers, onchange, x2many, relation widgets, binary/image fields, code editor, chatter. | `frontend/packages/webclient/src/index.ts`, `internal/http/server.go`, `internal/base/data/*views.xml` | `form-renderer-worker` |
| P0 | Settings view | GoERP has Odoo-shaped settings blocks but lacks Enterprise Settings app tabs/sidebar, highlighted search, no-result state, dirty warning confirmation, Save/Stay/Discard, module install links, company/user scoped settings. | `frontend/packages/webclient/src/settings/settings_renderer.ts`, `frontend/packages/webclient/src/index.ts`, `internal/base/data/base_data.xml`, `internal/base/data/technical_menus.xml` | `settings-worker` |

## P1 Gaps

| Priority | Gap | Exact mismatch | Likely files/components | Assignment |
| --- | --- | --- | --- | --- |
| P1 | Dialogs | Target `new` modal shell exists locally, but Odoo dialog parity needs footer actions, inactive modal stack state, escape/backdrop policy, mobile fullscreen/bottom-sheet behavior, draggable header, confirmations, export/import/select-create dialogs. | `frontend/apps/webclient/src/main.ts`, `frontend/packages/webclient/src/index.ts`, `internal/http/server.go` modal CSS | `dialog-notification-worker` |
| P1 | Navbar/systray | GoERP shows static user/company/messages/activity items. Odoo needs dropdown services, company search/select/confirm/reset, user menu entries, debug menu, messaging/activity dropdowns, current app sections, More overflow. | `frontend/packages/webclient/src/webclient/navbar/navbar.ts`, `frontend/packages/webclient/src/webclient/shell.ts`, mail/activity services | `navbar-systray-worker` |
| P1 | Mobile shell | Mobile has no overflow, but not Odoo mobile behavior. Missing burger/sidebar with All Apps/current app sections, mobile back navigation, bottom sheets, compact action/control panel, mobile form/list density. | `frontend/packages/webclient/src/webclient/shell.ts`, `frontend/packages/webclient/src/webclient/navbar/navbar.ts`, `tools/web_visual_smoke/run.mjs` | `mobile-worker` |
| P1 | Kanban | Current kanban is ungrouped cards. Odoo needs grouped columns, drag/drop, quick create, folded columns, progress bars, group config, record menu, load more, empty helpers. | `frontend/packages/webclient/src/index.ts`, `internal/http/server.go` legacy kanban renderer | `kanban-worker` |
| P1 | Apps catalog/install | GoERP Apps view uses generated initials and raw module state. Odoo Apps needs icons, names, summaries, categories/search panel, Activate/Upgrade/Uninstall, Update Apps List, Scheduled Upgrades, uninstall/reset wizards. | `frontend/packages/webclient/src/home_menu/*`, `internal/http/server.go`, `internal/base` module endpoints | `apps-catalog-worker` |

## P2 Gaps

| Priority | Gap | Likely files/components | Assignment |
| --- | --- | --- | --- |
| P2 | Unsupported view types: pivot, graph, calendar, activity, cohort, gantt, dashboard. | `frontend/packages/webclient/src/index.ts`, view registry aliases, HTTP default arch generation | `views-worker` |
| P2 | OWL compatibility exports are still shallow for controllers, confirmation dialogs, popovers, statusbar, field widgets. | `frontend/packages/webclient/src/aliases/**`, `frontend/packages/webclient/src/index.ts` | `framework-compat-worker` |
| P2 | Visual regression suite needs default TS coverage for technical form/search, dialogs, apps install, systray dropdowns, settings search, mobile form/list. | `tools/web_visual_smoke/run.mjs`, `tools/web_visual_smoke/run.test.mjs`, `reports/uiux/verification` | `qa-visual-worker` |

## Recommended Order

1. `qa-visual-worker`: move technical form/search-menu smoke from legacy shell to default TS webclient.
2. `action-runtime-worker`: complete action stack, route restore, breadcrumb ownership, target modes, and client action handling.
3. `shell-theme-worker`: make launcher/navbar/settings/list default visual theme match Odoo Enterprise dark mode using clean-room tokens and generated/licensed icons.
4. `form-renderer-worker`: implement Odoo form architecture and widgets.
5. `list-renderer-worker` + `search-control-worker`: complete list/search/control panel behavior.
6. `settings-worker`: finish Settings app parity.
7. `navbar-systray-worker`, `mobile-worker`, `kanban-worker`, `apps-catalog-worker`: finish secondary shell surfaces.

## Blockers

- No blocker for continuing implementation.
- Do not copy Odoo Enterprise assets or source. Use reference behavior only.
- Accounting remains excluded for phase 2.
