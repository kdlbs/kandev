---
system: office
requirements:
  - REQ-OFFICE-SCHEDULER-002
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001
system_design:
  - ../../specs/office/system-design/scheduler-01.md
  - ../../specs/tasks/system-design/workflow-explicit-completion-signal.md
created: 2026-09-26
status: done
---

# Implementation Plan: Assignment wake respects step auto-start eligibility

## Overview

ISSUE-5: assigning an agent to an authoritative Office task sitting on a workflow
step with no `auto_start_agent` on_enter action (Backlog, `events:{}`) still
queued a `task_assigned` run and launched the agent outside the workflow. The
agent then did work on Backlog, called `step_complete_kandev` (which returned
`accepted:true` even though the step never reads that signal), and the card
later hit a CONFLICT moving to Work because a Backlog session already existed —
followed by a second, legitimate run on Work.

Gate every producer of an assignee `task_assigned` wake behind one shared
eligibility predicate: the task's current workflow step must have an
`auto_start_agent` on_enter action. Additively report step applicability from
`step_complete_kandev` so an agent can tell a recorded-but-inert signal from
one that will actually move the task.

## Confirmed root cause

Four producers of `task_assigned` runs for an Office task's assignee never
consulted the task's current workflow step:

1. `apps/backend/internal/office/scheduler/reactivity.go` `reactToAssigneeChange`
   — always queued a wake on any assignee change (reached from the dashboard
   PATCH path). This produced the beta evidence: two runs 49s apart for the
   same task, the first with no `workflow_step_id` (Backlog reactivity path),
   the second with `workflow_step_id` = Work (the orchestrator's legitimate
   `on_enter auto_start_agent`).
2. `apps/backend/internal/office/service/event_subscribers.go`
   `queueTaskAssignedRun` — checked only `IsFromOffice` and a non-empty agent.
3. `apps/backend/internal/office/service/scheduler_recovery.go`
   `recoverUnstartedTasks` — re-queued `task_assigned` for any Office `TODO`
   task with a runner and no run, with no step filter.
4. `apps/backend/internal/office/onboarding/service.go`
   `maybeCreateOnboardingTask` — onboarding never sets `StartAgent`/`PlanMode`
   when creating the task, so a custom workflow's start step and auto-start
   step can genuinely differ.

## Backend

### Shared eligibility predicate

Add `apps/backend/internal/office/shared/assignment_eligibility.go`:
`IsAssignmentWakeEligible(ctx, log, stepIDs, steps, taskID, source) bool`, using
the task's current workflow step's
`wfmodels.WorkflowStep.HasOnEnterAction(wfmodels.OnEnterAutoStartAgent)` — the
same predicate the orchestrator's own auto-start path already uses. Empty
`workflow_step_id` is eligible (no workflow to respect). A step lookup error,
nil step getter, or unresolved step ID fails open (queue the wake) with a WARN
log; an eligible/ineligible outcome that resolves cleanly is logged at Info
(`office.assignment_wake.step_ineligible`).

### Producer wiring

- `scheduler/reactivity.go`: `reactToAssigneeChange` gains a `ctx` parameter and
  checks eligibility after the previous-assignee interrupt is set, before
  `queue(...)`. The interrupt is unaffected — only the new wake is gated.
- `service/event_subscribers.go`: `queueTaskAssignedRun` checks eligibility
  after the existing `IsFromOffice` guard.
- `service/scheduler_recovery.go`: `recoverUnstartedTasks` skips ineligible
  tasks in its per-task loop.
- `onboarding/service.go`: `maybeCreateOnboardingTask` checks eligibility
  before queueing the initial wake; the task itself is still created either
  way — only the wake is gated.
- `service/service.go` and `onboarding/service.go` each gain a
  `SetWorkflowStepGetter` setter (optional; nil fails open).
- `backendapp/main.go` wires `services.Workflow` (the existing workflow
  service, structurally satisfying `shared.AssignmentStepGetter`) into
  `Workspaces.SetWorkflowStepGetter` (shared singleton with `TreeControls`,
  wiring both the event-subscriber and recovery-sweep paths in one call) and
  `Onboarding.SetWorkflowStepGetter`.

