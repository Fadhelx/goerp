# UI Parity Verifier Coordinator Report

Date: 2026-06-29
HEAD verified: 4b94269b
Role: verifier/explorer first.
Scope: launcher, Settings > Technical menus, menu reachability, relation dropdowns, action shell, Apps catalog, Users/Groups forms, mobile launcher/settings navigation.

## Reference

Local Odoo reference tree exists:

- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise`

No local Odoo reference server was listening on checked ports. No fresh Odoo DOM capture was made.

Stored Odoo reference screenshots used for visual comparison context:

- `reports/uiux/kant_resume_20260628/odoo_reference/launcher-desktop.png`
- `reports/uiux/kant_resume_20260628/odoo_reference/action-shell.png`
- `reports/uiux/kant_resume_20260628/odoo_reference/technical-dropdown.png`
- `reports/uiux/kant_resume_20260628/odoo_reference/relation-dropdown.png`
- `reports/uiux/kant_resume_20260628/odoo_reference/apps-catalog.png`

## GoERP Evidence

Screenshots:

- `reports/uiux/kant_verifier_coordinator_20260629/goerp/launcher-desktop.png`
- `reports/uiux/kant_verifier_coordinator_20260629/goerp/default-navbar-nested-menus-desktop.png`
- `reports/uiux/kant_verifier_coordinator_20260629/goerp/default-navbar-technical-dropdown-open-desktop.png`
- `reports/uiux/kant_verifier_coordinator_20260629/goerp/default-relation-dropdown-desktop.png`
- `reports/uiux/kant_verifier_coordinator_20260629/goerp/default-webclient-action-desktop.png`
- `reports/uiux/kant_verifier_coordinator_20260629/goerp/default-apps-catalog-parity-desktop.png`
- `reports/uiux/kant_verifier_coordinator_20260629/goerp/default-users-flow-desktop.png`
- `reports/uiux/kant_verifier_coordinator_20260629/goerp/default-groups-form-notebook-desktop.png`
- `reports/uiux/kant_verifier_coordinator_20260629/goerp/default-mobile-launcher-parity.png`

Metrics:

- `reports/uiux/kant_verifier_coordinator_20260629/goerp/manifest.json`
- `reports/uiux/kant_verifier_coordinator_20260629/goerp/metrics-summary.json`
- `reports/uiux/kant_verifier_coordinator_20260629/goerp/compact-metrics.json`

## Result

Focused GoERP smoke: 9 passed / 0 failed.

Passed scenarios:

- `launcher-desktop`
- `default-navbar-nested-menus-desktop`
- `default-navbar-technical-dropdown-open-desktop`
- `default-relation-dropdown-desktop`
- `default-webclient-action-desktop`
- `default-apps-catalog-parity-desktop`
- `default-users-flow-desktop`
- `default-groups-form-notebook-desktop`
- `default-mobile-launcher-parity`

## DOM Metrics

Launcher:

- App count: `2`
- Navbar height: `46px`
- Banner top: `70px`
- First app row top: `171px`
- Icon size: `70x70`
- Developer tools control: present
- Odoo Studio control: present
- Mail/activity controls on launcher: hidden

Settings > Technical:

- Technical dropdown expanded: `true`
- Dropdown hidden: `false`
- Raw grouped entries: `45`
- First visible labels: `Email`, `Outgoing Mail Servers`, `Actions`, `Actions`, `Reports`, `Window Actions`, `Client Actions`, `Server Actions`

Relation dropdown:

- Many2one dropdown open: `true`
- Placement: `bottom-start`
- Width source: `field`
- Input width: `489px`
- Dropdown width: `489px`
- Search more: present
- X2many keyboard active option moved from index `0` to `1`

Apps catalog:

- Cards: `77`
- Visible cards: `24`
- Top actions: `Update Apps List`, `Apply Scheduled Upgrades`, `Import Module`
- Sidebar categories include `Technical`
- First card height: `94px`
- First icon: `50x50`
- Generated clean-room icons: `77`

Users form:

- Access notebook: present
- Sections: `Roles`, `Master Data`, `Extra Rights`
- Smart buttons: `8 Groups`, `138 Access Rights`, `25 Record Rules`
- Master Data select controls: white background, dark text, `360px` width

Groups form:

- Tabs: `Users`, `Inherited`, `Menus`, `Views`, `Access Rights`, `Record Rules`, `Notes`
- Smart button: `1 Users`
- Readonly controls: white background, dark text, `181px` width
- Users grid row includes `Administrator / admin / Administrator`

Mobile:

- Launcher app count: `2`
- Icon size: `70x70`
- Horizontal overflow: `0px`
- Settings opens action content, not launcher shell

## Gaps Found

No bounded non-conflicting source patch was indicated by the focused checks.

Residual verifier limits:

- Fresh Odoo reference DOM was not captured because no local Odoo reference server was available.
- Launcher and Apps catalog still use clean-room generated background/icons, not proprietary Odoo assets. This is legally required unless a written provenance/license decision is provided.
- Overall 100% visual parity still requires independent screenshot comparison against a live Odoo reference session.

## Commands

- `pnpm -C frontend build`
- `GORP_HTTP_ADDR=127.0.0.1:8094 go run ./cmd/gorpd serve`
- `node tools/web_visual_smoke/run.mjs --base-url=http://127.0.0.1:8094 --out=reports/uiux/kant_verifier_coordinator_20260629/goerp --timeout-ms=30000 --scenario=launcher-desktop --scenario=default-navbar-technical-dropdown-open-desktop --scenario=default-navbar-nested-menus-desktop --scenario=default-relation-dropdown-desktop --scenario=default-webclient-action-desktop --scenario=default-apps-catalog-parity-desktop --scenario=default-users-flow-desktop --scenario=default-groups-form-notebook-desktop --scenario=default-mobile-launcher-parity`
- `jq` metrics extraction into `metrics-summary.json` and `compact-metrics.json`

## Changed Files

Source files changed: none.

Generated evidence only:

- `reports/uiux/kant_verifier_coordinator_20260629/`
