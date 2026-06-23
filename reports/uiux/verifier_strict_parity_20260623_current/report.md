# Strict UI/UX Parity Verification

Date: 2026-06-23
Workspace: `/Users/fadhelalqaidoom/Documents/gorp`
Role: independent verifier
Status: FAIL

## Evidence

- GoERP current screenshots: `reports/uiux/verifier_strict_parity_20260623_current/goerp/`
- Odoo reference screenshots: `reports/uiux/verifier_strict_parity_20260623_continued/odoo/`
- Side-by-side comparisons: `reports/uiux/verifier_strict_parity_20260623_current/comparisons/`
- Pixel metrics: `reports/uiux/verifier_strict_parity_20260623_current/metrics/image-pixel-metrics.json`
- Evidence manifest: `reports/uiux/verifier_strict_parity_20260623_current/evidence-manifest.json`

## Checks Run

- Started GoERP local server: `GORP_HTTP_ADDR=127.0.0.1:8123 go run ./cmd/gorpd serve`
- Ran smoke harness:
  - `default-webclient-takeover`
  - `default-apps-install-desktop`
  - `default-apps-catalog-detail-desktop`
  - `settings-desktop`
  - `default-navbar-nested-menus-desktop`
  - `default-navbar-technical-dropdown-open-desktop`
  - `default-technical-search-desktop`
  - `default-technical-form-desktop`
  - `default-relation-dropdown-desktop`
  - `default-scheduled-action-form-desktop`
  - `default-users-flow-desktop`
  - `default-groups-form-notebook-desktop`
  - `default-mobile-launcher-parity`
  - `default-mobile-server-actions-flow`
- Captured direct browser evidence:
  - `current-apps-launcher-click.png`
  - `current-scheduled-actions-list.png`
  - `current-users-list.png`
  - `current-groups-list.png`
- Computed screenshot deltas with same-size PNG comparisons.
- Inspected side-by-side evidence only.

## Pass/Fail Table

