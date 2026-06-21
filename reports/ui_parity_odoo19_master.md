# UI Parity Master Report

Date: 2026-06-21
Scope: Odoo 19 reference at `http://127.0.0.1:8071/odoo` versus GoERP at `http://127.0.0.1:8069/web`.
Accounting: skipped.
Output constraint: only this report file was created.

## Live Availability

| Target | Result |
| --- | --- |
| Odoo 19 reference | Listening on `127.0.0.1:8071`; `/odoo` redirects to `/web/login?redirect=%2Fodoo%3F`. |
| GoERP | Not listening on `127.0.0.1:8069`; browser and curl both returned connection refused. |

Evidence:

- `curl -I http://127.0.0.1:8071/odoo` returned `303 SEE OTHER` to `/web/login?redirect=%2Fodoo%3F`.
- `lsof -nP -iTCP:8071 -sTCP:LISTEN` showed Python process `PID 14063`.
- `curl -I http://127.0.0.1:8069/web` failed with connection refused.
- `lsof -nP -iTCP:8069 -sTCP:LISTEN` returned no listener.

## Screenshots And Observations

Odoo reference screenshot captured:

- Visible page: login screen, not authenticated webclient.
- Layout: centered auth card, soft geometric background, logo placeholder, email/password fields, password visibility button, primary purple login button, signup/passkey options, database manager link, `Powered by Odoo`.
- Live DOM text: `Email`, `Choose a user`, `Password`, `Reset Password`, `Log in`, `Use a Passkey`, `Manage Databases`, `Powered by Odoo`.
- Live classes observed on login include `o_home_menu_background`, `oe_login_form`, `o_login_auth`, `o_database_list`, `o_show_password`, `o_caps_lock_warning`, `o_main_components_container`, `o_overlay_container`, `o_notification_manager`.

GoERP screenshot not captured:

- `http://127.0.0.1:8069/web` refused connection.
- No live GoERP DOM could be compared in this run.

## Selector Match Matrix

Source-level match verified against GoERP source and allowed Odoo 19 reference tree. Live GoERP verification is blocked until port `8069` is running.

| Selector/Class | GoERP source | Odoo 19 reference source |
| --- | --- | --- |
| `o_web_client` | yes | yes |
| `o_main_navbar` | yes | yes |
| `o_menu_toggle` | yes | yes |
| `o_menu_toggle_icon` | yes | yes |
| `o_navbar_apps_menu` | yes | yes |
| `o_menu_systray` | yes | yes |
| `o_action_manager` | yes | yes |
| `o_control_panel` | yes | yes |
| `o_control_panel_main` | yes | yes |
| `o_control_panel_main_buttons` | yes | yes |
| `o_control_panel_breadcrumbs` | yes | yes |
| `o_control_panel_actions` | yes | yes |
| `o_control_panel_navigation` | yes | yes |
| `o_cp_pager` | yes | yes |
| `o_pager` | yes | yes |
| `o_searchview` | yes | yes |
| `o_searchview_input_container` | yes | yes |
| `o_searchview_input` | yes | yes |
| `o_searchview_dropdown_toggler` | yes | yes |
| `o_app` | yes | yes |
| `o_app_icon` | yes | yes |
| `o_list_view` | yes | yes |
| `o_form_view` | yes | yes |
| `o_form_sheet` | yes | yes |

Non-matches from the same bounded scan:

- `o_kanban_renderer`: present in Odoo 19 reference source; missing in GoERP source.
- `o_app_launcher`: present in GoERP source; not found in the Odoo 19 reference source scan.
- `o_app_name`: present in GoERP source; not found in the Odoo 19 reference source scan.

## Visible Gaps

P0 gaps:

- GoERP is unavailable locally on `127.0.0.1:8069`; normal-user UI testing cannot start.
- Odoo reference is blocked at login; authenticated Odoo 19 webclient parity cannot be compared without a valid session.
- GoERP cannot yet prove live parity for navbar, app launcher, list, form, control panel, settings, technical menus, app install flow, or record open flow in this run.

