---
id: "01-persist-workspace-mcp-catalog"
title: "Persist the workspace MCP catalog"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-001
acceptance_criteria:
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.1
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.2
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.3
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.6
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.7
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.8
system_design:
  - ../../specs/agents/system-design/workspace-mcp-configuration.md
---

# Task 01: Persist the Workspace MCP Catalog

## Summary

Create the workspace-owned definition domain, storage, and sanitized CRUD API.
This work establishes identity, revision, authorization, and secret-reference
boundaries used by every later work order.

## In scope

- Add `MCPServerDefinition`, transport configuration, source metadata,
  revision, and secret-binding models in `internal/agent/mcpconfig`.
- Model remote, managed-package, and existing-executable modes. Require exact
  versions for managed packages.
- Add replayable SQLite and Postgres schema for definitions.
- Implement catalog repository and service operations with optimistic revision
  checks, runtime-name uniqueness, enable state, and delete.
- Add workspace-authorized list, create, get, patch, and delete handlers.
- Sanitize every response and error. Validate secret references without
  returning values.
- Extend workspace cleanup, reset fixtures, and repository fakes.

## Out of scope

- Registry discovery and curated templates.
- Scope selections and runtime delivery.
- Frontend settings surfaces.

## Acceptance

- Catalog writes enforce workspace ownership, reserved names, unique normalized
  runtime names, supported transports, and revision conflicts.
- List, mutation, and error responses expose no secret values.
- Catalog mutation performs no package download, connection, or execution.
- SQLite and Postgres schema replay and cleanup preserve existing profile MCP
  rows.

## Verification

```bash
cd apps/backend && go test ./internal/agent/mcpconfig ./internal/agent/settings/store ./internal/agent/settings/handlers
cd apps/backend && go test ./internal/task/repository/sqlite -run 'Test.*(Migration|Cleanup|Workspace)'
```

Write catalog service and handler tests first. The initial tests must fail
because no workspace definition contract exists.

## Files likely touched

- `apps/backend/internal/agent/mcpconfig/types.go`
- `apps/backend/internal/agent/mcpconfig/service.go`
- `apps/backend/internal/agent/mcpconfig/catalog.go`
- `apps/backend/internal/agent/mcpconfig/catalog_test.go`
- `apps/backend/internal/agent/settings/store/sqlite.go`
- `apps/backend/internal/agent/settings/store/postgres_schema_test.go`
- `apps/backend/internal/agent/settings/store/sqlite_migration_test.go`
- `apps/backend/internal/agent/settings/handlers/handlers.go`
- `apps/backend/internal/agent/settings/handlers/mcp_catalog_handlers.go`
- `apps/backend/internal/agent/settings/handlers/mcp_catalog_handlers_test.go`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`, if task-owned
  selection counts require task repository joins

## Dependencies

None.

## Risks

- Existing settings repositories combine profile and agent storage. Keep the
  new catalog interface focused instead of expanding one broad interface.
- Literal values in legacy transport fields can be sensitive. Tests must assert
  that list and error payloads remain redacted.

## Parallelism

`sequential`

## Inputs

- Requirement sections 001 and 004.
- System-design sections `Catalog domain`, `API handlers`, `Persistence`, and
  `Security`.
- Existing `mcpconfig.Service` and profile MCP handler patterns.
- ADR-2026-09-01 workspace ownership and stable identity decision.

## Results

- Added workspace-owned catalog models, validation, and optimistic revision
  service operations for remote, managed npm, and existing executable modes.
- Added SQLite/Postgres-compatible catalog schema and repository round trips,
  including workspace-qualified reads, writes, deletes, and conflict handling.
- Added sanitized catalog CRUD routes with workspace authorization and no
  materialization, connection, download, or executable probing on save.
- Verification passed:
  `go test ./internal/agent/mcpconfig ./internal/agent/settings/store ./internal/agent/settings/handlers`
  and
  `go test ./internal/task/repository/sqlite -run 'Test.*(Migration|Cleanup|Workspace)'`.
