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
| `TestPeerMessageInitialLaunch_Delivery` | 001.1, 001.2 | One initial prompt, one later follow-up, preserved attribution |
| `TestPeerMessageInitialLaunch_AdmissionOrders` | 001.2 | Message before bootstrap, during bootstrap, and after readiness |
| `TestPeerMessageInitialLaunch_RollbackOwnership` | 001.3 | No stale state, turn, queue, or task restoration |
| `TestExistingWorkspaceStart_ActiveAgent` | 001.3 | No configuration, description overwrite, new process, or stop |
| `TestPeerMessageInitialLaunch_FailureOwnership` | 001.3 | Same-execution stale attempt cannot fail or stop its successor |

Include Auto-run OFF, genuine owned failure, cancellation, and a prepared
workspace without an agent. Preserve a waiting older FIFO entry where applicable.
Each request is accounted for once under existing merge policy. Separate retries
remain separate submissions; do not assert unsupported global deduplication.

## End-to-end evidence

Enter through `handleMessageTask` while a real orchestrator launch is blocked
in a controllable runtime collaborator. Use authoritative session storage and
the actual queue. Assert provider acceptance of the original brief followed by
the queued message after the first turn ends. Observe configuration/start/stop
counts and durable session/turn state. A fake launcher that only counts handler
calls does not reproduce this incident.

There are no rendered changes. The backend agent-facing flow provides the
end-to-end evidence; no browser scenario is required.

## Work orders

- [x] [Task 01: Preserve initial launch ownership](task-01-preserve-launch-ownership.md) (complete)

## Verification results

Implementation completed on 2026-09-26. Verification passed:

- `go test ./internal/mcp/handlers -run '^TestPeerMessageInitialLaunch_' -count=1`.
- `go test -race ./internal/mcp/handlers ./internal/orchestrator ./internal/orchestrator/executor ./internal/orchestrator/messagequeue -count=1`.
- `go test -race ./internal/mcp/handlers -run '^(TestPeerMessageInitialLaunch_QueuesBeforeWorkflowTurnPreparation|TestPeerMessageInitialLaunch_QueueRejectionDoesNotRollbackWinningProgress)$' -count=1`.
- `go test ./internal/mcp/... -run '^$'`.
- `make -C apps/backend build` (passed; macOS outputs remained unsigned because codesign tools are unavailable in the environment).
- `python3 scripts/list-docs.py validate`: 309 decisions and 1185 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.

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
