---
created: 2026-09-26
status: complete
requirements:
  - REQ-TASKS-PARENT-CHILD-MESSAGE-INTERRUPT-001
system_design:
  - ../../specs/tasks/system-design/parent-child-message-interrupt.md
legacy_specs: []
---

# Implementation Plan: Peer messages during initial launch

## Overview

Preserve an accepted child launch when a parent sends a queued follow-up during
startup. Implement one complete repair across message admission, executor start,
and failure ownership. These boundaries must agree before the repair is safe.

The task system owns this repair because it owns session launch and peer-message
delivery. Existing criteria cover busy delivery. Criteria 001.2 and 001.3 clarify
the preparation interval and losing-start behavior. No product choice remains open.

## Evidence and root cause

Target task `34f9a791-450a-4574-91bb-7c3398bf59a0` failed on 2026-09-26.
The running binary was `v0.96.0-15-g5c3dc31b2b6`.
Source inspection used checkout `b82bfd2cead`.

Retained backend file `backend-logs-2026-09-26-000030.log` records this sequence.
Times use Lisbon time, UTC+1.

- 14:55:47.641: parent submits `message_task_kandev`, mode `queued`, during setup.
- 14:55:55.390: original launch reaches `RUNNING`.
- 14:55:56.192: competing start fails its stale `STARTING` write against `RUNNING`.
- Failure handling marks the session `FAILED`; message rollback restores `CREATED`.
- 14:56:24.871: the retried message attempts configuration of the same live execution.
- Agentctl rejects configuration: `cannot configure while agent is running`.
- Bootstrap failure cleanup stops the original agent.

The recorded stack enters `dispatchPreparedTaskMessage`, `StartCreatedSession`,
`LaunchPreparedSession`, then `startAgentOnExistingWorkspaceWithRequest`.
Session snapshots and serialized launch calls do not establish exclusive
ownership throughout asynchronous bootstrap. Failure handling then affects
work owned by another start.

## Scope

### In scope

- Queue admission while an initial launch owns a still-created session.
- Atomic start-or-queue classification and non-destructive duplicate-start outcomes.
- Failure, rollback, and cleanup ownership for the same session and execution.
- Original brief, sender metadata, FIFO policy, and accepted-message preservation.
- Deterministic regression tests through real handler and orchestrator wiring.

### Out of scope

- Provider error copy, UI markup, new public tool parameters, or new session states.
- Historical session repair, automatic recovery loops, or parent-side sleeps.
- Idempotency across separate MCP submissions without an admission identity.
- Changes to Office scheduling, Auto-run, capacity, explicit interrupt authorization,
  workflow recipient selection, or provider-level crash delivery guarantees.

## Technical approach

Implement the owning design's **Initial launch ownership** section.
Use `acquireSessionLifecycleLock` for the orchestrator decision and preserve the
executor session lock. Document lock order before changing these paths.
Do not hold a guard across synchronous callbacks that reacquire it.

Re-read launch ownership before peer turn preparation. If startup already owns
the session, return a typed internal outcome and queue against its captured identity.
Keep `CheckQueueAdmissionReadiness` after insertion to cover readiness winning first.
Do not swallow a message by reporting launch success without admitting its prompt.

Recheck runtime startup/running evidence before existing-workspace configuration.
Distinguish a prepared workspace from an active agent. Route contention away from
`handleSessionLaunchFailure`. Fence rollback and cleanup against the mutation owner.
Same-execution retries require startup-attempt evidence, not only execution equality.
Reuse conditional persistence and current startup-generation mechanisms.
No schema change is planned. If existing primitives cannot enforce this boundary,
revise the design before adding persistence.

## Tests

All tests use channel barriers or injected callbacks, never timing sleeps.
Add these tests in the work order's new focused files:

