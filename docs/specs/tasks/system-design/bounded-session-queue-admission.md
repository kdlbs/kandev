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

All queue mutation handlers map this sentinel to the existing retryable
`UNAVAILABLE` response. Operator mutation wrappers wait 35 seconds and preserve
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
