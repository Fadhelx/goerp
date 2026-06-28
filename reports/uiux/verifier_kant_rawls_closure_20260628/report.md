# Verifier: Kant Rawls Closure UI Patch

## Result

| Gate | Status | Evidence |
| --- | --- | --- |
| Launcher spacing | PASS | Odoo banner/app row `70/171`; GoERP `70/171` |
| Clean-room launcher assets | PASS | GoERP background `1920x1080` JPEG hash differs from Odoo source hash; icons are `100x100` PNG data URIs with `data-generated-icon=clean-room` |
| Action navbar/control panel color | PASS | Odoo navbar/control `rgb(38, 42, 54)`; GoERP navbar/control `rgb(38, 42, 54)` |
| Technical plural labels | PASS | Required plurals present; singular variants absent; `35` item leaves, `45` visible labels |
| Settings > Technical nested dropdown | PASS | Dropdown opened; Server Actions click reached active title `Server Actions` |
| Relation dropdown/autocomplete controls | PASS | Many2one `Mail Server`; x2many dropdown open; active descendant present |
| Apps catalog kanban/action/detail flow | PASS | `26` GoERP cards; action buttons present; detail modal count `1` |
| Mobile launcher smoke | PASS | `2` apps; background present; no horizontal overflow; Settings opens action view |

## Screenshots

GoERP:
- `reports/uiux/verifier_kant_rawls_closure_20260628/goerp/launcher-desktop.png`
- `reports/uiux/verifier_kant_rawls_closure_20260628/goerp/default-webclient-action-desktop.png`
- `reports/uiux/verifier_kant_rawls_closure_20260628/goerp/action-shell-agent.png`
- `reports/uiux/verifier_kant_rawls_closure_20260628/goerp/default-navbar-technical-dropdown-open-desktop.png`
- `reports/uiux/verifier_kant_rawls_closure_20260628/goerp/default-relation-dropdown-desktop.png`
- `reports/uiux/verifier_kant_rawls_closure_20260628/goerp/default-apps-catalog-parity-desktop.png`
- `reports/uiux/verifier_kant_rawls_closure_20260628/goerp/default-apps-catalog-detail-desktop.png`
- `reports/uiux/verifier_kant_rawls_closure_20260628/goerp/default-mobile-launcher-parity.png`

Odoo reference:
- `reports/uiux/verifier_kant_rawls_closure_20260628/odoo_reference/launcher-desktop.png`
- `reports/uiux/verifier_kant_rawls_closure_20260628/odoo_reference/action-shell.png`
- `reports/uiux/verifier_kant_rawls_closure_20260628/odoo_reference/technical-dropdown.png`
- `reports/uiux/verifier_kant_rawls_closure_20260628/odoo_reference/relation-dropdown.png`
- `reports/uiux/verifier_kant_rawls_closure_20260628/odoo_reference/apps-catalog.png`

## DOM Evidence

- `reports/uiux/verifier_kant_rawls_closure_20260628/dom/verification-summary.json`
- `reports/uiux/verifier_kant_rawls_closure_20260628/dom/goerp-smoke-metrics.json`
- `reports/uiux/verifier_kant_rawls_closure_20260628/dom/goerp-action-shell-metrics.json`
- `reports/uiux/verifier_kant_rawls_closure_20260628/dom/odoo-launcher-metrics.json`
- `reports/uiux/verifier_kant_rawls_closure_20260628/dom/odoo-action-shell-metrics.json`
- `reports/uiux/verifier_kant_rawls_closure_20260628/dom/odoo-technical-dropdown-metrics.json`
- `reports/uiux/verifier_kant_rawls_closure_20260628/dom/odoo-relation-dropdown-metrics.json`
- `reports/uiux/verifier_kant_rawls_closure_20260628/dom/odoo-apps-catalog-metrics.json`
- `reports/uiux/verifier_kant_rawls_closure_20260628/dom/asset-noncopy-check.json`
- `reports/uiux/verifier_kant_rawls_closure_20260628/dom/goerp-cleanroom-icon-metadata.json`

## Commands

- `go test -count=1 ./internal/http -run TestWebAliasesAndAssets`
- `node frontend/packages/webclient/src/home_menu/home_menu.test.mjs`
- `node frontend/packages/webclient/src/webclient/shell.test.mjs`
- `node --test tools/web_visual_smoke/run.test.mjs`
- `GORP_HTTP_ADDR=127.0.0.1:8077 go run ./cmd/gorpd serve`
- `node tools/web_visual_smoke/run.mjs --base-url http://127.0.0.1:8077 --out reports/uiux/verifier_kant_rawls_closure_20260628/goerp --timeout-ms 30000 --scenario launcher-desktop --scenario default-webclient-action-desktop --scenario default-navbar-nested-menus-desktop --scenario default-navbar-technical-dropdown-open-desktop --scenario default-relation-dropdown-desktop --scenario default-apps-catalog-parity-desktop --scenario default-apps-catalog-detail-desktop --scenario default-mobile-launcher-parity`
- `./odoo/odoo-bin --addons-path=enterprise,addons,odoo/addons -d gorp_ref_ui_verifier_kant_20260628_180701 --db-filter='^gorp_ref_ui_verifier_kant_20260628_180701$' --http-interface=127.0.0.1 --http-port=8076 --max-cron-threads=0 --workers=0 --no-database-list --log-level=warn`
- `agent-browser` screenshot and DOM eval commands for Odoo reference
- `make ci`