| Test | Criteria | Evidence |
| --- | --- | --- |
| `TestPeerMessageInitialLaunch_Delivery` | 001.1, 001.2 | Losing start queues the follow-up without replacing the active initial turn; Auto-run OFF keeps it pending |
| `TestPeerMessageInitialLaunch_HandlerKeepsFollowUpQueuedUntilTurnBoundary` | 001.2 | Real handler and blocked runtime retain the follow-up while the initial turn is active and Auto-run is OFF |
| `TestPeerMessageInitialLaunch_QueuesBeforeWorkflowTurnPreparation` | 001.2 | In-flight initial launch queues before a second on-turn-start transition or mutation |
| `TestPeerMessageInitialLaunch_PreservesOlderFIFOEntry` | 001.2 | Existing queued work stays ahead of the accepted follow-up |
| `TestPeerMessageInitialLaunch_QueueRejectionDoesNotRollbackWinningProgress` | 001.3 | Full message handler returns queue-full while preserving the winning RUNNING session, task, turn, metadata, and queue |
| `TestPeerMessageInitialLaunch_RollbackFencesSameStateSessionProgress` | 001.3 | Same-state session progress prevents stale session, task, and queue restoration |
| `TestPeerMessageInitialLaunch_RollbackFencesTaskOnlyProgress` | 001.3 | A task state and workflow-step change cannot be rewound when the session row is unchanged |
| `TestPeerMessageInitialLaunch_RollbackFencesSiblingSessionProgressDuringPreparation` | 001.3 | Failed dispatch preserves a newly selected session and the queue transferred to that owner |
| `TestHandleMessageTask_DispatchErrorAfterSessionSwitchPreservesNewSessionOwner`, `TestHandleMessageTask_DispatchErrorPreservesNewSessionOwnerOutsideReview`, `TestHandleMessageTask_DispatchErrorAfterExistingSessionSwitchPreservesQueues` | 001.3 | Session-switch failures keep the selected owner, task progression, and transferred queue |
| `TestHandleMessageTask_ParentInterruptDuringInitialLaunchAdmission` | 001.2 | Losing parent interrupt queues its exact message and interrupts once |
| `TestStartCreatedSession_ActiveSessionReturnsBusy` | 001.3 | A stale session snapshot cannot launch over a newer active session |
| `TestStartCreatedSessionForPeerMessage_ContendedLaunchReturnsBusy` | 001.3 | Concurrent peer launch loses admission without waiting or mutation |
| `TestStartCreatedSessionForPeerMessage_ActiveRuntimeReturnsBusy` | 001.3 | Active runtime is refused before configuration or start |
| `TestStartCreatedSessionForPeerMessage_RejectsReplacedSessionIdentity` | 001.3 | A replaced session incarnation cannot inherit launch admission |
| `TestBeginPeerMessageStartWaitsForCancelAndRejectsTerminalSession` | 001.3 | Cancellation guard contention rechecks terminal state before queueing |
| `TestBeginPeerMessageStartAuthorizesPairBeforeRepositoryRead` | 001.3 | Session authorization precedes repository access |
| `TestNewServiceSessionStartingCallbackClassifiesRunningCASConflict` | 001.3 | Production `NewService` callback classifies a RUNNING CAS winner as busy |
| `TestLaunchInitialCreatePromptBusyDoesNotFailLiveSession` | 001.3 | Initial-create failure handling preserves the live launch |
| `TestScheduleTaskForSessionDoesNotRegressConcurrentInProgress` | 001.3 | A concurrent IN_PROGRESS write cannot be overwritten with SCHEDULING |
| `TestBootstrapFailureCASMissDoesNotStopSameExecutionRetry` | 001.3 | Stale startup failure cannot stop a same-execution retry |
| `TestCommitBootstrapFailureIfCurrentAttemptRejectsSameExecutionRetry` | 001.3 | Repository failure CAS rejects a stale session revision |
| `TestPostgresBootstrapFailureIfCurrentAttemptRejectsSameExecutionRetry` | 001.3 | Environment-gated PostgreSQL failure CAS rejects a stale session revision |
| `TestExistingWorkspaceStart_ActiveAgent`, `TestExistingWorkspaceStart_PreparedWorkspaceCanStart` | 001.3 | Active work is refused while a prepared workspace without an agent remains startable |

Genuine owned failure, cancellation, Auto-run OFF, and prepared-workspace cases
are covered by the tests above and `TestBootstrapFailureProjection`. Preserve a
waiting older FIFO entry where applicable. Each request is accounted for once
under existing merge policy. Separate retries remain separate submissions; do
not assert unsupported global deduplication.

## End-to-end evidence

Enter through `handleMessageTask` while the real orchestrator launch is blocked
in a controllable runtime collaborator. Use authoritative session storage and
the actual queue. Assert acceptance of the original brief and retention of the
queued follow-up while the initial turn remains active with Auto-run OFF. The
existing queue-readiness tests cover dispatch at the later turn boundary.
Observe durable session and turn state. A fake launcher that only counts
handler calls does not reproduce this incident.

There are no rendered changes. The backend agent-facing flow provides the
end-to-end evidence; no browser scenario is required.

## Work orders

- [x] [Task 01: Preserve initial launch ownership](task-01-preserve-launch-ownership.md) (complete)

## Verification results

Implementation completed on 2026-09-26. Final fixup verification is recorded in
Task 01 below and must match the current PR head.

## Risks

- Existing locks cover different intervals. Nesting them incorrectly can deadlock synchronous events.
- Broad `CREATED` checks can block legitimate first messages on prepared sessions.
- Execution equality cannot distinguish retries within the same runtime.
- Returning success for contention can silently lose the follow-up unless queue admission succeeds.
- Removing all rollback can break genuine workflow-entry failure recovery.

## Related delivery and documentation

The completed [queue wakeup package](../mcp-queued-message-wakeup/plan.md) owns
post-insertion readiness. Its implementation and tests remain intact. This
package adds startup admission coverage and does not reopen its completed work
order. The requirements, design, and delivery records were updated. Public
documentation and tool parameters did not change.
