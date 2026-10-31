---
created: 2026-10-29
status: done
requirements:
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-004
system_design:
  - ../../specs/tasks/system-design/queued-session-ownership.md
legacy_specs: []
---

# Implementation plan: Explicit deferred-launch discard

## Overview

A deferred workflow launch can sit queued for hours. Until now the only way to
remove one was to let the automatic replay sweep decide it was no longer
replayable, which is a system judgment, not a user decision. This package adds
one explicit, task-scoped discard for a launch that is still valid, and answers
with what actually happened to the durable record.

The [requirements](../../specs/tasks/requirements/queued-session-ownership.md)
and [design](../../specs/tasks/system-design/queued-session-ownership.md) stay
owned by the queued session ownership pair: the deferred record and its
recipient belong to the task system, and agent capacity keeps owning admission.
Read the "Deferred entry ownership" and "Explicit discard" design sections.

Work order: [task 01](task-01-explicit-deferred-launch-discard.md).

## Contract status

The transport is a proposal, not an agreed contract. The issue asking upstream
about the shape (`kdlbs/kandev#4508`) has no maintainer answer, so this package
implements the recommended safe shape and keeps it replaceable:

- method and path: `POST /api/v1/tasks/:id/deferred-launch/discard`
- response: `200 {"outcome": "already_clear" | "discarded" | "retained", "reason"?: ...}`
- authorization: the existing task-write and executor-mutation checks; no new
  global operator permission is introduced

A maintainer who prefers a different verb, path, field set, or an operator-only
permission can change the thin transport without touching the durable safety
rules, because those live in the shared settlement routine rather than in the
handler.

## Scope

### In scope

- One explicit discard entry point for a pending ceiling-deferred launch.
- A closed three-way outcome plus a closed set of retained reasons, so an
  incomplete discard is never reported as a no-op or a success.
- One settlement routine shared by the automatic replay drop and the explicit
  path. The request takes the existing dispatch claim for the observed record
  first, then checks task ownership, session-inventory readability, and the
  atomic-clear capability before settling the served automation run. It checks
  release eligibility again after settlement, then clears only the ceiling keys
  and releases the exact automation start owner inside one task transaction.
  Failures detected before settlement leave the run and record unchanged. If
  the final transaction fails after settlement, the record remains for retry;
  the next sweep sees the closed run and completes the drop instead of replaying
  it.
- A live dispatch claim, a successor record, and another execution's ownership
  marker are never cleared.
- Task-write plus executor-mutation authorization, reused from task mutation.

### Out of scope

- Starting, resuming, stopping, or steering any session; performing a manual
  launch; changing task state directly.
- New permissions, roles, database schema, configuration keys, metrics, or a
  second queue.
- Frontend surfaces. The existing queue projection already disappears when the
  ceiling record is cleared; a discard affordance is a separate UI decision.
- Changing the automatic replay drop's own reasons or timing.

## Delivery order

One work order. The durable service and its verification come first in the same
change as the transport, because a transport without a proven settlement path
would be the exact "bool only" shortcut this package replaces.

## Verification

Permanent tests cover the discard outcomes, a live claim, a successor winning a
compare-and-set, unreadable ownership inventory, an unavailable atomic clear,
stopped retry ownership, and cleanup retry after the final metadata write fails.
SQLite tests prove that a failed owner removal rolls back the deferred-record
write. Service and HTTP tests cover authorization and the 200, 404, and 503
responses.

The touched Go files are formatted. Focused package tests cover the new
orchestrator, repository, service, and handler paths. CI remains responsible
for the broader package and repository checks.

## Boundaries

No production deployment, restart, production database write, or live endpoint
call. No new permission, role, schema, secret, or lock bypass, and no change to
model routing, work-order order, or limits.
