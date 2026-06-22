# Many2one Relation Link UI Slice

Date: 2026-06-22

Scope: frontend/UI only. Accounting excluded. Clean-room implementation.

Implemented:
- Readonly many2one form values render as `.o_field_many2one` links.
- Links expose field, relation, and target record id metadata.
- Links use hash-safe form routes and action-service navigation.
- Light and Enterprise-like shell themes style relation links.

Evidence:
- Manual DOM check on `http://127.0.0.1:8077/web` found `model_id -> ir.model/81` with text `mail.mail`.
- Main visual smoke output: `reports/uiux/main_many2one_link_20260622/`.
- UI agent evidence: `reports/uiux/kant_relation_widget_20260622/`.

Commands passed:
- `pnpm -C frontend test`
- `go test ./internal/http -run 'TestWebAliasesAndAssets|TestModuleLifecycle' -count=1`
- `node --test tools/web_visual_smoke/run.test.mjs`
- `pnpm -C frontend build`
- `node tools/web_visual_smoke/run.mjs --base-url=http://127.0.0.1:8077 --out=reports/uiux/main_many2one_link_20260622 --scenario=default-technical-form-desktop --timeout-ms=60000`

Remaining gaps:
- Editable many2one autocomplete/dropdown behavior.
- Access-error handling when opening target records.
- External-link/open-dialog variants.
- x2many embedded relation navigation.
- Full relation widget parity.
