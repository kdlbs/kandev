---
id: "02-completion-policy-hook"
title: "Typed completion-policy hook"
status: done
wave: 2
depends_on: ["01-step-predicate"]
plan: "plan.md"
requirements:
  - REQ-TASKS-COMPLETION-005
acceptance_criteria:
  - AC-TASKS-COMPLETION-005.2
  - AC-TASKS-COMPLETION-005.3
system_design:
  - ../../specs/tasks/system-design/task-completion.md
---

# Task 02: Typed completion-policy hook

## Summary

Replace the Office-specific admission code in the task repository with a
generic `completion.Policy` hook. The service passes the human admission as an
explicit typed parameter, and Office registers its approver policy through
`backendapp`. Resolves #5250.

## In scope

- New package `internal/task/repository/completion`: `Admission`, `Scope`,
  `AdmissionKind`, `Policy`, `Querier`, and `RefusedError`.
- Task repository: `SetCompletionPolicies` and one `admitCompletionTx`, which
  runs the generic scope fence, then the completing-step check, then the
  policies. Repository write methods that can complete a task take a
  `*completion.Admission`. `CompleteOfficeTaskAsHuman` becomes
  `CompleteTaskOnFinalStep`. `lockTaskHierarchy` only locks.
  `refreshOfficeAdmissionLimitTx` keys on a non-nil admission.
- Workflow-change and exact cross-step writes preserve typed admission in the
  mutation transaction. Non-human callers pass nil explicitly.
- Completion-policy SQL uses the workflow repository's dialect binding.
- `backendapp` registers the policy through one helper and uses a named reader
  interface for approver-name resolution.
- Task service: build the admission in `MoveTaskWithOptions`,
  `UpdateTaskAsHuman`, `UpdateTaskStateAsHuman`, and `CompleteOfficeTaskAsHuman`
  (kept as the service name the Office adapter calls). Pass the admission to
  the one repository call it authorizes. Map `RefusedError{Code:
  "approvals_pending"}` to `ApprovalsPendingMoveError`.
- Delete `models.OfficeHumanCompletionIntent`, its context helpers, the string
  intent kinds, and the clears in `UpdateTask`, `MoveTaskWithOptions`,
  `enqueueTaskPublication`, `dispatchStepEntry`, and `updateTaskState`.
- New `internal/office/completionpolicy` with the approver policy (Office-only
  and `FinalStepDone`-only rules included). `backendapp` registers it and
  panics at startup when it is missing.
- Update the Office human Done system design from PR #4642
  (`docs/specs/office/system-design/human-done-final-step.md`) to describe the
  hook instead of the context intent.

## Out of scope

- Deleting the service-level fallback (Task 03).
- Bulk preflight (Task 05).

## Acceptance

- The task repository completion-admission path has no Office-specific names,
  identifiers, or context-carried authority. Office policy code remains in
  `internal/office/completionpolicy`, registered through `backendapp`.
  `grep -rn "OfficeHumanCompletionIntent" apps/backend` finds nothing.
- All `internal/backendapp/office_human_atomic_*` tests pass on SQLite and
  PostgreSQL without changing what they assert.
- A new test proves that a step-entry action writing to the same task after a
  human completing move runs with no admission. It is neither refused for scope
  nor granted human authority.

## Verification

```bash
(cd apps/backend && go test ./internal/task/repository/... ./internal/task/service/... ./internal/task/handlers/... ./internal/office/... ./internal/backendapp/...)
(cd apps/backend && KANDEV_TEST_POSTGRES_DSN="$KANDEV_TEST_POSTGRES_DSN" go test -run 'OfficeHuman|CompletionPolicy|TaskTransfer' ./internal/backendapp/... ./internal/task/repository/sqlite/...)
bash scripts/lint-go-changed $(git diff --name-only origin/main -- '*.go')
python3 scripts/list-docs.py validate
```

## Files likely touched

- New `apps/backend/internal/task/repository/completion/completion.go`
- `apps/backend/internal/task/repository/sqlite/office_completion.go` (deleted; replaced by `completion_admission.go`), `task.go`, `task_hierarchy_admission.go`, `step_entry_dispatch.go`, `base.go`, `task_transfer.go`
- `apps/backend/internal/task/repository/repoerrors/office_completion.go` (deleted)
- `apps/backend/internal/task/models/completion_gates.go`
- `apps/backend/internal/task/service/office_approval_guard.go`, `service_workflow.go`, `service_tasks.go`, `service_events.go`
- New `apps/backend/internal/office/completionpolicy/policy.go` and tests
- `apps/backend/internal/backendapp/adapters_office_completion.go`, `main.go`
- `docs/specs/office/system-design/human-done-final-step.md`

## Dependencies

Task 01.

## Risks

- Repository interface signature changes ripple into service test fakes. Keep
  nil as the non-human default so engine and orchestrator callers do not change.
- The PostgreSQL scope fence previously ran during `lockTaskHierarchy`. Moving
  it after the lock must keep the before-and-after scope comparison in
  `lockOfficeCompletionTaskScope` (made generic). The PostgreSQL transfer race
  tests are the guard.

## Parallelism

`sequential`

## Inputs

- System design: "Completion policy hook".
- PR #4642: `office_completion.go`, `office_approval_guard.go`, `adapters_office_completion.go`.

## Results

Added typed task-repository admissions and registered the Office policy through
`backendapp`. Removed context-carried human authority. SQLite and PostgreSQL
Office admission and task-transfer checks passed, including concurrent state,
explicit-state policy, session approval race, and missing-workflow refusal.
Follow-up review threaded human completion admission through the atomic
workflow-change write, made repository admission parameters explicit, reused
the dialect-aware workflow query binder, and wired the policy registration
helper in production. The human cross-workflow move regression passes for a
non-Office task; an Office move into a completing step with a pending
approver returns 409 without writes. Exact-operation move call sites are
system-attributed plugin adapters, so their human-admission rejection remains
unreachable from HTTP and WebSocket callers. The earlier work-order Results
record the PostgreSQL admission and transfer checks passing; this follow-up
could not repeat them because KANDEV_TEST_POSTGRES_DSN is unset. The full
backendapp suite passed after the active-session translation test was updated
to use the typed refusal sentinel; task repository, service, and handler
package suites also passed in the work-order run. The final serial verification
passed across task repository, service, handler, Office, and backendapp
packages, and `make -C apps/backend build` passed. The changed-Go wrapper
reported zero lint issues but exceeded its five-minute package-loading limit;
the same package set passed with a 15-minute lint timeout. The PostgreSQL
rerun remained unavailable because KANDEV_TEST_POSTGRES_DSN is unset.
