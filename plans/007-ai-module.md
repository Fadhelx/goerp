# Plan 007: Build AI Module

> Executor instructions: implement AI as an installable module with strict security boundaries.
>
> Drift check: run `git diff --stat -- internal/ai addons/ai frontend/packages/ai`.

## Status

- Priority: P2
- Effort: L
- Risk: HIGH
- Depends on: `plans/002-base-kernel-orm-modules.md`, `plans/003-security-users-acl-rules.md`, `plans/004-web-owl-theme.md`, `plans/005-automation-mail-scheduler.md`
- Category: security
- Planned at: no commits, 2026-06-16

## Why This Matters

Odoo 19 Enterprise includes an AI app with agents, topics, sources, API keys, embeddings, tool calling, logging, and server-action integration. The Go ERP AI module must enforce permissions before retrieval and tool execution.

## Current State

Reference path:
- `/Users/fadhelalqaidoom/Desktop/odoo/odoo19/enterprise/ai`

Local inventory:
- controllers: `agent.py`, `main.py`
- models: `ai_agent.py`, `ai_agent_source.py`, `ai_composer.py`, `ai_embedding.py`, `ai_prompt_button.py`, `ai_topic.py`, `ir_actions_server.py`, `mail_thread.py`, `res_config_settings.py`
- ORM: `orm/field_vector.py`
- utilities: citations, logging, HTML extraction, LLM API service, providers
- tests: access, agent, source, embedding, logging, methods, provider integration, tool calling, schema validation

Scoped source count from local inventory:
- `ai`: 207 files

Provider/model inventory:
- Providers: OpenAI, Google Gemini
- OpenAI chat models in source inventory: `gpt-3.5-turbo`, `gpt-4`, `gpt-4o`, `gpt-4.1`, `gpt-4.1-mini`, `gpt-5`, `gpt-5-mini`
- Gemini chat models in source inventory: `gemini-2.5-pro`, `gemini-2.5-flash`, `gemini-1.5-pro`, `gemini-1.5-flash`
- Embedding models: `text-embedding-3-small`, `gemini-embedding-001`
- Vector field uses PostgreSQL vector semantics

Public docs state AI agents have purpose, prompt, topics, tools, and sources; AI settings support Gemini and OpenAI providers.

## Commands You Will Need

| Purpose | Command | Expected on success |
|---------|---------|---------------------|
| AI tests | `go test ./internal/ai ./addons/ai` | exit 0 |
| All tests | `go test ./...` | exit 0 |
| Frontend tests | `pnpm -C frontend test -- ai` | exit 0 |

## Scope

In scope:
- `internal/ai/providers`
- `internal/ai/prompts`
- `internal/ai/agents`
- `internal/ai/topics`
- `internal/ai/sources`
- `internal/ai/embeddings`
- `internal/ai/tools`
- `internal/ai/audit`
- `addons/ai`
- `frontend/packages/ai`

Out of scope:
- live provider calls in CI
- storing raw provider keys in DB
- arbitrary ORM/SQL tools
- unbounded document ingestion

## Steps

### Step 1: Add provider abstraction

Interface:
- chat completion
- embeddings
- model metadata
- timeout/retry policy
- token usage return

Providers:
- OpenAI-compatible
- Gemini-compatible stub
- deterministic mock provider

Verify: `go test ./internal/ai/providers` exits 0.

### Step 2: Add AI settings and secret handling

Settings:
- default provider
- default chat model
- default embedding model
- token budget
- rate limits
- prompt defaults

Secrets:
- env or secret store reference
- no raw key logs
- no key in exports

Verify: `go test ./addons/ai -run TestAISettings` exits 0.

### Step 3: Add agents, topics, sources

Models:
- `ai.agent`
- `ai.topic`
- `ai.agent.source`
- `ai.prompt.button`
- `ai.embedding`

Support:
- purpose
- prompt
- topics
- sources
- active flag
- tool allowlist

Verify: `go test ./internal/ai/agents ./internal/ai/sources` exits 0.

### Step 4: Add RAG and vector field

Implement:
- chunking
- embedding jobs
- vector store adapter
- PostgreSQL vector adapter or guarded no-vector fallback
- metadata filters
- per-record ACL check before retrieval result use

Verify: `go test ./internal/ai/embeddings -run TestPermissionFilteredRetrieval` exits 0.

### Step 5: Add tool calling

Rules:
- tools are explicit registry entries
- schema-validated inputs
- service-layer functions only
- Env from user context
- no raw SQL
- audit every call

Verify: `go test ./internal/ai/tools -run TestToolAuthorization` exits 0.

### Step 6: Add AI UI

Implement:
- AI button
- chat panel
- prompt buttons
- source citations
- error states
- admin settings

Verify: `pnpm -C frontend test -- ai` exits 0.

### Step 7: Add audit and eval harness

Log:
- user
- company
- agent
- prompt ID
- model
- token usage
- latency
- tools
- permission result

Add golden eval tests with mock provider.

Verify: `go test ./internal/ai ./addons/ai` exits 0.

## Test Plan

- Provider mock tests.
- Secret redaction tests.
- Retrieval ACL tests.
- Tool authorization tests.
- Prompt schema tests.
- Audit logging tests.
- Frontend chat panel tests.

## Done Criteria

- AI module installs.
- Mock-provider tests pass.
- No provider key appears in logs or exports.
- Tool calls cannot bypass record rules.

## STOP Conditions

- User requests hardcoded API keys.
- RAG retrieval cannot enforce record rules.
- Tool calls require raw SQL or unvalidated arbitrary actions.

## Maintenance Notes

Add live provider tests only behind explicit env flags.
