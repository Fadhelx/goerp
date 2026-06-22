# Strict UI/UX Parity Verification

Date: 2026-06-23
Workspace: `/Users/fadhelalqaidoom/Documents/gorp`
Role: read-only verifier
Scope: Odoo UI/UX parity. Accounting excluded.

## Result

Status: FAIL

Blocking gaps:
- Technical dropdown is present in DOM but visually clipped in GoERP.
- Technical menu is incomplete versus Odoo debug menu.
- Groups form x2many/one2many chrome is missing or empty.

No source files edited. Report and screenshots only.

## Inputs

GoERP:
- Local: `http://127.0.0.1:8074/web`
- Production: `https://goerp.fadhelalqaidoomxyz.xyz/web`
- Production: `https://api.fadhelalqaidoomxyz.xyz/web`

Odoo reference:
- Source/runtime input: `/Users/fadhelalqaidoom/Desktop/odoo/odoo19`
- Temporary reference server: `http://127.0.0.1:8075`
- Existing and live reference screenshots used.

## Evidence

Evidence directory:
- `reports/uiux/verifier_strict_parity_20260623/`

Key GoERP current evidence:
- `manual-launcher-current.png`
- `manual-technical-dropdown-current-v2.png`
- `manual-server-actions-list-current.png`
- `manual-relation-dropdown-current.png`
- `default-technical-form-desktop.png`
- `default-users-flow-desktop.png`
- `manual-groups-inherited-by-current.png`
- `manual-delegation-list-current.png`
- `manifest.json`

Key Odoo reference evidence:
- `odoo-reference-live-launcher.png`
- `odoo-reference-live-settings-debug.png`
- `odoo-reference-live-technical-dropdown.png`
- `odoo-reference-live-server-actions-list.png`
- `odoo-reference-live-server-action-form.png`
- `odoo-reference-live-relation-dropdown.png`
- `odoo-reference-live-users-form.png`
- `odoo-reference-live-groups-inherited-tab.png`

Production evidence:
- `prod_goerp/manifest.json`
- `prod_api/manifest.json`
- `prod_goerp/default-navbar-nested-menus-desktop.png`
- `prod_api/default-navbar-nested-menus-desktop.png`

## Commands Run

```sh
agent-browser skills get core
lsof -nP -iTCP:8069 -iTCP:8070 -iTCP:8074 -sTCP:LISTEN
curl -fsS -I http://127.0.0.1:8074/web/health
curl -fsS -I https://goerp.fadhelalqaidoomxyz.xyz/web/health
curl -fsS -I https://api.fadhelalqaidoomxyz.xyz/web/health
node tools/web_visual_smoke/run.mjs --base-url http://127.0.0.1:8074 --out reports/uiux/verifier_strict_parity_20260623 --scenario ...
python3 /Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo-bin --version
python3 /Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo-bin --addons-path=... --http-interface=127.0.0.1 --http-port=8075 -d <local-reference-db> --no-database-list
agent-browser --session odoo-ref ... screenshot ...
node tools/web_visual_smoke/run.mjs --base-url https://goerp.fadhelalqaidoomxyz.xyz --out reports/uiux/verifier_strict_parity_20260623/prod_goerp --scenario ...
node tools/web_visual_smoke/run.mjs --base-url https://api.fadhelalqaidoomxyz.xyz --out reports/uiux/verifier_strict_parity_20260623/prod_api --scenario ...
rg -n "..." frontend internal
git status --short
```

`make ci` not run. No implementation work was completed.

## Smoke Results

Local `127.0.0.1:8074`:
- PASS: `default-navbar-nested-menus-desktop`
- PASS: `default-webclient-action-desktop`
- PASS: `default-action-dialog-desktop`
- PASS: `default-technical-form-desktop`
- PASS: `default-search-menu-desktop`
- PASS: `default-view-switch-desktop`
- PASS: `default-mobile-launcher-parity`
- PASS: `default-users-flow-desktop`
- FAIL: `default-webclient-takeover`
- FAIL: `default-technical-search-desktop`
- FAIL: `default-groups-form-notebook-desktop`

Production:
- `goerp`: launcher FAIL; nested menus, technical form, users flow PASS.
- `api`: launcher FAIL; nested menus, technical form, users flow PASS.

## Gap Table

