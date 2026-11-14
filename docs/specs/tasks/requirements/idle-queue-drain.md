---
status: active
system: tasks
created: 2026-10-20
owners:
  - kandev
---

# Idle Queue Drain

## Overview

Queued messages are dispatched when an event offers them to the session: a
turn ends, the agent boots or resumes, a message is enqueued, or the user asks.
A message that is still queued after those events, such as a dispatch that
timed out and was returned to the queue, waits on an idle session until a later
event that may never come. The existing ceiling sweep is the backstop.

## Requirements

### REQ-TASKS-IDLE-QUEUE-DRAIN-001: Queued work on an idle session is eventually offered

**Intent:** A queued message on an idle session with Auto-run on is delivered
without a new event, under the same guards as any other drain.

**User story:** As a user or agent sending a follow-up, I want a queued message
to reach an idle session even when its first dispatch was returned to the queue,
so that the conversation does not silently stop.

#### Acceptance criteria

- **AC-TASKS-IDLE-QUEUE-DRAIN-001.1:** When a session holds a queued message
  that is not reserved by an in-flight dispatch, the session is promptable, and
  its Auto-run policy is on, the ceiling sweep shall offer the head message for
  dispatch without a turn end, enqueue, boot, or user request.
- **AC-TASKS-IDLE-QUEUE-DRAIN-001.2:** The sweep shall dispatch through the same
  path as an Auto-run drain. It shall not change the Auto-run policy, shall not
  dispatch to a running or otherwise non-promptable session, and shall keep the
  existing recovery-block, clarification, cancellation, and in-flight guards.
  A session whose cancellation guard is held shall be skipped for that pass
  rather than waited on, so one busy session cannot stall the sweep.
- **AC-TASKS-IDLE-QUEUE-DRAIN-001.3:** Repeated attempts for one session shall
  back off from one minute up to fifteen minutes, and the backoff shall reset
  once the session no longer holds queued messages and has no dispatch in
  flight. A message returned to the queue by an in-flight dispatch shall wait
  out the current backoff.
- **AC-TASKS-IDLE-QUEUE-DRAIN-001.4:** Queued rows whose session no longer
  exists shall not be offered.

## Compatibility and exclusions

No new setting, goroutine, or dead-letter queue. Queue identity, Auto-merge,
admission, and session targets are unchanged; a message stays on the session it
was queued for.

## Design and delivery

- [System design](../system-design/idle-queue-drain.md)
- [Delivery package](../../../plans/idle-queue-drain-backstop/plan.md)
