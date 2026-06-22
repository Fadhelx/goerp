# Kant UI Parity Slice: Many2many Tag Editor

Date: 2026-06-22
Scope: frontend/UI only. Accounting excluded.
Reference rule: clean-room Odoo-style behavior. No Odoo source or proprietary assets copied.

## Implemented

- Generic form edit mode now renders editable many2many tag widgets and bounded one2many list editors.
- Many2many tag editor supports:
  - existing selected tags
  - remove selected tag
  - relation search via `name_search`
  - add searched record
  - save payload as Odoo x2many set command
  - discard back to readonly form
- Bounded one2many editor supports:
  - existing row rendering
  - row field edit
  - add transient row
  - remove persisted/transient row
  - save payload as Odoo create/update/unlink commands
- Enterprise dark theme CSS now covers the editable many2many widget and dropdown.
- Groups form visual smoke now verifies the editable many2many widget exists in live `/web`.

## Changed Files

- `frontend/packages/webclient/src/index.ts`
- `frontend/packages/webclient/src/index.test.mjs`
- `internal/http/server.go`
- `tools/web_visual_smoke/run.mjs`
- `reports/uiux/kant_many2many_editor_20260622.md`
- `reports/uiux/kant_many2many_editor_20260622/*`

## Verification

- `pnpm -C frontend test -- index.test.mjs`: passed.
- `node --test tools/web_visual_smoke/run.test.mjs`: passed.
- `pnpm -C frontend build`: passed.
- `go test ./internal/http -run TestWebAliasesAndAssets -count=1`: passed.
- `git diff --check`: passed.
- `node tools/web_visual_smoke/run.mjs --base-url=http://127.0.0.1:8079 --out=reports/uiux/kant_many2many_editor_20260622 --scenario=default-groups-form-notebook-desktop --timeout-ms=60000`: passed.

## Evidence

- Screenshot: `reports/uiux/kant_many2many_editor_20260622/default-groups-form-notebook-desktop.png`
- Manifest: `reports/uiux/kant_many2many_editor_20260622/manifest.json`
- Manifest assertions:
  - `x2many_widget_count`: 1
  - `x2many_editor_count`: 1
  - `x2many_editor_state.relation`: `res.groups`
  - `x2many_editor_state.input_role`: `combobox`
  - `x2many_editor_state.input_autocomplete`: `list`

## Remaining Gaps

- One2many inline/list editing is bounded to simple embedded lists; full Odoo widget/domain/onchange parity remains incomplete.
- Many2many create/edit relation dialogs remain incomplete.
- Many2many access-error and no-create/no-open option states remain incomplete.
- Browser smoke verifies widget presence and discard. Add/remove/save behavior is covered by the focused frontend regression.
- Full `make ci` was not run by this UI worker during this slice.
