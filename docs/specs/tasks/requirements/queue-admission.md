---
status: active
system: tasks
created: 2026-09-14
owners:
  - kandev
---

# Queue Admission Requirements

## Overview

Users need a reliable result when submitting a prompt to a busy session.
The task system owns admission identity, persistence, and recovery. Composers expose this contract on desktop and phone.

An admission is one submission attempt, including its automatic retry. Queue editing, merging, and dispatch are later operations.

## Requirements

### REQ-TASKS-QUEUE-ADMISSION-001: Recoverable queue submission

**Intent:** Preserve user input and prevent duplicate admission when a response is lost.

#### Acceptance criteria

- **AC-TASKS-QUEUE-ADMISSION-001.1:** Every ordinary composer queue submission shall have a stable admission identity, including submissions without plan comments.
- **AC-TASKS-QUEUE-ADMISSION-001.2:** Repeating an accepted admission with the same identity and payload shall not append content, claim attachments, or consume comments again.
  This applies after automatic merge, manual removal, queue clearing, reservation, dispatch, and backend restart.
- **AC-TASKS-QUEUE-ADMISSION-001.3:** Reusing an admission identity with a different payload shall fail without changing the accepted work.
  An admission shall remain bound to its original task, session, and session incarnation.
- **AC-TASKS-QUEUE-ADMISSION-001.4:** A queue submission shall allow 10 seconds for a response, or 30 seconds when it includes attachments.
  After uncertain transport failure, the client shall reconcile acceptance and retry at most once with the same identity and payload.
- **AC-TASKS-QUEUE-ADMISSION-001.5:** Confirmed acceptance shall clear only the submitted draft. A failed queue refresh after acceptance shall not report admission failure.
  Rejected or unresolved submissions shall preserve the draft and attachments. A later unrelated draft shall remain unchanged.
- **AC-TASKS-QUEUE-ADMISSION-001.6:** Queue capacity and validation rejections shall show localized rejection feedback.
  An unresolved timeout or connection loss shall show uncertain delivery feedback without claiming that the server rejected the message.
- **AC-TASKS-QUEUE-ADMISSION-001.7:** Reliable admission shall preserve Auto-merge, capacity, attachment ownership, Auto-run, and plan-comment consumption rules.
  In particular, a compatible ordinary submission shall remain eligible for automatic merging.
- **AC-TASKS-QUEUE-ADMISSION-001.8:** Task chat and Quick Chat shall provide equivalent recovery and draft preservation on desktop and phone.
  Phone feedback shall remain readable with the existing touch-accessible Send action.

## Compatibility and exclusions

Clients that omit admission identity retain their existing behavior without automatic replay guarantees.
Existing plan-comment identities remain valid. A second deliberate submission is a new admission, not a replay of an edited draft.
No provider-level exactly-once execution or offline outbox is promised. Browser reload does not restore an unacknowledged submission automatically.

## Related contracts

- [Resume prompt queue](resume-prompt-queue.md) owns startup eligibility and deferred dispatch.
- [Plan comments](plan-comments.md) owns comment selection and atomic consumption.
- [Automatic merge overrides](../../ui/requirements/message-queue-auto-merge-session-overrides.md) owns merge policy and capacity exceptions.

## Implementation plans

- [Queue admission reliability](../../../plans/queue-admission-reliability/plan.md)
