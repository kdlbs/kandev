---
id: "01-rebind-task-terminal-queries"
title: "Rebind Task Terminal Queries"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-TERMINALS-001
acceptance_criteria:
  - AC-TASKS-TASK-TERMINALS-001.1
  - AC-TASKS-TASK-TERMINALS-001.2
  - AC-TASKS-TASK-TERMINALS-001.3
system_design:
  - ../../specs/tasks/system-design/task-terminal-persistence.md
---

# Task 01: Rebind Task Terminal Queries

## Summary

Add a real PostgreSQL regression for the ordinary terminal descriptor
lifecycle. Then make each parameterized statement use placeholder syntax for
the selected driver. Preserve the current SQLite and terminal-service contracts.

## In scope

- Add PostgreSQL create, get, list, rename, state, removal, and task-wide removal
  lifecycle assertions.
- Rebind all parameterized reads through the reader pool.
- Rebind all parameterized writes through the writer pool.

## Out of scope

- Schema, API, service, PTY, WebSocket, frontend, and localization changes.
- Sequence-allocation redesign or new concurrency behavior.
- Broad repository verification beyond the task-defined checks.

## Acceptance

- The PostgreSQL lifecycle regression fails before the production correction
  with SQLSTATE `42601` and passes after it.
- Every parameterized ordinary terminal repository method works against both
  SQLite and PostgreSQL with unchanged results.
- The targeted terminal packages and changed-file backend lint pass.

## Verification

```bash
go test ./internal/terminal/... -count=1
KANDEV_TEST_POSTGRES_DSN=<isolated-dsn> go test ./internal/terminal/repository -run TestPostgresTerminalRepositoryLifecycle -count=1 -v
golangci-lint run ./... --new-from-rev="$(git merge-base HEAD origin/main)" --timeout=5m
```

## Files likely touched

- `apps/backend/internal/terminal/repository/sqlite.go`
- `apps/backend/internal/terminal/repository/postgres_test.go`

## Dependencies

None.

## Risks

- The PostgreSQL test must use a real isolated schema so a SQLite-only or mock
  test cannot hide placeholder syntax errors.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-TASK-TERMINALS-001` and its acceptance criteria.
- `docs/specs/tasks/system-design/task-terminal-persistence.md`.
- GitHub issue #3371 and the isolated PostgreSQL reproduction evidence.
- Existing terminal repository tests and `internal/testutil` PostgreSQL helpers.

## Results

- Added `postgres_test.go` with create, get, list, rename, state, single-removal,
  and task-wide removal coverage.
- Added driver rebinding to `Create`, `Get`, `ListByTask`, `Rename`, `SetState`,
  `Delete`, and `DeleteByTask`.
- `TestPostgresTerminalRepositoryLifecycle` failed before the correction with
  SQLSTATE `42601`. It passed all three subtests after the correction.
- `go test ./internal/terminal/... -count=1` passed 29 tests in three packages.
- The changed-file backend linter reported zero issues.
