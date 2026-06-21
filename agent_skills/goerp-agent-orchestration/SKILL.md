---
name: goerp-agent-orchestration
description: Coordinate parallel GoERP agents, Eve blueprints, verification lanes, and dashboard updates without overlapping writes.
---

# GoERP Agent Orchestration

Use this skill when managing multiple agents for GoERP delivery.

## Objective

Move the platform toward Odoo 19 base parity faster by splitting work into independent lanes, verifying each lane, and integrating only evidence-backed changes.

## Inputs

- Workspace: `/Users/fadhelalqaidoom/Documents/gorp`
- Source references:
  - `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo`
  - `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise`
  - `/Users/fadhelalqaidoom/Desktop/odoo/odoo18/odoo18-addons`
  - `/Users/fadhelalqaidoom/Desktop/odoo/odoo17/odoo17-addons`
- Progress source: `reports/agent_audit_backlog.md`
- Live dashboard: `reports/progress_dashboard.html`
- Eve blueprints: `agents/eve/`

## Nonnegotiable Rules

- Skip accounting feature work in phase 1.
- Do not copy proprietary Odoo Enterprise or OI code/assets.
- Give every worker a dedicated goal, exact write scope, and verification command.
- Keep immediate blocking integration work local.
- Do not let two workers own the same files unless one is read-only.
- Treat worker output as untrusted until the diff and commands are inspected.
- Close completed agents when their result has been integrated.

## Lane Template

Use this shape for every delegated lane:

```text
Dedicated /goal: <specific outcome>.

Workspace: /Users/fadhelalqaidoom/Documents/gorp.
Role: <worker|explorer|verification>.
Ownership: <exact files/packages or read-only>.
Rules: skip accounting, do not copy proprietary source/assets, do not revert other edits.
Tasks:
1. Inspect current local implementation.
2. Compare source behavior if relevant.
3. Implement or report one bounded slice.
4. Add focused tests.
5. Return changed files, commands, blockers, remaining gaps.
```

## Recommended Parallel Lanes

- UI master: `/web`, theme, app launcher, menus, list/form/control panel, browser evidence.
- Backend base: actions, views, menus, ACLs, record rules, automation, scheduled actions.
- Mail/chatter: Odoo mail Store, composer, followers, subscription, activities.
- OI parity: `oi_workflow`, `oi_workflow_advance`, `oi_delegation`, `oi_login_as`.
- Verification: focused tests, full CI, dashboard regeneration, deploy checks.
- Agent docs: repo skills and Eve blueprints.

## Integration Loop

1. Read each returned result.
2. Inspect `git diff` for changed files.
3. Reject or fix out-of-scope changes.
4. Run the narrowest proving test.
5. Regenerate dashboard only after all accepted DONE lines are present.
6. Run `make ci` before release or production deploy.
7. Commit one coherent slice with a factual message.

## Eve Blueprint Usage

Eve agents are filesystem-first. The local blueprints follow:

- `agent/instructions.md`: identity and always-on rules.
- `agent/agent.ts`: model/runtime config.
- `agent/skills/`: reusable Markdown procedures.
- `agent/subagents/`: child-agent instructions.
- `agent/tools/`: typed helper tools for repeatable checks.
- `agent/sandbox/`: sandbox runtime config.
- `agent/schedules/`: recurring verification prompts.

Use `agents/eve/goerp-platform-maintainer` for backend/runtime/OI work.
Use `agents/eve/goerp-ui-verifier` for UI visual parity and browser checks.

## Verification Matrix

Before final delivery, collect:

```text
Backend tests: go test ./internal/http ./internal/mail ./internal/base ./internal/runtime
OI tests: go test ./addons/oi_base ./addons/oi_workflow ./addons/oi_workflow_advance ./addons/oi_delegation ./addons/oi_login_as
Frontend tests: pnpm -C frontend typecheck && pnpm -C frontend lint && pnpm -C frontend test && pnpm -C frontend build
Dashboard: go run ./tools/progress_dashboard --out reports/progress_dashboard.html
CI: make ci
Production: service active, release symlink, /web 200, dashboard 200
```
