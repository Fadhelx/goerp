# Strict UI Parity Verification

Date: 2026-06-23
Workspace: `/Users/fadhelalqaidoom/Documents/gorp`
Role: independent verifier/gatekeeper
Status: FAIL

## Evidence

Report directory: `reports/uiux/verifier_strict_parity_20260623_continued/`

Screenshots:
- GoERP: `reports/uiux/verifier_strict_parity_20260623_continued/goerp/`
- Odoo reference: `reports/uiux/verifier_strict_parity_20260623_continued/odoo/`

Metrics:
- `reports/uiux/verifier_strict_parity_20260623_continued/metrics/image-pixel-metrics.json`
- `reports/uiux/verifier_strict_parity_20260623_continued/evidence-manifest.json`

Runtime:
- GoERP: `http://127.0.0.1:8077/web`
- Odoo 19 reference: `http://127.0.0.1:8075/odoo?debug=assets`

## Pass/Fail Table

| Flow | Result | Evidence | Pixel/DOM observations | Blocking gaps |
|---|---:|---|---|---|
| Launcher | FAIL | `goerp/01-launcher-topbar.png`, `odoo/01-launcher-topbar.png` | Pixel delta: 44.25%. GoERP shows 4 tiles. Odoo shows 2 tiles. | App set differs: GoERP has Approvals, Delegation, Settings, Apps. Odoo has Apps, Settings. Icons and topbar user/database treatment differ. |
| Topbar | FAIL | `goerp/01-launcher-topbar.png`, `goerp/02-topbar-user-menu.png`, `odoo/01-launcher-topbar.png` | GoERP topbar text: `Debug`, `My Company`, avatar. Odoo topbar includes enterprise/debug icons, `My Company`, user label, database badge. | Missing Odoo database badge placement and icon set. User menu/topbar spacing differs. |
| Apps | FAIL | `goerp/03-apps.png`, `odoo/03-apps.png` | Pixel delta: 45.10%. GoERP pager: 1-26 / 26. Odoo pager: 1-77 / 77. | GoERP uses local module cards and placeholder icons. Odoo uses app kanban cards, search-panel categories, app actions, colored icons, and Update/Import controls. |
| Settings | FAIL | `goerp/04-settings.png`, `odoo/04-settings.png` | Pixel delta: 70.51%. Both show settings blocks, but control rendering differs. | GoERP shows input-like setting fields and different sidebar/topbar treatment. Odoo uses native action links, section spacing, icon row, database badge, and enterprise chrome. |
| Settings > Technical nested menu | FAIL | `goerp/05-settings-technical-dropdown.png`, `odoo/05-settings-technical-dropdown.png` | Pixel delta: 77.76%. GoERP dropdown DOM items: 55. Odoo dropdown DOM items: 35 visible menu items. | Ordering and labels differ. GoERP lacks exact Odoo labels `IAP Accounts`, `Tours`, `Fields Selection`, `ManyToMany Relations`, `Paper Format`. GoERP adds extra entries including `Automation Rules`, `Scheduled Messages`, `Apps`. |
| Server Actions list | FAIL | `goerp/06-server-actions-list.png`, `odoo/06-server-actions-list.png` | Pixel delta: 60.52%. GoERP pager: 1-20 / 20. Odoo pager: 1-7 / 7. | GoERP lacks Odoo `Top-level actions` filter chip and slider icon. First rows have blank Model cells where Odoo shows human model names. Dataset and chrome differ. |
| Server Action form | FAIL | `goerp/07-server-action-form.png`, `odoo/07-server-action-form.png` | Pixel delta: 20.08%. GoERP form DOM fields: 8. Odoo form DOM fields: 31. | GoERP lacks Odoo smart button and `Create Contextual Action` button. GoERP shows duplicate Model labels and empty code area. Odoo shows model value, scheduled-action smart button, and populated code. |
| Scheduled Actions list | FAIL | `goerp/08-scheduled-actions-list.png`, `odoo/08-scheduled-actions-list.png` | Pixel delta: 27.22%. GoERP pager: 1-9 / 9. Odoo pager: 1-2 / 2. | Columns differ. GoERP shows `Name`, `Active`, `Repeat Every`, `Interval Unit`, `Next Execution Date`, `Action Type`; Odoo shows `Priority`, `Action Name`, `Model`, `Next Execution Date`, interval fields, active checkbox. |
| Scheduled Action form | FAIL | `goerp/09-scheduled-action-form.png`, `odoo/09-scheduled-action-form.png` | Pixel delta: 20.76%. | Form layout and field grouping differ. GoERP shows `Scheduler User`, `Execute Every`, toggle, priority. Odoo shows `Run Manually`, model, allowed groups, scheduled action breadcrumb, and native header controls. |
| Relation many2one dropdown/autocomplete | FAIL | `goerp/10-relation-many2one-dropdown.png`, `odoo/10-relation-many2one-dropdown.png` | Pixel delta: 39.28%. GoERP dropdown labels: 8 model options plus `Search more...`. Odoo dropdown labels: `Mail Server`, `Search more...`. | GoERP current Model field is empty and options differ from reference autocomplete result. Dropdown placement and field state differ. |
| Users access screen | FAIL | `goerp/11-users-list.png`, `goerp/12-users-access-form.png`, `odoo/11-users-list.png`, `odoo/12-users-access-form.png` | Users form pixel delta: 30.67%. | GoERP shows raw role value `group_system` and checkbox list. Odoo shows smart buttons, role sections, grouped extra-rights layout, and native access-rights form structure. |
| Groups access screen | FAIL | `goerp/13-groups-list.png`, `goerp/14-groups-access-form.png`, `odoo/13-groups-list.png`, `odoo/14-groups-access-form.png` | Groups form pixel delta: 38.58%. GoERP tabs: 5. Odoo tabs: 14. | GoERP lacks Odoo group tabs including Menus, Views, Record Rules, Notes. Odoo form includes user list and access-rule notebooks; GoERP shows reduced inherited/users/access/comment set. |
| Mobile launcher | FAIL | `goerp/15-mobile-launcher.png`, `odoo/15-mobile-launcher.png` | Pixel delta: 58.90%. | GoERP mobile launcher app set and spacing differ from Odoo. Odoo reference shows 2 launcher apps; GoERP shows 4. |
| Mobile form | FAIL | `goerp/16-mobile-form.png`, `odoo/16-mobile-form.png` | Pixel delta: 45.50%. | GoERP mobile form keeps full debug/topbar chrome and different action controls. Odoo mobile form has compact form chrome and different field/control density. |

