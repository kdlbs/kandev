---
created: 2026-09-04
status: complete
requirements:
  - REQ-TASKS-WIP-LIMIT-PULL-SYSTEM-001
system_design:
  - ../../specs/tasks/system-design/wip-limit-pull-system.md
legacy_specs: []
---

# Implementation Plan: Tree Archive Feeder Pulls

## Overview

Restore WIP feeder backfill after task-tree archive and delete operations. The
single work order first proves the missing cascade trigger with a SQLite-backed
regression test. Then it wires vacancy reconciliation into the handoff cascade
and runs the requested backend gates.

## Confirmed root cause

`HandoffService.ArchiveTaskTree` and `HandoffService.DeleteTaskTree` mutate task
rows directly and bypass `Service.ArchiveTask` and `Service.DeleteTask`. The
single-task paths call `pullNextTaskOnVacate` after their mutations commit. The
cascade paths do not retain the pre-mutation workflow-step IDs. They also do not
invoke a vacancy reconciler after the loop. Consequently, admitted legacy feeder
tasks with no `queued_for_step_id` remain stranded. Startup
`ReconcileQueuedTasks` only discovers tasks with an explicit queue destination.

The existing public `Service.ReconcileFeederPulls(ctx, workflowID,
feederStepID)` is not the correct seam for this event: it treats its step ID as
a feeder source and finds destinations that pull from it. Tree archive/delete
instead needs to reconcile the destination step that lost WIP occupants.

## Scope

### In scope

- Capture the pre-mutation workflow step for each task that the cascade
  actually archives or deletes.
- Reconcile each distinct vacated step once after the mutation loop commits,
  using a caller-cancellation-independent context.
- Apply the same behavior to archive and delete cascades.
- Wire a narrow vacancy-reconciliation dependency from backend composition to
  `HandoffService`.
- Prove feeder promotion, batching, and `wip_pull` transition attribution with
  a SQLite-backed Go regression test.

### Out of scope

- Changing WIP admission, candidate ordering, feeder routing, or restart
  reconciliation semantics.
- Frontend, API, schema, migration, runtime-feature-flag, or public-doc changes.
- Refactoring the wider handoff cascade or task-service promotion machinery.

## Technical approach

### Vacancy reconciliation seam

- In `apps/backend/internal/task/service/handoff_service.go`, add a narrow
  `VacatedStepReconciler` dependency and setter on `HandoffService`.
- In `apps/backend/internal/task/service/service_workflow.go`, expose the
  matching task-service method as a thin wrapper over
  `pullNextTaskOnVacate(ctx, stepID, "")`. The task service continues to own
  candidate selection, transactional admission, event publication, and
  `wip_pull` attribution.
- In `apps/backend/internal/backendapp/helpers.go`, wire `p.taskSvc` into the
  handoff service alongside the existing event-publisher and resource-cleaner
  dependencies.

### Cascade batching

- In `apps/backend/internal/task/service/handoff_cascade.go`, read each task
  before its archive or delete mutation. Add its non-empty workflow step to a
  set only after that mutation succeeds. Archive CAS misses remain skipped.
- After each deepest-first mutation loop, iterate the distinct step IDs in
  deterministic order and call the reconciler with
  `context.WithoutCancel(...)`. Reconciliation remains best effort. The task
  service logs lookup, count, and promotion errors. These errors do not fail or
  roll back the completed archive or delete.

## Tests

- `AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.2` maps to
  `TestHandoffTaskTreeLifecycleBackfillsFeederVacancies` in
  `apps/backend/internal/task/service/handoff_cascade_feeder_pull_test.go`.
- The table cases create a WIP-limited destination with a feeder, fill its
  slots, and seed admitted untagged feeder candidates. Each case then archives
  or deletes a two-task tree through `HandoffService`.
- RED proves that candidates remain in the feeder before the new wiring. GREEN
  proves that available slots are filled and the remaining candidate stays in
  the feeder. It also proves one reconciliation for the shared step and
  `wip_pull` attribution for each promoted candidate.

## Work orders

- [x] [Task 01: Backfill feeder vacancies after task-tree lifecycle mutations](task-01-backfill-tree-lifecycle-vacancies.md)

## Verification results

- RED: both archive and delete cases left `feeder-first` in `waiting-step`.
- GREEN: the focused regression test passed all archive and delete assertions.
- The task-service package passed 1,567 tests after review regressions added
  coverage for concurrent placement changes and partial cascade failures.
- The full backend suite passed after removing the session's pinned
  `KANDEV_INTERNAL_CONFIG_FILE` and `KANDEV_INTERNAL_CONFIG_HOME_FILE` values.
- Backend lint passed with zero issues.

## Risks

- Reading the pre-mutation task row must not cause a skipped archive CAS to
  trigger reconciliation.
- Multiple tasks from one step must produce one reconciliation, while tasks
  from different steps must each wake their own destination.
- Post-commit reconciliation must retain caller identity for authorization but
  discard caller cancellation, matching the existing cascade cleanup context.

The repository mutation returns the vacated workflow step from the same
transaction that archives or deletes the task. A deferred batch finalizer also
reconciles successful mutations when a later tree mutation fails.
