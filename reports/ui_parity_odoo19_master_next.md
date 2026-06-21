# GoERP `/web` UI Parity Audit vs Odoo 19

Date: 2026-06-21  
Scope: GoERP `/web` against local Odoo 19 reference. Accounting excluded.  
Reference input: `/Users/fadhelalqaidoom/Desktop/odoo` selectors and behavior only.  
Local HEAD: `05d533e0cdf56ded3966cbb8284371ab34d6fda8`

## Current Status

- Production `/web`: `200 OK` at `https://api.fadhelalqaidoomxyz.xyz/web`.
- Local `/web`: `200 OK` on `127.0.0.1:8074`; `/web/health` returned `{"status":"ok"}`.
- `make ci`: passed.
- Source files changed by this audit: none.
- Report file changed by this audit: `reports/ui_parity_odoo19_master_next.md`.
- Existing dirty worktree entries were present before this report and were not edited.

## Local Browser Evidence

| Area | Observed result | Status |
|---|---:|---|
| Shell selectors | `.o_web_client`: 1, `.o_action_manager`: 1, `.o_main_navbar`: 1 | Pass, structural |
| Systray shell | `.o_menu_systray button`: 5 | Partial |
| Launcher | 4 apps: Approvals, Delegation, Settings, Apps; `.o_app[data-app-key]` | Partial |
| Settings | 3 `.app_settings_block`, 14 `.o_setting_box`, save/discard/search present | Partial |
| Technical list | Server Actions opened; model `ir.actions.server`; 20 rows; `table.o_list_renderer.o_list_table`: 1 | Partial |
| Search menu | `.o_filter_menu`, `.o_group_by_menu`, `.o_favorite_menu` present | Partial |
| Search filters | Active/Archived/Code filters present; Active facet applies | Partial |
| Search group by | Active group-by applies; `.o_grouped_list.o_list_renderer`: 1; `.o_group_header`: 1 | Partial |
| Mobile 390x844 | No horizontal overflow; Settings still 3 blocks/14 boxes | Partial |

Mobile gaps observed: GoERP uses `#mobileMenu.o-mobile-menu-toggle`; Odoo reference uses `.o_mobile_menu_toggle`, `.o_burger_menu`, `.o_app_menu_sidebar`, and `.o_bottom_sheet`. Systray remains displayed on the checked mobile viewport.

## Odoo 19 Reference Selectors

Reference paths used for selector and behavior comparison:

- Shell/action: `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/web/static/src/webclient/webclient.xml:4`, `webclient_layout.scss:8`, `webclient/actions/action_container.js:10`, `webclient/actions/action_service.js:1054`.
- Control/search: `search/control_panel/control_panel.xml:5`, `search/search_bar/search_bar.xml:6`, `search/search_bar_menu/search_bar_menu.xml:5`, `search/search_panel/search_panel.xml:4`.
- Launcher: `webclient/navbar/navbar.xml:75`, `webclient/navbar/navbar.xml:104`, `enterprise/web_enterprise/static/src/webclient/home_menu/home_menu.xml:4`, `home_menu_service.js:63`.
- Settings: `webclient/settings_form_view/settings_form_view.xml:3`, `settings/settings_page.xml:4`, `views/form/setting/setting.xml:4`.
- List/form/kanban: `views/list/list_renderer.xml:7`, `views/form/form_controller.xml:4`, `views/form/form_compiler.js:215`, `views/kanban/kanban_renderer.xml:4`.
- Systray/dialog/mobile: `webclient/navbar/navbar.xml:33`, `core/dialog/dialog.xml:4`, `core/bottom_sheet/bottom_sheet.xml:3`, `webclient/burger_menu/burger_menu.xml:6`.

## P0 Gaps

