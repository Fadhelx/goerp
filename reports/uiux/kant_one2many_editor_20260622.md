# Kant UI Parity Slice: One2many Inline Editor

Date: 2026-06-22
Scope: frontend/UI only. Accounting excluded.
Reference rule: clean-room Odoo-style behavior. No Odoo source or proprietary assets copied.

## Implemented

- Generic form edit mode now renders one2many fields as inline list editors when the form view provides nested list fields.
- One2many editor supports:
  - existing rows
  - inline char/text/number/selection/boolean controls
  - remove existing rows
  - add new rows
  - save payload as Odoo x2many command tuples
  - discard through existing form discard flow
- Readonly one2many tag display now uses row `description` as a display fallback when `display_name`, `name`, and `label` are absent.
- Enterprise dark theme CSS now covers the inline one2many table, controls, and actions.

## Changed Files

- `frontend/packages/webclient/src/index.ts`
- `frontend/packages/webclient/src/index.test.mjs`
- `internal/http/server.go`
- `tools/web_visual_smoke/run.mjs`
- `tools/web_visual_smoke/run.test.mjs`
- `reports/uiux/kant_one2many_editor_20260622.md`

## Verification

- `pnpm -C frontend test -- index.test.mjs`: passed.
- `node --test tools/web_visual_smoke/run.test.mjs`: passed.
- `pnpm -C frontend build`: passed.
- `go test ./internal/http -run TestWebAliasesAndAssets -count=1`: passed.
- `git diff --check`: passed.
- `node tools/web_visual_smoke/run.mjs --base-url=http://127.0.0.1:8079 --out=reports/uiux/kant_one2many_editor_20260622 --scenario=default-delegation-one2many-desktop --timeout-ms=60000`: passed.
- `make ci`: passed.

## DOM Evidence

Focused renderer regression covers:
- one2many editor root class: `gorp-one2many-editor`
- relation metadata: `ir.actions.server.line`
- initial row count: 2
- inline field values: `Old line`, `Drop line`
- update command: `[1, 201, {description, quantity}]`
- unlink command: `[3, 202, false]`
- create command: `[0, false, {description, quantity}]`
- readonly re-render tags: `Updated line`, `New line`

## Browser Evidence

- Added live `/web` visual smoke against Delegation assigned roles one2many.
- Scenario: `default-delegation-one2many-desktop`.
- Result: passed.
- Evidence: `reports/uiux/kant_one2many_editor_20260622/default-delegation-one2many-desktop.png`.
- Manifest: `reports/uiux/kant_one2many_editor_20260622/manifest.json`.

## Remaining Gaps

- One2many many2one autocomplete cells remain readonly.
- One2many create/edit modal dialogs remain incomplete.
- One2many delete/create flags from nested view attrs are not fully enforced.
