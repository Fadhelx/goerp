# Independent UI Parity Verification

Date: 2026-06-23 06:39 +03
Workspace: `/Users/fadhelalqaidoom/Documents/gorp`
Role: read-only verifier
Status: FAIL

## Evidence Paths

- GoERP screenshots: `reports/uiux/independent_ui_parity_20260623_063912/goerp/`
- Fresh Odoo screenshots: `reports/uiux/independent_ui_parity_20260623_063912/odoo/`
- Side-by-side comparisons: `reports/uiux/independent_ui_parity_20260623_063912/comparisons/`
- Pixel metrics: `reports/uiux/independent_ui_parity_20260623_063912/metrics/image-pixel-metrics.json`
- Evidence manifest: `reports/uiux/independent_ui_parity_20260623_063912/evidence-manifest.json`
- Groups Odoo fallback screenshots: `reports/uiux/verifier_strict_parity_20260623_continued/odoo/13-groups-list.png`, `reports/uiux/verifier_strict_parity_20260623_continued/odoo/14-groups-access-form.png`

## Commands Run

- `python3 /Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo-bin --version`
- `python3 /Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo-bin --addons-path=/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons,/Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise -d gorp_ref_ui_20260623_063912 -i base,web_enterprise --without-demo=all --http-interface=127.0.0.1 --http-port=8135 --gevent-port=8138 --no-database-list --workers=0 --max-cron-threads=0 --data-dir=/tmp/gorp_odoo_ref_data_20260623_063912 --stop-after-init --log-level=warn`
- `python3 /Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo-bin --addons-path=/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons,/Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise -d gorp_ref_ui_20260623_063912 --http-interface=127.0.0.1 --http-port=8135 --gevent-port=8138 --no-database-list --workers=0 --max-cron-threads=0 --data-dir=/tmp/gorp_odoo_ref_data_20260623_063912 --log-level=warn`
- `GORP_HTTP_ADDR=127.0.0.1:8133 go run ./cmd/gorpd serve`
- Independent Chrome DevTools capture scripts for Odoo and GoERP.
- Pillow image metric/comparison script.

Authentication payloads are omitted from this report.

## Capture Notes

- Odoo version: `Odoo Server 19.0`.
- Fresh Odoo reference DB initialized and captured on `127.0.0.1:8135`.
- GoERP captured on `127.0.0.1:8133`.
- Fresh Odoo minimal DB exposed `Settings`, `Apps`, and `Users`; it did not expose a `Groups` menu. Groups comparison uses the prior captured Odoo reference screenshots listed above.
- Odoo relation dropdown first capture failed with CDP error `Object reference chain is too long`; retry succeeded and produced `odoo/07-relation-dropdown.png`.
- Source files were not edited. Nothing was staged.

## Pass/Fail Matrix

