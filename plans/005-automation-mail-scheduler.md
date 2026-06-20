# Plan 005: Build Automation, Scheduled Actions, Mail, Activities, Notifications

> Executor instructions: implement backend automation and communication primitives.
>
> Drift check: run `git diff --stat -- internal/automation internal/scheduler internal/mail internal/notifications internal/queue`.

## Status

- Priority: P1
- Effort: L
- Risk: MED
- Depends on: `plans/002-base-kernel-orm-modules.md`, `plans/003-security-users-acl-rules.md`
- Category: architecture
- Planned at: no commits, 2026-06-16

## Why This Matters

Odoo base behavior depends on cron jobs, server actions, automated actions, mail templates, activities, chatter, notifications, and queued email. OI workflow and delegation depend on the same primitives.

## Current State

Reference paths:
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/base_automation`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/mail`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo/addons/base/models/ir_cron.py`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/odoo/addons/base/models/ir_actions.py`
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/odoo/addons/mail/models`

Local Odoo 19 mail inventory includes controllers, crons, templates, activity types, discuss data, mail models, and tests.

Required local model inventory:
- scheduler: `ir.cron`, `ir.cron.trigger`, `ir.cron.progress`
- automation: `base.automation`, extensions to `ir.actions.server`
- mail: `mail.thread`, `mail.message`, `mail.mail`, `mail.notification`, `mail.followers`, `mail.activity`, `mail.template`, `mail.alias`, `mail.message.subtype`, `fetchmail.server`, `discuss.channel`, `discuss.channel.member`, `mail.guest`, `mail.presence`

## Commands You Will Need

| Purpose | Command | Expected on success |
|---------|---------|---------------------|
| Tests | `go test ./internal/automation ./internal/scheduler ./internal/mail ./internal/notifications ./internal/queue` | exit 0 |
| All tests | `go test ./...` | exit 0 |
| Race | `go test -race ./...` | exit 0 |

## Scope

In scope:
- `internal/scheduler`
- `internal/automation`
- `internal/actions/server.go`
- `internal/mail`
- `internal/notifications`
- `internal/queue`
- `internal/base/cron.go`
- `internal/base/mail_server.go`
- `internal/base/mail_template.go`
- `internal/base/activity.go`
- migrations and fixtures

Out of scope:
- mass mailing marketing
- live chat
- VoIP
- SMS/WhatsApp provider integrations
- full Discuss UI

## Steps

### Step 1: Implement scheduler and cron model

Implement:
- `ir.cron` equivalent metadata
- `ir.cron.trigger`
- `ir.cron.progress`
- interval number/type
- nextcall
- active flag
- user context
- locking to prevent duplicate execution
- best-effort timing
- retry and failure state

Verify: `go test ./internal/scheduler -run TestCron` exits 0.

### Step 2: Implement server actions

Action types:
- execute Go registered action
- create/write records
- send email
- enqueue job
- webhook placeholder

Do not support arbitrary Python code. For compatibility, represent scripts as disabled metadata or controlled expression/action DSL.

Verify: `go test ./internal/automation -run TestServerAction` exits 0.

### Step 3: Implement automated actions

Triggers:
- on create
- on write
- on archive
- on unarchive
- on create and write
- on unlink
- onchange
- message
- webhook
- time based
- manual

Support trigger fields and domains.

Verify: `go test ./internal/automation -run TestAutomatedActions` exits 0.

### Step 4: Implement mail templates and outbox

Implement:
- templates
- render context
- recipient resolution
- outbox queue
- SMTP config
- retry/backoff
- dead-letter status
- no secret logging

Verify: `go test ./internal/mail -run TestMailQueue` exits 0.

### Step 5: Implement activities and notifications

Implement:
- activity type
- activity record
- assign user
- deadline
- done/cancel
- in-app notifications
- bus event abstraction

Verify: `go test ./internal/notifications ./internal/mail -run TestActivity` exits 0.

### Step 6: Add integration tests

Test:
- automation creates activity
- cron sends queued email
- failed SMTP retries
- time-based automation uses last-run window
- duplicate job suppression

Verify: `go test ./internal/... -run TestAutomationMailIntegration` exits 0.

## Test Plan

- Scheduler locking.
- Idempotent job execution.
- Email retry and dead-letter.
- Template rendering and escaping.
- Automation trigger domains.
- Permission checks for server actions.

## Done Criteria

- `go test ./...` exits 0.
- Mail secrets never appear in logs.
- Time-based automation has last-run/current-run tests.
- OI workflow can depend on these APIs.

## STOP Conditions

- Server actions require arbitrary Python execution.
- Scheduler cannot guarantee single execution under concurrent workers.
- Mail rendering exposes unescaped user content.

## Maintenance Notes

Keep provider integrations behind interfaces.
