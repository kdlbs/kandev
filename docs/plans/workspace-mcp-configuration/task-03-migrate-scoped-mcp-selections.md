---
id: "03-migrate-scoped-mcp-selections"
title: "Migrate scoped MCP selections"
status: done
wave: 2
depends_on:
  - "01-persist-workspace-mcp-catalog"
plan: "plan.md"
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-001
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-003
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-007
acceptance_criteria:
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.4
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.5
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.1
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.2
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.5
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.6
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-007.1
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-007.2
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-007.3
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-007.4
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-007.5
system_design:
  - ../../specs/agents/system-design/workspace-mcp-configuration.md
---

# Task 03: Migrate Scoped MCP Selections

## Summary

Persist typed MCP selections for profiles, repositories, tasks, and task
sessions. Add an idempotent compatibility importer that translates existing
raw profile MCP rows into workspace definitions and selections.

## In scope

- Add typed selection tables, repositories, atomic replace operations, and
  workspace consistency validation.
- Add workspace-contextual profile selection APIs for global and workspace
  profiles.
- Add repository, task, and task-session selection APIs.
- Add catalog selection-impact counts, preserve associations for disabled
  definitions, and block deletion until the user confirms affected scopes.
- Extend HTTP and WebSocket task-create requests with optional
  `mcp_server_ids` and persist them in the task transaction.
- Extend task-session creation with optional session additions.
- Implement deterministic per-profile-workspace legacy import keys, import
  state, transactional writes, retry, and runtime fallback markers.
- Preserve and test the legacy table. Do not drop or rewrite it.

## Out of scope

- Effective union and agent delivery.
- ACP live application.
- Selection UI.

## Acceptance

- Every selection write rejects cross-workspace, unknown, disabled, or
  wrong-owner identifiers and replaces one scope atomically.
- Disabling a definition preserves its associations, while guarded deletion
  reports and removes confirmed associations in one transaction.
- A global profile can hold different selections in two workspaces without
  either result leaking into the other.
- Legacy import is replayable, redacted, and falls back per profile-workspace
  pair until the complete import transaction succeeds.

## Verification

```bash
cd apps/backend && go test ./internal/agent/mcpconfig ./internal/agent/settings/store ./internal/agent/settings/handlers -run 'Test.*(Selection|LegacyMCP|McpMigration)'
cd apps/backend && go test ./internal/task/service ./internal/task/handlers ./internal/task/repository/sqlite -run 'Test.*(MCP|Mcp|CreateTask)'
```

Write selection isolation and legacy replay tests first. The first RED run must
show that current storage cannot represent workspace-contextual global profile
choices.

## Files likely touched

- `apps/backend/internal/agent/mcpconfig/selections.go`
- `apps/backend/internal/agent/mcpconfig/selections_test.go`
- `apps/backend/internal/agent/mcpconfig/legacy_import.go`
- `apps/backend/internal/agent/mcpconfig/legacy_import_test.go`
- `apps/backend/internal/agent/settings/store/sqlite.go`
- `apps/backend/internal/agent/settings/store/postgres_schema_test.go`
- `apps/backend/internal/agent/settings/store/sqlite_migration_test.go`
- `apps/backend/internal/agent/settings/handlers/mcp_selection_handlers.go`
- `apps/backend/internal/task/dto/requests.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/task_ws_handlers.go`
- `apps/backend/internal/task/service/service_requests.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/postgres_schema_test.go`

## Dependencies

- Task 01 supplies stable definition IDs, workspace ownership, and catalog
  writes used by legacy import.

## Risks

- Task creation has HTTP, WebSocket, and Kandev MCP transports. Every request
  mapper must preserve the optional selection field.
- Existing global profiles can have no obvious workspace row until used. Keep
  the legacy fallback for unimported pairs instead of inventing a global
  definition owner.
- Legacy values can include credentials. Import diagnostics must contain names
  and reason codes only.

## Parallelism

`sequential`

## Inputs

- Requirement sections 003 and 007.
- System-design sections `Selection model`, `API handlers`, `Persistence`, and
  `Legacy migration`.
- Existing task-create request parity tests and agent settings migration tests.

## Results

- Added typed profile, repository, task, and task-session selection persistence with workspace and owner authorization.
- Added task and session request propagation for MCP definition IDs and retained task selections across session operations.
- Added idempotent profile-workspace legacy import with deterministic identities and legacy fallback markers.
- Added deletion impact accounting and invalid cross-workspace or disabled-definition coverage.
- Verification passed through MCP selection, migration, settings handler, task service, and task handler tests.