| Flow | Result | Evidence | Delta | Exact mismatch |
|---|---:|---|---:|---|
| App launcher/menu grid | FAIL | `goerp/01-launcher.png`, `odoo/01-launcher.png`, `comparisons/launcher.png` | 99.97% | App count matches. Icons do not match. GoERP uses cube fallback icons; Odoo uses colored app icons. Topbar lacks Odoo database badge placement and exact systray/icon treatment. Background gradient/glow placement differs. |
| Apps install/update catalog | FAIL | `goerp/02-apps.png`, `odoo/02-apps.png`, `comparisons/apps.png` | 100.00% | Both show `1-77 / 77`, but icons/artwork differ across cards. GoERP search chip/search input placement differs. Category counts/labels differ. Topbar user/database treatment differs. Card density and left panel spacing do not match Odoo. |
| Settings main page | FAIL | `goerp/03-settings.png`, `odoo/03-settings.png`, `comparisons/settings.png` | 100.00% | GoERP approximates sections but lacks Odoo row icons and teal action links. Search field width/placement differs. Document Layout actions differ. Topbar identity/database badge differs. |
| Settings > Technical nested dropdown | FAIL | `goerp/04-technical-dropdown.png`, `odoo/04-technical-dropdown.png`, `comparisons/technical_dropdown.png` | 100.00% | GoERP dropdown starts at `Actions`; Odoo starts at `Email`. GoERP has 55 dropdown items; Odoo has 35. GoERP includes extra `Users`, `Groups`, `Companies`, `Languages`, `Automation Rules`, `Apps`; Odoo includes `IAP Accounts`, `Tours`, `Fields Selection`, `ManyToMany Relations`, `Paper Format`. Width, order, grouping, and nesting fail. |
| Server Actions list | FAIL | `goerp/05-server-actions-list.png`, `odoo/05-server-actions-list.png`, `comparisons/server_actions_list.png` | 100.00% | GoERP shows `1-20 / 20`; Odoo shows `1-7 / 7`. GoERP lacks `Top-level actions` filter chip. First GoERP rows have blank Model cells where Odoo has model names. Dataset, pager, and control panel differ. |
| Server Action form | FAIL | `goerp/06-server-action-form.png`, `odoo/06-server-action-form.png`, `comparisons/server_action_form.png` | 100.00% | Same record name, but layout fails. GoERP has extra stacked action buttons and reduced field layout. Odoo has compact form header, Model/Allowed Groups/Type row, proper smart button placement, and visible code content. |
| Relation field dropdown/autocomplete | FAIL | `goerp/07-relation-dropdown.png`, `odoo/07-relation-dropdown.png`, `comparisons/relation_dropdown.png` | 100.00% | Dropdown result now matches `Mail Server` + `Search more...`; still fails strict parity because GoERP opens it in existing edit form with different field width, placement, form chrome, and code area. Odoo reference is new form with `Set an explicit name`. |
| Scheduled Actions list | FAIL | `goerp/08-scheduled-actions-list.png`, `odoo/08-scheduled-actions-list.png`, `comparisons/scheduled_actions_list.png` | 100.00% | GoERP columns: `Action Name`, `Active`, `Interval`, `Interval Unit`, `Next Execution Date`, `Action Type`. Odoo columns: `Priority`, `Action Name`, `Model`, `Next Execution Date`, `Interval Number`, `Interval Unit`, `Active`. Filter chip and active checkbox rendering differ. |
| Scheduled Action form | FAIL | `goerp/09-scheduled-action-form.png`, `odoo/09-scheduled-action-form.png`, `comparisons/scheduled_action_form.png` | 100.00% | GoERP lacks Odoo Model, Allowed Groups, Scheduler User, Execute Every, Active toggle placement, Priority, and exact code content. Form density and field ordering differ. |
| Users list | FAIL | `goerp/10-users-list.png`, `odoo/10-users-list.png`, `comparisons/users_list.png` | 100.00% | Columns match, but GoERP lacks Odoo `Internal Users` filter chip. Avatar color/icon differs. Search field and topbar differ. |
| Users form | FAIL | `goerp/11-users-form.png`, `odoo/11-users-form.png`, `comparisons/users_form.png` | 100.00% | GoERP lacks exact Odoo access-rights layout. Odoo shows role radio section, master data, extra rights in two columns, and exact smart buttons. GoERP still renders boxed checklist/table structure. |
| Groups list | FAIL | `goerp/12-groups-list.png`, fallback Odoo `13-groups-list.png`, `comparisons/groups_list.png` | 100.00% | Row count and main columns align, but GoERP lacks Odoo `Internal Groups` filter chip and checkbox column. Topbar/search/pager styling differs. |
| Groups form | FAIL | `goerp/13-groups-form.png`, fallback Odoo `14-groups-access-form.png`, `comparisons/groups_form.png` | 100.00% | Tabs now align by label, but form layout fails. GoERP lacks Odoo list grid under Users tab, `Add a line`, row headers, exact smart button placement, and full-width sheet geometry. |

## Blocking Gaps

### P0

1. Technical menu nesting/order is not Odoo.
   - Required: exact visible Odoo menu order, groups, labels, and dropdown dimensions.
   - Current: GoERP 55 items vs Odoo 35; wrong first group; missing `IAP Accounts`, `Tours`, `Fields Selection`, `ManyToMany Relations`, `Paper Format`.

2. Access-rights screens are still not Odoo.
   - Required: Odoo Users role radio/access sections and Groups tabbed x2many/list grids.
   - Current: GoERP Users form uses boxed checklist/table layout; Groups form lacks Odoo list grid and `Add a line` behavior.

3. Server/Scheduled action forms are not native Odoo layout.
   - Required: exact field groups, headers, smart buttons, code content, filter chips, and scheduled action controls.
   - Current: lists/forms use different fields, missing chips, extra buttons, and wrong form density.

### P1

1. Launcher and Apps icon parity.
   - App counts now mostly align, but icon art, topbar database badge, search chip layout, card spacing, and category counts still fail.

2. Settings page detail parity.
   - Section skeleton is closer, but row icons, teal action links, search sizing, document layout actions, and topbar are still visibly off.

3. Relation dropdown context parity.
   - Autocomplete result list is close for `mail`, but field geometry and form context are not Odoo-identical.

### P2

1. Pixel-level theme polish.
   - Full-screen diffs remain 99.97-100.00% on all checked flows.

2. Fresh Odoo Groups route setup.
   - Fresh minimal Odoo reference did not expose Groups menu; future runs should install/configure the same reference access menu set before comparing Groups.

## Normal User Verdict

No. GoERP would not look like original Odoo to a normal user. Major screens are closer than earlier captures, but strict parity still fails on nested menus, access forms, action forms, topbar/database identity, icons, filter chips, field layouts, and exact Odoo interaction structure.
