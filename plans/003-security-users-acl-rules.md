# Plan 003: Build Security, Users, Groups, ACLs, Record Rules

> Executor instructions: implement authorization as core ORM behavior.
>
> Drift check: run `git diff --stat -- internal/security internal/record internal/model internal/base migrations testdata`.

## Status

- Priority: P1
- Effort: L
- Risk: HIGH
- Depends on: `plans/002-base-kernel-orm-modules.md`
- Category: security
- Planned at: no commits, 2026-06-16

## Why This Matters

Odoo security is data-driven. Access rights apply at model level. Record rules apply record-by-record after access rights. Field groups restrict field visibility. Sudo and multi-company behavior can cross data boundaries if uncontrolled.

## Current State

Reference paths:
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo/addons/base/models/res_users.py`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo/addons/base/models/ir_rule.py`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo/addons/base/models/ir_model.py`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo/addons/base/security`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo/addons/base/data/res_users_data.xml`

Required core models:
- `res.users`
- `res.groups`
- `res.company`
- `ir.model.access`
- `ir.rule`
- `ir.model.fields`
- `ir.ui.menu`
- `ir.actions.*`

## Commands You Will Need

| Purpose | Command | Expected on success |
|---------|---------|---------------------|
| Security tests | `go test ./internal/security ./internal/record` | exit 0 |
| All tests | `go test ./...` | exit 0 |
| Race | `go test -race ./...` | exit 0 |
| Vet | `go vet ./...` | exit 0 |

## Scope

In scope:
- `internal/security`
- `internal/base/users.go`
- `internal/base/groups.go`
- `internal/base/company.go`
- `internal/base/access.go`
- `internal/base/rules.go`
- `internal/http/session.go` only if Plan 001 created HTTP shell
- migrations for security tables
- `testdata/odoo19/security`

Out of scope:
- OAuth/LDAP/SAML
- MFA
- portal website UI
- login-as impersonation; covered in Plan 008

## Steps

### Step 1: Add identity models

Implement:
- users
- groups
- implied groups
- companies
- allowed companies
- user active flag
- login/email/name
- password hash placeholder with Argon2id or bcrypt

Verify: `go test ./internal/security -run TestUsersGroupsCompanies` exits 0.

### Step 2: Add session and API token policy

Implement:
- hashed session token storage
- API token hash storage
- expiry
- revoke
- last-used timestamp
- no raw token persistence

Verify: `go test ./internal/security -run TestTokens` exits 0.

### Step 3: Add ACL enforcement

Implement ACL permissions:
- read
- write
- create
- unlink

Order:
1. authenticate user
2. resolve groups including implied groups
3. check model ACL
4. apply record rules for read/write/unlink
5. enforce field group visibility

Verify: `go test ./internal/security -run TestACL` exits 0.

### Step 4: Add record rule evaluator

Use domain AST from Plan 002.

Supported context symbols:
- `user.id`
- `user.company_id`
- `user.company_ids`
- `time`

Rules:
- default allow if ACL allows and no matching rule applies
- global rules intersect
- group rules union within their groups
- create checks pre-insert constraints and post-create visibility

Verify: `go test ./internal/security -run TestRecordRules` exits 0.

### Step 5: Add field group filtering

Implement field metadata group restrictions:
- hidden from read
- rejected on write
- rejected on create
- excluded from view metadata where applicable

Verify: `go test ./internal/security -run TestFieldGroups` exits 0.

### Step 6: Add audit log

Log:
- login success/failure
- logout
- password change
- API token create/revoke/use
- ACL/rule/group/user/company changes
- permission denied
- sudo/bypass use

Redact request bodies and secrets.

Verify: `go test ./internal/security -run TestAuditLog` exits 0.

## Test Plan

- ACL allow/deny per operation.
- Group inheritance.
- Multi-company isolation.
- Record rule logical operators.
- Record rule invalid domains.
- Sudo path cannot be reached through normal request env.
- Field group filtering.
- Audit events for sensitive actions.

## Done Criteria

- `go test ./...` exits 0.
- All ORM CRUD paths call security checks.
- No raw tokens are stored.
- Field group tests prove hidden fields are not exposed.

## STOP Conditions

- Security requires dynamic string eval.
- Raw SQL callers bypass ORM security outside explicit internal migrations.
- Multi-company semantics cannot be represented with current Env.

## Maintenance Notes

Menu visibility is not authorization. Backend RPC and service calls must enforce ACL/rules independently.
