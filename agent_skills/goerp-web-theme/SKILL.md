---
name: goerp-web-theme
description: Build, audit, or verify the GoERP `/web` interface so it feels like minimal Odoo Enterprise instead of a generic ERP shell.
---

# GoERP Web Theme

Use this skill when changing `/web`, app launcher, menus, lists, forms, assets, or theme behavior.

## Target

The normal user should think the app is Odoo:

- title `Odoo`.
- body/root classes such as `.o_web_client`, `.o_main_navbar`, `.o_action_manager`.
- enterprise-like app launcher as the first useful screen.
- restrained business UI with compact controls, list views, form sheets, breadcrumbs, and action menus.
- no generic demo/admin shell cues.

## Primary Files

- `internal/http/server.go`: HTML, CSS, JS, web routes, asset routes.
- `internal/http/server_test.go`: shell, route, and DOM regression tests.
- `internal/runtime/bootstrap_test.go`: bootstrapped menu/action/asset checks.
- `reports/agent_audit_backlog.md`: completed UI slice summary.
- `reports/progress_dashboard.html`: regenerated live build dashboard.

## UI Rules

- Do not build a marketing page.
- Do not copy proprietary Odoo Enterprise or OI source/assets.
- Do not add visible usage instructions, keyboard-shortcut explanations, or feature descriptions.
- Do not expose normal-user controls named `Developer RPC`, `Build dashboard`, or `Gorp`.
- Do not show technical field/model controls in the ordinary surface.
- Use Odoo terminology: Apps, Settings, Technical, Server Actions, Scheduled Actions, Automation Rules, Access Rights, Record Rules.
- Keep cards only for repeated app/module items. Do not nest cards.
- Keep layout stable on mobile and desktop.
- Avoid purple/blue gradients, decorative orbs, bokeh, and emoji.

## Verification Checklist

Run the local server:

```sh
GORP_HTTP_ADDR=:8069 go run ./cmd/gorpd serve
```

Check:

- `/web` returns 200.
- `/web` contains `<title>Odoo</title>`.
- `.o_web_client`, `.o_main_navbar`, `.o_action_manager`, app launcher, list view, and form sheet are present.
- forbidden strings are absent: `Gorp`, `Developer RPC`, `Build dashboard`, `Create Demo Partner`, `Backend connected`.
- Apps launcher opens installed modules.
- Settings and Technical menus open real list/form records.
- `/web/assets/manifest?bundle=web.assets_backend&debug=assets` emits debug asset URLs.
- at least one `/web/assets/debug/web.assets_backend/...` URL returns 200.

Use Browser or Computer Use for visual checks when the task affects layout. Prefer DOM evidence plus one screenshot or explicit observation.

## Test Commands

```sh
go test ./internal/http
go test ./internal/runtime -run 'TestBootstrapOIExposesHTTPModulesAssetsMenusAndViews'
```

Run `go test ./...` before pushing/deploying UI changes.