P1 gaps:

- GoERP needs an Odoo-like login/auth screen for unauthenticated access if `/web` is not public.
- GoERP source still contains local shell classes such as `gorp-navbar`, `gorp-action`, and `o-launcher-button`; these do not block selectors, but they indicate the UI is still a custom shell layer.
- App launcher parity is partial. Need Odoo-style app grid behavior, app menu grouping, active app menu state, search, icons, and Settings/Apps entry behavior.
- Navbar parity is partial. Need real app switcher menu behavior, company switcher, user menu, systray dropdowns, notification/activity counters, and keyboard behavior.
- Control panel parity is partial. Need real search model dropdowns, filters, group-bys, favorites, pager totals, view switcher state, and breadcrumbs tied to route/action state.
- View parity is partial. Need full list renderer behavior, form renderer widgets, statusbar, chatter/activity side panel, relational widgets, dialogs, modals, and `o_kanban_renderer`.
- Theme parity is partial. Need final Odoo Enterprise-like spacing, typography, colors, borders, hover states, empty states, and mobile behavior using provenance-safe generated assets only.

P2 gaps:

- Need screenshot regression coverage for `/web`, app launcher, Settings, Apps install/update, technical menus, list view, form view, create/edit/save/discard, search dropdown, user menu, systray, and mobile width.
- Need CI job that fails when key Odoo selectors disappear.
- Need browser report artifact that records exact URL, viewport, screenshot, selector counts, and console errors.

## Prioritized Implementation Tasks

1. Restore local GoERP availability on `127.0.0.1:8069` and add a health check for `/web`.
2. Provide or create a valid Odoo 19 reference session on `127.0.0.1:8071` before visual parity comparison.
3. Add GoERP unauthenticated login page parity with Odoo login selectors and layout.
4. Replace remaining inline/custom shell behavior with the shared OWL-style webclient/action-manager path.
5. Implement real app launcher and Settings/Apps workflows: app grid, module install/update buttons, Settings entry, Technical menus.
6. Complete navbar/systray parity: app switcher, company menu, user menu, messages, activities, counters, dropdown panels.
7. Complete control panel parity: Odoo search model, filters, group-bys, favorites, pager totals, view switcher, action breadcrumbs.
8. Complete renderer parity: list, form, kanban, chatter, statusbar, relational fields, dialogs, modals, notifications.
9. Add `o_kanban_renderer` implementation and tests.
10. Add automated browser visual checks against both live Odoo and GoERP once both sessions are available.

## Verification Steps

Run after GoERP and Odoo reference are both available:

1. Check listeners:
   - `curl -sS -I http://127.0.0.1:8069/web`
   - `curl -sS -I http://127.0.0.1:8071/odoo`
2. Open Odoo reference and authenticate.
3. Open GoERP `/web`.
4. Capture desktop screenshots at `1280x720` for:
   - Odoo app launcher
   - GoERP app launcher
   - Odoo Settings/Apps
   - GoERP Settings/Apps
   - Odoo list view
   - GoERP list view
   - Odoo form view
   - GoERP form view
5. Assert GoERP selector counts for:
   - `.o_web_client`
   - `.o_main_navbar`
   - `.o_action_manager`
   - `.o_control_panel`
   - `.o_control_panel_main`
   - `.o_menu_systray`
   - `.o_searchview`
   - `.o_list_view`
   - `.o_form_view`
   - `.o_kanban_renderer`
6. Verify no visible custom branding/classes leak into user-facing text: `GoERP`, `Gorp`, `Developer RPC`, `Build dashboard`.
7. Run focused frontend checks:
   - `pnpm -C frontend typecheck`
   - `pnpm -C frontend lint`
   - `pnpm -C frontend test`
   - `pnpm -C frontend build`
8. Run project CI before claiming implementation complete:
   - `make ci`
