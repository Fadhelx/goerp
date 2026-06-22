# Odoo 19 Target-New Dialog Chrome Parity

Date: 2026-06-22

Scope:
- Default TypeScript `/web` target-new action dialogs.
- Accounting skipped for phase 2.

Implemented:
- Modal form/settings actions suppress the normal action control panel.
- Modal form actions open directly in edit mode.
- Modal form/settings Save and Discard controls render in the modal footer.
- Form XML footer buttons are extracted from the form header/body and rendered in the modal footer.
- Modal form action menus are disabled for target-new forms.
- List/kanban modal actions retain the normal action layout and control panel.
- Dialog content height is viewport bounded; body scrolls and footer remains visible.
- Mobile dialog footer wraps/constrains buttons while retaining fullscreen dialog behavior.
- Required-field validation on footer workflow buttons now has a dedicated DOM regression.

Agent Audits:
- Source audit confirmed Odoo 19 target-new form dialogs use header/body/footer, suppress normal control panel, start editable, put Save/Discard and XML footer buttons in the footer, keep list/kanban control-panel layout, and fullscreen on mobile.
- Local audit found missing XML footer extraction, action menu leakage, and viewport overflow; all three were fixed in this slice.

Verification:
- `pnpm -C frontend exec tsc -p tsconfig.json --noEmit`
- `pnpm -C frontend test -- index.test.mjs apps/webclient/src/main.test.mjs`
- `pnpm -C frontend test -- index.test.mjs main.test.mjs`
- `pnpm -C frontend build`
- `node tools/web_visual_smoke/run.mjs --base-url=http://127.0.0.1:8088 --out=reports/uiux/main_dialog_chrome_20260622 --timeout-ms=60000 --scenario=default-action-dialog-desktop --scenario=default-technical-form-desktop --scenario=default-mobile-server-actions-flow`

Artifacts:
- `reports/uiux/main_dialog_chrome_20260622/manifest.json`
- `reports/uiux/main_dialog_chrome_20260622/default-action-dialog-desktop.png`
- `reports/uiux/main_dialog_chrome_20260622/default-technical-form-desktop.png`
- `reports/uiux/main_dialog_chrome_20260622/default-mobile-server-actions-flow.png`

Remaining Deferred Gaps:
- Advanced dialog body/footer simplification for every wizard model is not complete.
- Mobile header back affordance is close-button based, not exact Odoo mobile header behavior.
