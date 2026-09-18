---
id: "01-edit-versions"
title: "Persist edit versions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-PLAN-SAFE-002
acceptance_criteria:
  - AC-TASKS-PLAN-SAFE-002.1
  - AC-TASKS-PLAN-SAFE-002.2
  - AC-TASKS-PLAN-SAFE-002.3
  - AC-TASKS-PLAN-SAFE-002.5
system_design:
  - ../../specs/tasks/system-design/plan-safe-edits.md
---

# Task 01: Persist edit versions

## Summary

Persist a version for each committed plan edit and expose coherent agent reads. Preserve history coalescing, browser payloads, comments, and implementation markers.

## In scope

- Add the version migration, backfill, model field, scans, and rotation to every production content/title repository writer.
- Add service snapshot reads and MCP read metadata without changing exact content.
- Update repository interface test doubles and upgrade fixtures where required. Cover committed-result fallback after a post-write read fails.

## Out of scope

Replacement admission, exact edits, revision tools, and rendered UI.

## Acceptance

- Versions survive restart and change for identical writes, browser writes, coalescing, and delete/recreate.
- Migration replay and rollback preserve existing data. Marker/comment updates do not change the version.
- Agent reads return exact content with its matching version. No generated browser types or UI controls are required.

## TDD and evidence

Seed a plan, coalesce another write into the same revision, and assert the version changes. Add migration replay, restart, rollback, and marker-only cases. Extend PostgreSQL coverage using the existing isolated-database helper.

Test entry points and acceptance mappings are in [the manifest](plan.md#tests).
Update existing assertions only where the design explicitly changes their contract.

## Verification

Run from the repository root. Set `KANDEV_TEST_POSTGRES_DSN` to an isolated test database for PostgreSQL coverage.
Record a missing DSN as skipped coverage, never as a pass.
The normal repository command includes the environment-gated PostgreSQL tests.

```bash
(cd apps/backend && go test ./internal/task/repository/... ./internal/task/service ./internal/mcp/handlers ./internal/mcp/server -count=1)
(cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
git diff --check
```

## Files changed

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/plan.go`
- `apps/backend/internal/task/repository/sqlite/plan_version_test.go (new)`
- `apps/backend/internal/task/repository/sqlite/document_plan_postgres_test.go`
- `apps/backend/internal/task/service/plan_service.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/server/handlers.go`
- `apps/backend/internal/persistence/storeconformance/upgrade_test.go and testdata/upgrades/`

The implementation also updates the shared MCP read/write bridge and related
server and contract tests.

## Dependencies

None.

## Risks

A revision ID is not a write version. Do not use timestamps or reset a numeric counter on recreation.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/plan-safe-edits.md).
- [System design](../../specs/tasks/system-design/plan-safe-edits.md).
- [Decision](../../decisions/2026-09-16-conditional-agent-plan-writes.md).
- Existing `plan_service_concurrency_test.go`, `task_plan_guard_test.go`, and `task_plan_append_mode_test.go`.
- Backend guidance and the TDD backend-testing reference.

## Results

Implemented durable `task_plans.write_version` storage with replayable SQLite
and PostgreSQL-compatible migration/backfill behavior. Repository content/title
writes rotate the opaque version, while marker and comment writes preserve it.
Agent reads now return exact content with the matching version, and metadata
history reads use a bounded byte-count query.

Repository, service, MCP handler, and MCP server tests passed. The PostgreSQL
checks are environment-gated and were not run without a test DSN.
