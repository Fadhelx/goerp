# Current GoERP UI Runtime Audit

Date: 2026-06-21
Workspace: `/Users/fadhelalqaidoom/Documents/gorp`
Target: `http://127.0.0.1:8073/web`

## Evidence

- Visual smoke: passed 7/7 scenarios.
- Console errors: none returned by `agent-browser errors`.
- Console log: none returned by `agent-browser console`.
- No frontend source files changed.
- Existing dirty frontend files were not touched.
- Normal-user browser UX was not verified from runtime. Seeded visible credentials expose admin only. Backend coverage exists in `TestBootstrapWebNormalUserCanOpenRecordsListAndForm`.

Screenshots:
- `reports/uiux/subagents/current-audit/launcher-desktop.png`
- `reports/uiux/subagents/current-audit/launcher-mobile.png`
- `reports/uiux/subagents/current-audit/settings-desktop.png`
- `reports/uiux/subagents/current-audit/technical-list-desktop.png`
- `reports/uiux/subagents/current-audit/search-menu-desktop.png`
- `reports/uiux/subagents/current-audit/technical-form-desktop.png`
- `reports/uiux/subagents/current-audit/technical-list-mobile.png`
- `reports/uiux/subagents/current-audit/technical-form-mobile-current.png`
- `reports/uiux/subagents/current-audit/mobile-menu-current.png`
- `reports/uiux/subagents/current-audit/apps-install-current.png`
- `reports/uiux/subagents/current-audit/apps-search-menu-current.png`
- `reports/uiux/subagents/current-audit/manifest.json`

## Prioritized Findings

### P0. `/web` runtime still behaves like a static shell, not an Odoo web client

Evidence:
- `reports/uiux/subagents/current-audit/technical-list-desktop.png`
- `reports/uiux/subagents/current-audit/technical-form-desktop.png`
- `reports/uiux/subagents/current-audit/search-menu-desktop.png`

Observed:
- List/form/view switcher surfaces render.
- Action manager behavior is shallow.
- No row selection checkbox/action flow.
- No cog/action menu parity.
- No real facet chips in active search.
- No persistent URL/action state visible.
- Form fields render as basic text inputs.

Expected Odoo-style behavior:
- Action stack with breadcrumb ownership.
- Control panel with search model, facets, filters, group by, favorites, pager, view switcher, action/cog menus.
- List renderer with row selection and action dropdown.
- Form renderer with header actions, smart buttons, tabs, widgets, statusbar/chatter where model/view requires it.

### P0. Form header layout breaks on desktop and mobile

Evidence:
- `reports/uiux/subagents/current-audit/technical-form-desktop.png`
- `reports/uiux/subagents/current-audit/technical-form-mobile-current.png`

Observed:
- Save/Discard overlaps breadcrumb/title area.
- Mobile title and breadcrumb collide.
- Breadcrumb text wraps under buttons.

Expected Odoo-style behavior:
- Stable control-panel rows.
- Breadcrumb/title never overlaps primary buttons.
- Mobile form header compacts controls without text collision.

### P0. Apps/install flow lacks Odoo app catalog behavior

Evidence:
- `reports/uiux/subagents/current-audit/apps-install-current.png`
- `reports/uiux/subagents/current-audit/apps-search-menu-current.png`

Observed:
- Cards show technical module names: `Ai`, `Analytic`, `Automation`, `Base`, `Setup`.
- Cards show generated initials, not app icons.
- No descriptions, categories, official/industry sidebar, tags, Learn More, Module Info, or activation workflow.
- Search options button toggles active styling but no visible menu opens.

Expected Odoo-style behavior:
- App catalog cards with product names, descriptions, icons, category sidebar, app filters, Activate/Learn More/Module Info actions, and list/kanban switch.

### P1. Settings and Technical menu surface is card-based and sparse

Evidence:
- `reports/uiux/subagents/current-audit/settings-desktop.png`

Observed:
- Settings renders menu destinations as rectangular cards with `Open` buttons.
- Technical menus are available but visually generic.
- Search is present but not equivalent to Odoo settings search and section navigation.

Expected Odoo-style behavior:
- Settings app with Odoo setting blocks, typed controls, app sections, anchors, and technical menu hierarchy.

### P1. List/search control-panel state is incomplete

Evidence:
- `reports/uiux/subagents/current-audit/technical-list-desktop.png`
- `reports/uiux/subagents/current-audit/search-menu-desktop.png`
- `reports/uiux/subagents/current-audit/technical-list-mobile.png`

Observed:
- Search menu shows Group By and Favorites but filter visibility/placement is weak.
- Pager renders as static `1-20 / 20`.
- View buttons are text labels.
- Mobile controls crowd one horizontal band.

Expected Odoo-style behavior:
- Search facets and filter chips.
- Group By and Favorites with dropdown state.
- Pager controls with previous/next affordances.
- Icon-based view switcher with accessible labels.
- Mobile search/control panel with responsive stacking.

### P1. Navbar and mobile menu are only partial Odoo equivalents

Evidence:
- `reports/uiux/subagents/current-audit/launcher-desktop.png`
- `reports/uiux/subagents/current-audit/launcher-mobile.png`
- `reports/uiux/subagents/current-audit/mobile-menu-current.png`

Observed:
- Desktop navbar has launcher, company, user, messages, activities.
- Mobile menu opens top navigation links only.
- Company switcher is absent on mobile.
- User/company/debug systray behavior is not Odoo-like.

Expected Odoo-style behavior:
- Full systray menu behaviors, user menu items, company switching, debug tools, messaging/activity dropdowns, and mobile-safe navigation.

### P1. Launcher visual parity remains low

Evidence:
- `reports/uiux/subagents/current-audit/launcher-desktop.png`
- `reports/uiux/subagents/current-audit/launcher-mobile.png`

Observed:
- 4 app tiles: Approvals, Delegation, Settings, Apps.
- Icons are generated letter blocks.
- Desktop search bar and app grid spacing differ from Odoo Enterprise.
- No subscription/register banner or app metadata behavior.

Expected Odoo-style behavior:
- Enterprise home menu with recognizable app icons, tuned spacing, search/filter behavior, and integrated systray.

### P2. Runtime default login path is admin-oriented

Evidence:
- `reports/uiux/subagents/current-audit/manifest.json`
- Runtime session inspection: unauthenticated session is `uid:0`; shell auto-retries populated admin/admin credentials after protected endpoint 401.

Observed:
- Browser runtime inspection lands in admin workflows by default.
- Seeded visible credentials include admin/public only; public is inactive.
- Normal-user browser flow cannot be verified without creating data or using test-only helpers.

Expected Odoo-style behavior:
- Real login screen/session handling.
- Non-admin demo/internal user path for workflow verification.
- Normal user sees only allowed apps and no Settings/Technical menus.

## Verification Commands

- `GORP_HTTP_ADDR=127.0.0.1:8073 go run ./cmd/gorpd serve`
- `node tools/web_visual_smoke/run.mjs --base-url=http://127.0.0.1:8073 --out=reports/uiux/subagents/current-audit`
- `agent-browser open http://127.0.0.1:8073/web`
- `agent-browser errors`
- `agent-browser console`

`make ci` was not run. No implementation work was performed.
