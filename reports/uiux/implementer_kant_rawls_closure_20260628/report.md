# Implementer Evidence: Rawls Closure 2026-06-28

Verifier source: `reports/uiux/verifier_kant_nash_closure_20260628/report.md`

## Gap Table

| Rawls failure | Reference metric | GoERP metric after patch | Status |
| --- | ---: | ---: | --- |
| Launcher banner top | `70px` | `70px` | PASS |
| Launcher app row top | `171px` | `171px` | PASS |
| Launcher background asset | `/web_enterprise/static/img/background-dark.jpg` | `/web_enterprise/static/img/background-dark.jpg`, `Content-Type: image/jpeg` | PASS |
| Launcher icon asset type | `data:image/png` | `2` PNG icons, `0` SVG icons | PASS |
| Action navbar color | `rgb(38, 42, 54)` | `rgb(38, 42, 54)` | PASS |
| Action control panel color/height | `rgb(38, 42, 54)`, `58px` | `rgb(38, 42, 54)`, `58px` | PASS |
| Technical singular labels | plural labels required | `singularFailures: []`, plural labels present | PASS |
| Technical/relation/apps regression guards | Existing pass areas | Focused smoke passed | PASS |

## Evidence

GoERP:
- `goerp/launcher-desktop.png`
- `goerp/launcher-dom.json`
- `goerp/action-shell.png`
- `goerp/action-shell-dom.json`
- `goerp/technical-dropdown.png`
- `goerp/technical-dropdown-dom.json`
- `goerp/background-asset-headers.txt`
- `goerp/background-asset.sha256`

Reference:
- `odoo_reference/launcher-desktop.png`
- `odoo_reference/launcher-dom.json`
- `odoo_reference/launcher-rects.json`
- `odoo_reference/action-shell.png`
- `odoo_reference/action-shell-dom.json`
- `odoo_reference/technical-dropdown.png`
- `odoo_reference/technical-dropdown-dom.json`
- `odoo_reference/relation-dropdown.png`
- `odoo_reference/relation-dropdown-dom.json`

Smoke:
- `smoke/manifest.json`
- `smoke/launcher-desktop.png`
- `smoke/default-webclient-action-desktop.png`
- `smoke/default-navbar-technical-dropdown-open-desktop.png`
- `smoke/default-relation-dropdown-desktop.png`
- `smoke/default-mobile-launcher-parity.png`
- `smoke/default-apps-catalog-parity-desktop.png`

## Checks

- `go test ./internal/http -run TestWebAliasesAndAssets`
- `node frontend/packages/webclient/src/home_menu/home_menu.test.mjs`
- `node frontend/packages/webclient/src/webclient/shell.test.mjs`
- `node --test tools/web_visual_smoke/run.test.mjs`
- `pnpm -C frontend build`
- `node tools/web_visual_smoke/run.mjs --base-url http://127.0.0.1:8075 --out reports/uiux/implementer_kant_rawls_closure_20260628/smoke --scenario launcher-desktop --scenario default-webclient-action-desktop --scenario default-navbar-technical-dropdown-open-desktop --scenario default-relation-dropdown-desktop --scenario default-mobile-launcher-parity --scenario default-apps-catalog-parity-desktop`
- `make ci`

## Status

Owned-scope gaps from Rawls are closed by DOM metrics and focused smoke. Independent verifier rerun is still the acceptance gate.
