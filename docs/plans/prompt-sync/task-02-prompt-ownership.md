---
id: "02-prompt-ownership"
title: "Prompt ownership and read-only guards"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-PROMPT-SYNC-003
  - REQ-INTEGRATIONS-PROMPT-SYNC-004
acceptance_criteria:
  - AC-INTEGRATIONS-PROMPT-SYNC-003.1
  - AC-INTEGRATIONS-PROMPT-SYNC-003.2
  - AC-INTEGRATIONS-PROMPT-SYNC-003.3
  - AC-INTEGRATIONS-PROMPT-SYNC-003.5
  - AC-INTEGRATIONS-PROMPT-SYNC-004.1
  - AC-INTEGRATIONS-PROMPT-SYNC-004.2
  - AC-INTEGRATIONS-PROMPT-SYNC-004.4
  - AC-INTEGRATIONS-PROMPT-SYNC-004.5
system_design:
  - ../../specs/integrations/system-design/prompt-sync.md
---

# Task 02: Prompt ownership and read-only guards

## Summary

Add sync ownership to saved prompts and make synced prompts read-only to
operators and agents. Add the sync-only apply and release methods and the
reference helpers. No sync source is wired yet.

## In scope

- `custom_prompts.source` and `custom_prompts.source_path` as idempotent
  `ADD COLUMN` migrations (SQLite and PostgreSQL), plus the matching model
  fields and every SELECT and scan.
- `ErrPromptSyncManaged`. Ownership-guarded operator update, agent update,
  and delete (`AND source = 'manual'`). HTTP `409` and MCP validation-error
  mapping.
- DTO, MCP summary and read, and boot payload: `source`, `source_path`, and
  effective `allow_agent_edits`.
- `Service.ApplySyncedPrompts(ctx, desired, frozenNames, referenceTexts)`,
  following the reconciliation table in the system design, and
  `Service.ReleaseSyncedPrompts(ctx)`. Each publishes one change event only
  when something changed.
- `Service.UnresolvedPromptReferences(ctx, text)` and
  `Service.ReferencedPromptNames(ctx, texts, names)`.

## Out of scope

Fetching, configuration, routes for sync, and UI.

## Acceptance

- Upgrade replay keeps every existing prompt `manual` and editable, and the
  schema replay is idempotent on SQLite (and on PostgreSQL when the DSN is
  set).
- Operator, HTTP, and agent writes to a synced prompt fail with
  `ErrPromptSyncManaged` and leave the content unchanged. A manual create
  using a synced name returns `ErrPromptAlreadyExists`.
- Apply covers these cases: create, update, unchanged (no write, no event),
  clash with a manual prompt and with a built-in prompt (skipped, existing
  unchanged), frozen, referenced removal kept with a warning, unreferenced
  removal deleted, and longest-match reference counting.

## Verification

```bash
cd apps/backend
go test -trimpath ./internal/prompts/... -count=1
go test -trimpath ./internal/mcp/handlers -run Prompt -count=1
KANDEV_TEST_POSTGRES_DSN=${KANDEV_TEST_POSTGRES_DSN:-} go test -trimpath ./internal/prompts/store -run Postgres -count=1
go run ./cmd/sqlguard ./internal
```

## Likely files

- `apps/backend/internal/prompts/{models/prompt.go,store/sqlite.go,store/agent_writes.go,store/interface.go,store/sync_writes.go}`
- `apps/backend/internal/prompts/service/{service.go,agent_writes.go,sync_apply.go,references.go}`
- `apps/backend/internal/prompts/{dto/dto.go,handlers/handlers.go,controller/controller.go}`
- `apps/backend/internal/mcp/handlers/{config_prompt_handlers.go,config_prompt_writes.go}`
- The boot-payload prompt projection (search for `FromPrompt` callers).

## Risks

- Built-in seed refreshes update rows by known hashes. Confirm that they
  never touch `source`.
- `ListPromptsForReferenceExpansion` must keep its byte budgets after the
  columns are added.

## Results

- `go test -trimpath ./internal/prompts/... -count=1`: pass.
- `go test -trimpath ./internal/mcp/handlers -run Prompt -count=1`: pass.
- `go test -trimpath ./internal/prompts/store -run Postgres -count=1`: skipped;
  `KANDEV_TEST_POSTGRES_DSN` was not set locally.
- `go run ./cmd/sqlguard ./internal`: pass.
- `go test -race ./internal/persistence/storeconformance -count=1`: pass.
- `python3 scripts/list-docs.py validate` and
  `python3 scripts/lint-spec-files.py --all`: pass.
