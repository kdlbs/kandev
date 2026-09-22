---
id: 02-persist-workflow-script-runs
title: Persist workflow script runs
status: done
wave: 2
depends_on:
  - 01-define-script-action-contract
plan: plan.md
requirements:
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-004
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-004.1
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-004.2
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-004.3
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-004.5
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-004.6
system_design:
  - ../../specs/tasks/system-design/workflow-step-script-actions.md
---

# Task 02: Persist workflow script runs

## Summary

Create replay-safe SQLite and Postgres storage for immutable script claims,
admission state, message/process identity, and terminal results.

## In scope

- Cross-database schema and repository interface.
- Stable occurrence identities for entry, turn completion, and exit.
- Atomic claim-or-load behavior and legal status transitions.
- Retention tied to task audit lifetime.

## Out of scope

- Agentctl execution, workflow lifecycle gates, and message rendering.

## Acceptance

1. A unique occurrence/action key returns one immutable run snapshot under
   duplicates and concurrent claims on both databases.
2. Admission attempt, process/message IDs, terminal results, and interruption
   reasons survive restart with identical repository behavior.
3. Workflow edits after a claim cannot mutate the recorded command or policy.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/task/repository/sqlite
cd apps/backend && go test ./internal/task/repository/postgres
cd apps/backend && go test -race -tags fts5 ./internal/task/repository/sqlite
git diff --check
```

## Files likely touched

- `apps/backend/internal/task/models/workflow_script_run.go`
- `apps/backend/internal/task/repository/interfaces.go`
- `apps/backend/internal/task/repository/sqlite/`
- `apps/backend/internal/task/repository/postgres/`
- Replayable schema migrations and repository tests.

## Dependencies

- Task 01 supplies normalized config snapshots.

## Risks

- An occurrence key based only on task and step would suppress later valid
  runs.
- SQLite and Postgres conflict-return behavior must remain equivalent.

## Parallelism

`parallel-safe` with Task 03 after Task 01. This task owns task persistence and
occurrence identity; Task 03 owns agentctl/runtime process code.

## Inputs

- Step-entry ledgers, completed-turn identity, and durable move identities.
- Existing replayable migration and repository conventions.

## Results

Implemented a shared SQLite/Postgres workflow script run ledger with a unique
occurrence key, immutable command/policy/session snapshots, stable process
request identity, durable admission/message/process state, one-way terminal
completion, interruption recovery, and task-lifetime retention through the
task foreign key. Duplicate and concurrent claims return the stored winner.

Verification: focused SQLite tests and model tests pass. PostgreSQL coverage is
environment-gated and exercises the same claim and lifecycle paths when
`KANDEV_TEST_POSTGRES_DSN` is configured; `git diff --check` is clean.
