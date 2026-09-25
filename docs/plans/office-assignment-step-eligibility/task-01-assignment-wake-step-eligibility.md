---
id: "01-assignment-wake-step-eligibility"
title: "Gate assignment wakes on workflow step auto-start eligibility"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-SCHEDULER-002
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001
system_design:
  - docs/specs/office/system-design/scheduler-01.md
  - docs/specs/tasks/system-design/workflow-explicit-completion-signal.md
---

# Task 01: Gate assignment wakes on workflow step auto-start eligibility

## Intent

Stop the four `task_assigned` producers from launching an agent on a workflow
step that never decided to run it, and let an agent distinguish a recorded
completion signal that will actually move the task from one that will not.

## Acceptance

- Assigning, reassigning, creating, or updating an Office task whose current
  step has no `auto_start_agent` on_enter action queues zero `task_assigned`
  runs, across reactivity, event-subscriber, and recovery-sweep producers. A
  reassignment's interrupt of the previous assignee's session still fires.
- The same actions on a step with an `auto_start_agent` on_enter action queue
  exactly one run, unchanged from prior behavior.
- A task with no workflow step bound is eligible (unchanged behavior).
- A step lookup failure fails open (queues the wake) and logs at Warn; a clean
  eligible/ineligible resolution logs at Info.
- Onboarding's initial task wake is gated by the same predicate; the task is
  still created regardless of eligibility.
- `step_complete_kandev` additively returns `advances` (bool) and, when false,
  a `note`, without changing `accepted:true`. A step lookup failure omits both
  fields.

## Files likely touched

- `apps/backend/internal/office/shared/assignment_eligibility.go` (new)
- `apps/backend/internal/office/scheduler/reactivity.go`
- `apps/backend/internal/office/scheduler/reactivity_step_eligibility_test.go` (new)
- `apps/backend/internal/office/scheduler/assignment_rate_limit_inscope_producer_test.go`
- `apps/backend/internal/office/scheduler/reactivity_producer_key_audit_test.go`
- `apps/backend/internal/office/service/service.go`
- `apps/backend/internal/office/service/event_subscribers.go`
- `apps/backend/internal/office/service/scheduler_recovery.go`
- `apps/backend/internal/office/service/assignment_step_eligibility_test.go` (new)
- `apps/backend/internal/office/service/scheduler_recovery_step_eligibility_test.go` (new)
- `apps/backend/internal/office/onboarding/service.go`
- `apps/backend/internal/office/onboarding/assignment_step_eligibility_test.go` (new)
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/step_complete_advances_test.go` (new)
- `apps/backend/internal/backendapp/main.go`

## Dependencies

None.

## Parallelism

Sequential. The shared predicate, its four call sites, and the wiring in
`main.go` are one cohesive change.

## Inputs

- Spec: `docs/specs/office/requirements/scheduler.md`
  (`REQ-OFFICE-SCHEDULER-002`) and
  `docs/specs/tasks/requirements/workflow-explicit-completion-signal.md`
  (`REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001`).
- Beta evidence: two `task_assigned` runs 49s apart for the same task, the
  first with no `workflow_step_id` while the card sat on Backlog, the second
  with `workflow_step_id` = Work.
- Existing predicate: `wfmodels.WorkflowStep.HasOnEnterAction`, already used by
  the orchestrator's own auto-start path.

## Verification

Write the eligibility regressions first and confirm they fail against
pre-fix production code:

```bash
cd apps/backend && go test ./internal/office/scheduler/... -run TestBetaAssignmentEligibility -count=1 -v
cd apps/backend && go test ./internal/office/service/... -run 'TestQueueTaskAssignedRun_StepEligibility|TestOfficeRecoveryHandler_StepEligibility' -count=1 -v
cd apps/backend && go test ./internal/office/onboarding/... -run 'TestMaybeCreateOnboardingTask_Step' -count=1 -v
cd apps/backend && go test ./internal/mcp/handlers/... -run TestHandleStepComplete_Advances -count=1 -v
```

After the fix, run the full scoped suite:

```bash
cd apps/backend && go test ./internal/office/... ./internal/workflow/... ./internal/mcp/... -count=1
cd apps/backend && make lint
```

## Output contract

Report the exact changed files, the red-phase failure reason for each new test
file, and the green-phase results. Update this task and `plan.md` in the same
conversation.

## Results

- Red phase: for each of the four producer gates, temporarily reverting the
  production check reproduced the exact wrong-run-queued failure in the
  corresponding new test (`reactivity_step_eligibility_test.go`:
  `backlog_step_suppresses...` and `same-agent...backlog...` cases;
  `assignment_step_eligibility_test.go`; `scheduler_recovery_step_eligibility_test.go`;
  `assignment_step_eligibility_test.go` in `onboarding`). For Slice B,
  temporarily removing the `advances`/`note` block reproduced "does not
  contain 'advances'" in both cases of
  `TestHandleStepComplete_AdvancesFieldReflectsAutoAdvanceRequiresSignal`,
  while the lookup-failure omission test still passed as expected (a no-op
  regardless of the gate).
- Green phase:
  `cd apps/backend && go test ./internal/office/... ./internal/workflow/... ./internal/mcp/... -count=1`
  — all packages pass. `go build ./...` clean.
  `go test ./internal/backendapp/... -count=1` green.
  `make lint` (`golangci-lint run ./...`) — `0 issues.`
