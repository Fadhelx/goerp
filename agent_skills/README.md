# GoERP Agent Skills

These skills teach agents how to work inside this repository without rediscovering the platform each turn.

Use them as task prompts, Codex skill sources, or Eve `agent/skills` material.

Skills:

- `goerp-platform-kernel`: base ORM, registry, metadata, runtime bootstrap, security, automation, scheduler, mail, AI, and deployment checks.
- `goerp-web-theme`: `/web` UI, Odoo Enterprise-style theme, OWL-compatible contracts, and browser verification.
- `goerp-oi-parity`: OI workflow, workflow advance, delegation, and login-as parity delivery.
- `goerp-agent-orchestration`: parallel-agent delivery, Eve blueprint usage, verification synthesis, and dashboard handoff.

Rules for all agents:

- Keep accounting feature work out of phase 1 unless `GORP_ENABLE_ACCOUNTING=1` is explicit.
- Use `/Users/fadhelalqaidoom/Desktop/odoo` only as reference input.
- Do not copy proprietary Odoo Enterprise or OI source/assets.
- Update `reports/agent_audit_backlog.md` and regenerate `reports/progress_dashboard.html` after completed slices.
- Run focused tests for touched packages. Run `go test ./...` or `make ci` before release claims.
