# GoERP Eve Agent Blueprints

These are Eve-ready agent directories for GoERP work.

Eve is filesystem-first: an agent is a directory with required `instructions.md`, optional `agent.ts`, `tools/`, `skills/`, `subagents/`, `channels/`, `connections/`, `sandbox/`, and `schedules/`. The official Eve README also notes that docs are bundled after install at `node_modules/eve/docs`.

Use:

```sh
npx eve@latest init goerp-agents
```

Then copy one blueprint directory from this folder into the generated Eve project, or copy its `agent/` contents over the generated `agent/` directory.

Blueprints:

- `goerp-platform-maintainer`: coordinates base/runtime/OI/backend parity slices.
- `goerp-ui-verifier`: audits and verifies `/web` for Odoo Enterprise-style UI.

These blueprints are not wired into the Go runtime. They are source-controlled agent definitions for external Eve execution.

Blueprints include:

- `agent/tools/`: deterministic helpers that return GoERP-specific checklists.
- `agent/sandbox/`: Node sandbox runtime config for browser/source inspection.
- `agent/schedules/`: recurring prompts for dashboard and UI drift checks.
