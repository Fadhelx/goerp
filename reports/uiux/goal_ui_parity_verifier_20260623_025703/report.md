# GoERP UI Parity Verification

Date: 2026-06-23
Workspace: `/Users/fadhelalqaidoom/Documents/gorp`
Mode: read-only UI verifier
Status: FAIL

## Evidence

Directory: `reports/uiux/goal_ui_parity_verifier_20260623_025703/`

GoERP:
- `goerp/default-webclient-takeover.png`
- `goerp/goerp-settings.png`
- `goerp/default-navbar-technical-dropdown-open-desktop.png`
- `goerp/default-technical-search-desktop.png`
- `goerp/default-technical-form-desktop.png`
- `goerp/goerp-relation-dropdown.png`
- `goerp/goerp-apps-catalog.png`
- `goerp/default-kanban-view-desktop.png`
- `goerp/manifest.json`

Odoo reference:
- `odoo/odoo-launcher-topbar.png`
- `odoo/odoo-settings.png`
- `odoo/odoo-technical-dropdown.png`
- `odoo/odoo-server-actions-list.png`
- `odoo/odoo-server-action-form.png`
- `odoo/odoo-relation-dropdown.png`
- `odoo/odoo-apps-kanban.png`

Metrics:
- `notes/odoo-settings-metrics.json`
- `notes/odoo-technical-dropdown-metrics.json`
- `notes/odoo-apps-kanban-metrics.json`

## Screen Matrix

| Screen | Result | Severity | Evidence | Finding |
|---|---:|---:|---|---|
| App launcher/topbar | FAIL | P1 | `goerp/default-webclient-takeover.png`, `odoo/odoo-launcher-topbar.png` | GoERP shows 4 launcher tiles with custom icon styling. Odoo reference shows 2 tiles for this DB, Odoo cube icons, Studio/debug systray, user/database label. |
| Settings | FAIL | P1 | `goerp/goerp-settings.png`, `odoo/odoo-settings.png` | GoERP settings renderer is custom card/grid layout with simplified sections. Odoo uses native settings form sections, action buttons, help icons, checkboxes, Developer Tools, About, and license/expiration area. |
| Settings > Technical dropdown behavior | PASS | P2 | `goerp/default-navbar-technical-dropdown-open-desktop.png` | GoERP dropdown opens visibly, overlays content, and scrolls. |
| Settings > Technical menu completeness | FAIL | P0 | `goerp/default-navbar-technical-dropdown-open-desktop.png`, `odoo/odoo-technical-dropdown.png` | GoERP visible Technical menu has 15 items. Odoo visible dropdown has 36 items in the same viewport, including Reports, Window Actions, Client Actions, Embedded Actions, Configuration Wizards, User-defined Defaults, IAP Accounts, Customized Views, User-defined Filters, Tours, Decimal Accuracy, Assets, Fields Selection, Model Constraints, ManyToMany Relations, Attachments, Logging, Profiling, Scheduled Action Triggers, Paper Format, External Identifiers, Sequences, System Parameters, User Devices. |
| Server Actions list | FAIL | P1 | `goerp/default-technical-search-desktop.png`, `odoo/odoo-server-actions-list.png` | GoERP shows raw usage keys such as `ir_cron` and `ir_actions_server`. Odoo shows human usage labels such as Scheduled Action and Server Action, with native filter chip and list chrome. |
| Server Action form | FAIL | P1 | `goerp/default-technical-form-desktop.png`, `odoo/odoo-server-action-form.png` | GoERP form uses custom band, custom pills, simplified sheet, and different control panel placement. Odoo has native breadcrumb/pager, smart button, contextual action button, native form spacing, and native status/action controls. |
| Relation dropdown/autocomplete | FAIL | P1 | `goerp/goerp-relation-dropdown.png`, `odoo/odoo-relation-dropdown.png` | GoERP dropdown shows current technical value plus create actions only. Odoo shows multiple autocomplete candidates and `Search more...`; current value is human-readable. |
| Apps kanban/catalog | FAIL | P1 | `goerp/goerp-apps-catalog.png`, `odoo/odoo-apps-kanban.png` | GoERP catalog uses gray placeholder icons, 26 local modules, compact cards, and disabled-looking filters. Odoo Apps kanban has 83 app cards, rich icons, search panel categories, native view switcher, and app actions. |
| Kanban reachable | PARTIAL | P2 | `goerp/default-kanban-view-desktop.png`, `odoo/odoo-apps-kanban.png` | GoERP Server Actions kanban is reachable. It is not a comparable Odoo screen for parity because the Odoo reachable kanban is Apps. |

## P0 Findings

1. Technical menu completeness fails.
   - Evidence: `goerp/default-navbar-technical-dropdown-open-desktop.png`, `odoo/odoo-technical-dropdown.png`.
   - GoERP visible count: 15 items.
   - Odoo visible count: 36 items.
   - Acceptance: Technical menu must expose every reference Technical entry available in the same Odoo fixture, with matching labels, grouping, ordering, dropdown width, scroll behavior, and click navigation.

## P1 Findings

1. Relation dropdown/autocomplete fails.
   - Evidence: `goerp/goerp-relation-dropdown.png`, `odoo/odoo-relation-dropdown.png`.
   - Acceptance: many2one dropdown must show multiple human-readable matches, highlight current value, include `Search more...`, keep native dropdown width/position, and support keyboard selection.

2. Settings page fails visual and structural parity.
   - Evidence: `goerp/goerp-settings.png`, `odoo/odoo-settings.png`.
   - Acceptance: Settings must match Odoo settings form layout: horizontal app menu, left settings app rail behavior, section headers, action buttons, help icons, checkbox rows, save/discard placement, and native spacing.

