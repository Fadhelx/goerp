# GoERP UI/UX Parity Audit: Odoo Enterprise 19

Date: 2026-06-21  
Workspace: `/Users/fadhelalqaidoom/Documents/gorp`  
Odoo reference roots:
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise`

Scope:
- Inspect GoERP local `/web`.
- Inspect GoERP production target `/web`.
- Run and inspect local Odoo 19 Community plus Enterprise `web_enterprise`.
- Compare admin and normal-user UX surfaces.
- Produce implementation tasks for making GoERP feel like Odoo Enterprise before later restyling.

Constraints followed:
- Odoo source and assets used as reference only.
- No proprietary Odoo source or asset content copied into this report.
- No secrets reproduced.
- No destructive git commands run.
- Report-only work.

## 1. Audit Result

GoERP has Odoo-compatible labels, routes, and several shell selectors, but it does not yet behave or feel like the Odoo Enterprise web client.

Current parity estimate: 25-35%.

Main reason: GoERP `/web` is a bespoke static shell with embedded HTML/CSS/JS in `internal/http/server.go`. Odoo Enterprise is an action-driven component web client with a real action stack, control panel, search model, view registry, field widgets, chatter, systray, responsive mobile mode, and app metadata.

Production result:
- `https://api.fadhelalqaidoomxyz.xyz/web`
- Result: `502 Bad Gateway`
- Screenshot: `reports/uiux/screenshots/goerp-production-web.png`

Local GoERP result:
- URL: `http://127.0.0.1:8071/web`
- Health: `200 {"status":"ok"}`
- Browser shell loaded.

Local Odoo result:
- URL: `http://127.0.0.1:8072/odoo`
- Source run succeeded.
- Temp database: `goerp_ui_audit_20260621`
- Installed modules for UI inspection: `base`, `web_enterprise`, `contacts`, `crm`
- Temp record created for statusbar/chatter inspection: `UI Audit Opportunity`

## 2. Evidence

### GoERP Screenshots

Desktop:
- `reports/uiux/screenshots/goerp-desktop-apps.png`
- `reports/uiux/screenshots/goerp-desktop-technical-list.png`
- `reports/uiux/screenshots/goerp-desktop-form.png`
- `reports/uiux/screenshots/goerp-desktop-apps-install.png`

Mobile width 390:
- `reports/uiux/screenshots/goerp-mobile-apps.png`
- `reports/uiux/screenshots/goerp-mobile-technical-list.png`
- `reports/uiux/screenshots/goerp-mobile-form.png`

Production:
- `reports/uiux/screenshots/goerp-production-web.png`

### Odoo Screenshots

Desktop:
- `reports/uiux/screenshots/odoo-initial-web.png`
- `reports/uiux/screenshots/odoo-desktop-apps.png`
- `reports/uiux/screenshots/odoo-desktop-apps-catalog.png`
- `reports/uiux/screenshots/odoo-desktop-settings-debug.png`
- `reports/uiux/screenshots/odoo-desktop-server-actions-list.png`
- `reports/uiux/screenshots/odoo-desktop-server-action-form.png`
- `reports/uiux/screenshots/odoo-desktop-contacts-list.png`
- `reports/uiux/screenshots/odoo-desktop-contact-form.png`
- `reports/uiux/screenshots/odoo-desktop-crm-pipeline.png`
- `reports/uiux/screenshots/odoo-desktop-crm-opportunity-form.png`

Mobile width 390:
- `reports/uiux/screenshots/odoo-mobile-home.png`
- `reports/uiux/screenshots/odoo-mobile-contacts-list.png`
- `reports/uiux/screenshots/odoo-mobile-contact-form.png`

## 3. What Already Matches Odoo

### Shell Identity

GoERP matches:
- Browser title is `Odoo`.
- Body uses `.o_web_client`.
- Main navbar uses `.o_main_navbar`.
- Main content uses `.o_action_manager`.
- App launcher route exists at `/web`.
- App cards exist.
- Settings and Apps entries exist.
- Forbidden visible strings were absent in the inspected shell.

Files:
- `internal/http/server.go`
- `internal/http/server_test.go`

### Admin Navigation

