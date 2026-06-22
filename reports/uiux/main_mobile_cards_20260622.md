# Mobile List Card Slice

Date: 2026-06-22
Scope: default `/web` mobile list cards, accounting excluded.
Reference policy: clean-room only. `/Users/fadhelalqaidoom/Desktop/odoo` used as reference input only.

## Implemented

- Mobile list records now behave as full clickable/keyboard-openable cards.
- Cards now expose a title/header region instead of a separate `Open` button.
- Server Action mobile cards show human state labels, not raw selection values.
- Secondary mobile card fields skip empty values and avoid duplicating title/state.
- Mobile card CSS now uses tighter Odoo-like header, state badge, focus, and spacing.

## Evidence

- Focused visual smoke: `reports/uiux/main_mobile_cards_20260622/manifest.json`
- Focused screenshots: `reports/uiux/main_mobile_cards_20260622/`

## Verification

- `pnpm -C frontend test -- index.test.mjs`
- `pnpm -C frontend build`
- `node --test tools/web_visual_smoke/run.test.mjs`
- `node tools/web_visual_smoke/run.mjs --base-url=http://127.0.0.1:8087 --out=reports/uiux/main_mobile_cards_20260622 --timeout-ms=60000 --scenario=default-mobile-server-actions-flow --scenario=technical-list-mobile --scenario=default-technical-search-desktop`

Focused visual smoke passed 3/3 scenarios.

## Remaining

- Mobile search panel behavior.
- Mobile form edit/save flow.
- Mobile relation editors and dialogs.
- Mobile systray/company/debug menus.
- Mobile back-stack behavior beyond current breadcrumbs.
