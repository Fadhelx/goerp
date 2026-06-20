# Plan 006: Build Accounting Module And Enterprise Accounting Extension Points

> Executor instructions: implement accounting in phases. Use Odoo 19 accounting source as behavior inventory.
>
> Drift check: run `git diff --stat -- internal/accounting addons/accounting testdata/accounting`.

## Status

- Priority: P1
- Effort: L
- Risk: HIGH
- Depends on: `plans/002-base-kernel-orm-modules.md`, `plans/003-security-users-acl-rules.md`, `plans/005-automation-mail-scheduler.md`
- Category: architecture
- Planned at: no commits, 2026-06-16

## Why This Matters

Accounting is compliance-sensitive. It must not be a loose custom model set. It needs journals, accounts, moves, move lines, taxes, reconciliation, lock dates, reports, access control, and multi-company isolation.

## Current State

Reference paths:
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/account`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise/account_accountant`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise/account_reports`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise/account_asset`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise/account_budget`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise/account_followup`

Local Odoo 19 `account` inventory includes:
- controllers: portal, catalog, terms, document downloads
- data: account data, reports, sequences, mail templates, onboarding, services crons
- models: accounts, journals, moves, move lines, payments, payment terms, taxes, reconciliation, reports, lock exceptions, analytic accounting
- tests

Scoped source counts from local inventory:
- `account`: 476 files
- `enterprise/account*`: 2000+ files

Highest-risk local accounting files:
- `models/account_move.py`: 7330 lines
- `models/account_move_line.py`: 3741 lines
- `models/account_tax.py`: 5206 lines
- `enterprise/account_reports/models/account_report.py`: 8097 lines

## Commands You Will Need

| Purpose | Command | Expected on success |
|---------|---------|---------------------|
| Accounting tests | `go test ./internal/accounting ./addons/accounting` | exit 0 |
| All tests | `go test ./...` | exit 0 |
| Race | `go test -race ./...` | exit 0 |

## Scope

In scope:
- `internal/accounting`
- `addons/accounting`
- `addons/accounting_enterprise_hooks`
- `testdata/accounting`
- accounting migrations

Out of scope:
- country-specific localization beyond generic chart template
- bank sync providers
- EDI providers
- payroll
- full enterprise reports in phase 1

## Steps

### Step 1: Create accounting module manifest

Create installable `accounting` module depending on:
- base
- mail
- automation

Define model registrations and seed data load order.

Verify: `go test ./addons/accounting -run TestManifest` exits 0.

### Step 2: Implement accounting core models

Models:
- `account.account`
- `account.account.tag`
- `account.account.type`
- `account.bank.statement`
- `account.bank.statement.line`
- `account.cash.rounding`
- `account.chart.template`
- `account.code.mapping`
- `account.journal`
- `account.move`
- `account.move.line`
- `account.payment`
- `account.payment.method`
- `account.payment.method.line`
- `account.payment.term`
- `account.payment.term.line`
- `account.tax`
- `account.tax.group`
- `account.tax.repartition.line`
- `account.fiscal.position`
- `account.partial.reconcile`
- `account.full.reconcile`
- `account.reconcile.model`
- `account.reconcile.model.line`
- `account.report`
- `account.report.line`
- `account.report.expression`
- `account.report.column`
- `account.report.external.value`

Verify: `go test ./internal/accounting -run TestModels` exits 0.

### Step 3: Implement posting invariants

Rules:
- move must balance
- posted moves immutable except allowed fields
- sequence assigned on post
- company/currency consistency
- receivable/payable partner required
- tax lines generated deterministically
- lock dates enforced

Verify: `go test ./internal/accounting -run TestPosting` exits 0.

### Step 4: Implement chart, journal, tax, fiscal position data

Create generic chart fixture:
- receivable
- payable
- bank
- cash
- income
- expenses
- liabilities
- equity
- current-year earnings

Create journals:
- sales
- purchases
- bank
- cash
- miscellaneous
- tax return

Verify: `go test ./addons/accounting -run TestInstallChart` exits 0.

### Step 5: Implement reconciliation

Support:
- partial reconcile
- full reconcile
- residual amount
- receivable/payable matching
- payment allocation basics

Verify: `go test ./internal/accounting -run TestReconcile` exits 0.

### Step 6: Implement access controls

Groups:
- invoice user
- basic accounting
- read-only accounting
- billing user
- accountant
- adviser/admin
- read-only auditor

Record rules:
- company isolation
- branch constraints where company tree exists

Verify: `go test ./addons/accounting -run TestAccountingSecurity` exits 0.

### Step 7: Add enterprise extension points

Create separate hooks for:
- accountant dashboards and lock-date wizards
- assets
- budgets
- reports
- followup
- bank statement import
- batch payments
- ISO20022/SEPA
- external tax providers
- Intrastat/SAF-T
- invoice/bank statement extraction
- loans and transfers

Do not implement full Enterprise behavior in this plan. Define interfaces and install guards.

Verify: `go test ./addons/accounting_enterprise_hooks` exits 0.

## Test Plan

- Move balance tests.
- Tax computation tests.
- Fiscal position mapping tests.
- Lock date tests.
- Multi-company isolation tests.
- Reconciliation tests.
- Module install tests.

## Done Criteria

- Generic accounting module installs in test registry.
- Posting and reconciliation tests pass.
- No country-specific legal claim is made.
- Enterprise hooks compile and do not require enterprise assets/code.

## STOP Conditions

- Target jurisdiction is required for legal reports.
- Enterprise source-copying is requested without license decision.
- Accounting tests require external bank/EDI providers.

## Maintenance Notes

Accounting reports should be expanded only after core posting and reconciliation are stable.