3. Launcher/topbar fails Enterprise parity.
   - Evidence: `goerp/default-webclient-takeover.png`, `odoo/odoo-launcher-topbar.png`.
   - Acceptance: same fixture must show the same standard launcher app set, Odoo-style icons, same topbar systray set, same user/database label treatment, same banner placement, and same background treatment.

4. Server Actions list/form fail native chrome parity.
   - Evidence: `goerp/default-technical-search-desktop.png`, `goerp/default-technical-form-desktop.png`, `odoo/odoo-server-actions-list.png`, `odoo/odoo-server-action-form.png`.
   - Acceptance: list values must be human labels, not raw technical keys; control panel, filter chips, pager, action menu, form breadcrumb, smart buttons, and form field layout must match Odoo reference.

5. Apps catalog/kanban fails parity.
   - Evidence: `goerp/goerp-apps-catalog.png`, `odoo/odoo-apps-kanban.png`.
   - Acceptance: Apps view must match Odoo app kanban card density, icon treatment, search panel categories, view switcher, action buttons, and module count for the same reference input fixture.

## Commands Run

```sh
agent-browser skills get core --full
git status --short
lsof -nP -iTCP:8069 -iTCP:8070 -iTCP:8071 -iTCP:8072 -iTCP:8073 -iTCP:8074 -iTCP:8075 -iTCP:8076 -sTCP:LISTEN
python3 /Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo-bin --version
psql -lqt
GORP_HTTP_ADDR=127.0.0.1:8077 go run ./cmd/gorpd serve
python3 /Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo-bin --addons-path=/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons,/Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise -d gorp_ref_ui_20260623_010238 --http-interface=127.0.0.1 --http-port=8075 --gevent-port=8078 --no-database-list --workers=0 --max-cron-threads=0 --data-dir=/tmp/gorp_odoo_ref_data_20260623
node tools/web_visual_smoke/run.mjs --base-url=http://127.0.0.1:8077 --out=reports/uiux/goal_ui_parity_verifier_20260623_025703/goerp --timeout-ms=25000 --scenario default-webclient-takeover --scenario default-webclient-action-desktop --scenario default-navbar-technical-dropdown-open-desktop --scenario default-technical-search-desktop --scenario default-technical-form-desktop --scenario default-kanban-view-desktop
agent-browser --session odoo-ui-parity-20260623 open http://127.0.0.1:8075/web/login
agent-browser --session odoo-ui-parity-20260623 fill @e3 admin
agent-browser --session odoo-ui-parity-20260623 fill @e4 admin
agent-browser --session odoo-ui-parity-20260623 click @e1
agent-browser --session odoo-ui-parity-20260623 open 'http://127.0.0.1:8075/odoo?debug=assets'
agent-browser --session odoo-ui-parity-20260623 screenshot reports/uiux/goal_ui_parity_verifier_20260623_025703/odoo/odoo-launcher-topbar.png
agent-browser --session odoo-ui-parity-20260623 screenshot reports/uiux/goal_ui_parity_verifier_20260623_025703/odoo/odoo-settings.png
agent-browser --session odoo-ui-parity-20260623 screenshot reports/uiux/goal_ui_parity_verifier_20260623_025703/odoo/odoo-technical-dropdown.png
agent-browser --session odoo-ui-parity-20260623 screenshot reports/uiux/goal_ui_parity_verifier_20260623_025703/odoo/odoo-server-actions-list.png
agent-browser --session odoo-ui-parity-20260623 screenshot reports/uiux/goal_ui_parity_verifier_20260623_025703/odoo/odoo-server-action-form.png
agent-browser --session odoo-ui-parity-20260623 screenshot reports/uiux/goal_ui_parity_verifier_20260623_025703/odoo/odoo-relation-dropdown.png
agent-browser --session odoo-ui-parity-20260623 screenshot reports/uiux/goal_ui_parity_verifier_20260623_025703/odoo/odoo-apps-kanban.png
agent-browser --session goerp-ui-parity-20260623 open 'http://127.0.0.1:8077/web?debug=1#action=7&model=ir.actions.server&view_type=form&id=25&menu_id=8'
agent-browser --session goerp-ui-parity-20260623 screenshot reports/uiux/goal_ui_parity_verifier_20260623_025703/goerp/goerp-relation-dropdown.png
agent-browser --session goerp-ui-parity-20260623 open 'http://127.0.0.1:8077/web?debug=1#action=3&model=res.config.settings&view_type=form&menu_id=1'
agent-browser --session goerp-ui-parity-20260623 screenshot reports/uiux/goal_ui_parity_verifier_20260623_025703/goerp/goerp-settings.png
agent-browser --session goerp-ui-parity-20260623 open 'http://127.0.0.1:8077/web?debug=1'
agent-browser --session goerp-ui-parity-20260623 screenshot reports/uiux/goal_ui_parity_verifier_20260623_025703/goerp/goerp-apps-catalog.png
shasum -a 256 reports/uiux/goal_ui_parity_verifier_20260623_025703/goerp/*.png reports/uiux/goal_ui_parity_verifier_20260623_025703/odoo/*.png
```

## Blockers

- Exact data parity is limited. Odoo reference DB exposes launcher Apps/Settings only; GoERP fixture exposes Approvals/Delegation/Settings/Apps.
- Odoo reference needed `debug=assets` after first launch because temp data-dir lacked prior filestore asset attachments. Screenshots were captured after `debug=assets` made the reference UI usable.
- `make ci` was not run. No implementation work was completed.

## Constraints

- No implementation files edited.
- Reference source used only as runtime input.
- No Odoo Enterprise or OI code/assets copied into GoERP source.
- Report contains UI labels and artifact paths only; no source contents or secrets.
