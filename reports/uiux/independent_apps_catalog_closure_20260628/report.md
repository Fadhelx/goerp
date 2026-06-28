# Independent Apps Catalog Closure Verification 2026-06-28

Status: PASS

## Required Checks

| Check | Result | Metric |
| --- | --- | --- |
| Apps route action kanban catalog | PASS | action=1, model=ir.module.module, view=kanban, renderer=1, cards=77, generic_fields_first_24=0 |
| App-store cards and clean-room icons | PASS | clean_room_icons=77/77, first_card=322x109, same_row_delta=0 |
| Sidebar facets/categories | PASS | sidebar=1, headers=APPS, CATEGORIES, labels=16 |
| Expected buttons | PASS | activate=74, learn_more=76, module_info=1, first_card_actions=Activate + Learn More |
| Create button absent | PASS | create_button_count=0 |
| Pager/count alignment | PASS | catalog_total=77, visible=77, pager=1-77 / 77 |
| Launcher/action shell regressions | PASS | smoke_passed=5/5, brand=Apps, nav=Apps |

## Smoke Scenarios

| Scenario | Status | Duration ms | Screenshot |
| --- | --- | ---: | --- |
| launcher-desktop | passed | 271 | launcher-desktop.png |
| default-webclient-action-desktop | passed | 350 | default-webclient-action-desktop.png |
| default-launcher-back-mode-desktop | passed | 382 | default-launcher-back-mode-desktop.png |
| default-apps-catalog-parity-desktop | passed | 551 | default-apps-catalog-parity-desktop.png |
| default-apps-catalog-detail-desktop | passed | 760 | default-apps-catalog-detail-desktop.png |

## Reference Comparison

Odoo reference source root: /Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo
Odoo reference screenshots found: false
Odoo reference source selector lines:
- Apps action/model/menus: /Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo/addons/base/views/ir_module_views.xml: lines 196, 198, 215
- Apps kanban create=false/can_open=false: /Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo/addons/base/views/ir_module_views.xml: line 153
- Activate/Module Info/Learn More buttons: /Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo/addons/base/views/ir_module_views.xml: lines 66, 134, 180, 183, 164, 182
- Apps navbar/menu shell: /Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/web/static/src/webclient/navbar/navbar.xml: lines 75, 77, 111

## Failing Selectors/Metrics

None.

## Commands Run

- sed -n '1,240p' /Users/fadhelalqaidoom/.agents/skills/odoo-product-platform/SKILL.md
- sed -n '1,240p' /Users/fadhelalqaidoom/.agents/skills/agent-browser/SKILL.md
- agent-browser skills get core
- git status --short
- rg/sed/find repo mapping commands
- GORP_HTTP_ADDR=127.0.0.1:8073 go run ./cmd/gorpd serve
- curl -fsS http://127.0.0.1:8073/web/health
- node tools/web_visual_smoke/run.mjs --list
- node tools/web_visual_smoke/run.mjs --base-url=http://127.0.0.1:8073 --out=reports/uiux/independent_apps_catalog_closure_20260628/smoke --timeout-ms=45000 --scenario=launcher-desktop --scenario=default-webclient-action-desktop --scenario=default-launcher-back-mode-desktop --scenario=default-apps-catalog-parity-desktop --scenario=default-apps-catalog-detail-desktop
- rg/find reference discovery under /Users/fadhelalqaidoom/Desktop/odoo
- agent-browser --session goerp-apps-closure open 'http://127.0.0.1:8073/web?independent_apps_catalog=1'
- agent-browser --session goerp-apps-closure click @e8
- agent-browser --session goerp-apps-closure screenshot reports/uiux/independent_apps_catalog_closure_20260628/agent-browser-apps-catalog.png
- agent-browser --session goerp-apps-closure eval --stdin

## Evidence Paths

- reports/uiux/independent_apps_catalog_closure_20260628/report.md
- reports/uiux/independent_apps_catalog_closure_20260628/agent-browser-dom-metrics.json
- reports/uiux/independent_apps_catalog_closure_20260628/agent-browser-apps-catalog.png
- reports/uiux/independent_apps_catalog_closure_20260628/odoo-reference-selector-summary.json
- reports/uiux/independent_apps_catalog_closure_20260628/smoke/manifest.json
- reports/uiux/independent_apps_catalog_closure_20260628/smoke/launcher-desktop.png
- reports/uiux/independent_apps_catalog_closure_20260628/smoke/default-webclient-action-desktop.png
- reports/uiux/independent_apps_catalog_closure_20260628/smoke/default-launcher-back-mode-desktop.png
- reports/uiux/independent_apps_catalog_closure_20260628/smoke/default-apps-catalog-parity-desktop.png
- reports/uiux/independent_apps_catalog_closure_20260628/smoke/default-apps-catalog-detail-desktop.png
- reports/uiux/independent_apps_catalog_closure_20260628/gorpd-8073.log
