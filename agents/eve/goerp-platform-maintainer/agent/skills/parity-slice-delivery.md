---
description: Deliver one Odoo/OI parity slice with evidence.
---

# Parity Slice Delivery

1. Define the exact Odoo behavior in one sentence.
2. Locate the source files and local files.
3. Write down the local gap.
4. Implement only that gap.
5. Add regressions that would fail before the change.
6. Update `reports/agent_audit_backlog.md`.
7. Run `go run ./tools/progress_dashboard --out reports/progress_dashboard.html`.
8. Run focused tests and then `go test ./...` if releasing.

Evidence must include file paths, test commands, and observed runtime behavior when relevant.