| ID | Gap | GoERP selectors/files implicated | Odoo 19 reference | Assignment |
|---|---|---|---|---|
| P0-01 | Replace inline production `/web` shell with bundled runtime owner. Current production shell is still inline server JS/HTML. | `internal/http/server.go:4968-4985`, `6334-6455`, `7107-7136`; selectors `.o_web_client`, `.o_action_manager`, `.o_control_panel`. TS shell exists separately in `frontend/packages/webclient/src/webclient/shell.ts:15-41`. | `webclient.xml:4`, `action_container.js:10`, `webclient_layout.scss:8`. | `runtime-shell-worker`: own `/web` mount, asset loading, boot contract. Do not change renderers. |
| P0-02 | Implement real action stack/router/dialog targets. Current flow is panel switching plus direct menu opening, not Odoo action-service parity. | `internal/http/server.go:7111-7136`, `7569-7572`; selectors `#appsView`, `#settingsView`, `#recordsView`, `#recordPanel`, `.o_action_manager`. | `action_service.js:1054`, `1121`, `1230`; selectors `.o_action`, `.o_action_manager`, `.modal`. | `action-stack-worker`: action service, breadcrumb stack, target `current/new/main`, mobile view selection. |
| P0-03 | Complete search model/control panel parity. Current selectors exist and filter/group-by smoke works, but full search arch parsing, date periods, custom filter/group dialogs, autocomplete, persisted `ir.filters`, and search-panel integration remain incomplete. | `internal/http/server.go:6420-6436`, `6700-6883`; `frontend/packages/webclient/src/control_panel/control_panel.ts:73-180`; `frontend/packages/webclient/src/search/search_model.ts:54-140`; selectors `.o_cp_searchview`, `.o_searchview_facet`, `.o_search_bar_menu`, `.o_filter_menu`, `.o_group_by_menu`, `.o_favorite_menu`. | `search_bar.xml:6`, `search_bar.xml:90`, `search_bar_menu.xml:5`, `search_panel.xml:4`, `control_panel.xml:5`. | `search-control-worker`: own search arch parser, search model, dropdown dialogs, favorites persistence, autocomplete. |
| P0-04 | Replace hardcoded Settings panel with real settings form behavior. Current Settings renders fixed section data, not Odoo settings compiler/controller behavior. | `internal/http/server.go:7118-7123`, `7346-7436`; `internal/base/data/base_data.xml:95-99`; `internal/base/data/technical_menus.xml:3-9`, `137-143`; selectors `#settingsView.o_form_view.o_settings_view`, `.app_settings_block`, `.o_setting_box`. | `settings_form_view.xml:3`, `settings_page.xml:4`, `settings_form_controller.js:25`, `views/form/setting/setting.xml:4`. | `settings-worker`: settings tabs, `.o-settings-form-view`, dirty warning, search highlights, save/discard confirmation. |
| P0-05 | Complete form renderer parity. Current form uses flattened labels/inputs and lacks Odoo form renderer structure. | `internal/http/server.go:7132-7136`, `8081-8138`; `frontend/packages/webclient/src/index.ts:3324-3367`; selectors `#recordForm`, `.o_form_view`, `.o_form_sheet`, `.gorp-form-view`. | `form_controller.xml:4`, `form_compiler.js:215`, `form_compiler.js:441`, `form_compiler.js:656`; selectors `.o_form_view_container`, `.o_form_renderer`, `.o_form_sheet_bg`, `.o_form_sheet`, `.o_group`, `.o_inner_group`. | `form-renderer-worker`: form compiler, sheets/groups/notebooks, modifiers, onchange, x2many, statusbar, chatter layout. |

## P1 Gaps

