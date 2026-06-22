# X2many Tag UI Slice

Date: 2026-06-22

Scope: frontend/UI only. Accounting excluded. Clean-room implementation.

Implemented:
- Readonly many2many and one2many values render as Odoo-shaped tag widgets.
- Widgets expose field, type, relation, and count metadata.
- Tuple/object/id display values are deduped.
- Common x2many command values are handled safely.
- Persisted tag records can navigate through action-service form opening.
- Light and Enterprise-like shell themes style x2many tags.

Evidence:
- Visual smoke output: `reports/uiux/main_x2many_tags_20260622/`.
- Scenario: `default-groups-form-notebook-desktop`.
- Result: passed. Groups form rendered one x2many widget for `inherited_by_ids`, relation `res.groups`, no duplicate form field wrapper.
- Live fixture note: the current Groups fixture has zero inherited tags, so label rendering is covered by focused frontend regressions.

Commands passed:
- `pnpm -C frontend build`
- `node frontend/packages/webclient/src/index.test.mjs`
- `node --test tools/web_visual_smoke/run.test.mjs`
- `go test ./internal/http -run TestWebAliasesAndAssets`
- `node tools/web_visual_smoke/run.mjs --base-url=http://127.0.0.1:8078 --out=reports/uiux/main_x2many_tags_20260622 --scenario=default-groups-form-notebook-desktop --timeout-ms=60000`
- `git diff --check`

Remaining gaps:
- Editable x2many list/tag editors.
- Live populated x2many smoke fixture data.
- Access-error handling.
- Inline create/edit dialogs.
- Full relation widget parity.
