---
status: draft
system: tasks
requirements:
  - REQ-TASKS-RESTART-ORPHAN-SESSIONS-002
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

## UI and verification

Reuse the current desktop chat and dedicated phone task layout. Show normal
recovery progress and the retained transcript. Use existing retry controls for
actual failures. See the [plan](../../../plans/orphaned-session-open-recovery/plan.md)
for previews and the desktop/phone test matrix.

| Requirement | Design sections |
| --- | --- |
| REQ-TASKS-RESTART-ORPHAN-SESSIONS-002 | Ownership and control flow; State and persistence; Focus and message delivery; UI and verification |

## Related decision

The [session-open decision](../../../decisions/2026-09-18-session-open-resumes-conversation.md)
already separates provider recovery from prompt dispatch. The user's September 21
clarification extends that behavior to interrupted running tasks. The requirement
and this design preserve the rationale without a separate ADR.
