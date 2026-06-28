# GoERP vs Odoo 19 Enterprise UI Parity

Date: 2026-06-28
GoERP: `http://127.0.0.1:8075/web?debug=1`
Odoo 19 Enterprise: `http://127.0.0.1:8076/odoo?debug=1`

## Result

| Area | Status | Evidence |
|---|---:|---|
| Main shell / desktop launcher | FAIL | `goerp/01-launcher-dom.json`, `goerp/01-launcher-desktop.png`, `odoo/01-launcher-dom.json`, `odoo/01-launcher-desktop.png` |
| App switcher / launcher styling | FAIL | `goal_ui_parity_20260628_current_goerp_smoke/manifest.json`, `goerp/14-mobile-launcher-dom.json`, `odoo/15-mobile-launcher-dom.json` |
| Settings technical nested menus | PASS | `goal_ui_parity_20260628_current_goerp_smoke/default-navbar-technical-dropdown-open-desktop.png`, `odoo/03-technical-dropdown-retry-dom.json` |
| Apps catalog | FAIL | `goerp/04-apps-catalog-dom.json`, `goerp/04-apps-catalog.png`, `odoo/04-apps-catalog-dom.json`, `odoo/04-apps-catalog.png` |
| Server Actions New flow | PASS | `goerp/09-server-action-new-correct-dom.json`, `goerp/09-server-action-new-correct.png`, `odoo/06-server-action-new-dom.json`, `odoo/06-server-action-new.png` |
| Scheduled Actions list/form | PASS | `goerp/11-scheduled-actions-list-dom.json`, `goerp/12-scheduled-action-form-dom.json`, `odoo/09-scheduled-actions-list-dom.json`, `odoo/10-scheduled-action-form-dom.json` |
| Relation dropdowns / external-link buttons | PASS | `goerp/10-relation-dropdown-correct-dom.json`, `odoo/08-relation-dropdown-dom.json`, `odoo/10-scheduled-action-form-dom.json` |
| Users / Groups forms | PASS | `goal_ui_parity_20260628_current_goerp_smoke/default-users-flow-desktop.png`, `goal_ui_parity_20260628_current_goerp_smoke/default-groups-form-notebook-desktop.png`, `odoo/12-users-form-dom.json`, `odoo/14-groups-form-dom.json` |
| Mobile launcher / Settings | FAIL | `goerp/14-mobile-launcher-dom.json`, `goerp/15-mobile-settings-dom.json`, `odoo/15-mobile-launcher-dom.json`, `odoo/16-mobile-settings-dom.json` |

## P0 Gaps

1. Main launcher shell is not Enterprise parity.
   - GoERP: `bodyBg=rgb(231, 233, 237)`, `homeBgImage=none`.
   - GoERP harness failures: `launcher-desktop`, `default-webclient-takeover`.
   - Selectors: `.o_web_client`, `.o_home_menu`, `.o_app`, `.o_app_search_input`.
   - URL: `http://127.0.0.1:8075/web?debug=1`.

2. Action shell topbar/control panel styling fails Enterprise parity.
   - GoERP harness failure: `default-webclient-action-desktop`.
   - Metrics: topbar height `46`, background `rgb(113, 75, 103)`, launcher width `36`.
   - GoERP harness failure: `default-technical-search-desktop`.
   - Metrics: control panel background `rgb(255,255,255)`, missing shadow, list header `rgb(247,247,247)`.
   - Selectors: `.o_main_navbar`, `.o_control_panel`, `.gorp-list-view`.

3. Apps catalog DOM is not Odoo-compatible.
   - Odoo: `.o_kanban_renderer=1`, `.o_kanban_record=83`.
   - GoERP direct DOM: `.o_kanban_renderer=0`, `.o_kanban_record=0`.
   - GoERP URL: `http://127.0.0.1:8075/web?debug=1#action=16&model=ir.module.module&view_type=kanban&menu_id=73`.
   - Odoo URL: `http://127.0.0.1:8076/odoo/apps?debug=1`.

4. Mobile launcher and Settings navigation fail parity.
   - GoERP mobile launcher: `bodyBg=rgb(231, 233, 237)`, `homeBgImage=none`.
   - Odoo mobile Settings after launcher click: `settings_container=12`, `control_panel=1`.
   - GoERP mobile Settings after launcher click: `settings_container=0`, `control_panel=0`; body remains launcher/banner.
   - GoERP harness failures: `default-mobile-launcher-parity`, `default-mobile-server-actions-flow`.
   - Viewport: `390x844`.

## Commands Run

```sh
agent-browser doctor --offline --quick
/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo-bin --version
createdb -h 127.0.0.1 -p 5432 gorp_ref_ui_20260628_161611
/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo-bin -d gorp_ref_ui_20260628_161611 --db_host=127.0.0.1 --db_port=5432 --db_user=fadhelalqaidoom --without-demo=all --addons-path=/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons,/Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise -i base,web,base_setup,web_enterprise --stop-after-init --no-http --log-level=warn
/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo-bin -d gorp_ref_ui_20260628_161611 --db_host=127.0.0.1 --db_port=5432 --db_user=fadhelalqaidoom --addons-path=/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons,/Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise --http-port=8076 --log-level=warn
GORP_HTTP_ADDR=127.0.0.1:8075 go run ./cmd/gorpd serve
node tools/web_visual_smoke/run.mjs --base-url=http://127.0.0.1:8075 --out=reports/uiux/goal_ui_parity_20260628_current_goerp_smoke
agent-browser --session odoo-ref ...
agent-browser --session goerp-current ...
```

`make ci` not run. No implementation work was performed.

## Evidence Roots

- `reports/uiux/goal_ui_parity_20260628_independent/`
- `reports/uiux/goal_ui_parity_20260628_current_goerp_smoke/`
