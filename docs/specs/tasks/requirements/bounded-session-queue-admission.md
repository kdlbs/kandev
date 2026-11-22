---
status: active
system: tasks
created: 2026-10-15
owners:
  - kandev
---

# Bounded Session Queue Admission

## Overview

An operator queue action must finish waiting when an admitted callback or task
purge is stuck. A bounded acquisition failure does not revoke accepted work or
cancel a callback that already acquired admission.

## Requirements

### REQ-TASKS-BOUNDED-QUEUE-ADMISSION-001: Bounded queue acquisition

**Intent:** Keep session queue operations responsive under lock contention.

- **AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.1:** When session admission or a task
  purge barrier is occupied, acquisition shall honor caller cancellation and
  an earlier caller deadline, otherwise return a retryable timeout within a
  30-second acquisition budget. Session waiters shall acquire in arrival order.
- **AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.2:** After acquisition, a callback shall
  retain the caller's context and its own error semantics. Expiring or releasing
  the acquisition budget shall not cancel admitted publication or other work.
- **AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.3:** When a queued dispatch cannot acquire
  admission, its taken message shall be requeued without losing its recorded
  user-message or attachment ownership. Queue mutations, including Clear all,
  Merge with above, Reorder, edit leases, Auto-run, and Auto-merge, shall report
  a retryable `UNAVAILABLE` result through existing queue feedback. Desktop
  and phone controls shall retain pending entries and allow an explicit retry.
- **AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.4:** A persistently failing attachment
  cleanup shall stop its current retry loop after 60 attempts while retaining
  the durable obligation for the existing recovery pass.

## Compatibility and exclusions

Preserve [queue admission identity and replay](queue-admission.md), Auto-merge,
capacity, session incarnation, attachment authorization, edit leases, task purge
ownership, and detached status publication. The same API feedback applies on
desktop and phone. No new settings, primary-session policy, provider retry,
background worker, or recurring verification is introduced.

## Design and delivery

- [System design](../system-design/bounded-session-queue-admission.md)
- [Delivery package](../../../plans/bounded-session-queue-admission/plan.md)
