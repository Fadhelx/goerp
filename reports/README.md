# Compatibility Reports

Regenerate inventory:

```sh
go run ./tools/source_inventory --config inventory/sources.yaml --out reports/source_inventory.json
```

Append generated parity entries for new source files:

```sh
go run ./tools/parity_check --inventory reports/source_inventory.json --coverage reports/parity.yaml --write-missing
```

Rewrite generated parity entries from the current inventory:

```sh
go run ./tools/parity_check --inventory reports/source_inventory.json --coverage reports/parity.yaml --rewrite
```

Check parity coverage:

```sh
go run ./tools/parity_check --inventory reports/source_inventory.json --coverage reports/parity.yaml
```

Refresh build dashboard:

```sh
go run ./tools/progress_dashboard --out reports/progress_dashboard.html
```

Parity statuses:
- `pending`: mapped but not implemented
- `implemented`: implemented and verified
- `intentionally_omitted`: excluded by policy or scope with a reason
- `blocked`: cannot proceed without a named decision or dependency

Gate rules:
- every non-static inventory record must have a parity record
- every parity record must have a valid status, target, reason, and verification command unless blocked
- static assets are omitted from the parity gate unless explicitly inventoried as source
- source reports include hashes and line counts only, never file contents

Reports must not contain source code or secrets.

Parallel audit backlog: `reports/agent_audit_backlog.md`.