### `step_complete_kandev` applicability field (Slice B)

`internal/mcp/handlers/handlers.go` `handleStepComplete` additively sets
`advances` (bool, from the current step's `AutoAdvanceRequiresSignal`) and,
when `false`, a `note` explaining the signal was recorded but will not move the
task. `accepted:true` is unchanged. A step lookup failure omits both fields
rather than guessing.

## Tests

- `internal/office/scheduler/reactivity_step_eligibility_test.go` —
  `TestBetaAssignmentEligibility` table: Backlog suppresses the wake but not
  the interrupt; Work queues exactly one wake; same-agent reassignment on Work
  still wakes with no interrupt; same-agent on Backlog wakes nothing; empty
  step and step-getter error both fail open.
- `internal/office/service/assignment_step_eligibility_test.go` —
  `queueTaskAssignedRun` via task-created/task-updated events: Backlog task
  queues 0 runs, Work task queues 1.
- `internal/office/service/scheduler_recovery_step_eligibility_test.go` —
  recovery sweep: Backlog `TODO` task queues 0 runs.
- `internal/office/onboarding/assignment_step_eligibility_test.go` — onboarding
  task landing on Backlog skips the wake but still creates the task; landing on
  an auto-start step queues the wake.
- `internal/mcp/handlers/step_complete_advances_test.go` — signal-gated step
  returns `advances:true` with no `note`; non-signal-gated step returns
  `advances:false` with a `note`; an unresolved step omits both fields.

All five test files were written first and confirmed to fail against the
pre-fix production code for the correct behavioral reason (not a compile
error), then confirmed green after the corresponding production change.

## E2E Tests

None. The user-visible outcome changes (assigning on Backlog no longer starts
an agent or session), but there is no UI surface change — the fix is entirely
at the producer boundary, and Go tests prove it directly.

## Verification Results

- `cd apps/backend && go test ./internal/office/... ./internal/workflow/... ./internal/mcp/... -count=1`
  — all packages pass (`internal/office/scheduler`, `internal/office/service`,
  `internal/office/onboarding`, `internal/office/shared`, `internal/mcp/handlers`,
  `internal/workflow/...`, and every sibling office/workflow/mcp package).
- `cd apps/backend && go build ./...` — clean.
- `cd apps/backend && go test ./internal/backendapp/... -count=1` — green
  (`main.go` wiring change).
- `cd apps/backend && make lint` (`golangci-lint run ./...`) — `0 issues.`
- `cd apps/backend && gofmt -l <touched files>` — no output (already formatted).
- `python3 scripts/lint-spec-files.py --all` and
  `python3 scripts/list-docs.py validate` — passed after adding this plan and
  the requirement/system-design updates.

## Implementation Waves And Parallel Candidates

Wave 1 (sequential):

- [x] [task-01-assignment-wake-step-eligibility](task-01-assignment-wake-step-eligibility.md)

Single task: the shared predicate, all four producer gates, and the additive
`step_complete_kandev` field are one cohesive change with one shared helper: no
parallel-safe split.

## Risks

- A future fifth `task_assigned` producer could reintroduce the defect by not
  calling `shared.IsAssignmentWakeEligible`. There is no compile-time
  enforcement; the shared package and its doc comments are the guardrail.
- Fail-open on lookup failure means a step-getter outage temporarily reverts to
  pre-fix behavior (wakes ungated) rather than silently dropping legitimate
  work — an intentional tradeoff, not a gap.

## Out of scope

- The live-session move guard (CONFLICT on move while a session runs).
- `stepIsSignalGated` / orchestrator signal handling; `accepted` semantics.
- Assignment rate limiting, dedup keys, generation, coalescing.
- Review/Approval participant fan-out (`queue_run_for_each_participant`); the
  orchestrator's `queueOfficeAutoStartRun` auto-start path.
- ISSUE-7 (New Task dialog assignee) — a separate, unrelated card.
