# Odoo 19 Date Filter Period Search Parity

Date: 2026-06-22

Scope:
- Default `/web` search date filter periods.
- Accounting skipped for phase 2.

Implemented:
- Parsed Odoo-style `date`, `default_period`, `start_year`, `end_year`, `start_month`, and `end_month` search filter metadata.
- Generated month, quarter, and year period menu entries for date/datetime filters.
- Added generated date-filter facet metadata for period IDs, default years, field type, and range bounds.
- Added current-month default behavior and automatic year selection for month/quarter periods.
- Added last-year removal behavior that clears generated periods for that date filter.
- Built inclusive date/datetime range domains with server-compatible flat prefix tokens.
- Preserved fallback date filters on models without explicit search arches and preferred business date fields before audit fields.
- Marked fallback date-filter menu children active after rerender.
- Added browser smoke coverage for date period menu, month selection, auto-selected year, rendered facets, and filtered rows.

Remaining Deferred Gaps:
- Nested custom date-period options inside search XML are not parsed yet.
- Datetime bounds do not yet convert browser-local boundaries to UTC like full Odoo web.
- Multi-period facet label punctuation is not exact Odoo slash formatting.
- Full favorite/default precedence is covered for parser-level behavior, not full saved favorite UI lifecycle.

Verification:
- `pnpm -C frontend exec tsc -p tsconfig.json --noEmit`
- `pnpm -C frontend test -- search/search_model.test.mjs index.test.mjs search/search_arch_parser.test.mjs control_panel/control_panel.test.mjs`
- `node --test tools/web_visual_smoke/run.test.mjs`
- `pnpm -C frontend build`
- `node tools/web_visual_smoke/run.mjs --base-url=http://127.0.0.1:8088 --out=reports/uiux/main_date_filter_periods_20260622 --timeout-ms=60000 --scenario=default-date-filter-period-menu-desktop --scenario=default-date-groupby-menu-desktop --scenario=default-search-menu-desktop`

Artifacts:
- `reports/uiux/main_date_filter_periods_20260622/manifest.json`
- `reports/uiux/main_date_filter_periods_20260622/default-date-filter-period-menu-desktop.png`
- `reports/uiux/main_date_filter_periods_20260622/default-date-groupby-menu-desktop.png`
- `reports/uiux/main_date_filter_periods_20260622/default-search-menu-desktop.png`