GoERP matches partially:
- Administrator user is present.
- Settings app exists.
- Technical menu is reachable from Settings.
- Technical categories exist.
- Server Actions list is reachable.
- Server Action form opens from the list.

Observed GoERP admin path:
- `/web`
- Settings
- Technical
- Actions
- Server Actions
- Open record form

### Module/App Install Surface

GoERP matches partially:
- Apps view exists.
- Module list loads.
- Install state displays.
- Install button state exists.

Current limitation:
- GoERP displays technical module names and raw install state.
- Odoo displays app catalog names, icons, categories, descriptions, activation buttons, module info, and pager/search state.

### Backend API Direction

GoERP has useful Odoo-compatible backend pieces:
- Web routes.
- Session bootstrap.
- Menu loading.
- Action loading.
- View retrieval.
- Basic list/form data loading.
- Asset manifest endpoint.
- Module list/install endpoints.

Files:
- `internal/http/server.go`
- `internal/runtime/bootstrap.go`
- `internal/base/data/technical_menus.xml`
- `internal/base/data/technical_views.xml`

### Frontend Package Direction

`frontend/packages/webclient/src/index.ts` contains partial Odoo-like concepts:
- Window action rendering.
- List rendering.
- Form rendering.
- Statusbar field rendering.
- Action menus.
- Export dialogs.
- User group widget.
- Odoo-compatible aliases.

Current limitation:
- This package is not the actual `/web` shell.
- It still exposes `Gorp`/debug-oriented development UI.
- The server shell in `internal/http/server.go` remains the inspected production UI.

### Normal User Access Direction

Normal-user behavior is partly covered by tests.

Evidence:
- `internal/runtime/bootstrap_test.go` verifies normal user menu filtering.
- Normal users should see business apps such as Approvals/Delegation.
- Normal users should not see Settings/Technical menus.

Browser limitation:
- No default non-admin browser credential/session was available during the audit.
- Normal-user browser UX was not inspected end to end.

## 4. Prioritized Gaps To 100% Odoo Enterprise Parity

### P0. Replace Static `/web` Shell With Real Web Client Runtime

Current:
- `/web` is generated by `webClientShellHTML` in `internal/http/server.go`.
- UI state is handled by embedded script.
- Action stack, router, services, view registry, search model, and controller lifecycle are absent or incomplete.

Odoo Enterprise behavior:
- Web client owns action execution.
- The action manager controls breadcrumbs, current controller, view switching, and navigation.
- Control panel, search model, menus, dialogs, notifications, and services are composed around actions.

Implementation target:
- `/web` should bootstrap a frontend app bundle.
- Server should provide session, menus, assets, action descriptors, view descriptors, and RPC endpoints.
- UI behavior should live in frontend packages, not embedded server HTML.

### P0. Enterprise Home Menu/App Launcher

Current GoERP:
- App cards use plain generated initials.
- Background and layout are not Odoo Enterprise-like.
- Search exists but lacks Odoo launcher behavior.
- Duplicate Delegation app appeared.
- No systray/stateful topbar integration.

Odoo Enterprise:
- Dark Enterprise home background.
- App icon grid with real app metadata.
- App search/filter.
- Desktop and mobile launcher modes.
- Systray remains available.
- User/company menus remain integrated.

Implementation target:
- Build provenance-safe Odoo-like app icons or generated icon set.
- Normalize app metadata.
- Deduplicate apps.
- Match launcher layout, spacing, focus, search behavior, keyboard behavior, and mobile mode.

### P0. Action Manager, Control Panel, Breadcrumbs, Search

Current GoERP:
- Breadcrumb is simple.
- Search is plain text.
- No filter facets.
- No Group By.
- No Favorites.
- No view switcher.
- No action dropdown.
- No cog menu.
- No pager parity.

Odoo Enterprise:
- Search bar with facets.
- Filter, Group By, Favorites menus.
- Pager with record range.
- View switcher.
- Action and cog menus.
- Breadcrumb/action stack integration.
- Mobile control panel variants.

Implementation target:
- Implement action service and action stack.
- Implement search model.
- Implement reusable control panel.
- Implement breadcrumbs and pager.
- Wire every list/form/kanban action through the same controller contract.

### P0. List View Renderer

