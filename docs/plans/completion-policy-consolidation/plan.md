---
created: 2026-11-11
status: complete
requirements:
  - REQ-TASKS-COMPLETION-005
system_design:
  - ../../specs/tasks/system-design/task-completion.md
legacy_specs: []
---

# Implementation Plan: Completion policy consolidation

## Overview

This package resolves the five follow-ups from PR #4642 (Office human Done
moves to the final step) as one change set:

| Issue | Problem | Work order |
| --- | --- | --- |
| #5251 | Three definitions of final or completing step | 01 |
| #5250 | Office identifiers and context-carried authority in the task repository | 02 |
| #5252 | Unused service-level approval fallback | 03 |
| #5253 | Routine runs closed by step name and assignee only | 04 |
| #5254 | Bulk move stops halfway on a refusal | 05 |

All five should be fixed. #5250 is a correctness hazard as well as a layering
problem: any new post-commit path that forgets to clear the context intent
inherits human authority or fails with a false conflict. Doing #5250 and #5252
separately would mean writing the hook twice. #5254 needs the policy that #5250
creates. They share one requirement (`REQ-TASKS-COMPLETION-005`) and one
design section.

**Prerequisite.** Work orders 01, 02, 03, and 05 change code that only exists
on PR #4642 (`pr/4642` head `6d07098fe99`, fork `point-source/kandev`). On
2026-11-11 that PR is open and its backend CI is failing. Start these work
orders after #4642 merges, from a branch rebased on `main`. Alternatively,
apply 01 to 03 on the #4642 branch before it merges. That is the cheaper
option because 02 deletes code that #4642 adds. Work order 04 depends only on
`main` and can start now.

Order: 01 provides the shared predicate and SQL fragments that 02 uses. 02
creates the hook. 03 deletes the fallback that 02 makes unreachable. 05 calls
the hook in preflight.

## Scope

### In scope

- One Go and SQL definition each for "final step" and "completing step".
- A typed completion-policy hook in the task repository, with Office approvers
  registered through `backendapp`.
- Removal of `OfficeHumanCompletionIntent` and every manual clear.
- Removal of the service-level Office approval fallback.
- Routine-run close-out from committed task state. Removal of the step-name
  `finalizeDone` path.
- Whole-batch preflight and a structured partial result for both bulk moves,
  with localized web feedback.

### Out of scope

- Changing Office approver semantics (`REQ-OFFICE-HUMAN-DONE-002`), WIP
  admission, or completion gate rules.
- Making the bulk move a single transaction (rejected in the design).
- The Office activity-log write in `handleTaskMoved` keeps its assignee
  condition.
- Changes to the frontend for single-task moves.

## Technical approach

### 01: Predicate (`internal/workflow/models`)

- `terminal.go`: add `IsFinalStep`, rename `IsTerminalStep` to
  `CompletesTaskOnEnter`, and add `FinalStepSQL(alias)` and
  `CompletingStepSQL(alias)`.
- Callers: `orchestrator/event_handlers_workflow.go`,
  `event_handlers_children_completed.go`, `workflow_store.go`,
  `task_launch_workflow.go`, and `task/service/service_workflow.go`.
- `task/repository/sqlite/office_completion.go`: replace the inline SQL in
  `officeCompletionStepPolicyTx` and `updateHumanTaskState`.
- `office/repository/sqlite/participants.go`: `IsTaskWorkflowStepTerminal`
  becomes `IsTaskOnFinalStep` and uses `FinalStepSQL`. `blockers.go`:
  `IsTaskInTerminalStep` becomes `IsTaskInTerminalState`.

### 02: Hook (`internal/task/repository/completion`, `internal/office/completionpolicy`)

- New `completion.Admission`, `Scope`, `AdmissionKind`, `Policy`, `Querier`,
  and `RefusedError`.
- `task/repository/sqlite`: `SetCompletionPolicies`. The generic scope fence
  and completing-step check move into one `admitCompletionTx`, called from
  `updateTaskTx` and from the state-only write. Remove the extra query from
  `lockTaskHierarchy`. `refreshOfficeAdmissionLimitTx` becomes
  `refreshHumanAdmissionLimitTx`. `CompleteOfficeTaskAsHuman` becomes
  `CompleteTaskOnFinalStep`. Delete `office_completion.go` and
  `repoerrors.OfficeApprovalsPendingError` and replace them with generic files.
- `task/service`: build `*completion.Admission` in the service entry points and
  pass it explicitly. Delete the context set and clear sites in
  `service_tasks.go`, `service_workflow.go`, `service_events.go`, and
  `step_entry_dispatch.go`.
- `task/models/completion_gates.go`: delete `OfficeHumanCompletionIntent`.
- `office/completionpolicy`: the approver policy. `backendapp`: register the
  policy and keep the startup panic when it is missing.

### 03: Fallback removal (`internal/task/service/office_approval_guard.go`)

- Delete `guardOfficeHumanCompletion`, `atomicOfficeCompletionConfigured`, the
  `PendingApproversForCompletion` method, and the legacy `persistHumanTaskState`
  branch.
- Replace `SetOfficeApprovalGuard` with `SetApproverNameResolver`.
- Migrate `office_approval_guard_test.go` and the dependent tests to a real
  SQLite repository with the Office policy.

### 04: Effects (`internal/office/service`)

- New `routine_run_completion.go` subscriber on `task.state_changed`.
- `event_subscribers.go`: delete `finalizeDone`, `categorizeStep`, and the
  `stepCategory` type when they become unused.