| ID | Sev | Screen | Observed GoERP | Expected Odoo | Likely owning files | Verification scenario |
|---|---:|---|---|---|---|---|
| G01 | P0 | Settings > Technical dropdown | Button expands in DOM, but screenshot shows no visible dropdown. Menu element is clipped by `.o_menu_sections` with `overflow: hidden`. | Dropdown must visibly overlay below navbar and be scrollable. | `frontend/packages/webclient/src/webclient/navbar/navbar.ts`, `frontend/themes/enterprise-like/src/theme.ts` | Open Settings, click Technical, assert screenshot has visible menu below navbar. |
| G02 | P0 | Settings > Technical menus | GoERP exposes 15 Technical items. Odoo reference exposes 73 dropdown entries in debug mode. Missing many standard Technical menus. | All available Technical menus visible and navigable. | `internal/base/data/technical_menus.xml`, `frontend/packages/webclient/src/webclient/navbar/navbar.ts`, `frontend/packages/webclient/src/home_menu/home_menu.ts` | Count visible Technical menu entries against Odoo reference; open several missing entries. |
| G03 | P0 | Groups form, x2many/list chrome | `default-groups-form-notebook-desktop` timed out waiting for x2many tags. Current `Inherited By` tab is empty. | Groups form shows embedded list widgets with rows and Add a line controls. | `frontend/packages/webclient/src/index.ts`, `internal/base/data/technical_views.xml` | Open Groups > first record > inherited tabs; verify embedded list rows, delete icons, Add a line. |
| G04 | P1 | Settings page | GoERP shows action-card settings blocks and extra `Apps & AI` app. Odoo shows settings sections such as Users, Languages, Companies, Emails, Permissions with proper setting rows. | Settings renderer should match Odoo settings app layout and block structure. | `frontend/packages/webclient/src/settings/settings_renderer.ts`, `internal/base/data/base_data.xml` | Open Settings in debug mode; compare left app list, search, block headers, setting rows. |
| G05 | P1 | Launcher | GoERP launcher has extra app tiles, custom icon treatment, missing visible close affordance on registration banner, and different systray set. Production launcher also fails strict background audit. | Odoo launcher uses reference background, banner with close button, correct app set/icons, and matching systray. | `frontend/packages/webclient/src/home_menu/home_menu.ts`, `frontend/packages/webclient/src/home_menu/app_metadata.ts`, `frontend/packages/webclient/src/webclient/navbar/navbar.ts`, `frontend/themes/enterprise-like/src/theme.ts` | Open `/web`; compare app grid, banner, systray, background, icon shape. |
| G06 | P1 | Server Actions list | GoERP shows raw usage values such as `ir_cron` and `ir_actions_server`; topbar context is global app menu. | Odoo shows human labels such as Scheduled Action / Server Action and contextual Settings navbar. | `frontend/packages/webclient/src/index.ts`, `frontend/packages/webclient/src/control_panel/control_panel.ts`, `internal/base/data/technical_views.xml` | Open Technical > Server Actions; verify list labels, control panel, breadcrumbs, pager, action/cog menus. |
| G07 | P1 | Server Actions form | GoERP form layout differs: custom band, missing Odoo status button placement, no chatter, different sheet density and type-button layout. | Odoo form shows contextual action button, form sheet, type pills, code notebook, chatter, pager, and action menu. | `frontend/packages/webclient/src/index.ts`, `frontend/packages/webclient/src/control_panel/control_panel.ts` | Open first Server Action form; compare header, sheet, status/action controls, notebook, chatter. |
| G08 | P1 | Users form | GoERP shows raw role value `group_system`, simplified access panel, no avatar/statusbar/smart buttons, fewer tabs. | Odoo Users form shows avatar, Confirmed/Invited statusbar, smart buttons, contact link, grouped access controls, Preferences/Calendar/Security tabs. | `frontend/packages/webclient/src/index.ts`, `internal/base/data/technical_views.xml`, `internal/base/security/base_groups.xml` | Open Users > Administrator; compare form header, statusbar, smart buttons, tabs, access controls. |
| G09 | P1 | Relation dropdowns | Many2one dropdown opens, but options use technical model names and custom dropdown styling. Odoo displays human names and native autocomplete styling. | Relation fields should match Odoo autocomplete/dropdown behavior and visual treatment. | `frontend/packages/webclient/src/index.ts`, `frontend/packages/webclient/src/settings/settings_renderer.ts` | Edit Server Action Model; open relation dropdown; compare option labels, Search more, keyboard behavior, placement. |
| G10 | P2 | Read-only one2many verification coverage | Delegation list has no rows in read-only verification path, so fresh one2many capture was limited without creating fixture data. | A verifier-safe seeded record should exist for x2many/list/form chrome checks. | Test/demo data setup, `internal/base/data/technical_views.xml`, `frontend/packages/webclient/src/index.ts` | Provide non-mutating demo record; open form and verify one2many list and mobile cards. |

## Summary Counts

P0: 3
P1: 6
P2: 1

## Verification Notes

- Reference screenshots were captured from a live Odoo 19 Enterprise server started from `/Users/fadhelalqaidoom/Desktop/odoo/odoo19`.
- Reference source was used only as input.
- No proprietary Odoo or OI code/assets copied into source.
- Reports contain no source contents.