Current GoERP:
- Basic table.
- No row selection checkboxes.
- No action menu when rows selected.
- No optional columns.
- No column resizing/reordering.
- No relational/activity widgets.
- No grouped lists.
- No inline editable behavior.

Odoo Enterprise:
- Dense list renderer.
- Row selection.
- Header controls.
- Optional fields.
- Sort indicators.
- Activity and many2one widgets.
- Grouped lists.
- Inline edit where enabled.
- Responsive mobile list cards.

Implementation target:
- Build a list controller/renderer pair.
- Support row selection and selection state.
- Support field widgets by type.
- Support list arch attributes.
- Support mobile card rendering.

### P0. Form View Renderer, Field Widgets, Chatter, Statusbar

Current GoERP:
- Server Action form renders generic fields.
- Many2one value can display as JSON-like array.
- No form sheet fidelity.
- No button box.
- No notebooks parity.
- No statusbar in inspected shell.
- No chatter in inspected shell.

Odoo Enterprise:
- Form sheet layout.
- Header buttons.
- Statusbar stages.
- Button boxes and smart buttons.
- Notebooks/tabs.
- Field widgets by type.
- Onchange/save/discard lifecycle.
- Chatter with Send message, Log note, Activities, followers, messages.

Implementation target:
- Build form compiler/renderer/controller.
- Implement field widget registry.
- Implement many2one display/name_get and selector behavior.
- Implement selection/statusbar widgets.
- Implement notebooks and groups.
- Implement chatter service and panel.

### P0. Apps Install Flow

Current GoERP:
- Apps list shows technical modules.
- Cards lack Odoo catalog hierarchy.
- No Odoo-like descriptions/icons/categories.
- No pager/search facet parity.
- No Activate/Learn More/Module Info parity.

Odoo Enterprise:
- Apps catalog is a full action view.
- Cards include name, icon, description, install status, Activate, Learn More, Module Info.
- Left categories and search filters exist.
- Pager and action search state exist.

Implementation target:
- Normalize module metadata into app catalog metadata.
- Add categories.
- Add generated/provenance-safe app icons.
- Rework Apps as a regular Odoo action with control panel.
- Implement install progress and post-install refresh.

### P0. Mobile Responsiveness

Current GoERP:
- At 390 px width, page overflows horizontally.
- Launcher overflow: `documentElement.scrollWidth > innerWidth`.
- Technical list overflow.
- Form overflow.
- Navbar/topbar collapse is incomplete.

Odoo Enterprise:
- No horizontal overflow at 390 px on inspected launcher/list/form screens.
- Mobile navbar condenses.
- Lists become mobile-friendly cards.
- Forms become single-column.
- Chatter remains accessible.
- Control panel becomes compact.

Implementation target:
- Define mobile webclient breakpoint behavior.
- Add mobile navbar/burger/menu.
- Add mobile control panel.
- Add mobile list-card renderer.
- Enforce no document-level horizontal overflow.

### P0. Systray and Topbar

Current GoERP:
- No real systray was detected.
- No company switcher.
- No Discuss/Activities counters.
- No debug indicator.
- User menu is minimal.

Odoo Enterprise:
- Systray items are always part of the webclient frame.
- User/company/debug/activity/discuss/calendar integrations appear as applicable.
- Mobile uses compact variants.

Implementation target:
- Implement systray registry.
- Implement user menu.
- Implement company selector placeholder/behavior.
- Implement debug menu in debug mode.
- Add bus-backed activity/message indicators when backend exists.

### P1. Settings and Technical Menus

Current GoERP:
- Settings/Technical exists.
- Technical menu is materially smaller and flatter.
- Settings form is not Odoo settings form.

Odoo Enterprise:
- Settings app has general settings form with app sections.
- Technical menu appears in debug/admin mode and contains deep categories.
- Menus are grouped and ordered consistently.

Implementation target:
- Expand menu/action metadata.
- Implement settings form layout.
- Gate Technical by groups/debug/admin state.
- Add missing technical actions as data/views become available.

### P1. URL, Router, and Action State

Current GoERP:
- URL/state behavior is custom.
- Record URLs do not mirror Odoo action semantics.

Odoo Enterprise:
- URL state carries app/action/model/view/record/debug information.
- Back/forward navigation works through action state.
- Breadcrumbs reflect action stack.

