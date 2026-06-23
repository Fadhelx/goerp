# Independent UI Parity Verification After Implementation Changes

Date: 2026-06-23 08:04 +03
Workspace: `/Users/fadhelalqaidoom/Documents/gorp`
Role: read-only independent verifier
Status: FAIL

Pass/fail counts: required flows `0 PASS / 13 FAIL`; supplemental mobile flows `0 PASS / 2 FAIL`; total `0 PASS / 15 FAIL`.

## Source State

- Prior report checked: `reports/uiux/independent_ui_parity_20260623_063912/report.md`.
- UI source changes were newer than prior report, so verification proceeded.
- HEAD changed during the first capture. GoERP was restarted and recaptured after source stabilization.
- Final verified HEAD: `37da10e913bac044ebc879f2461e456c02809256`.
- Final UI dirty diff SHA-256: `9c624898bd4a1d87e5d19071ca33302be16ff20be53b054c623e82284ae7ebf3`.
- Stable polls: `07:59:49`, `08:00:19`, `08:00:49` +03 with same HEAD and diff hash.
- Source files were not edited, staged, patched, or reverted by this verifier.

## Evidence Paths

- Report directory: `reports/uiux/independent_ui_parity_after_change_20260623_074623/`
- Odoo screenshots/DOM: `reports/uiux/independent_ui_parity_after_change_20260623_074623/odoo/`
- GoERP screenshots/DOM: `reports/uiux/independent_ui_parity_after_change_20260623_074623/goerp/`
- Side-by-side comparisons: `reports/uiux/independent_ui_parity_after_change_20260623_074623/comparisons/`
- Pixel metrics: `reports/uiux/independent_ui_parity_after_change_20260623_074623/metrics/image-pixel-metrics.json`
- Manifest: `reports/uiux/independent_ui_parity_after_change_20260623_074623/evidence-manifest.json`

## Commands Run

- `git rev-parse HEAD`
- `git diff -- frontend internal/http/server.go internal/base/data/technical_views.xml tools/web_visual_smoke/run.mjs | shasum -a 256`
- `find frontend internal/http tools -type f (...) -newer reports/uiux/independent_ui_parity_20260623_063912/report.md -print`
- `python3 /Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo-bin --version`
- `python3 /Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo-bin --addons-path=/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons,/Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise -d gorp_ref_ui_20260623_074623 -i base,web_enterprise --without-demo=all --http-interface=127.0.0.1 --http-port=8145 --gevent-port=8148 --no-database-list --workers=0 --max-cron-threads=0 --data-dir=/tmp/gorp_odoo_ref_data_20260623_074623 --stop-after-init --log-level=warn`
- `python3 /Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo-bin --addons-path=/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons,/Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise -d gorp_ref_ui_20260623_074623 --http-interface=127.0.0.1 --http-port=8145 --gevent-port=8148 --no-database-list --workers=0 --max-cron-threads=0 --data-dir=/tmp/gorp_odoo_ref_data_20260623_074623 --log-level=warn`
- `GORP_HTTP_ADDR=127.0.0.1:8143 go run ./cmd/gorpd serve`
- `source stability poll: 3 polls at 30s cadence, final head 37da10e913bac044ebc879f2461e456c02809256 and UI diff hash 9c624898bd4a1d87e5d19071ca33302be16ff20be53b054c623e82284ae7ebf3`
- `Restarted GoERP after HEAD changed, then recaptured GoERP screenshots from the restarted server`
- `Browser Plugin / Node REPL screenshot and DOM capture, desktop 1366x900 and mobile 390x844`
- `Python Pillow comparison metric script`
- `targeted secret-pattern scan over generated text evidence; no matches`

Authentication field values are omitted from this report.

## Pass/Fail Matrix

