---
status: draft
system: tasks
requirements:
  - REQ-TASKS-RESTART-ORPHAN-SESSIONS-002
  - REQ-TASKS-RESTART-ORPHAN-SESSIONS-003
---

# Session recovery after backend interruption

## Ownership and control flow

The task system owns recovery state. Both reconciliation passes must preserve
conversation availability after execution loss. This replaces the cancellation
policy shipped in PR #3833 and the equivalent healing action in PR #3832.

Use `reconcileActiveSessionOnStartup` in `internal/orchestrator/service.go` as
the behavior precedent. It preserves runtime identity, settles abandoned turns,
and projects non-surviving active sessions as `WAITING_FOR_INPUT` for lazy recovery.
It does not replay the prompt. Surviving executions remain running.

The periodic recovery path must also handle sessions without an executor row.
Keep the existing ticker, launch grace, runtime-aware liveness protections,
retry behavior, and per-session candidate identity. Replace orphan cancellation
with a conditional transition to recoverable `WAITING_FOR_INPUT`.

## State and persistence

No new session-state enum is required. Preserve transcript, workspace, resume
token, execution identity, and primary-session assignment. Clear the current
orphan cancellation error on successful recovery. Preserve historical diagnostics.

Settle only the abandoned turn identified by the recovery snapshot. Use the
existing interruption cleanup behavior, not archive cancellation effects.
Do not publish successful `turn.completed`, advance the workflow, or cancel
healthy siblings. Retire stale execution capacity claims through existing owners.
Do not expire questions or clear queued work solely because the backend restarted.

The final write must compare observed state, activity, and turn identity.
Recheck live execution and archive/stop ownership at the transition boundary.
A completed or replaced turn, renewed activity, or concurrent explicit stop
rejects the stale recovery attempt. No database transaction spans runtime I/O.
Recovery must be retryable after partial progress without duplicating effects.

`WAITING_FOR_INPUT` without an unfinished turn is normal idle state. The active
stall sweep must neither alert nor alter it, regardless of elapsed silence.
For interrupted work, retain useful stall diagnostics and recoverable settlement.
The active pass retains its all-active-session eligibility boundary. An idle
sibling blocks that task-wide action; the restart pass remains per-session.

## Focus and message delivery

On task focus, use existing `GetTaskSessionStatus` and `session.launch` resume
flow with automatic admission. Opening the application does not eagerly start
all interrupted sessions. Focusing restores provider context but sends no prompt.

Use normal queued prompt delivery for a message sent during startup. Deliver
that new message once after readiness. Never resend the old prompt to simulate
continuation. Recover legacy `CANCELLED` plus exact orphan-reason rows through
the compatibility exception in the
[stall recovery design](session-stall-visibility.md#recovery-when-opening-a-task).
Other terminal states retain their existing rules.

When provider context cannot be restored, retain the same Kandev transcript and
workspace. Expose the existing recovery fallback without claiming native context
restoration. Honor auto-start prevention and all existing launch ownership guards.

## Interruption marker

Set or preserve `interrupted_at` for lost mid-turn work. Idle reconciliation
must not clear it. Successful correlated provider recovery clears the marker;
STARTING entry alone does not. Follow the
[warning design](interrupted-task-indicator.md) for publication, stale callbacks,
and the shared sidebar/card renderer.

## Interrupted automatic starts

An automatic step start writes `SCHEDULING` before it creates or reuses a
session. A restart in that window leaves a task in `SCHEDULING` that no startup
pass lists: `reconcileTaskLifecycleTokens` lists durable tokens,
`ReconcileQueuedTasks` lists workflow queue destinations, the ceiling sweep
lists deferred-launch records, and `reconcileDependencyLaunchesOnStartup` lists
pending dependency launches. The task keeps showing Scheduling with no owner.

`reconcileSchedulingOrphansOnStartup` adds one pass over
`ListSchedulingTasks` (non-archived, non-ephemeral, non-automation `SCHEDULING`
tasks, oldest update first). It runs once, inside the existing background
startup sweep and before `reconcileTaskLifecycleTokens`, so a task that a token
still owns is skipped instead of being seen as orphaned between token clearing
and session creation. Shutdown cancels and waits for that sweep, then waits for
tracked orphan re-entry workers. Each wait uses the existing 10-second
`sendNowClaimRecoveryTimeout`; detached work can outlive a timed-out join.

A candidate is skipped when it has a workflow queue destination, any
deferred-launch record, a lifecycle token, a recorded launch error, an Office
owner, a step without `auto_start_agent`, or an unresolved dependency (`003.2`).
For a remaining candidate:

- With no active session, `autoStartTaskForStep` starts the step through the
  ordinary automatic path, which applies ceiling, WIP, prompt, and session
  serialization rules (`003.1`).
- With a `WAITING_FOR_INPUT` or `IDLE` session and no turn accepted for the
  current entry,
  `processOnEnter` runs the current step's entry on that session, like a
  same-step re-entry. `on_exit` does not run because the task never left the
  step. The entry-dispatch guard still rejects a stale route (`003.3`). The
  entry runs detached, like the launch above, so a slow resume does not hold
  the lifecycle-token pass behind it.
- With conversational evidence for the current entry, only the task state
  moves from `SCHEDULING` to `IN_PROGRESS` by compare-and-set (`003.3`).
- A `CREATED`, `STARTING`, or `RUNNING` session means a launch is live and is
  not touched (`003.3`, `003.4`).

Turn creation records `workflow_step_transition_id_at_start` together with the
step stamp after taking the task row lock. The ordinary writer and the
conversation-receipt writer share that operation. Recovery compares the turn's
entry ID with the newest ledger row; a stale entry snapshot or ledger read error
fails closed. Older turns without that private recovery stamp use the ledger's
entry time as a compatibility boundary. Lifecycle-only turns never count.

An unfinished route recovers both the immutable transition ID and its source
step. A prepared destination or a committed route for a different recipient or
entry cannot suppress this context. `processOnEnter` retains the normal route,
recipient, credential, session lifecycle, and admission guards. Office ownership
read errors leave the task unchanged.

State-only repair uses `UpdateTaskStateIfCurrentEntry`. Its transaction takes
the task row lock before re-reading the ledger, and compares the observed
workflow, step, entry ID, task state, revision, and archive boundary before the
write. Same-step revisits cannot reuse old turn evidence even when timestamps
match. Only a committed repair publishes through the existing task-service event
publisher, with the database-stored timestamp read before commit; a refused repair emits no
state-change event.

This pass does not resume interrupted conversations. A session with accepted evidence for the current entry is not prompted again, so `002.2` and `002.3` still hold.

## UI and verification

Reuse the current desktop chat and dedicated phone task layout. Show normal
recovery progress and the retained transcript. Use existing retry controls for
actual failures. See the [plan](../../../plans/orphaned-session-open-recovery/plan.md)
for previews and the desktop/phone test matrix.

| Requirement | Design sections |
| --- | --- |
| REQ-TASKS-RESTART-ORPHAN-SESSIONS-002 | Ownership and control flow; State and persistence; Focus and message delivery; UI and verification |
| REQ-TASKS-RESTART-ORPHAN-SESSIONS-003 | Interrupted automatic starts |

## Related decision

The [session-open decision](../../../decisions/2026-09-18-session-open-resumes-conversation.md)
already separates provider recovery from prompt dispatch. The user's September 21
clarification extends that behavior to interrupted running tasks. The requirement
and this design preserve the rationale without a separate ADR.
