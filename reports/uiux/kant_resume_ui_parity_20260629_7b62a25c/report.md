# UI parity continuation report

Commit checked: `7b62a25c`

## Implemented fix

- Fixed Settings action chrome contrast in the action shell.
- Changed Settings block header background to `rgb(38, 42, 54)`.
- Changed Settings action links from `#00d4c8` to `#8ddad8`.

## Files changed

- `frontend/packages/webclient/src/settings/settings_renderer.ts`
- `frontend/apps/webclient/src/main.ts`
- `frontend/apps/webclient/src/main.test.mjs`

## Evidence paths

Pre-fix smoke failure:

- `reports/uiux/kant_resume_ui_parity_20260629_7b62a25c/goerp/manifest.json`
- `reports/uiux/kant_resume_ui_parity_20260629_7b62a25c/goerp/default-webclient-action-desktop.png`

Post-fix focused evidence:

- `reports/uiux/kant_resume_ui_parity_20260629_7b62a25c/goerp_after_settings_contrast/manifest.json`
- `reports/uiux/kant_resume_ui_parity_20260629_7b62a25c/goerp_after_settings_contrast/default-webclient-action-desktop.png`

Post-fix broad evidence:

- `reports/uiux/kant_resume_ui_parity_20260629_7b62a25c/goerp_after_patch/manifest.json`
- `reports/uiux/kant_resume_ui_parity_20260629_7b62a25c/goerp_after_patch/launcher-desktop.png`
- `reports/uiux/kant_resume_ui_parity_20260629_7b62a25c/goerp_after_patch/default-navbar-nested-menus-desktop.png`
- `reports/uiux/kant_resume_ui_parity_20260629_7b62a25c/goerp_after_patch/default-navbar-technical-dropdown-open-desktop.png`
- `reports/uiux/kant_resume_ui_parity_20260629_7b62a25c/goerp_after_patch/settings-desktop.png`
- `reports/uiux/kant_resume_ui_parity_20260629_7b62a25c/goerp_after_patch/default-webclient-action-desktop.png`
- `reports/uiux/kant_resume_ui_parity_20260629_7b62a25c/goerp_after_patch/default-user-preferences-dialog-desktop.png`
- `reports/uiux/kant_resume_ui_parity_20260629_7b62a25c/goerp_after_patch/default-action-dialog-desktop.png`
- `reports/uiux/kant_resume_ui_parity_20260629_7b62a25c/goerp_after_patch/default-custom-filter-dialog-desktop.png`
- `reports/uiux/kant_resume_ui_parity_20260629_7b62a25c/goerp_after_patch/default-apps-catalog-parity-desktop.png`
- `reports/uiux/kant_resume_ui_parity_20260629_7b62a25c/goerp_after_patch/default-apps-catalog-detail-desktop.png`
- `reports/uiux/kant_resume_ui_parity_20260629_7b62a25c/goerp_after_patch/default-relation-dropdown-desktop.png`
- `reports/uiux/kant_resume_ui_parity_20260629_7b62a25c/goerp_after_patch/default-relation-dialog-actions-desktop.png`
- `reports/uiux/kant_resume_ui_parity_20260629_7b62a25c/goerp_after_patch/default-users-list-parity-desktop.png`
- `reports/uiux/kant_resume_ui_parity_20260629_7b62a25c/goerp_after_patch/default-users-flow-desktop.png`
- `reports/uiux/kant_resume_ui_parity_20260629_7b62a25c/goerp_after_patch/default-groups-list-parity-desktop.png`
- `reports/uiux/kant_resume_ui_parity_20260629_7b62a25c/goerp_after_patch/default-groups-form-notebook-desktop.png`
- `reports/uiux/kant_resume_ui_parity_20260629_7b62a25c/goerp_after_patch/default-webclient-mobile.png`
- `reports/uiux/kant_resume_ui_parity_20260629_7b62a25c/goerp_after_patch/default-mobile-launcher-parity.png`

Agent-browser evidence:

- `reports/uiux/kant_resume_ui_parity_20260629_7b62a25c/agent-browser-web.png`
- `reports/uiux/kant_resume_ui_parity_20260629_7b62a25c/agent-browser-web-snapshot.txt`

Reference Odoo evidence used:

- `reports/uiux/goal_ui_parity_20260628_independent/odoo/01-launcher-desktop.png`
- `reports/uiux/goal_ui_parity_20260628_independent/odoo/02-settings-desktop.png`
- `reports/uiux/goal_ui_parity_20260628_independent/odoo/03-technical-dropdown.png`
- `reports/uiux/goal_ui_parity_20260628_independent/odoo/04-apps-catalog.png`
- `reports/uiux/goal_ui_parity_20260628_independent/odoo/08-relation-dropdown.png`
- `reports/uiux/goal_ui_parity_20260628_independent/odoo/11-users-list.png`
- `reports/uiux/goal_ui_parity_20260628_independent/odoo/13-groups-list.png`
- `reports/uiux/goal_ui_parity_20260628_independent/odoo/15-mobile-launcher.png`
- `reports/uiux/goal_ui_parity_20260628_independent/odoo/16-mobile-settings.png`

## Post-fix DOM metrics

- Launcher: 2 apps, 46px navbar, banner top 70px, first app top 171px, 70px icons, developer tools visible, Studio visible.
- Technical dropdown: expanded, visible, 10 grouped headers, 35 items, reference order true.
- Settings action shell: title color `rgb(244, 245, 247)`, title background `rgb(38, 42, 54)`, low contrast action count 0, low contrast muted count 0.
- Relation dropdown: `bottom-start`, dark theme, background `rgb(75, 77, 89)`, active option present, `Search more...` present.
- Apps catalog: 77 cards, 24 visible cards, 50px icons, 94px first-card height, action entries visible in navbar sections.
- Mobile Settings: first block y=189, sidebar h=40, horizontal overflow 0, title color/background valid.

## Commands run

```sh
node tools/web_visual_smoke/run.mjs --base-url http://127.0.0.1:8069 --out reports/uiux/kant_resume_ui_parity_20260629_7b62a25c/goerp --scenario launcher-desktop --scenario default-navbar-nested-menus-desktop --scenario default-navbar-technical-dropdown-open-desktop --scenario settings-desktop --scenario default-webclient-action-desktop --scenario default-user-preferences-dialog-desktop --scenario default-action-dialog-desktop --scenario default-custom-filter-dialog-desktop --scenario default-apps-catalog-parity-desktop --scenario default-apps-catalog-detail-desktop --scenario default-relation-dropdown-desktop --scenario default-relation-dialog-actions-desktop --scenario default-users-list-parity-desktop --scenario default-users-flow-desktop --scenario default-groups-list-parity-desktop --scenario default-groups-form-notebook-desktop --scenario default-webclient-mobile --scenario default-mobile-launcher-parity --timeout-ms 30000
pnpm -C frontend build
node frontend/apps/webclient/src/main.test.mjs
node frontend/packages/webclient/src/settings/settings_renderer.test.mjs
node --test tools/web_visual_smoke/run.test.mjs
node tools/web_visual_smoke/run.mjs --base-url http://127.0.0.1:8069 --out reports/uiux/kant_resume_ui_parity_20260629_7b62a25c/goerp_after_settings_contrast --scenario default-webclient-action-desktop --timeout-ms 30000
node tools/web_visual_smoke/run.mjs --base-url http://127.0.0.1:8069 --out reports/uiux/kant_resume_ui_parity_20260629_7b62a25c/goerp_after_patch --scenario launcher-desktop --scenario default-navbar-nested-menus-desktop --scenario default-navbar-technical-dropdown-open-desktop --scenario settings-desktop --scenario default-webclient-action-desktop --scenario default-user-preferences-dialog-desktop --scenario default-action-dialog-desktop --scenario default-custom-filter-dialog-desktop --scenario default-apps-catalog-parity-desktop --scenario default-apps-catalog-detail-desktop --scenario default-relation-dropdown-desktop --scenario default-relation-dialog-actions-desktop --scenario default-users-list-parity-desktop --scenario default-users-flow-desktop --scenario default-groups-list-parity-desktop --scenario default-groups-form-notebook-desktop --scenario default-webclient-mobile --scenario default-mobile-launcher-parity --timeout-ms 30000
make ci
agent-browser --session gorp-ui-parity open http://127.0.0.1:8069/web
agent-browser --session gorp-ui-parity snapshot -i -c
agent-browser --session gorp-ui-parity screenshot reports/uiux/kant_resume_ui_parity_20260629_7b62a25c/agent-browser-web.png
agent-browser --session gorp-ui-parity close
```

## Results

- Initial broad smoke: failed `default-webclient-action-desktop`.
- Focused post-fix smoke: passed.
- Post-fix broad smoke: passed 18/18 scenarios.
- Focused frontend tests: passed.
- `make ci`: passed.
- Agent-browser screenshot/snapshot: captured.

## Remaining deltas/blockers

- Exact Odoo Enterprise proprietary background/icon assets are not copied. Current assets are clean-room approximations unless a written license/provenance decision allows exact asset use.
- No local smoke blocker remains in this slice.
- Independent verifier must still validate before final parity is claimed.
