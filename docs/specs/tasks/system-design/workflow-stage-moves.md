---
status: draft
system: tasks
requirements:
  - REQ-TASKS-WORKFLOW-STAGE-MOVES-001
  - REQ-TASKS-WORKFLOW-STAGE-MOVES-002
  - REQ-TASKS-WORKFLOW-STAGE-MOVES-003
  - REQ-TASKS-WORKFLOW-STAGE-MOVES-004
---

# Workflow Stage Move Integrity System Design

## Purpose and boundaries

The task system owns workflow-step transitions, the transition ledger, the
WIP queue, and the agent move surface. This design adds four narrow guards to
existing paths. It adds no endpoint, event, or table. It adds two columns to
the existing `pending_moves` table and one field to an existing task metadata
marker.

It does not change `on_enter` dispatch ownership, the ceiling admission order,
the completion-signal response contract, or HTTP and WebSocket move
validation.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-WORKFLOW-STAGE-MOVES-001` | [Deferred move origin](#deferred-move-origin) |
| `REQ-TASKS-WORKFLOW-STAGE-MOVES-002` | [Same-step admission entry](#same-step-admission-entry) |
| `REQ-TASKS-WORKFLOW-STAGE-MOVES-003` | [Entry prompt before signal advance](#entry-prompt-before-signal-advance) |
| `REQ-TASKS-WORKFLOW-STAGE-MOVES-004` | [Agent move target validation](#agent-move-target-validation) |

## Deferred move origin

### Components

- `mcp/handlers.(*Handlers).deferMoveTask` records a pending move when the
  caller's turn is active.
- `orchestrator/messagequeue.PendingMove` and the `pending_moves` table persist
  it through turn end, WIP promotion, snapshot restore, and restart.
- `orchestrator.(*Service).applyPendingMove` replays it when the turn ends.
- `task/repository/sqlite.(*Repository).updateTaskWithWorkflowStepAdmissionAttempt`
  commits the deferred step change atomically with its source check.

### Data model

`PendingMove` gains two additive fields:

| Field | Column | Meaning |
| --- | --- | --- |
| `FromStepID` | `from_step_id TEXT NOT NULL DEFAULT ''` | Step the task occupied when the move was recorded. |
| `FromTransitionID` | `from_transition_id BIGINT NOT NULL DEFAULT 0` | Task's latest `task_step_transitions.id` when the move was recorded. |

Fresh databases create both columns in the table DDL. Existing databases add
them with idempotent `ALTER TABLE ... ADD COLUMN`. Every read, upsert,
snapshot restore, and sweep listing of `pending_moves` carries both values.
Legacy rows decode as the empty string and zero. The transition reference uses
`BIGINT` so PostgreSQL accepts the full signed 64-bit ledger identity range.

### Control flow

1. `deferMoveTask` reads the task it already validates, stores its current step
   as `FromStepID`, and reads `GetLatestTaskStepTransitionID` for
   `FromTransitionID`. A failed ledger read logs a warning and stores zero,
   which keeps the step-only check.
2. `applyPendingMove` loads the task.
   - If the current step equals the move's target, the existing
     current-target consume path runs unchanged.
   - If `FromStepID` is set and differs from the current step, the move is
     superseded (AC-TASKS-WORKFLOW-STAGE-MOVES-001.1).
   - If `FromStepID` equals the current step and `FromTransitionID` is set but
     differs from the latest ledger id, the move is superseded
     (AC-TASKS-WORKFLOW-STAGE-MOVES-001.2). An unreadable ledger does not
     supersede (AC-TASKS-WORKFLOW-STAGE-MOVES-001.6).
   - Otherwise the transition source is `FromStepID` when set, or the current
     step for a legacy row.
3. The atomic admission write compares the source step, and when
   `FromTransitionID` is set, the latest ledger id read in the same
   transaction. A mismatch returns `admissionSourceChangedError` without
   writing. `workflowStore.updateDeferredTransitionTask` maps that outcome to
   the sentinel `errDeferredMoveSourceChanged`, and `applyPendingMove` treats
   it as supersession (AC-TASKS-WORKFLOW-STAGE-MOVES-001.3).
4. Supersession removes the pending row and its hand-off prompt with the same
   compare-and-delete (`DeletePendingMoveIfMatch`) and prompt correlation that
   the existing invalid-target and stale-expiry drops use. A row replaced by a
   concurrent take or replace is left to its new owner. The drop is logged with
   `pendingMoveLogFields` (AC-TASKS-WORKFLOW-STAGE-MOVES-001.4).

The pending-move reaper keeps age-based expiry. Supersession is evaluated only
at replay, the one path that can move a card.

### Relation to exact pending-move cancellation

A proposed explicit cancellation operation removes one reviewed pending-move
generation on behalf of an authorized caller. That operation and this design
are complementary: cancellation is caller-initiated and authorization-bound;
supersession is automatic and ordering-bound. Both add fields to `PendingMove`
and to the same `pending_moves` column lists. Whichever lands second must carry
the other's columns in every read, write, and restore statement. Neither
changes the other's predicates.

## Same-step admission entry

### Components

- `task/repository/sqlite.(*Repository).PromoteQueuedTaskIfWorkflowStepHasCapacity`
  admits a queued task under the step capacity lock.
- `orchestrator.(*Service).handleTaskQueuePromotedWithAutoStartOnCreateClaimed`
  runs the admitted entry through `finalizeStepEnter`.
- `models.MetaKeyQueuePromotionPending` is the one-shot admission marker.

### Contract

A same-step admission records no ledger row, so `task.queue_promoted` carries
`step_transition_id` zero, and the ledger dispatcher in
`step_entry_dispatch.go` skips an empty entry ID. With identity zero, the
`processOnEnter` entry guard compares the previous route against the admitted
step and drops the entry as a route change. The admitted step's session is
never prepared and `auto_start_agent` never runs.

1. Inside the promotion transaction, after the task row is locked, a same-step
   admission reads the task's latest ledger id and stores it on the admission
   marker as `models.QueuePromotionEntryTransitionIDKey` (`entry_transition_id`).
   Its decimal string encoding preserves the full 64-bit ledger identity through
   untyped metadata JSON decoding. The reader also accepts earlier numeric markers.
   That row is the entry made when the task was queued.
2. The handler resolves the entry identity in this order: the delivered
   `step_transition_id` when non-zero, the pinned marker value, and then the
   task's latest ledger id for a marker written before pinning
   (AC-TASKS-WORKFLOW-STAGE-MOVES-002.3).
3. The resolved identity is passed to `finalizeStepEnter`. The existing entry
   guard then accepts the admitted entry
   (AC-TASKS-WORKFLOW-STAGE-MOVES-002.1). It rejects a delivery handled after
   the task re-entered the step, because the pinned id is no longer current
   (AC-TASKS-WORKFLOW-STAGE-MOVES-002.2).

Cross-step promotions keep their delivered ledger id and are unchanged.

## Entry prompt before signal advance

### Components

- `orchestrator.(*Service).validateCeilingDestination` decides whether a
  ceiling-deferred launch is still deliverable.
- `orchestrator.(*Service).allowEngineSignalCompletion` gates an
  `auto_advance_requires_signal` step on its completion signal.
- `models.CeilingDeferral`, `models.CeilingWorkflowEntryBinding`, and the
  task's deferred-launch record hold a queued entry prompt.

### Contract

1. In `validateCeilingDestination`, a destination session that is `STARTING`
   or `RUNNING` returns `ceilingEntryUnavailable` (retry on a later pass)
   instead of `ceilingEntrySuperseded` when the record carries a valid
   workflow-entry binding. An unbound record keeps `ceilingEntrySuperseded`
   (AC-TASKS-WORKFLOW-STAGE-MOVES-003.1, 003.2). A retained record still never
   replays into a running provider, because the same state check runs again on
   every pass.
2. `allowEngineSignalCompletion` receives the task. Before it consumes a
   matching completion signal, it reads the task's ceiling-deferred launch.
   If that launch is a `prompt_ensure` or `workflow_step_ensure` whose binding
   targets the current step and `validateCeilingEntry` reports it valid or
   unavailable, the gate holds the transition and leaves the session
   `WAITING_FOR_INPUT`. A binding decode error or validation error also holds
   (AC-TASKS-WORKFLOW-STAGE-MOVES-003.3, 003.5). An absent binding or a readable
   binding targeting a different step keeps the prior handling.
3. A delivered, superseded, or absent entry prompt does not hold. User
   cancellation and steps without `auto_advance_requires_signal` keep their
   existing paths (AC-TASKS-WORKFLOW-STAGE-MOVES-003.4). The completion signal
   stays recorded on the session, so the advance can happen on a later
   completion after the prompt is delivered.

### How a hold ends

Ceiling records have no age limit. A retained record is re-evaluated by the
ceiling sweep (`ceilingSweepInterval`, 20 seconds, plus release-driven
wake-ups), and `ceilingEntryUnavailable` settles as a retry, not a drop. A hold
therefore ends through one of these existing events:

- The destination session leaves `STARTING` or `RUNNING`. The record becomes
  valid and the next sweep delivers it when capacity allows.
- The task leaves the step, or the destination session ends. The record
  becomes superseded, the sweep drops it, and the gate no longer holds.

Limitation: while the destination stays `STARTING` or `RUNNING`, the hold
lasts as long as that session does. The task service's stall recovery can
settle a stalled session that has no live execution, but a session with a
live execution that never ends its turn keeps the hold. Each sweep in
that window logs one replay-retry warning for the record, as it already does
for other unavailable records. A user can end the hold by moving the task.

## Agent move target validation

### Components

- `workflow/models.StepRunsOnEntry` reports whether entering a step starts
  work by itself, from the step's `on_enter` actions and `PullFromStepID`.
- `workflow/move.ValidateEntryTarget` and `ErrEntryTargetIsManual` express
  the agent-surface rule.
- `mcp/handlers.(*Handlers).handleMoveTask` applies it.
  `moveTaskErrorMessage` builds agent-facing error text.
- `mcp/server` tool descriptions for `move_task_kandev` in both registrations.

### Contract

1. After the same-step completion check and before the deferred or immediate
   split, `handleMoveTask` rejects a request whose normalized entry options
   are non-nil when `ValidateEntryTarget` reports the target step as manual.
   The result is a `VALIDATION` error, and no pending move is recorded
   (AC-TASKS-WORKFLOW-STAGE-MOVES-004.1). An unreadable step or missing
   workflow controller fails open to the existing path, which reports its own
   not-found or validation error.
2. `StepRunsOnEntry` returns true for `auto_start_agent`, `queue_run`,
   `queue_run_for_each_participant`, `run_code_review`, or a non-empty pull
   source (AC-TASKS-WORKFLOW-STAGE-MOVES-004.2). It reads step configuration
   only, not step names.
3. `moveTaskErrorMessage` keeps the existing classification. For `VALIDATION`
   it returns the message of a known `workflow/move` sentinel. For `CONFLICT`
   it appends a fixed phrase for each recognized service reason: WIP limit,
   active session, archived task, different workspace, or step membership.
   Any other error keeps the existing generic text, so raw service text and
   internal identifiers do not reach the agent
   (AC-TASKS-WORKFLOW-STAGE-MOVES-004.3).
4. The `move_task_kandev` tool description and the `prompt` and
   `entry_options` parameter descriptions state the rule
   (AC-TASKS-WORKFLOW-STAGE-MOVES-004.4).
5. `task/service.MoveTaskWithOptions` keeps the session-or-auto-start rule for
   HTTP and WebSocket moves (AC-TASKS-WORKFLOW-STAGE-MOVES-004.5).

## Failure and recovery behavior

- Every guard that depends on an unreadable value fails toward the previous
  behavior, except AC-TASKS-WORKFLOW-STAGE-MOVES-003.5. An unreadable queued
  entry prompt holds the advance because a missed entry turn cannot be
  recovered after the task has moved on.
- New columns and marker fields are additive. A downgraded binary ignores
  them, and an upgraded binary treats their absence as legacy behavior.

## Observability

Supersession, held signal advances, and manual-target rejections each emit one
structured log entry with the task, session, step, and move identifiers. No
metric is added.

## Related documents

- [Workflow task step transition ledger](workflow-task-step-transition-ledger.md)
- [Deferred move on_enter dispatch identity](deferred-move-onenter-dispatch-identity.md)
- [MCP task move results](mcp-task-move-results.md)
- [Workflow explicit completion signal](workflow-explicit-completion-signal.md)
- [Session concurrency ceiling](../../agents/system-design/session-concurrency-ceiling.md)
