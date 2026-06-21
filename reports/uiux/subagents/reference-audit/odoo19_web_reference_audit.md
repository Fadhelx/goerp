# Odoo 19 Enterprise Web Reference Audit

Date: 2026-06-21
Workspace: `/Users/fadhelalqaidoom/Documents/gorp`
Reference input only: `/Users/fadhelalqaidoom/Desktop/odoo`
Scope: `/web` UI behavior. Accounting excluded.

## Inputs Inspected

Reference source:
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise/web_enterprise`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/web`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/base_setup`

Target source:
- `internal/http/server.go`
- `frontend/packages/webclient/src/home_menu/*`
- `frontend/packages/webclient/src/control_panel/*`
- `frontend/packages/webclient/src/search/*`
- `frontend/packages/webclient/src/router/*`
- `frontend/packages/webclient/src/services/*`

Runtime/screenshot artifacts:
- `reports/uiux/screenshots/odoo-desktop-apps.png`
- `reports/uiux/screenshots/odoo-desktop-settings-debug.png`
- `reports/uiux/screenshots/odoo-desktop-server-actions-list.png`
- `reports/uiux/screenshots/odoo-desktop-server-action-form.png`
- `reports/uiux/screenshots/odoo-mobile-home.png`
- `reports/uiux/screenshots/odoo-mobile-contact-form.png`
- `reports/uiux/screenshots/goerp-desktop-apps.png`
- `reports/uiux/screenshots/goerp-desktop-form.png`
- `reports/uiux/screenshots/goerp-mobile-apps.png`
- `reports/web_visual_smoke/settings-desktop.png`
- `reports/web_visual_smoke/technical-list-desktop.png`
- `reports/web_visual_smoke/technical-form-desktop.png`

## Findings

1. `/web` still depends on an inline static shell.
   - Evidence: `internal/http/server.go` writes `webClientShellHTML`.
   - Impact: frontend runtime modules are not the actual production web client.
   - Parity risk: action lifecycle, services, dialogs, routing, and view controllers cannot reach Odoo behavior from the current shell.

2. Enterprise launcher parity is low.
   - Reference: dark home menu, real app metadata, systray, company/user state, mobile home layout.
   - Target: light launcher, initials-based icons, duplicated Delegation app, desktop search box on mobile.
   - Required: provenance-safe generated/licensed icons. Do not copy Odoo assets.

3. Navbar/systray behavior is selector-level only.
   - Reference: enterprise navbar integrates app switcher, current app sections, debug, messages, activities, company switcher, user menu, and mobile burger behavior.
   - Target: visible shell elements exist, but dropdown services and mobile state are incomplete.

4. Settings is structurally wrong.
   - Reference: settings view compiles `<app>`, `<block>`, and `<setting>` nodes into app tabs, section blocks, searchable settings, dirty-state save/discard, and confirmation dialog.
   - Target: settings displays action cards for menus.
   - Impact: General Settings, Users & Companies, Technical, Translations, and installed-module settings cannot behave like Odoo.

5. Technical menus are reachable but not Odoo-like.
   - Reference: Technical is a Settings app section with top navigation and action views.
   - Target: Technical appears as card groups and simplified list/form views.
   - Missing: action breadcrumbs, nested menu context, debug items, cog/action menus.

6. Control panel/search is partial.
   - Target has facets, filters, group-by, favorites, pager, and view switch selectors.
   - Missing: full search arch parser behavior, saved favorites, custom filters/groups, date intervals, suggestions, group filter OR semantics, fetch-total pager behavior, action menu/cog menu integration.

7. List renderer is partial.
   - Reference list supports selection, row actions, sort, optional columns, column widths, grouped lists, decorations, keyboard flow, export/import/action menus, and desktop Enterprise patches.
   - Target list is table rendering plus basic grouping.

8. Form renderer is far from reference.
   - Reference form supports header buttons, smart buttons, statusbar, notebook tabs, modifiers, onchanges, relation widgets, x2many editors, chatter, image/avatar widgets, code editor behavior, and mobile field layout.
   - Target form is a generated field grid with save/discard and breadcrumb.

9. Apps install flow is unsafe/inaccurate.
   - Reference uses module actions and module state transitions.
   - Target directly writes `ir.module.module.state = installed`.
   - Required: call install/update methods, expose app catalog metadata, show real install/to-upgrade states, block invalid install actions.

10. Theme parity requires tokenized dark Enterprise mode.
    - Reference source contains enterprise dark/light SCSS layers for home menu, navbar, settings, list, search, and core components.
    - Target uses local CSS variables and light shell defaults in smoke captures.
    - Required: implement equivalent product tokens without copying proprietary SCSS/assets.

11. Mobile shell parity is missing.
    - Reference mobile home uses full dark launcher, compact systray, burger/back navigation, and mobile form density.
    - Target mobile uses clipped desktop navbar and desktop launcher spacing.

## Implementation Tasks

P0. Replace inline `/web` shell with bundled webclient runtime.
- Move UI behavior out of `internal/http/server.go`.
- Serve a compiled frontend bundle from `/web`.
- Keep server endpoints for session, assets, menus, actions, views, and RPC.
- Acceptance: `/web` loads without the inline shell; hash route supports `action`, `model`, `view_type`, `id`, `menu_id`, `debug`, and `cids`; action stack controls breadcrumbs and target modes.

P0. Build the action service/controller lifecycle.
- Implement action stack execution for `current`, `main`, and `new`.
- Add action container and dialog target handling.
- Wire route restore, breadcrumbs, close actions, and menu action loading.
- Acceptance: Server Actions list opens form and returns through breadcrumbs without duplicate panels or stale control panels.

P0. Implement Enterprise launcher/navbar/mobile shell.
- Normalize menu root app metadata and deduplicate apps.
- Add generated or licensed app icon set.
- Add systray dropdown services: messages, activities, company, debug, user.
- Add mobile burger/back navigation.
- Acceptance: desktop and mobile launcher match reference layout classes and state behavior; no duplicate Delegation tile.

P0. Implement real Settings view.
- Parse settings view architecture into app tabs, blocks, and settings.
- Support search, selected app tab, dirty state, save/discard, and confirmation dialog.
- Wire `res.config.settings` defaults and execute/save behavior.
- Acceptance: General Settings, Users & Companies, Technical, and Translations render as settings apps, not action cards.

P1. Complete search/control panel.
- Parse search views into filters, group-bys, favorites, fields, and date intervals.
- Add custom filters/groups and saved favorites.
- Implement pager fetch-total behavior.
- Add action/cog menu lanes.
- Acceptance: top-level filters, grouped filters, custom group-by, saved favorite activation, and pager next/previous match reference behavior.

P1. Complete list renderer.
- Add row selectors, bulk actions, sortable headers, optional columns, column width state, decoration classes, grouped rows, inline edit gates, and keyboard navigation.
- Acceptance: Server Actions list supports select-all, sort by Name/Model, active row open, grouped list, and stable pager.

P1. Complete form renderer.
- Implement form architecture rendering for sheets, groups, notebooks, headers, stat buttons, statusbars, modifiers, onchange/default handling, relation widgets, x2many widgets, binary/image fields, and code editor fields.
- Acceptance: Server Action form shows header action buttons, type-specific tabs/content, code editor field, allowed groups, pager, and mobile layout comparable to reference.

P1. Fix Apps install flow.
- Replace direct state write with module install/update methods.
- Add app metadata: display name, category, summary, icon token, dependency count, install state, upgrade state.
- Add search/category filters and install confirmation/progress.
- Acceptance: installed modules show disabled installed state; uninstalled modules run install method and refresh menu/action assets.

P2. Tokenize Enterprise theme.
- Create local theme tokens for navbar, home menu, settings, list, form, search, dropdowns, buttons, and mobile.
- Keep assets provenance-safe.
- Acceptance: screenshot comparisons show dark Enterprise shell, spacing, and density within agreed tolerance.

P2. Add parity verification suite.
- Add browser checks for launcher, navbar dropdowns, settings, technical list/form, search menu, apps install, list interactions, form tabs, mobile launcher, and mobile form.
- Store future outputs under `reports/uiux/verification/`.
- Acceptance: local and production smoke runs emit selector counts and screenshots for each state.

## Notes

- No frontend code modified.
- No accounting modules modified.
- No proprietary source or asset content copied.
- No new screenshots created in this pass.
