---
id: "02-blocker-failure-escalation"
title: "Escalate Office blocker failures once to the responsible agent"
status: done
wave: 2
depends_on:
  - "01-success-only-classification"
plan: "plan.md"
requirements:
  - REQ-OFFICE-BLOCKER-POLICY-002
acceptance_criteria:
  - AC-OFFICE-BLOCKER-POLICY-002.1
  - AC-OFFICE-BLOCKER-POLICY-002.2
  - AC-OFFICE-BLOCKER-POLICY-002.3
  - AC-OFFICE-BLOCKER-POLICY-002.4
  - AC-OFFICE-BLOCKER-POLICY-002.5
  - AC-OFFICE-BLOCKER-POLICY-002.6
  - AC-OFFICE-BLOCKER-POLICY-002.7
  - AC-OFFICE-BLOCKER-POLICY-002.8
system_design:
  - ../../specs/office/system-design/blocker-terminal-policy.md
---

# Task 02: Escalate blocker failures once

## Summary

Make every committed transition into `FAILED` or `CANCELLED` observable on
`task.state_changed`. Then add the `task_blocker_failed` subscriber. It wakes the
runner of the blocker's parent task, or the workspace CEO when there is none, once
per failure occurrence, listing every held dependent.

## In scope

- Confirm that the orchestrator's runtime `FAILED` writers already publish
  `task.state_changed`: they write through `backendapp.taskRepositoryAdapter`,
  which calls the publishing task service writers. No orchestrator change.
- `office/shared/runreasons.go`, `wakereasons.go`, `runs/service/metrics_vars.go`,
  the unattended list in `shared/runprovenance_completeness_test.go`
  (`AC-OFFICE-BUDGET-007.4` amended), and the `scheduler/run.go` and
  `service/run.go` aliases for `task_blocker_failed`.
- New `office/scheduler/blocker_failure.go`: `RegisterBlockerFailureSubscriber`,
  the transition filter, the stale re-read, held-dependent selection, recipient
  resolution through `GetTaskParentID`, `GetTaskAssignee`, and
  `ListAgentInstancesFiltered`, the occurrence key, and `QueueRunCtx` with
  `SkipCoalesce`.
- Registration in `backendapp.activateOfficeServices`. A subscription error fails
  activation.
- `office/service/prompt_blocker_failed.go` `buildBlockerFailedPrompt` and its
  payload enrichment.
- After implementation: add a row to the spec table in
  `apps/backend/internal/office/AGENTS.md` and update its count. Promote the specs
  and plan per the spec-driven-development workflow.

## Out of scope

- Actor plumbing on task events, sweeps, `agent_error` changes, and Inbox or UI
  changes.

## Acceptance

1. Publication: the runtime callback and the early launch-failure writer each emit
   exactly one `task.state_changed` with the correct `old_state`/`new_state`, and
   an archived skip emits none. The existing subscriber packages stay green.
2. Escalation: a failure transition with held dependents persists one
   `task_blocker_failed` run for the parent's runner, with the CEO as fallback. The
   payload lists every held dependent. There is no run for: no held dependents,
   `FAILED`→`CANCELLED`, a stale event, or no recipient (which logs a Warn).
   Duplicate delivery gives one run. A refail after a retry gives a second run. A
   recipient with an in-flight run gets a coalesced request, not a new run.
3. The registry tests and a prompt test pass. A real-bus test proves the
   subscriber's registration through the production activation path.

## Verification

From `apps/backend`:

```bash
go test -trimpath -race ./internal/office/shared/ ./internal/office/scheduler/ \
  ./internal/office/service/ ./internal/runs/service/ -count=1
go test -trimpath -race ./internal/orchestrator/ -run 'LaunchFailure|TaskStateChange|ChildrenCompleted' -count=1
go test -trimpath -race ./internal/task/statussummary/ ./internal/coordinator/... -count=1
go test -trimpath -race ./internal/backendapp/ -run 'Office' -count=1
```

From the repo root:

```bash
bash scripts/lint-go-changed <changed Go files>
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Likely files

- `apps/backend/internal/orchestrator/service.go`, `task_operations.go`
- `apps/backend/internal/office/scheduler/blocker_failure.go` (new) and its test
- `apps/backend/internal/office/shared/{runreasons,wakereasons,runprovenance}.go`
- `apps/backend/internal/runs/service/metrics_vars.go`
- `apps/backend/internal/office/service/prompt_builder.go`
- `apps/backend/internal/backendapp/` (Office activation)
- New: `apps/backend/internal/office/service/blocker_failure_escalation_test.go`

## Risks

- New `FAILED` publications reach every `task.state_changed` subscriber. Check
  the status-summary projector, the coordinator wake recorder, the outcomes
  recorder, and the handoff subscriber for unwanted reactions, such as duplicate
  activity rows. The task service's state-activity logger runs only in
  `publishTaskEvent` with `taskStateActivity`. Confirm that the orchestrator
  publisher does not write a duplicate activity row.
- The `updated_at` payload field must be the committed transition time. Assert it
  in the duplicate and refail tests.

## Results

Done, with four corrections to the original design, now reflected in the system
design:

- No publisher was added. Production runtime `FAILED` writes already publish
  through the task service, and existing task-service tests pin it. The duplicate
  activity-row risk therefore does not arise.
- The parent runner is woken on the parent task, and the CEO fallback is taskless.
  The run never names the failed blocker as its task, so it cannot launch the
  recipient on the failed task.
- Escalations skip queue-time coalescing, which would otherwise replace one
  blocker's payload with another's for the same recipient. Claim-time merge still
  applies. `task_blocker_failed` classifies as unattended, like `agent_error`.
- Office status changes (Office UI and agent runtime) never publish
  `task.state_changed`; manual testing found a UI cancel that escalated nothing.
  `reactToStatusChange` now runs the same evaluation on `cancelled`, and the
  occurrence key uses the blocker row's committed `updated_at` so both triggers
  share one identity. Tree cancel (`task.tree_hold_created`, cancel mode) runs it
  for each member it moved to `CANCELLED`, and a terminal parent's runner is no
  longer a recipient (CEO fallback). Making Office state writes publish
  `task.state_changed` is tracked in issue #5274.

Tests: `blocker_failure_escalation_test.go` (recipient, CEO fallback, no-recipient,
non-held dependents, `FAILED`→`CANCELLED`, stale event, success, duplicate delivery,
refail, distinct blockers, Office status path as user and agent, Office cancel of
a failed blocker, terminal parent, tree cancel, pause hold, tree cancel of a failed
blocker), `prompt_blocker_failed_test.go`, and
`backendapp/office_blocker_subscriber_wiring_test.go` (activation registration). The
activation check is a source guard, matching `scheduler_wiring_test.go`, rather than
a real-bus boot test.