## P0 Gaps

1. Apps and launcher fixture parity is not met.
   - Evidence: `goerp/01-launcher-topbar.png`, `goerp/03-apps.png`, `odoo/01-launcher-topbar.png`, `odoo/03-apps.png`.
   - Required: same app set, same module catalog count, same card structure, same icon treatment, same search/filter/action controls.

2. Users/Groups access screens are structurally incomplete.
   - Evidence: `goerp/12-users-access-form.png`, `goerp/14-groups-access-form.png`, `odoo/12-users-access-form.png`, `odoo/14-groups-access-form.png`.
   - Required: Odoo access-rights role sections, smart buttons, group notebooks, menus/views/access rights/record rules tabs, and native access widgets.

3. Technical menu is not exact Odoo parity.
   - Evidence: `goerp/05-settings-technical-dropdown.png`, `odoo/05-settings-technical-dropdown.png`.
   - Required: exact visible labels, grouping, order, width, scroll behavior, and navigation targets.

## P1 Gaps

1. Settings page chrome and controls differ.
2. Server Actions list/form chrome and record data differ.
3. Scheduled Actions list/form chrome and record data differ.
4. Many2one autocomplete does not match reference field state, results, or placement.
5. Mobile launcher and mobile form do not match reference density or chrome.

## Notes

- Source files were not edited.
- Odoo source/assets were used only as local runtime reference input.
- Screenshots and report artifacts were written only under `reports/uiux/verifier_strict_parity_20260623_continued/`.
- `make ci` was not run because this was read-only verification, not implementation completion.
