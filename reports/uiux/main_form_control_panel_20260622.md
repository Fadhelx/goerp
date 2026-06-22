# Form Control Panel Parity Slice

Date: 2026-06-22
Scope: default `/web` form control panel behavior, accounting excluded.
Reference policy: clean-room only. `/Users/fadhelalqaidoom/Desktop/odoo` is reference input only.

## Implemented

- Form views no longer render list/search controls in the control panel.
- Form breadcrumbs, Edit/Save/Discard buttons, and existing view-switch behavior remain intact.
- Desktop Server Action form smoke now asserts zero form search inputs and search option toggles.
- Mobile Server Action flow smoke now asserts zero form search inputs and search option toggles after opening a form.

## Evidence

- Focused visual smoke: `reports/uiux/main_form_control_panel_20260622/manifest.json`
- Screenshots: `reports/uiux/main_form_control_panel_20260622/`

## Verification

- `pnpm -C frontend test -- index.test.mjs control_panel/control_panel.test.mjs`: passed.
- `pnpm -C frontend build`: passed.
- `node --test tools/web_visual_smoke/run.test.mjs`: passed.
- `node tools/web_visual_smoke/run.mjs --base-url=http://127.0.0.1:8098 --out=reports/uiux/main_form_control_panel_20260622 --timeout-ms=60000 --scenario=default-technical-form-desktop --scenario=default-mobile-server-actions-flow`: passed.

## Remaining

- Form record pager parity is still incomplete.
- Mobile form edit/save flow still needs dedicated parity verification.
- Mobile back-stack behavior beyond current breadcrumbs remains incomplete.
