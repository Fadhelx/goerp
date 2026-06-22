# Date Group-by Search Slice

Date: 2026-06-22

Scope:
- Default `/web` search control panel date group-by behavior.
- Accounting remains deferred to phase 2.

Implemented:
- Search arch parser recognizes `<filter date="...">` metadata.
- Bare `group_by` on date/datetime fields defaults to month.
- Date/datetime group-by menus expose Year, Quarter, Month, Week, Day.
- Group-by interval facets carry category/value labels for Odoo-style chips.
- Fallback date group-by menu keeps active checked state after rerender.
- Focused browser smoke covers interval menu, year selection, facet rendering, and selected state.

Verification:
- `pnpm -C frontend build`
- `pnpm -C frontend test -- search/search_arch_parser.test.mjs search/search_model.test.mjs control_panel/control_panel.test.mjs index.test.mjs`
- `pnpm -C frontend typecheck`
- `node --test tools/web_visual_smoke/run.test.mjs`
- `node tools/web_visual_smoke/run.mjs --base-url=http://127.0.0.1:8088 --out=reports/uiux/main_date_groupby_20260622 --timeout-ms=60000 --scenario=default-date-groupby-menu-desktop --scenario=default-search-filter-click-desktop --scenario=default-search-menu-desktop`

Remaining:
- Full date filter period generation and domain toggles.
- `default_period` and custom period parity.
- Timezone-aware date/datetime domain bounds.
- Exact multi-level facet label punctuation.