Implementation target:
- Add router service.
- Serialize action state into URL.
- Support record/list/action restoration on reload.

### P1. Dialogs, Notifications, Import/Export

Current GoERP:
- Dialog and notification behavior is incomplete.
- Export/import UI parity is not visible in inspected `/web`.

Odoo Enterprise:
- Dialogs, confirmation flows, notifications, import/export, and warning banners are integrated services.

Implementation target:
- Add dialog service.
- Add notification service.
- Add confirmation flows.
- Wire export/import to list selection and access rights.

### P1. Access Rights In UI

Current GoERP:
- Normal-user menu filtering exists in tests.
- Browser verification is missing.
- UI does not yet clearly adapt per action rights in inspected screens.

Odoo Enterprise:
- Buttons, menus, technical screens, and action entries reflect user groups/access rights.

Implementation target:
- Add reusable rights checks in frontend action/view controllers.
- Add browser-test session fixtures for admin and normal user.
- Verify Settings/Technical absence for normal users.

### P2. Visual Tokens and Fine Detail

Current GoERP:
- Visual rhythm is close enough for a scaffold, not close enough for Enterprise parity.
- Density, shadows, borders, field heights, table spacing, and form sheet proportions diverge.

Odoo Enterprise:
- Stable typography, gray scale, navbar height, control panel spacing, form sheet width, table density, and mobile breakpoints.

Implementation target:
- Move all shell styling into theme packages.
- Define Odoo-like tokens without copying proprietary assets.
- Remove one-off inline styling.
- Add screenshot comparison checks.

## 5. Concrete File-Level Implementation Tasks

### Server Web Shell

Files:
- `internal/http/server.go`
- `internal/http/server_test.go`

Tasks:
- Replace `webClientShellHTML` as the primary UI with a bundle bootstrap page.
- Keep `/web`, `/odoo`, and compatibility aliases.
- Serve frontend bundle assets from a stable asset route.
- Expose bootstrap JSON for session, user, companies, debug state, menus, and asset URLs.
- Keep server tests for Odoo selectors and forbidden strings.
- Add tests for systray, control panel, action manager, app launcher, mobile-safe markup, and no visible GoERP branding.

### Frontend Webclient Root

Files:
- `frontend/packages/webclient/src/index.ts`
- New: `frontend/packages/webclient/src/webclient/*`
- New: `frontend/packages/webclient/src/services/*`
- New: `frontend/packages/webclient/src/router/*`

Tasks:
- Replace development shell identity with Odoo-compatible webclient root.
- Implement service registry.
- Implement action service.
- Implement router service.
- Implement dialog and notification services.
- Implement user/session service.
- Ensure root emits `.o_web_client`, `.o_action_manager`, `.o_main_navbar`.

### Enterprise Home Menu

Files:
- New: `frontend/packages/webclient/src/home_menu/*`
- New: `frontend/packages/webclient/src/webclient/navbar/*`
- `frontend/themes/enterprise-like/src/theme.ts`
- `frontend/packages/theme-tokens/src/index.ts`
- `internal/runtime/bootstrap.go`
- `internal/base/data/*`

Tasks:
- Build Enterprise-like home menu component.
- Add app search behavior.
- Deduplicate app list.
- Add generated/provenance-safe app icons.
- Add app metadata fields: display name, category, sequence, icon token, install state.
- Add mobile home menu layout.
- Add navbar integration with systray and user menu.

### Control Panel and Search

Files:
- New: `frontend/packages/webclient/src/search/*`
- New: `frontend/packages/webclient/src/control_panel/*`
- Existing: `frontend/packages/webclient/src/index.ts`

Tasks:
- Implement control panel component.
- Implement breadcrumbs.
- Implement pager.
- Implement search bar.
- Implement filter, group-by, favorites menus.
- Implement action menu.
- Implement cog menu.
- Implement view switcher.
- Bind control panel to action controllers.

### List Views

Files:
- New: `frontend/packages/webclient/src/views/list/*`
- New: `frontend/packages/webclient/src/fields/*`
- Existing: `frontend/packages/webclient/src/index.ts`
- Existing tests in `frontend/packages/webclient/src/*.test.mjs`