| ID | Gap | GoERP selectors/files implicated | Odoo 19 reference | Assignment |
|---|---|---|---|---|
| P1-01 | Complete list renderer parity. Current list lacks Odoo row selector, data-row/cell contract, optional columns, inline edit, drag handles, selected-action state, and mobile selector behavior. | `internal/http/server.go:7947-8024`; `frontend/packages/webclient/src/index.ts:1637-1725`; selectors `table.o_list_renderer.o_list_table`, `.o_mobile_list_cards`, missing `.o_data_row`, `.o_data_cell`, `.o_list_record_selector`, `.o_optional_columns_dropdown`. | `views/list/list_renderer.xml:7`, `list_renderer.xml:270`, `list_controller.xml:8`. | `list-renderer-worker`: list DOM contract, row selection, optional fields, sorting/editing, responsive list behavior. |
| P1-02 | Complete kanban renderer parity. Current kanban is ungrouped cards only. | `internal/http/server.go:8026-8078`; `frontend/packages/webclient/src/index.ts:1727-1775`; selectors `.o_kanban_renderer`, `.o_kanban_ungrouped`, `.o_kanban_record`; missing `.o_kanban_grouped`, `.o_kanban_group`, `.o_kanban_load_more`, `.o-kanban-button-new`. | `views/kanban/kanban_renderer.xml:4`, `kanban_controller.xml:4`, `kanban_record.js:279`. | `kanban-renderer-worker`: grouped columns, quick create, drag/reorder, load more, selection and keyboard behavior. |
| P1-03 | Implement systray dropdowns and registry behavior. Current systray buttons are mostly static counters/placeholders. | `internal/http/server.go:6353-6367`, `8151-8164`; `frontend/packages/webclient/src/webclient/navbar/navbar.ts:23-132`; selectors `.o_menu_systray`, `.o_mail_systray_item`, `.o_activity_menu`, `.o_switch_company_menu`, `.o_user_menu`. | `navbar.xml:33`, `navbar.js:107`, `user_menu.js:40`, `switch_company_menu.js:360`. | `systray-worker`: registry-driven systray entries, company/user/debug dropdowns, mail/activity menus. |
| P1-04 | Implement dialog and notification service parity. Current dialogs/notifications are bounded and not the Odoo modal stack. | `frontend/packages/webclient/src/index.ts:1524-1533`, `2086-2145`; shell lacks full `.o_dialog` stack. | `core/dialog/dialog.xml:4`, `dialog_service.js:40`, `core/notifications/notification_container.js:12`, `core/bottom_sheet/bottom_sheet.xml:3`. | `dialog-notification-worker`: modal stack, inactive modal state, footer portal, notifications, mobile fullscreen/bottom sheet. |
| P1-05 | Finish launcher/home menu parity without proprietary assets. Current launcher uses generated initials/tokens and lacks full Odoo menu metadata behavior. | `internal/http/server.go:7111-7115`, `7438-7521`; `frontend/packages/webclient/src/home_menu/home_menu.ts:15-77`; selectors `.o_app_launcher`, `.o_home_menu`, `.o_apps`, `.o_app`, `.o_app_icon`, `data-app-key`. | CE `navbar.xml:75`, `navbar.xml:104`; EE `home_menu.xml:4`, `home_menu.js:109`, `home_menu.js:313`. | `launcher-metadata-worker`: clean-room app metadata, `data-menu-xmlid` equivalent where safe, keyboard navigation, drag/reorder persistence. |
| P1-06 | Improve Apps catalog parity. Current Apps view is a local module grid/list, not Odoo app installation UX. | `internal/http/server.go:7125-7129`, `7510-7513`; selectors `#installView`, `#moduleGrid`, `.o_apps`. | CE app launcher/catalog behavior via `navbar.xml:75`; action/menu services via `navbar.js:205`. | `apps-catalog-worker`: app cards, install/update state, filters, module detail action, provenance-safe icons. |

## P2 Gaps

| ID | Gap | Evidence | Assignment |
|---|---|---|---|
| P2-01 | Add automated visual regression for `/web`. | Current reports have manual audit notes and ad hoc browser evidence; no committed selector/screenshot parity suite for launcher, Settings, Technical list/form, search menu, and mobile. | `visual-regression-worker`: Playwright/local browser smoke under `reports/ui_parity/` or existing test tooling. |
| P2-02 | Add production selector smoke. | Production checked by `HEAD /web` only. No authenticated selector smoke against production. | `release-smoke-worker`: production-safe read-only selector probe, no secrets in output. |
| P2-03 | Deepen mobile parity. | 390px no-overflow passed, but Odoo mobile selectors `.o_mobile_menu_toggle`, `.o_burger_menu`, `.o_app_menu_sidebar`, `.o_bottom_sheet`, `.o_mobile_sticky` are missing or incomplete. | `mobile-worker`: burger/sidebar, bottom sheet, mobile search/control panel, settings tab swipe. |
| P2-04 | Normalize selector contract. | No `data-testid` contract found in inspected `/web`; current tests depend on Odoo class strings and local dataset keys. | `selector-contract-worker`: stable test selectors layered over Odoo-compatible class contract. |

