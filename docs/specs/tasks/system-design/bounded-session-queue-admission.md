---
status: current
system: tasks
requirements:
  - REQ-TASKS-BOUNDED-QUEUE-ADMISSION-001
created: 2026-10-15
owners:
  - kandev
---

# Bounded Session Queue Admission Design

## Boundary and mapping

The messagequeue service owns admission acquisition; handlers own transport
feedback and the current attachment-cleanup retry loop. Durable queue identity
and replay remain owned by [queue admission](queue-admission.md).

| Criteria | Implementation boundary                                           |
| -------- | ----------------------------------------------------------------- |
| .1, .2   | `messagequeue.Service.WithSessionAdmission` and session gate      |
| .3       | Queued-dispatch error handling, queue handlers, queue API mapping |
| .4       | Existing handler attachment-cleanup obligation and recovery       |
| .5       | Orchestrator focus refresh and idle-suspension revalidation       |
| .6       | `messagequeue` admission entry holder record and timeout logging  |
| .7       | MCP deferred-move handler error mapping                           |
| .8       | Fresh-start recovery lifecycle and admission release              |
| .9       | Focus-floor retention                                             |
| .10      | Workflow parking revalidation                                     |

## Acquisition and callback lifetime

Replace the per-session mutex with a small FIFO waiter gate. Cancellation removes
an ungranted waiter; cancellation after grant hands admission to the next waiter.
Recheck context after grant. Admission entries retain their existing reference
count and disappear only when the last holder or waiter releases its reference.

Derive an acquisition context with a 30-second default, retaining an earlier
caller deadline. Acquire the lifecycle read barrier by `TryRLock` with a
20-millisecond context-aware pause, then the session gate under the same budget.
Nested same-session calls reuse their admission token; multi-session operations
retain their existing stable session-ID order.

Use the acquisition context only at lock acquisition. The admitted callback and
its token retain the caller context and the existing after-lifecycle-unlock
callbacks. Convert only an internally budgeted acquisition deadline into
`ErrSessionAdmissionTimeout`; callback errors are returned unchanged.

## Lock order with session lifecycle

Session locks are taken in one order: route operation lock, then session
lifecycle lock, then queue admission. Resume and recovery hold the lifecycle
lock while their continuation reaches prompt admission, so code that already
holds queue admission must never block on the lifecycle lock.

- Focus refresh of a live session and idle-suspension revalidation acquire the
  lifecycle lock without waiting. When it is busy, a lifecycle operation owns
  the session, so the session is not settled. Revalidation skips the candidate
  and the next reaper scan retries. Focus restarts the in-memory idle interval,
  which revalidation reads under the same admission, and leaves the persisted
  focus record for a settled execution.
- Focus on an idle-suspended session decides under admission, then releases it
  before resuming. The resume takes the route and lifecycle locks first and
  rechecks suspension provenance under them, so a concurrent resume or stop
  turns the focus into a no-op.
- A focus request that cannot take the lifecycle lock still resets the
  in-memory idle interval. The reaper keeps that floor while its executor row
  exists, including scans that skip the session because of temporary known
  work. It rechecks the interval while admitted before it accepts a suspension
  candidate. Passive resume checks workflow parking again after it reacquires
  the lifecycle locks, as required by .10.

Fresh-start recovery replays the original submission as a dispatch-only prompt.
The resume continuation, and with it the lifecycle lock, ends at provider
acceptance, matching ordinary first launch. Turn results and failures arrive
through the normal agent event path, not the recovery request. The agent
runtime watches every dispatch-only prompt for inactivity, as
[agent stall recovery](../../agents/system-design/agent-stall-recovery.md)
describes, so the replayed turn keeps its stall notice.

## Failure and recovery

Add the new timeout sentinel to the existing queued-dispatch requeue branch.
Preserve all other accepted-work and primary-ownership predicates; the separate
merged readiness-recovery contract owns those changes. Do not reinterpret uncertain
provider delivery as a safe acquisition failure.

Keep the existing queued worker's reservation until its requeue or restore
mutation returns. Retry only acquisition-budget timeouts through the existing
reserved-prompt callback owner's cancellable lifetime; do not start another
provider prompt or retry identity, storage, caller-cancellation, or uncertain
delivery errors. The caller waits, so a borrowed admission token stays within
its owner's lock lifetime. Stop cancels and joins compensation and suppresses
the subsequent pending drain. Reservations capture their callback owner at take
time, and the worker carries that immutable owner through compensation and
deferred draining. A later Start cannot lend its new owner to an old worker or
let that worker consume the new generation's pending drain. The deferred drain
is launched and joined under that same owner, including context-aware cancel
guard admission and SQL work. Reservation construction preserves the originating
owner through that admission. Lifecycle bookkeeping locks are released before I/O.
Each acquisition keeps its own 30-second budget;
the same live compensation can wait for contention to clear without dropping
the known-undelivered message.

All queue mutation handlers, including the MCP `move_task` deferred-move path,
map this sentinel to the existing retryable `UNAVAILABLE` response. Operator mutation wrappers wait 35 seconds and preserve
the structured admission error. Queue controls use the existing translated
queue-unavailable feedback. Identity, entry-not-found, merge limits, and reorder
drift retain their precedence. Identified queue admission keeps its existing
shorter uncertain-delivery reconciliation policy; its durable client ID makes
that retry idempotent.

Bound the attachment-cleanup loop at 60 attempts. Exhaustion releases only its
in-memory retry obligation; the persisted cleanup row remains available for
handler restart recovery. Successful cleanup, captured authorization, edit-lease
coordination, and attachment-transfer handling retain their current behavior.
Waiting for an active edit lease, including a lease discovered after admission,
does not spend a cleanup attempt. Keep the existing wake path until actual
cleanup or acknowledgement can run.

## Lock order inside admitted callbacks

Session admission is acquired before the per-session lifecycle lock. An
admitted callback must not wait for the lifecycle lock without a bound,
because the admission gate stays held for as long as the callback blocks.
Opportunistic callbacks use the existing `tryAcquireSessionLifecycleLock`.
When the lock is busy, the callback returns without side effects: the focus
refresh writes no persisted focus marker, but it restarts the in-memory idle
interval. The reaper keeps the candidate for a later scan and rechecks its idle
deadline under admission before suspension.

## Holder diagnostics

Each admission entry records when its current holder acquired the gate and
clears that time on release. A timed-out waiter logs one warning with the
session ID and how long the holder has held the gate. Callback timing and
errors are unchanged.

## Deferred MCP move

`SetPendingMove` returns `ErrSessionAdmissionTimeout` unchanged. The MCP
deferred-move handler maps that sentinel to `UNAVAILABLE` with a message
saying that the session is busy and the move can be retried. Other persistence
failures remain `INTERNAL_ERROR`.

## Scope and verification

Permanent tests cover FIFO cancellation and grant handoff, reference release,
SQLite mutations under admission contention, caller/callback context ownership,
actual queued-worker compensation and successful service shutdown, cleanup
attempt and acknowledgement budgets, active edit leases, and durable restart
recovery. API tests cover structured retryable errors and request budgets.
Desktop and phone browser tests refuse a Clear all request after the ordinary
transport timeout, retain the entries, display translated retry guidance, and
complete a real subsequent Clear all request with retries disabled.

The existing queue panel remains the desktop and phone entry point, with its
existing responsive controls and scroll owner. Only error feedback changes;
there is no new surface or viewport-specific state.

No schema, grant, runtime policy, provider retry, or deployment changes.