### 05: Bulk move (`internal/task/service`, `internal/task/handlers`, web)

- `service_workflow.go`: `preflightBulkMove` after
  `acquireBulkMoveStepLocks` in both bulk functions. Add the
  `BulkMoveRefusedError` and `BulkMovePartialError` types.
- `handlers/errors.go` and `task_http_handlers.go`: return a 409 body for each.
  `BulkMoveTasks` stops returning a blanket 500.
- Web: `lib/api/domains/kanban-api.ts`, `hooks/use-task-workflow-move.ts`,
  `components/kanban/task-multi-select-toolbar.tsx`, and seven locale catalogs.

## ASCII UI preview

`UI-01: Bulk move result toast`. Entry point: multi-select toolbar, then
**Move to**, then a step. The toast is shared by desktop and phone. On a phone
it uses the existing bottom toast placement. Structure is required; wording is
illustrative and localized.

```text
Preflight refusal (no task moved)
+------------------------------------------------------+
| (!) No tasks were moved                              |
|     "Fix login redirect": approval pending (Alice)   |
|     "Update docs": completion requirements not met   |
+------------------------------------------------------+

Partial result (concurrent change)
+------------------------------------------------------+
| (!) Moved 3 of 5 tasks                               |
|     "Fix login redirect" was not moved: approval     |
|     pending (Alice). Remaining tasks were not moved. |
+------------------------------------------------------+
```

The toast lists at most three refused titles, then "and N more". Cards that
were moved stay in the target column. Cards that were not moved return to
their source column. Maps to `AC-TASKS-COMPLETION-005.5` and
`AC-TASKS-COMPLETION-005.6`.

## Tests

| AC | Evidence |
| --- | --- |
| 005.1 | `internal/workflow/models/terminal_test.go` `TestFinalAndCompletingPredicates`; `internal/workflow/models/terminal_sql_test.go` `TestStepSQLMatchesGoPredicates` (SQLite and PostgreSQL) |
| 005.2 | Existing `internal/backendapp/office_human_atomic_*_test.go` (SQLite and PostgreSQL) unchanged in assertion; `internal/task/repository/sqlite/completion_policy_test.go` `TestPolicyRefusalLeavesTaskUnchanged` |
| 005.3 | `completion_policy_test.go` `TestStepEntryWriteDoesNotInheritAdmission`; `internal/task/service/completion_admission_test.go` `TestPostCommitWriteRunsWithoutHumanAuthority` |
| 005.4 | `internal/office/service/routine_run_completion_test.go` `TestRoutineRunClosesOnCommittedState` (custom step name, unassigned, FAILED, CANCELLED, duplicate delivery); `TestCompletingMoveWakesParentAndBlockers` |
| 005.5 | `internal/task/service/service_bulk_move_preflight_test.go` `TestBulkMoveRefusesWholeBatch` (both functions) |
| 005.6 | `service_bulk_move_preflight_test.go` `TestBulkMoveReportsPartialResult`; `internal/task/handlers/bulk_move_error_test.go`; `apps/web/hooks/use-task-workflow-move.test.ts` |

## E2E tests

- `apps/web/e2e/tests/kanban/task-multi-select-bulk-move-refused.spec.ts`
  (`chromium` project): a bulk move into a completing step with one Office task
  that has a pending approver. No card moves and the toast names the task.
  Covers `AC-TASKS-COMPLETION-005.5`.
- `apps/web/e2e/tests/kanban/mobile-task-multi-select-bulk-move-refused.spec.ts`
  (`mobile-chrome` project): the same flow through the phone multi-select toolbar.
- The partial result depends on a race, so its evidence is the Go barrier
  test and the hook unit test, not a browser test.

## Work orders

- [x] [Task 01: One final and completing step definition](task-01-step-predicate.md)
- [x] [Task 02: Typed completion-policy hook](task-02-completion-policy-hook.md)
- [x] [Task 03: Remove the service-level approval fallback](task-03-remove-service-fallback.md)
- [x] [Task 04: Completion effects from committed state](task-04-completion-effects.md)
- [x] [Task 05: Bulk move preflight and partial results](task-05-bulk-move-admission.md)

## Verification results

Implemented and verified. `make -C apps/backend build` passed. The full
`internal/task/service` suite passed, as did the focused task repository,
Office, orchestrator, handler, workflow predicate, and PostgreSQL admission and
task-transfer checks. Changed-package Go lint reported 0 issues with a 10-minute
timeout. Web unit tests (31), typecheck, i18n checks, desktop and mobile E2E,
documentation validation, and specification lint passed. The 200-task bulk
preflight benchmark completed in 1.484 seconds. An unfiltered broad backend test
run was stopped after the SQLite repository package exceeded 10 minutes; the
affected packages were verified with focused runs.

## Risks

- **Base drift.** PR #4642 may change before it merges. Re-read
  `office_completion.go` and `office_approval_guard.go` on the merged head
  before starting 02.
- **Signature churn.** 02 changes repository method signatures behind service
  interfaces. Test fakes that implement those interfaces need updates. Keep the
  nil-admission default so engine callers do not change.
- **Duplicate wakes.** 04 removes one of two blocker and parent wake paths.
  The characterization test must prove that the state-keyed path alone wakes
  for a custom step name and an unassigned task before `finalizeDone` is
  deleted.
- **Preflight cost.** 05 runs policy reads per task while holding the bulk step
  locks. Batches are small UI selections, but `BulkMoveTasks` moves a whole
  column. Measure a 200-task column in the test and record the duration.