## Independent Implementation Order

1. `runtime-shell-worker`: replace inline shell ownership and bundle boot. Blocks full parity.
2. `action-stack-worker`: action stack and dialog target semantics. Depends on runtime boot only.
3. `search-control-worker`: search/control panel behavior. Can proceed with current or new shell if interfaces are isolated.
4. `settings-worker`: settings form parity. Depends on action stack for correct action mount.
5. `form-renderer-worker`: full form renderer. Depends on action stack.
6. `list-renderer-worker` and `kanban-renderer-worker`: parallel after renderer interfaces are defined.
7. `systray-worker`, `dialog-notification-worker`, `launcher-metadata-worker`, `apps-catalog-worker`: parallel with renderer work after shell ownership is decided.
8. `visual-regression-worker`, `release-smoke-worker`, `mobile-worker`, `selector-contract-worker`: validation and hardening after P0 contracts stabilize.

## Commands Run

```text
git rev-parse HEAD
git status --short
sed -n '1,160p' AGENTS.md
sed -n '1,220p' reports/ui_parity_odoo19_master.md
sed -n '1,220p' reports/progress_dashboard.html
rg --files reports
rg -n "o_web_client|o_action_manager|o_control_panel|o_search_bar_menu|o_setting_box|o_kanban_renderer|o_list_renderer" internal/http frontend/packages/webclient
rg -n "o_web_client|o_action_manager|o_control_panel|o_search_bar_menu|o_setting_box|o_kanban_renderer|o_list_renderer|o_bottom_sheet|o_mobile_menu_toggle" /Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/web/static/src /Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise/web_enterprise/static/src
nl -ba internal/http/server.go | sed -n '6334,6455p'
nl -ba internal/http/server.go | sed -n '7107,7157p'
nl -ba internal/http/server.go | sed -n '7346,7436p'
nl -ba internal/http/server.go | sed -n '7901,8138p'
nl -ba internal/http/server.go | sed -n '7438,7521p'
nl -ba frontend/packages/webclient/src/control_panel/control_panel.ts | sed -n '69,180p'
nl -ba frontend/packages/webclient/src/search/search_model.ts | sed -n '1,140p'
nl -ba frontend/packages/webclient/src/webclient/navbar/navbar.ts | sed -n '23,132p'
nl -ba frontend/packages/webclient/src/index.ts | sed -n '1624,1775p'
nl -ba frontend/packages/webclient/src/index.ts | sed -n '3311,3367p'
nl -ba frontend/packages/webclient/src/home_menu/home_menu.ts | sed -n '15,90p'
GORP_HTTP_ADDR=127.0.0.1:8073 go run ./cmd/gorpd serve
GORP_HTTP_ADDR=127.0.0.1:8074 go run ./cmd/gorpd serve
curl -sS -I http://127.0.0.1:8074/web
curl -sS http://127.0.0.1:8074/web/health
curl -sS -I https://api.fadhelalqaidoomxyz.xyz/web
node_repl browser checks for desktop launcher, Settings, Technical Server Actions, search menu, group-by, and 390x844 mobile
make ci
```

## Blockers

- No source edits were allowed, so gaps were not fixed.
- Odoo reference was local source only; no Odoo 19 runtime screenshot session was launched.
- Proprietary Odoo/OI source and assets were not copied.
- Production was checked by HTTP status only; no authenticated production selector smoke was run.
- Accounting was excluded from analysis.
