---
created: 2026-11-03
status: complete
requirements:
  - REQ-TASKS-SESSION-TURN-SETTLEMENT-002
  - REQ-UI-MESSAGE-QUEUE-SEND-NOW-001
system_design:
  - ../../specs/tasks/system-design/session-turn-settlement.md
  - ../../specs/ui/system-design/message-queue-send-now.md
legacy_specs: []
---

# Implementation Plan: Send Now claim loss repair

## Overview

[Issue #4958](https://github.com/kdlbs/kandev/issues/4958): a session stayed
`RUNNING` after an agent-started turn, a user message was queued behind it, and
Send Now then lost that message. The history showed the message, but the agent
never received it and the queue entry was gone.

The [stuck turn and queue delivery repair](../stuck-turn-queue-delivery/plan.md)
(#4988) already fixed the main cause of the stuck turn inside agentctl. This
package closes the remaining backend gap and the two Send Now defects that lost
the message. Task 01 is independent. Task 03 depends on Task 02, because Task
02's ownership rule makes a leftover claim block its entry until Task 03
releases it.

## Evidence and root causes

| # | Defect | Root cause | Evidence |
| --- | --- | --- | --- |
| 1 | A status-less tool update can still mark the session busy | `recordActivity` flips a Ready execution to Running on any non-terminal `tool_update`, including one with an empty status. Since #4988 the agentctl finalizer ignores that update, so it never closes the turn. | Temporary ACP test on `main`: a retired background Bash call followed by the bridge's `terminal_exit`-only `tool_call_update` emits no completion when no turn content follows. `manager_events.go` checks only `isTerminalToolUpdate`. |
| 2 | FIFO takes a row a Send Now claim owns | A durable source keeps its row under a 30-second lease. The replacement prompt waited 30 seconds (`agentPromptReadyTimeout`) after the claim, so the lease expired first, and the readiness drain reserved the row with a new token. | Reporter timeline: claim at 21:15:25.768, readiness timeout at 21:15:55.793. Temporary SQLite test on `main`: after expiry, `ReserveHeadIfAutoRunForSession` returns the row with a new token. |
| 3 | A claim that cannot be restored vanishes | Restore then fails its token check (`send-now queue selection changed`). The running backend leaves the claim row behind, and both recovery paths delete it silently. Nothing marks the message, which stays in the history as if delivered. | Reporter database: claim row with `accepted = 0` and operation generation 3, matching `send_now_generation` 3; no `queued_messages` row; no delivery submission. The same test reproduces the exact restore error. |

The second report on the issue had the same stuck session, but its message had
no lease, so the restore succeeded and nothing was lost. That fits defect 2.

## Scope

### In scope

- Defects 1 to 3, with regression tests and the not-delivered badge on desktop
  and phone.

### Out of scope

- The agentctl finalizer and provider end-of-turn boundary, delivered by #4988.
- The 30-second readiness wait, and why the agent was not ready. The claim
  ownership rule makes the wait length irrelevant to correctness.
- Claims invalidated by deleting or replacing the task or session, which keep
  their existing behavior.
- Resending automatically. A not-delivered message is sent again by the user.
- Repairing history for sessions already affected. Their lost messages are not
  marked retroactively.

## Technical approach

### Backend turn start

`apps/backend/internal/agent/runtime/lifecycle/manager_events.go`:
`recordActivity` does not flip a Ready execution to Running for a `tool_update`
whose `ToolStatus` is empty. Activity timestamps still update. See
[Backend turn start](../../specs/tasks/system-design/session-turn-settlement.md#backend-turn-start).

### Claim ownership

`apps/backend/internal/orchestrator/messagequeue/repository_sqlite.go`:
`reserveHeadTx` returns no reservation when the head entry is a source of the
session's open Send Now claim. `repository_memory.go`: `reserveHeadLocked`
applies the same rule through `sendNowClaimContainsLocked`. See
[Claim ownership over leases](../../specs/ui/system-design/message-queue-send-now.md#claim-ownership-over-leases).

### Unrecoverable claims

`apps/backend/internal/orchestrator/queue_send_now.go` and
`queue_send_now_claim.go`: the three sites that end with
`ErrSendNowClaimChanged` call one release step. It releases the exact claim and
the source rows it still owns, then marks the user message, or records it in an
already-completed lifecycle-only turn, with `delivery_status: not_delivered`.
A successor-owned source keeps the message unmarked. The release retries
transient failures with the bounded claim mutation helper. The web chat bubble
renders that status with the existing badge pattern. See
[Unrecoverable claims](../../specs/ui/system-design/message-queue-send-now.md#unrecoverable-claims).

## ASCII UI preview

`UI-01: Not-delivered user message.` Entry point: task session chat, after a
Send Now could neither deliver nor restore its message. The badge reuses the
existing `blocked` status badge slot and styling; only the copy differs. Copy
is illustrative and must be localized.

Before (current behavior; the message has no status badge):

```text
                                       +---------------------------------+
                                       | Fix the button spacing (2 notes) |
                                       +---------------------------------+
  (looks delivered; the agent never received it)
```

After:

```text
                              [ Not delivered. Send it again. ]
                                       +---------------------------------+
                                       | Fix the button spacing (2 notes) |
                                       +---------------------------------+
```

Phone: the same bubble and badge in the single-column chat. The badge row
already wraps (`flex-wrap`), so it adds no horizontal overflow. Required: the
badge is a `role="status"` element above the user bubble with localized copy.
Illustrative: exact wording, color, and spacing. Maps to
`AC-UI-MESSAGE-QUEUE-SEND-NOW-001.14`.

## Tests

| Criteria | Evidence |
| --- | --- |
| `AC-TASKS-SESSION-TURN-SETTLEMENT-002.6` | `apps/backend/internal/agent/runtime/lifecycle/wakeup_statusless_test.go` (new) |
| `AC-UI-MESSAGE-QUEUE-SEND-NOW-001.13` | `apps/backend/internal/orchestrator/messagequeue/send_now_claim_ownership_test.go` (new), `apps/backend/internal/orchestrator/queue_send_now_claim_ownership_test.go` (new) |
| `AC-UI-MESSAGE-QUEUE-SEND-NOW-001.14` | `apps/backend/internal/orchestrator/queue_send_now_unrecoverable_test.go` (new; all three sites, source settling, successor guard, lifecycle-only turn, transient retry), `apps/backend/internal/task/service/service_message_delivery_status_test.go` (new; replay, admission, completed turn), `apps/web/components/task/chat/messages/chat-message.test.tsx` |

## E2E tests

| Flow | Criteria | Spec and project |
| --- | --- | --- |
| A message whose Send Now claim cannot be restored shows the not-delivered badge | `AC-UI-MESSAGE-QUEUE-SEND-NOW-001.14` | `tests/chat/send-now-not-delivered.spec.ts` for `chromium` and `tests/chat/mobile-send-now-not-delivered.spec.ts` for `mobile-chrome` (new) |

## Work orders

- [x] [Task 01: Ignore status-less updates at turn start](task-01-statusless-turn-start.md)
- [x] [Task 02: Let the claim own its sources](task-02-claim-owns-sources.md)
- [x] [Task 03: Mark unrecoverable claims not delivered](task-03-unrecoverable-claims.md)

## Risks

- A tool update with no status but real progress would no longer start a turn.
  The adapter already gives such updates `in_progress`, so an empty status means
  metadata only, and any turn content still flips the session.
- A claim row that is never released would block its entry. Task 03 releases
  every unrecoverable claim. Rows left by the old code are reconciled by the
  first startup after upgrade like any crash leftover: with a recovery-block
  store, startup parks the session behind an `unknown_send_now_outcome`
  recovery block, and resolving it runs the release step.
- The release verifies ownership, writes the message status while the claim is
  still present, then clears the exact claim and the source rows it still owns.
  A failed status write keeps the claim available for the bounded retry. A
  failure before the release transaction commits leaves the claim for the next
  startup reconciliation or recovery action. A persistent status failure leaves
  the message unmarked and the claim in place; the error is logged.
- The exact bridge frame in the first report is inferred from bridge 0.97.0. The
  backend rule depends only on the empty status, not on the frame's source.