| Screen | Result | Evidence | Pixel delta | Exact visible mismatch |
|---|---:|---|---:|---|
| Launcher | FAIL | GoERP `goerp/default-webclient-takeover.png`; Odoo `odoo/01-launcher-topbar.png`; comparison `comparisons/launcher.png` | 99.96% | App count now matches 2, but background gradient/glow differs. GoERP topbar lacks Odoo database badge placement and Odoo icon set. |
| Apps | FAIL | GoERP `goerp/current-apps-launcher-click.png`; Odoo `odoo/03-apps.png`; comparison `comparisons/apps.png` | 100.00% | GoERP shows `1-26 / 26`; Odoo shows `1-77 / 77`. GoERP uses module cards with generic icons and `Upgrade/Uninstall/Module Info`; Odoo uses colored app cards with `Activate/Learn More`, app top actions, and different category counts. |
| Settings | FAIL | GoERP `goerp/settings-desktop.png`; Odoo `odoo/04-settings.png`; comparison `comparisons/settings.png` | 100.00% | GoERP renders action-link grids. Odoo renders native Settings sections with left app sidebar, `Invite New Users`, `1 Active User`, Languages, Companies, Document Layout, and setting rows. |
| Settings > Technical nested menu | FAIL | GoERP `goerp/default-navbar-technical-dropdown-open-desktop.png`; Odoo `odoo/05-settings-technical-dropdown.png`; comparison `comparisons/technical_dropdown.png` | 100.00% | GoERP starts with `Actions`; Odoo starts with `Email`. GoERP order and labels differ: missing/renamed visible labels include `IAP Accounts`, `Tours`, `Fields Selection`, `ManyToMany Relations`, `Paper Format`. GoERP includes extras such as `Automation Rules`, `Scheduled Messages`, `Apps`. |
| Server Actions list | FAIL | GoERP `goerp/default-technical-search-desktop.png`; Odoo `odoo/06-server-actions-list.png`; comparison `comparisons/server_actions_list.png` | 100.00% | GoERP shows `1-20 / 20`; Odoo shows `1-7 / 7`. GoERP lacks `Top-level actions` filter chip. Record set and Model cells differ; first GoERP rows have blank Model cells where Odoo shows model names. |
| Server Action form | FAIL | GoERP `goerp/default-technical-form-desktop.png`; Odoo `odoo/07-server-action-form.png`; comparison `comparisons/server_action_form.png` | 99.99% | GoERP opens `Mail: Email Queue Manager`; Odoo opens `Base: Auto-vacuum internal data`. GoERP form has 5 visible field widgets; Odoo DOM evidence has 13 fields. Header controls, smart button placement, contextual button placement, and code content differ. |
| Scheduled Actions list | FAIL | GoERP `goerp/current-scheduled-actions-list.png`; Odoo `odoo/08-scheduled-actions-list.png`; comparison `comparisons/scheduled_actions_list.png` | 100.00% | GoERP shows `1-9 / 9`; Odoo shows `1-2 / 2`. GoERP columns are `Name, Active, Repeat Every, Interval Unit, Next Execution Date, Action Type`; Odoo columns are `Priority, Action Name, Model, Next Execution Date, Interval, Interval Unit, Active`. GoERP dates include `1970-01-01`; Odoo dates are `Jun 24, 1:02 AM`. |
| Scheduled Action form | FAIL | GoERP `goerp/default-scheduled-action-form-desktop.png`; Odoo `odoo/09-scheduled-action-form.png`; comparison `comparisons/scheduled_action_form.png` | 100.00% | GoERP opens `Mail: Email Queue Manager`; Odoo opens `Base: Auto-vacuum internal data`. GoERP lacks Odoo `Run Manually` button, `Allowed Groups`, `Priority`, Odoo active toggle placement, and Odoo compact code block sizing. |
| Many2one relation dropdown | FAIL | GoERP `goerp/default-relation-dropdown-desktop.png`; Odoo `odoo/10-relation-many2one-dropdown.png`; comparison `comparisons/relation_dropdown.png` | 99.99% | GoERP typed `mail` dropdown shows 8 options: `Incoming Mail Server`, `Mail Server`, `Activity`, `Mixin`, `Type`, `Alias`, `Domain`, `Blacklist`, plus `Search more...`. Odoo shows only `Mail Server` plus `Search more...`. GoERP state is an existing filled form; Odoo state is a new form with `Set an explicit name`. |
| Users list | FAIL | GoERP `goerp/current-users-list.png`; Odoo `odoo/11-users-list.png`; comparison `comparisons/users_list.png` | 100.00% | GoERP columns are `Name, Login, Email, Company, Groups, Active`. Odoo columns are `Name, Login, Role` with avatar and `Internal Users` filter chip. |
| Users access form | FAIL | GoERP `goerp/default-users-flow-desktop.png`; Odoo `odoo/12-users-access-form.png`; comparison `comparisons/users_access_form.png` | 100.00% | GoERP lacks Odoo avatar identity block, Related Partner field placement, smart buttons for Groups/Access Rights/Record Rules, and role radio layout. GoERP shows raw role `group_system` and only `Access Rights/Preferences`; Odoo shows `Access Rights/Preferences/Calendar/Security` plus grouped rights sections. |
| Groups list | FAIL | GoERP `goerp/current-groups-list.png`; Odoo `odoo/13-groups-list.png`; comparison `comparisons/groups_list.png` | 100.00% | GoERP shows `1-34 / 34` and columns `Full Name, Name, Category, Privilege, Share`. Odoo shows `1-13 / 13` and columns `Privilege, Name` with `Internal Groups` filter chip. |
| Groups access form | FAIL | GoERP `goerp/default-groups-form-notebook-desktop.png`; Odoo `odoo/14-groups-access-form.png`; comparison `comparisons/groups_access_form.png` | 100.00% | GoERP tabs are `Inherited, Users, Access, Comment, Inherited By`. Odoo tabs are `Users, Inherited, Menus, Views, Access Rights, Record Rules, Notes`. GoERP lacks Odoo users smart button and access-rule notebook structure. |
| Mobile launcher | FAIL | GoERP `goerp/default-mobile-launcher-parity.png`; Odoo `odoo/15-mobile-launcher.png`; comparison `comparisons/mobile_launcher.png` | 99.99% | App count now matches 2, but topbar icons, avatar/user treatment, background gradient/glow, banner wrapping, and tile spacing differ. |
| Mobile form | FAIL | GoERP `goerp/default-mobile-server-actions-flow.png`; Odoo `odoo/16-mobile-form.png`; comparison `comparisons/mobile_form.png` | 99.82% | GoERP uses a card-like mobile form with stacked action buttons and disabled scheduled action control. Odoo uses compact native mobile chrome with breadcrumb/header controls, `New`, `Create Contextual Action`, full-width fields, and smaller code notebook density. |

## P0 Tasks

1. Apps catalog parity.
   - Match Odoo app dataset volume, categories, colored icon treatment, top actions, pager, and card actions.
   - Current blocker: GoERP `26` module cards vs Odoo `77` app cards.

2. Users/Groups access parity.
   - Implement Odoo-style users form identity block, smart buttons, access tabs, grouped rights widgets, and groups notebook tabs: `Menus`, `Views`, `Access Rights`, `Record Rules`, `Notes`.
   - Current blocker: GoERP exposes reduced access forms and raw/group-derived widgets.

3. Settings and Technical menu parity.
   - Rebuild Settings into Odoo native section layout.
   - Match Technical dropdown exact visible order and labels, including `Email` first, `IAP Accounts`, `Tours`, `Fields Selection`, `ManyToMany Relations`, and singular `Paper Format`.

## P1 Tasks

1. Server Actions and Scheduled Actions data/layout parity.
   - Match reference record counts, default filters, list columns, form records, header buttons, smart buttons, dates, and code block density.

2. Many2one dropdown exact parity.
   - Match Odoo new-form state and filtered autocomplete cardinality for `mail`: only `Mail Server` plus `Search more...`.

3. Mobile chrome parity.
   - Match Odoo mobile launcher topbar/background spacing and Odoo mobile form header, field density, action placement, and notebook sizing.

## Notes

- Source files were not edited.
- Odoo source/assets were not copied.
- Generated artifacts contain screenshots, metrics, and sanitized DOM observations only.
- `make ci` was not run because this was read-only verification, not implementation completion.