Tasks:
- Split list renderer/controller out of monolithic `index.ts`.
- Support row selectors.
- Support action menu on selection.
- Support sortable columns.
- Support field widgets.
- Support grouped rows.
- Support optional fields.
- Support mobile list cards.
- Support pager integration.

### Form Views and Widgets

Files:
- New: `frontend/packages/webclient/src/views/form/*`
- New: `frontend/packages/webclient/src/views/form/statusbar/*`
- New: `frontend/packages/webclient/src/views/form/button_box/*`
- New: `frontend/packages/webclient/src/fields/*`
- `internal/http/server.go`

Tasks:
- Implement form controller, renderer, and arch compiler.
- Support groups, sheets, headers, notebooks, and buttons.
- Implement field widget registry.
- Implement many2one display values and search/select behavior.
- Implement selection/statusbar widget.
- Implement boolean, date, datetime, monetary, text, html, x2many, many2many tags, and activity widgets.
- Add onchange/save/discard flow.
- Add validation/error display.

### Chatter and Activity

Files:
- New: `frontend/packages/webclient/src/mail/*`
- New: `internal/mail/*` if missing
- `internal/http/server.go`

Tasks:
- Add backend endpoints for messages, followers, activities, and attachments.
- Add chatter panel for form views.
- Add Send message, Log note, Activity controls.
- Add empty-state and message list.
- Add activity systray integration.
- Add access-aware visibility.

### Apps Catalog

Files:
- `internal/http/server.go`
- `internal/runtime/bootstrap.go`
- `internal/base/data/*`
- New: `frontend/packages/webclient/src/apps/*`

Tasks:
- Convert Apps screen to a regular action/view.
- Add categories/sidebar.
- Add card metadata: functional name, summary, icon, status, category.
- Add Activate, Learn More, Module Info behavior.
- Add install progress state.
- Refresh menus after install.
- Add installed/not installed filters.

### Settings and Technical

Files:
- `internal/base/data/technical_menus.xml`
- `internal/base/data/technical_views.xml`
- `internal/runtime/bootstrap.go`
- New: `frontend/packages/webclient/src/settings/*`

Tasks:
- Implement Odoo-like settings form layout.
- Expand Technical menus by category.
- Gate Technical menu by debug/admin/groups.
- Add missing technical actions gradually.
- Add server action, scheduled action, access rights, record rules, views, menu items, models, fields, assets, and system parameters parity.

### Mobile

Files:
- `frontend/packages/webclient/src/webclient/mobile/*`
- `frontend/packages/webclient/src/views/list/*`
- `frontend/packages/webclient/src/views/form/*`
- `frontend/packages/theme-tokens/src/index.ts`
- `frontend/scripts/e2e.mjs`

Tasks:
- Add mobile webclient frame.
- Add mobile navbar and burger menu.
- Add compact control panel.
- Add mobile list-card renderer.
- Add single-column form layout.
- Add mobile chatter positioning.
- Add automated overflow checks at 390x844 and 430x932.

### Verification and Reports

Files:
- `frontend/scripts/e2e.mjs`
- `reports/uiux/*`
- `internal/http/server_test.go`

Tasks:
- Add screenshot capture flows for admin desktop.
- Add screenshot capture flows for admin mobile.
- Add screenshot capture flows for normal user.
- Add selector assertions for Odoo class names.
- Add no-horizontal-overflow assertions.
- Add app install flow assertion.
- Add list/form/chatter/statusbar assertions.

## 6. Suggested Subagent Lanes With Disjoint Write Scopes

### Lane A: Web Shell, Home Menu, Navbar

Write scope:
- `internal/http/server.go`
- `internal/http/server_test.go`
- `frontend/packages/webclient/src/webclient/**`
- `frontend/packages/webclient/src/home_menu/**`
- `frontend/packages/webclient/src/webclient/navbar/**`
- `frontend/themes/**`
- `frontend/packages/theme-tokens/**`

Deliverables:
- Bundle-backed `/web`.
- Enterprise-like app launcher.
- Navbar and systray frame.
- Desktop/mobile shell tests.

### Lane B: Action Manager, Control Panel, Search