| Flow | Result | Evidence | Pixel delta | Exact visible mismatch |
|---|---:|---|---:|---|
| Launcher/menu grid | FAIL | `odoo/01-launcher.png, goerp/01-launcher.png, comparisons/launcher.png` | 99.9990% | GoERP still uses cube fallback icons; Odoo uses official colored Apps/Settings icons. GoERP topbar shows extra systray/messages/activity treatment and a different user/database block. Normal user would see different app icons and chrome. |
| Apps catalog | FAIL | `odoo/02-apps.png, goerp/02-apps.png, comparisons/apps.png` | 99.4161% | Counts match at 1-77 / 77, but card art and module copy differ. GoERP exposes technical module names on cards, left filter spacing differs, and search/chip controls do not match Odoo geometry. |
| Settings main page | FAIL | `odoo/03-settings.png, goerp/03-settings.png, comparisons/settings.png` | 99.9430% | GoERP section grid and card geometry differ. Odoo has teal action links, exact left app icon row, wider centered search, and different visible section ordering; GoERP shows mauve arrow links and a different two-column layout. |
| Settings > Technical dropdown | FAIL | `odoo/04-technical-dropdown.png, goerp/04-technical-dropdown.png, comparisons/technical_dropdown.png` | 99.9608% | Dropdown opens, but visible text/geometry are not strict Odoo: GoERP pluralizes items such as Outgoing Mail Servers, Reports, Window Actions, Client Actions, Server Actions; Odoo reference uses singular labels. Visible dropdown height/anchor and background settings layout differ. |
| Server Actions list | FAIL | `odoo/05-server-actions-list.png, goerp/05-server-actions-list.png, comparisons/server_actions_list.png` | 99.8702% | Odoo shows 1-7 / 7 with Top-level actions chip. GoERP shows 1-20 / 20, no chip, extra AI/workflow rows, and different action/view controls. |
| Server Action form | FAIL | `odoo/06-server-action-form.png, goerp/06-server-action-form.png, comparisons/server_action_form.png` | 99.9884% | Odoo new form shows Set an explicit name plus Model and Allowed Groups only. GoERP shows Name input, Model, Type pill grid, Active, Code/Help tabs, and a large code editor. |
| Relation dropdown/autocomplete | FAIL | `odoo/07-relation-dropdown.png, goerp/07-relation-dropdown.png, comparisons/relation_dropdown.png` | 99.9845% | Both dropdowns show Mail Server and Search more..., but GoERP field is in a different form layout with different width, position, labels, and surrounding controls. Autocomplete result text alone is not enough for parity. |
| Scheduled Actions list | FAIL | `odoo/08-scheduled-actions-list.png, goerp/08-scheduled-actions-list.png, comparisons/scheduled_actions_list.png` | 99.9198% | Both show two records, but Odoo has All search chip, checkbox active rendering, Odoo column label Interval Number, and calendar view icon. GoERP lacks chip and renders Active as text true. |
| Scheduled Action form | FAIL | `odoo/09-scheduled-action-form.png, goerp/09-scheduled-action-form.png, comparisons/scheduled_action_form.png` | 99.9651% | Odoo form fields are Model, Allowed Groups, Scheduler User, Execute Every, Active toggle, Next Execution Date, Priority. GoERP uses Run As, Interval, pill-style interval unit buttons, text active value, different date, and different sheet geometry. |
| Users list | FAIL | `odoo/10-users-list.png, goerp/10-users-list.png, comparisons/users_list.png` | 99.7538% | One user row matches at data level, but GoERP lacks Odoo Internal Users search chip, checkbox/avatar column geometry, topbar/breadcrumb layout, and exact list controls. |
| Users form | FAIL | `odoo/11-users-form.png, goerp/11-users-form.png, comparisons/users_form.png` | 99.8770% | Odoo Access Rights tab shows Roles, Master Data, and Extra Rights in Odoo two-column layout. GoERP shows a boxed access matrix with many unrelated access rows and different avatar/smart-button placement. |
| Groups list | FAIL | `odoo/12-groups-list.png, goerp/12-groups-list.png, comparisons/groups_list.png` | 99.5163% | Rows align, but GoERP lacks Odoo Internal Groups search chip, checkbox column, sort indicator/pager treatment, and exact top control panel. |
| Groups form | FAIL | `odoo/13-groups-form.png, goerp/13-groups-form.png, comparisons/groups_form.png` | 99.9412% | Odoo form is full-width with field labels, smart Users button, Users tab grid, and Add a line. GoERP is a compact centered card and does not render the Odoo x2many list grid. |
| Mobile launcher supplemental | FAIL | `odoo/14-mobile-launcher.png, goerp/14-mobile-launcher.png, comparisons/mobile_launcher.png` | 99.9991% | GoERP mobile keeps cube fallback icons and different icon positions. Odoo mobile shows official app icons and different top/user chrome. |
| Mobile users form supplemental | FAIL | `odoo/15-mobile-users-form.png, goerp/15-mobile-users-form.png, comparisons/mobile_users_form.png` | 99.8688% | Odoo mobile user form retains Odoo role/master/extra-rights structure. GoERP mobile keeps the boxed access matrix structure and different tabs/chrome. |

## Blocking Gaps

### P0

1. Replace fallback cube launcher/app icons with Odoo-like visual icon system without copying proprietary assets. Launcher and Apps catalog fail immediately on icon parity.
2. Rebuild Odoo form sheets for Server Actions, Scheduled Actions, Users, and Groups. Current forms use different field groupings, density, smart buttons, and x2many/list structures.
3. Add exact Odoo search facets/control-panel behavior. Required chips missing: Top-level actions, All, Internal Users, Internal Groups.
4. Fix Users/Groups access screens. Users must show Roles, Master Data, Extra Rights like Odoo; Groups must show full-width tabs with x2many grids and Add a line.

### P1

1. Correct Technical dropdown labels, singular/plural text, width, anchor, and visible menu viewport to match Odoo.
2. Align Settings page row geometry, action link styling, search width, left app icon row, and visible section ordering.
3. Align list views: checkbox columns, sort indicators, active checkbox rendering, view switch icons, pager placement, and row datasets.
4. Align relation dropdown geometry. Result labels match, but field context and dropdown size/position do not.

### P2

1. Mobile launcher and mobile user form still fail icon, chrome, and access-form parity.
2. Full-screen pixel deltas remain 99.4%-100.0% on all required flows.

## Normal User Verdict

No. GoERP would not look like original Odoo 19 Enterprise to a normal user. Required flows remain strict FAIL despite some closer menu labels and list data.
