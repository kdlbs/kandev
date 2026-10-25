---
created: 2026-09-04
status: done
requirements:
  - REQ-TASKS-TASK-TERMINALS-001
system_design:
  - ../../specs/tasks/system-design/task-terminal-persistence.md
legacy_specs: []
---

# Implementation Plan: PostgreSQL Task Terminal Queries

## Overview

Make every parameterized ordinary task-terminal query use placeholder syntax
for the selected database driver. One work order adds PostgreSQL lifecycle
coverage first, applies the mechanical repository correction, and runs the
targeted backend checks.

## Scope

### In scope

- Add an environment-gated PostgreSQL regression for the ordinary terminal
  repository lifecycle.
- Rebind every parameterized read and write in the ordinary terminal
  repository.
- Preserve existing SQLite behavior and error wrapping.

### Out of scope

- Changes to terminal service, WebSocket handlers, or web components.
- Changes to terminal sequence allocation or concurrency semantics.
- New database schema or migrations.
- New terminal error presentation.

## Technical approach

Add `postgres_test.go` beside the existing SQLite repository tests. The test
constructs the real repository over an isolated PostgreSQL schema and exercises
all parameterized lifecycle methods. It must first fail with PostgreSQL syntax
error `42601` on the current implementation.

Update `apps/backend/internal/terminal/repository/sqlite.go` so reads call
`r.ro.Rebind` and writes call `r.db.Rebind` before passing statements to sqlx.
Do not change schema statements because they have no placeholders.

## Tests

- `AC-TASKS-TASK-TERMINALS-001.1` maps to PostgreSQL create and get assertions
  plus the existing SQLite sequence tests.
- `AC-TASKS-TASK-TERMINALS-001.2` maps to PostgreSQL open/all list assertions
  and ordered sequence assertions.
- `AC-TASKS-TASK-TERMINALS-001.3` maps to PostgreSQL rename, state, single
  delete, and task-wide delete assertions.

## E2E tests

The existing browser terminal suites cover the unchanged user, WebSocket, and
service flow. The new real-PostgreSQL lifecycle test covers the driver boundary
that caused the end-to-end failure. No frontend or protocol behavior changes.

## Work orders

- [x] [Task 01: Rebind Task Terminal Queries](task-01-rebind-task-terminal-queries.md)

## Verification results

- The PostgreSQL lifecycle regression passed three subtests on PostgreSQL 16.
- The terminal package suite passed 29 tests in three packages.
- The changed-file backend linter reported zero issues.

## Risks

- Omitting one parameterized method can leave a later lifecycle action broken
  on PostgreSQL even if creation starts working.
- PostgreSQL integration coverage skips when `KANDEV_TEST_POSTGRES_DSN` is not
  configured, so the implementation run must provide an isolated database.
