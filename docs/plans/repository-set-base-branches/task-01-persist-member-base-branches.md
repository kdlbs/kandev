---
id: "01-persist-member-base-branches"
title: "Persist member base branches"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-REPOSITORY-SETS-001
  - REQ-WORKSPACES-REPOSITORY-SETS-002
acceptance_criteria:
  - AC-WORKSPACES-REPOSITORY-SETS-001.8
  - AC-WORKSPACES-REPOSITORY-SETS-002.1
system_design:
  - ../../specs/workspaces/system-design/repository-sets.md
---

# Task 01: Persist Member Base Branches

## Summary

Add an optional base branch to each persisted set member. Expose the value
through service, HTTP, WebSocket, event, and boot-state contracts.

## In scope

- Add the replay-safe SQLite and PostgreSQL column migration.
- Update repository-set models, scans, inserts, and replacements.
- Accept ordered member objects and validate non-empty bases.
- Keep `repository_ids` as a no-base compatibility input.
- Include `base_branch` in DTOs and semantic events.

## Out of scope

- Frontend state and UI behavior.
- Git branch existence requests during mutations.
- Changes to repository defaults or task rows.

## Acceptance

- Existing member rows migrate with an empty base and keep their order.
- Create and update persist every member base atomically.
- Unsafe bases and conflicting input shapes produce no write.

## Verification

Run this command from `apps/backend`.

```bash
go test ./internal/task/repository/sqlite ./internal/task/service ./internal/task/dto ./internal/task/handlers ./internal/backendapp
```

## Files likely touched

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/repository_set.go`
- `apps/backend/internal/task/service/service_repository_sets.go`
- `apps/backend/internal/task/dto/dto.go`
- `apps/backend/internal/task/handlers/repository_set_handlers.go`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- Existing repository-set tests beside these files

## Dependencies

None.

## Risks

- A schema query can reference the new column before the migration adds it.
- A compatibility request can conflict with the new ordered member field.

## Parallelism

`sequential`

## Inputs

- `REQ-WORKSPACES-REPOSITORY-SETS-001`
- `REQ-WORKSPACES-REPOSITORY-SETS-002`
- System-design sections for data, persistence, service, and events

## Results

- Added `repository_set_items.base_branch` to the fresh schema and replayable
  migration, with SQLite legacy-row and PostgreSQL parity coverage.
- Persisted ordered member bases through repository, service, DTO, HTTP, WebSocket,
  boot-state, and repository-set event paths.
- Preserved `repository_ids` compatibility input and rejected unsafe or conflicting
  member payloads atomically.
- Verification: `go test ./internal/task/repository/sqlite ./internal/task/service
./internal/task/dto ./internal/task/handlers ./internal/backendapp` (3848 passed).
