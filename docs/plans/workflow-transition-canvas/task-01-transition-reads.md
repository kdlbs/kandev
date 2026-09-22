---
id: "01-transition-reads"
title: "Read recorded moves and route groups"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-WORKFLOW-HISTORY-001
  - REQ-PLUGINS-WORKFLOW-HISTORY-002
acceptance_criteria:
  - AC-PLUGINS-WORKFLOW-HISTORY-001.1
  - AC-PLUGINS-WORKFLOW-HISTORY-001.2
  - AC-PLUGINS-WORKFLOW-HISTORY-001.3
  - AC-PLUGINS-WORKFLOW-HISTORY-002.1
  - AC-PLUGINS-WORKFLOW-HISTORY-002.2
system_design:
  - ../../specs/plugins/system-design/workflow-transition-history.md
---

# Task 01: Read recorded moves and route groups

## Summary

Expose bounded read methods from the task service over the existing transition
ledger. Keep ledger writers unchanged and make SQLite/Postgres reads agree.

## In scope

- Add a task-history read ordered by descending ledger ID with an opaque,
  task-bound cursor and a `limit+1` continuation check.
- Add grouped workflow-side counts for within-workflow moves and entries/exits.
  Join to retained tasks and workspace ownership; include archived tasks.
- Add indexes needed for the grouped read on existing and new SQLite/Postgres
  stores.

## Out of scope

- Plugin or canvas routes, permission grants, UI, and transition writes.

## Acceptance

1. A task with creation, forward, backward, detach, and same-time moves yields
   stable pages without duplicate or lost IDs; a missing/deleted step does not
   erase its row.
2. Grouped results count retained moves exactly once and separate entries and
   exits, including archived tasks but excluding deleted tasks.
3. Existing database upgrades create the new indexes idempotently on SQLite
   and Postgres, and read methods make no writes.

## Verification

```bash
(cd apps/backend && go test ./internal/task/repository/sqlite ./internal/task/service)
(cd apps/backend && go test ./internal/persistence/storeconformance/...)
```

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/step_transitions.go`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/step_transitions_*_test.go`
- `apps/backend/internal/task/service/service.go`
- `apps/backend/internal/task/service/step_transition_reads.go` and tests

## Dependencies

None.

## Risks

Workflow-side aggregation can scan a large ledger without both source and
destination indexes. Query plans and parity tests must cover that path.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/workflow-transition-history.md)
- [System design](../../specs/plugins/system-design/workflow-transition-history.md)
- Existing ledger writer and [task ledger design](../../specs/tasks/system-design/workflow-task-step-transition-ledger.md)

## Results

Implemented task-ID cursor reads and workspace route grouping over the
retained ledger. Added task-ID, source-workflow, and destination-workflow
indexes. SQLite and Postgres schema replay share the same indexes; the
Postgres query test is gated on `KANDEV_TEST_POSTGRES_DSN`.

Passed after the final schema edit:

```text
go test ./internal/task/repository/sqlite ./internal/task/service ./internal/persistence/storeconformance/...
```

Postgres runtime parity was skipped locally because no DSN is configured.
