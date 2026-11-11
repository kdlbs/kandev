---
status: current
system: office
requirements:
  - REQ-OFFICE-BLOCKER-POLICY-001
  - REQ-OFFICE-BLOCKER-POLICY-002
---

# Office Blocker Terminal Policy System Design

## Purpose and boundaries

Office reads blocker state through one repository classifier and applies it in three
places: assignment admission, the resolution cascade, and the legacy step-move
resolution path. This design replaces "terminal" with "satisfied" in that
classifier, removes the step-name shortcut, and adds one escalation producer for
failure transitions. Kanban dependency policy (`task/service/service_dependencies.go`)
is not changed or imported. Office keeps its own reader, which now agrees with Kanban
about which states satisfy a blocker.

Generic run admission, claim-time merge, pause and replay, rate limits, and causation
stay owned by the shared runs service and the
[global scheduler decision](../../../decisions/2026-08-01-global-run-scheduler-ownership.md).

## Requirement mapping

| Requirement / criteria | Section |
| --- | --- |
| AC-OFFICE-BLOCKER-POLICY-001.1, .5 | [Single classifier](#single-classifier); [Admission](#admission) |
| AC-OFFICE-BLOCKER-POLICY-001.2, .3 | [Resolution cascade](#resolution-cascade) |
| AC-OFFICE-BLOCKER-POLICY-001.4 | [Legacy step-move path](#legacy-step-move-path) |
| AC-OFFICE-BLOCKER-POLICY-001.6 | [Unchanged surfaces](#unchanged-surfaces) |
| AC-OFFICE-BLOCKER-POLICY-002.1, .2, .5 | [Failure escalation producer](#failure-escalation-producer) |
| AC-OFFICE-BLOCKER-POLICY-002.3 | [Delivery of runtime state writes](#delivery-of-runtime-state-writes) |
| AC-OFFICE-BLOCKER-POLICY-002.4, .6 | [Idempotency and coalescing](#idempotency-and-coalescing) |
| AC-OFFICE-BLOCKER-POLICY-002.7, .8 | [Recipient resolution](#recipient-resolution); [Prompt and registry](#prompt-and-registry) |

## Current behavior (grounding)

- `office/repository/sqlite/blockers.go` `IsTaskInTerminalStep` returns true for
  `COMPLETED`, `FAILED` and `CANCELLED`. It has three callers:
  `shared.IsAssignmentWakeEligible`, `scheduler.allBlockersResolvedExcept`, and
  `service.resolveAndWakeIfUnblocked`.
- `scheduler/task_completion.go` and `reactToStatusChange` (on `done`) run
  `cascadeBlockersResolved` only on success.
- `service.handleTaskMoved` runs `finalizeDone` → `queueBlockersResolvedRuns` when
  `categorizeStep` reports Done or Cancelled by step name.
- The orchestrator's runtime state callback (`SetOnTaskStateChange` in
  `orchestrator/service.go`) and the early launch-failure writer
  (`recordSessionLaunchFailure`) write `FAILED` through `backendapp.taskRepositoryAdapter`,
  which delegates to the task service's conditional writers. Those writers already
  publish `task.state_changed` with the previous state.
- Office status changes (`dashboard.UpdateTaskStatus`, which the Office UI and the
  agent runtime `UpdateTaskStatusAsAgent` action share) write through the task
  repository and publish only `office.task.status_changed`, never
  `task.state_changed`. They reach the scheduler through `reactToStatusChange`.
- Tree cancel (`service.CancelTaskTree`) bulk-writes `CANCELLED` through
  `BulkUpdateTaskState` under a restorable hold. It publishes no state event, only
  `task.tree_hold_created` with the hold.
- Making every Office state write publish `task.state_changed` is tracked
  separately in issue #5274.

## Single classifier

Replace `IsTaskInTerminalStep` with one reader:

```go
type BlockerStatus int // BlockerPending, BlockerSatisfied, BlockerFailed
func (r *Repository) GetBlockerStatus(ctx, taskID string) (BlockerStatus, error)
```

`COMPLETED` maps to satisfied, `FAILED`/`CANCELLED` map to failed, and every other
state maps to pending. A missing row returns `sql.ErrNoRows`, as it does today, so
callers keep the unknown-read rule. The reader ignores `archived_at`. Remove
`IsTaskInTerminalStep` and update the `shared.AssignmentTaskLookup` and scheduler
repository interfaces. No other production caller exists. `GetTaskTerminalStatus`
and the children helpers are unrelated and stay as they are.

## Admission

`shared.IsAssignmentWakeEligible` refuses when any readable blocker is not
satisfied, whether it is pending or failed. Read errors keep today's rule: log at
Warn, continue scanning, and preserve admission only when no readable blocker is
unsatisfied. Producers, logging keys, interrupt order, and recovery quota handling
are unchanged. The Info log for a refusal adds the field `blocker_status`
(`pending` or `failed`).

## Resolution cascade

`allBlockersResolvedExcept` and `resolveAndWakeIfUnblocked` require every blocker
other than the triggering one to be satisfied. The triggering blocker is already
known to be `COMPLETED` on the success paths. Keys
(`task_blockers_resolved:<dependent>:<runner>:<BlockerDigest>`), payloads, and the
Office-dependent isolation in `task_completion.go` are unchanged. A retried blocker
that reaches `COMPLETED` re-enters the same success bridge, which gives
AC-001.3 without new code.

## Legacy step-move path

`resolveAndWakeIfUnblocked` also reads the triggering blocker through
`GetBlockerStatus` and returns without waking unless it is satisfied. A move into a
step named Cancelled therefore wakes nobody unless the task's state is `COMPLETED`.
This happens when the orchestrator's terminal-step completion wrote `done`, in
which case the success bridge produces the same digest key and the two paths
converge. `finalizeDone`'s routine-run sync and parent wake are unchanged.

## Failure escalation producer

Add `scheduler/blocker_failure.go` with one evaluation,
`escalateBlockerFailure(blocker, failedState)`, and two triggers:

- `RegisterBlockerFailureSubscriber(eb)` subscribes to `events.TaskStateChanged`
  for task-service writes. Register it once in `backendapp.activateOfficeServices`
  next to `RegisterTaskCompletionSubscriber`. A subscription error fails
  activation. It decodes `task_id`, `old_state` and `new_state`, and returns when
  any is empty, when `new_state` is not `FAILED`/`CANCELLED`, or when `old_state`
  is `FAILED`/`CANCELLED` (AC-002.5).
- `reactToStatusChange` evaluates an Office status change to `cancelled` (the
  Office status vocabulary has no `failed`), unless the pre-update state was
  already `FAILED` or `CANCELLED` (AC-002.5). Evaluation errors are logged; they
  do not fail the status mutation.
- The same subscriber registration also subscribes to `task.tree_hold_created`.
  For a `cancel` hold it lists the hold members and evaluates each member the
  hold moved to `CANCELLED`: members whose recorded prior status was `FAILED` or
  `CANCELLED` are skipped (AC-002.5). Members inside the tree are now terminal,
  so only dependents outside the tree count as held. `pause` holds change no
  state and are ignored.

The three triggers are disjoint: neither an Office status write nor a tree
cancel publishes `task.state_changed`. All derive the same occurrence key, so a
transition seen by more than one would still queue one run.

Evaluation steps:

1. Re-read the blocker with `GetTaskExecutionFields`. Return when its current state
   no longer equals the failed state. This keeps a stale event from waking anyone
   after a fast retry.
2. Read the blocker row's committed `updated_at` (`GetTaskUpdatedAt`) as the
   occurrence.
3. Run `ListTasksBlockedBy(blocker)`. Keep the dependents that are Office-owned, in
   the blocker's workspace, unarchived, and not in a terminal state. When none
   remain, return (AC-002.2).
   A dependency edge whose dependent row is missing is skipped.
4. Resolve the recipient ([below](#recipient-resolution)).
5. Queue through `QueueRunCtx` with `Reason: task_blocker_failed`, the task ID
   chosen by recipient resolution, `WorkspaceID`, the system actor, and the key
   defined below.

The blocker may be a Kanban task. As in the success bridge, Office owns the outcome
for its dependents, not the blocker's type.

## Recipient resolution

1. When the blocker has a `parent_id`, read the parent with
   `GetTaskExecutionFields`. When the parent is not `COMPLETED`, `FAILED` or
   `CANCELLED` and has a runner, that runner is the recipient, and the run's
   `task_id` is the parent, as for `task_children_completed`. A terminal parent no
   longer coordinates its children; after a tree cancel the parent is usually
   inside the cancelled tree. The run never names the failed blocker as its task,
   so the wake cannot launch the recipient on the failed task.
2. Otherwise, including a missing parent row, use the first CEO agent in the workspace with a taskless run. Use the
   same `ListAgentInstancesFiltered(role=ceo)` lookup that `queueCEOAgentError`
   uses. Step 1 reads `parent_id` with `GetTaskParentID`.
3. When neither step yields a recipient, log
   `office.blocker_failed.unescalated` at Warn with the blocker and dependent IDs,
   then return (AC-002.7).

There is no actor check, because `task.state_changed` carries no actor and both
triggers must behave the same. An Office
agent that cancels the blocker from its own run has that run in flight, so
claim-time merge absorbs the wake into it (AC-002.6). A human canceller, or an
agent session outside any Office run, is treated like any other cause, and the
recipient is woken.

## Idempotency and coalescing

Key: `task_blocker_failed:<blocker_id>:<recipient>:<updated_at>`, where
`updated_at` is the blocker row's committed `updated_at`, read after the state
check and formatted as UTC RFC3339Nano. Duplicate delivery of one transition
produces one key, whichever trigger observes it. A
later failure after a retry commits a new `updated_at`, so it is a new occurrence
(AC-002.4). The request sets `SkipCoalesce`. Queue-time coalescing replaces the
queued row's payload, so two blockers failing within the 5-second window would
otherwise drop the first blocker's escalation. Claim-time merge
(AC-OFFICE-SCHEDULER-001.8) still applies, so an in-flight run for the recipient,
including a concurrent `agent_error`, absorbs the wake (AC-002.6). No cross-reason
correlation is added.

## Delivery of runtime state writes

Task-service writes publish `task.state_changed` with the previous state. The
orchestrator's runtime callback and early launch-failure writer go through
`backendapp.taskRepositoryAdapter`, whose `UpdateTaskStateIfNotArchived` and
`UpdateTaskStateIfPrimarySessionState` call the task service writers that publish
on a committed change and stay silent for an archived skip. The existing
task-service tests pin that publication. Office status changes and tree cancel
are covered by their own triggers instead. This design adds no publisher.

`RestoreTaskTree` returns members to their prior states. An escalation already
queued for a restored blocker is then stale, which is harmless: restoring is the
retry it asks for.

## Prompt and registry

- Declare `RunReasonTaskBlockerFailed = "task_blocker_failed"` in
  `office/shared/runreasons.go` and alias it in `scheduler/run.go` and
  `service/run.go`.
- Register it as event priority in `shared/wakereasons.go` and in
  `runs/service/metrics_vars.go`. The existing registry tests enforce these
  declarations. Like `agent_error`, it stays off the attended allowlist in
  `shared/runprovenance.go`: a system escalation is unattended.
- Payload: `{blocker_task_id, blocker_state, held_dependent_ids[]}`.
- `prompt_blocker_failed.go` adds `buildBlockerFailedPrompt`, dispatched from
  `BuildPrompt`, and its payload enrichment. The held-dependent list renders at most
  20 entries plus a remainder count. It names the
  blocker and its state, lists the held dependents, and states that they stay held
  until the blocker is retried or replaced, or its edge is removed. Agent prompts are
  English by design, so no i18n is involved.

## Unchanged surfaces

Kanban `DependencyStatusForTask`/`dependencyTally`, `AreAllChildrenTerminal` and
the wave helpers, the `agent_error` producer, the Inbox Failed query, and every
other wake reason stay as they are.

## Failure and recovery

- When a read fails in the escalation handler, it returns an error through the
  existing subscriber path. It queues nothing, and no retry loop is added.
- If a `task.state_changed` publication itself fails, that failure transition is
  not escalated. The requirement excludes sweeps. The dependent stays held, and
  the Inbox Failed bucket still lists a `FAILED` blocker.
- A known failed blocker always holds admission. Recovery skips the dependent under
  its existing quota rules.

## Persistence and migration

No schema, flag, or setting changes. Existing chains that relied on a failed blocker
releasing its dependent now stay held, and their coordinator is woken. This is the
intended behavior change.

## Observability

Structured logs only: `office.assignment_wake.blocker_pending` gains
`blocker_status`, and `office.blocker_failed.queued` and
`office.blocker_failed.unescalated` are added. Run-reason counters come from the
existing `metrics_vars` registry. No metric labels are added.

## Testing

Real-schema fixtures from `service/assignment_blocker_*_test.go` cover persisted
edges, real writers, the real bus, and actual run admission:

- Admission matrix: failed and cancelled blockers hold across all three producers,
  while mixed completed and pending sets keep their existing outcomes.
- Order independence: B fails then A completes, and A completes then B fails. Both
  give zero `task_blockers_resolved`. B is then retried to `COMPLETED`, which gives
  one.
- Step path: a move to a Cancelled-named step without `COMPLETED` gives zero.
- Escalation: one run for a parent's runner, CEO fallback (no parent, runner-less
  parent, terminal parent), Office status cancel as user and agent, tree cancel
  (outside dependents only; pause hold and already-failed member → zero), no
  recipient → Warn and
  zero, no held dependents → zero, duplicate delivery → one, refail after retry →
  two, two distinct blockers → two, `FAILED`→`CANCELLED` → zero, and terminal,
  archived, or Kanban dependents → zero.
- Activation: a guard test requires `activateOfficeServices` to register the
  subscriber and fail activation on a subscription error.
- Registry and prompt: the existing wake-reason registry tests, plus a prompt test.

## Related ADRs

- [Global run scheduler ownership](../../../decisions/2026-08-01-global-run-scheduler-ownership.md)
