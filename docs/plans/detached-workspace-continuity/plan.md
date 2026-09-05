---
created: 2026-09-04
status: completed
requirements:
  - REQ-TASKS-DETACHED-WORKSPACE-CONTINUITY-001
  - REQ-TASKS-DETACHED-WORKSPACE-CONTINUITY-002
system_design:
  - ../../specs/tasks/system-design/detached-workspace-continuity.md
legacy_specs:
  - ../../specs/tasks/requirements/subtask-detachment.md
---

# Implementation Plan: Detached Workspace Continuity

## Overview

Make detachment transfer shared-workspace stewardship atomically, route every
child creation surface through the same workspace attachment step, and prevent
stale cleanup from acting after an ownership generation changes. The work is one
sequential backend package because the schema, repository transaction, service
flow, and regression tests share one ownership contract.

## Scope

### In scope

- Persist ownership generations for workspace groups and task environments.
- Atomically detach a task and transfer group and environment stewardship.
- Fence task-resource and workspace-group cleanup by owner and generation.
- Move child workspace attachment into the canonical task creation sequence.
- Cover SQLite, PostgreSQL, concurrency, restart recovery, and all creation
  entry points named by the requirements.

### Out of scope

- Copying a shared workspace during detachment.
- Frontend interaction changes or new user-facing copy.
- Changing workflow placement, descendant relationships, blockers, or active
  session lifecycle during detachment.
- Broad task cleanup refactoring beyond ownership and generation checks.

## Technical approach

### Persistence and models

- Add `OwnershipGeneration` to Office `WorkspaceGroup` and task
  `TaskEnvironment` models.
- Add replayable `ownership_generation` migrations and fresh-schema columns in
  the task and Office SQLite/PostgreSQL repositories.
- Extend environment and group queries, inserts, updates, and cleanup snapshots
  to preserve the generation.

### Atomic ownership operations

- Replace the repository's narrow `DetachTask` update with a transaction that
  locks child, former parent, active group, and materialized environment; checks
  cleanup barriers; updates hierarchy, roles, owners, and generations; and
  commits once.
- Replace unguarded environment owner transfers with expected-owner and
  expected-generation compare-and-swap operations used by direct delete,
  cascade archive/delete, rollback, and detachment.
- Add generation-aware workspace-group cleanup claim and completion methods so
  stale cleanup cannot change status or remove resources.

### Canonical creation

- Add workspace-policy inputs and an injected attachment coordinator to the task
  service create sequence.
- Resolve and attach policy before `task.created` publication and before Created
  returns; reuse partial-task rollback on failure.
- Remove handler-owned REST and MCP attachment calls and cover WebSocket,
  plugin, workflow/internal child, REST, and MCP creation through the service
  boundary.

### Cleanup and recovery

- Compare live environment owner and generation with the cleanup snapshot before
  task environment and worktree teardown.
- Treat superseded snapshots and retries as safe no-ops while preserving error
  reporting for genuinely failed current-generation cleanup.
- Prove that restart processing cannot revive an old generation's authority.

## Tests

- `apps/backend/internal/task/repository/sqlite/detached_workspace_continuity_test.go`:
  SQLite transaction, idempotency, rollback, cleanup-barrier, concurrent detach,
  and stale-generation cases.
- `apps/backend/internal/task/repository/sqlite/detached_workspace_continuity_postgres_test.go`:
  PostgreSQL row-lock serialization and fresh/replayed migration coverage.
- `apps/backend/internal/office/repository/sqlite/workspace_group_generation_test.go`:
  group generation claims, membership role uniqueness, stale status writes, and
  SQLite/PostgreSQL schema replay.
- `apps/backend/internal/task/service/service_detachment_test.go` and a focused
  cleanup test file: detached child continuity after parent archive/delete,
  stale durable-job recovery, and unchanged physical workspace binding.
- Handler, MCP, plugin adapter, and `CreateChildTask` tests: every creation route
  reaches the service-owned workspace attachment coordinator and rolls back on
  failure.

## E2E tests

Skipped. The existing detach UI and API response do not change. Repository and
service integration tests exercise the workspace lifecycle guarantee below the
UI.

## Work orders

- [x] [Task 01: Transfer workspace ownership](task-01-transfer-ownership.md) *(completed)*

## Verification results

Completed on 2026-09-04.

- Atomic detach, stewardship transfer, role updates, and ownership generations
  are covered by service and repository regressions.
- Current-owner cleanup barriers, guarded owner-generation transfers, stale
  environment cleanup, and generation-fenced workspace cleanup are covered.
- The task service now owns workspace attachment before publishing or returning
  a newly created task; attachment failure rolls the task row back.
- Focused repository tests passed: 27 tests across task and Office persistence.
- Focused service tests passed: 19 tests.
- Creation entry-point tests passed: 15 tests across HTTP, MCP, plugins, and
  backend composition.
- Final combined ownership and creation regressions passed: 76 tests across
  seven packages.
- Race-enabled ownership tests passed: 37 tests across three packages.
- Backend lint passed with zero issues.
- Specification linter tests passed: 30 tests; all specification files passed.
- PostgreSQL integration tests were not run because no isolated
  `KANDEV_TEST_POSTGRES_DSN` was configured.
- The full backend suite reported 9,907 passes, 16 skips, and seven unrelated
  config-discovery failures caused by the VM's existing
  `/root/.kandev/config.yaml` overriding those tests' temporary home fixtures.

## Risks

- Task and Office repositories initialize tables in a defined order; both fresh
  and replayed schemas must converge before cross-table detachment queries run.
- Cleanup invokes external filesystem or provider operations, so its database
  claim must prevent ownership transfer for the complete destructive window.
- External-ID deduplication must not attach a retry's workspace policy to an
  existing task.