Write scope:
- `frontend/packages/webclient/src/services/**`
- `frontend/packages/webclient/src/router/**`
- `frontend/packages/webclient/src/control_panel/**`
- `frontend/packages/webclient/src/search/**`
- `frontend/packages/webclient/src/index.ts` integration only

Deliverables:
- Action stack.
- Breadcrumbs.
- Pager.
- Search facets.
- Filter/group/favorite menus.
- View switcher.

### Lane C: List Views

Write scope:
- `frontend/packages/webclient/src/views/list/**`
- `frontend/packages/webclient/src/fields/**`
- `frontend/packages/webclient/src/index.ts` exports only
- `frontend/packages/webclient/src/*list*.test.mjs`

Deliverables:
- Odoo-like list renderer.
- Selection/action/cog behavior.
- Field widget display.
- Mobile list cards.

### Lane D: Form Views and Field Widgets

Write scope:
- `frontend/packages/webclient/src/views/form/**`
- `frontend/packages/webclient/src/fields/**`
- `frontend/packages/webclient/src/*form*.test.mjs`
- Related `internal/http/server.go` form RPC endpoints only

Deliverables:
- Form sheet.
- Header buttons.
- Statusbar.
- Notebooks.
- Field widget registry.
- Save/discard/onchange flow.

### Lane E: Chatter, Mail, Activity

Write scope:
- `frontend/packages/webclient/src/mail/**`
- `internal/mail/**`
- Related `internal/http/server.go` mail/chatter endpoints only

Deliverables:
- Chatter panel.
- Messages.
- Log note.
- Activities.
- Followers.
- Activity systray.

### Lane F: Apps, Settings, Technical

Write scope:
- `internal/base/data/**`
- `internal/runtime/bootstrap.go`
- `frontend/packages/webclient/src/apps/**`
- `frontend/packages/webclient/src/settings/**`
- Settings/apps tests only

Deliverables:
- Apps catalog parity.
- Settings form.
- Technical menu/action expansion.
- Debug/admin gating.

### Lane G: Verification

Write scope:
- `frontend/scripts/e2e.mjs`
- `reports/uiux/**`
- Browser/e2e test files only

Deliverables:
- Repeatable screenshot suite.
- Desktop/mobile parity checklist.
- Normal-user/admin route checks.
- Production smoke check.

## 7. Reference Source Map

Use these only as behavioral reference. Do not copy source or assets.

Enterprise home/navbar:
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise/web_enterprise/views/webclient_templates.xml`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise/web_enterprise/static/src/webclient/home_menu/`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise/web_enterprise/static/src/webclient/navbar/`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise/web_enterprise/static/src/webclient/burger_menu/`

Core webclient:
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/web/static/src/webclient/`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/web/static/src/webclient/actions/`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/web/static/src/webclient/user_menu/`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/web/static/src/webclient/switch_company_menu/`

Search/control panel:
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/web/static/src/search/`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/web/static/src/search/control_panel/`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/web/static/src/search/breadcrumbs/`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/web/static/src/search/action_menus/`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/web/static/src/search/cog_menu/`

Views:
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/web/static/src/views/list/`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise/web_enterprise/static/src/views/list/`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/web/static/src/views/form/`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/web/static/src/views/kanban/`

Settings:
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/web/static/src/webclient/settings_form_view/`

Chatter/mail/activity:
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/mail/static/src/chatter/`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/mail/static/src/core/web/`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/mail/static/src/core/common/`

Reference tests:
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise/web_enterprise/static/tests/`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/web/static/tests/`

## 8. Verification Checklist

### Start GoERP

Command:

```sh
GORP_HTTP_ADDR=127.0.0.1:8071 go run ./cmd/gorpd serve
```

Checks:
- `curl http://127.0.0.1:8071/web/health`
- `curl "http://127.0.0.1:8071/web/assets/manifest?bundle=web.assets_backend&debug=assets"`
- Open `http://127.0.0.1:8071/web`.

### Start Odoo Reference

Init command used:

```sh
cd /Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo
./odoo-bin server --addons-path=/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons,/Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise -d goerp_ui_audit_20260621 -i base,web_enterprise --without-demo=all --http-interface=127.0.0.1 --http-port=8072 --stop-after-init --log-level=warn
```

Run command used:

```sh
cd /Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo
./odoo-bin server --addons-path=/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons,/Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise -d goerp_ui_audit_20260621 --db-filter=^goerp_ui_audit_20260621$ --http-interface=127.0.0.1 --http-port=8072 --log-level=warn
```

Login:
- URL: `http://127.0.0.1:8072/web`
- User: `admin`
- Credential: local temp Odoo database admin credential.

Note:
- Odoo 19 emitted a warning that `--without-demo=all` is treated as boolean true.
- Run succeeded.

### Desktop Admin Checks

Required GoERP URLs/screens:
- `/web` home launcher.
- Apps catalog/install flow.
- Settings.
- Settings -> Technical.
- Technical -> Server Actions list.
- Server Action form.
- A business record list.
- A business record form.

Assertions:
- Title is `Odoo`.
- `.o_web_client` exists.
- `.o_main_navbar` exists.
- `.o_action_manager` exists.
- Control panel exists.
- Breadcrumbs reflect action stack.
- Search facets render.
- Filter, Group By, Favorites menus render.
- Pager renders range.
- View switcher renders.
- Row selection works.
- Form sheet renders.
- Header buttons render.
- Statusbar renders when field exists.
- Chatter renders when model supports chatter.
- Systray renders.
- No visible GoERP/Gorp branding in parity mode.

### Mobile Checks

Viewports:
- `390x844`
- `430x932`

Required screens:
- Home launcher.
- Apps catalog.
- Technical list.
- Record list.
- Record form with chatter.

Assertions:
- `document.documentElement.scrollWidth <= window.innerWidth`
- Navbar remains usable.
- App grid fits.
- List uses mobile layout or fits without page-level overflow.
- Form is single-column.
- Chatter is reachable.
- Search/control panel has mobile layout.

### Normal User Checks

Required setup:
- Browser-login-capable non-admin user.
- Session fixture or seeded credential.

Assertions:
- Normal user can open `/web`.
- Business apps display.
- Approvals/Delegation display if assigned.
- Settings does not display.
- Technical does not display.
- Admin-only menus/actions do not display.
- Record create/edit buttons respect access rights.

Current status:
- Backend tests cover menu filtering.
- Browser verification is still required.

### Apps Install Checks

Assertions:
- Apps view is action-based.
- App catalog names are functional names, not raw module names.
- Categories/sidebar exist.
- Search/filter state exists.
- Activate/Install state exists.
- Install progress is visible.
- Menus refresh after install.
- Installed app opens from launcher.

### Test Commands

Targeted:

```sh
go test ./internal/http
go test ./internal/runtime -run TestBootstrapOIExposesHTTPModulesAssetsMenusAndViews
pnpm -C frontend test:e2e
```

Required before reporting implementation complete:

```sh
make ci
```

Status for this audit:
- `make ci` was not run because this was report-only audit work, not implementation completion.

## 9. Exact Blockers and Caveats

Production:
- `https://api.fadhelalqaidoomxyz.xyz/web` returned `502 Bad Gateway`.

GoERP local:
- Port `8069` was already occupied by another `gorpd` process.
- Audit used `127.0.0.1:8071`.

Odoo local:
- Odoo 19 source launched successfully.
- No runtime blocker remained.
- Temp database and temp records were used only for UI inspection.

Normal user browser inspection:
- Blocked by missing default non-admin browser credential/session.
- Existing tests verify part of the behavior.
- Add seeded normal-user fixture before next browser pass.

Licensing/provenance:
- Do not copy Odoo Enterprise icons, images, SCSS, XML templates, JS, or screenshots into product code.
- Use generated or independently created assets.
- Use Odoo source paths only to study behavior and structure.

## 10. Recommended Execution Order

1. Replace `/web` static shell with bundle-backed frontend root.
2. Build action service, router, control panel, and search model.
3. Build Enterprise-like home menu and navbar/systray.
4. Build list/form renderers and field widget registry.
5. Build chatter/activity/statusbar behavior.
6. Convert Apps and Settings to action-based views.
7. Add mobile webclient behavior and no-overflow tests.
8. Add normal-user browser fixture and access-rights checks.
9. Run screenshot parity suite against Odoo reference.
10. Run `make ci`.
