---
id: "01-step-predicate"
title: "One final and completing step definition"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-COMPLETION-005
acceptance_criteria:
  - AC-TASKS-COMPLETION-005.1
system_design:
  - ../../specs/tasks/system-design/task-completion.md
---

# Task 01: One final and completing step definition

## Summary

Define "final step" and "completing step" once in `internal/workflow/models`,
in both Go and SQL, and derive every existing check from those definitions.
Resolves #5251.

## In scope

- Add `IsFinalStep(step, next)`. Rename `IsTerminalStep` to
  `CompletesTaskOnEnter` and update all five production callers.
- Add `FinalStepSQL(alias)` and `CompletingStepSQL(alias)` predicate fragments.
- Use the fragments in `officeCompletionStepPolicyTx` and
  `updateHumanTaskState`. The explicit Done check becomes "current step
  satisfies `FinalStepSQL`" instead of "highest position, then highest id".
- Rename Office `IsTaskWorkflowStepTerminal` to `IsTaskOnFinalStep` and
  implement it with `FinalStepSQL`. Rename `IsTaskInTerminalStep` to
  `IsTaskInTerminalState`.
- Add a caller comment that says whether the caller asks the final-step
  question or the completing-step question.

## Out of scope

- The legacy name predicate `IsTerminalStepName`. It stays private to
  migration and portable import.
- Any change in which steps complete tasks.

## Acceptance

- One table-driven fixture set, including tied maximum positions, an unchecked
  final step, and a checked non-final step, gives identical answers from the Go
  predicates and the SQL fragments on SQLite and PostgreSQL.
- `grep -rn "position>\|position >" apps/backend/internal --include=*.go` finds
  no remaining hand-written final-step SQL outside `internal/workflow/models`
  and migrations.

## Verification

```bash
(cd apps/backend && go test ./internal/workflow/models/... ./internal/office/repository/sqlite/... ./internal/office/dashboard/... ./internal/task/repository/sqlite/... ./internal/orchestrator/... ./internal/task/service/...)
(cd apps/backend && KANDEV_TEST_POSTGRES_DSN="$KANDEV_TEST_POSTGRES_DSN" go test -run 'TestStepSQLMatchesGoPredicates' ./internal/workflow/models/...)
bash scripts/lint-go-changed $(git diff --name-only origin/main -- '*.go')
```

## Files likely touched

- `apps/backend/internal/workflow/models/terminal.go`, `terminal_test.go`, new `terminal_sql_test.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`, `event_handlers_children_completed.go`, `workflow_store.go`, `task_launch_workflow.go`
- `apps/backend/internal/task/service/service_workflow.go`
- `apps/backend/internal/task/repository/sqlite/office_completion.go` (PR #4642)
- `apps/backend/internal/office/repository/sqlite/participants.go`, `blockers.go`, `terminal_step_test.go`
- `apps/backend/internal/office/dashboard/service.go`, `service_tasks.go`

## Dependencies

PR #4642 merged, or this work order applied on its branch.

## Risks

- The tie-break change for explicit Done: with two steps tied at the maximum
  position, both now count as final. Before, only the step with the highest ID
  counted. Cover this in the fixture and in a dashboard test.

## Parallelism

`sequential`

## Inputs

- System design: "Completing-step predicate".
- Existing `terminal_test.go` and `office/repository/sqlite/terminal_step_test.go`.

## Results

Added the shared Go and SQL definitions for final and completing steps, and
migrated the workflow, Office, and task callers. Predicate parity passed on
SQLite and PostgreSQL. Focused Office, orchestrator, and service tests passed.
