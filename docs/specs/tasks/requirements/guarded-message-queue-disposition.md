---
status: active
system: tasks
created: 2026-09-01
owners:
  - kandev
---

# Guarded Message Queue Census and Disposition Requirements

## Overview

Long-running Coordinator sessions can accumulate stale pending messages while
continuing to receive distinct task, peer, and human input. The task system
must let an agent inspect and dispose of exact pending entries without exposing
an unrestricted message-body API or allowing a broad queue clear.

## Requirements

### REQ-TASKS-GUARDED-MESSAGE-QUEUE-001: Content-free queue census and exact disposition

The calling agent can inspect and dispose of pending entries only in its own
current task session.

#### Acceptance criteria

- **AC-TASKS-GUARDED-MESSAGE-QUEUE-001.1:** When an in-session agent requests a queue census, the system shall return visible pending entries in FIFO order with immutable entry IDs, opaque snapshot claims, safe provenance, content hashes and sizes, capacity, and the current count without returning message bodies.
- **AC-TASKS-GUARDED-MESSAGE-QUEUE-001.2:** When the task ID, session ID, or workspace binding does not match the server-derived caller principal, the system shall reject census and disposition without reading or changing the queue.
- **AC-TASKS-GUARDED-MESSAGE-QUEUE-001.3:** When a caller submits an entry ID and claim from a census, the system shall remove that row only if the same session-scoped row still matches the complete observed snapshot.
- **AC-TASKS-GUARDED-MESSAGE-QUEUE-001.4:** When an entry changed, was already removed, or was replaced after census, the system shall report `changed` or `not_found` for that entry and shall not remove a different or newer row.
- **AC-TASKS-GUARDED-MESSAGE-QUEUE-001.5:** When exact entries are disposed concurrently or the same request is retried, at most one caller shall report each row as `removed`; every caller shall receive atomic before and after counts plus a per-entry outcome.
- **AC-TASKS-GUARDED-MESSAGE-QUEUE-001.6:** Queue census and disposition shall preserve persisted FIFO order, capacity policy, durable reserved-delivery rows, and restart recovery for all entries not exactly removed.
- **AC-TASKS-GUARDED-MESSAGE-QUEUE-001.7:** The system shall emit structured audit evidence containing caller-bound session identity, before and after counts, entry IDs, and per-entry outcomes without logging message bodies.

### REQ-TASKS-GUARDED-MESSAGE-QUEUE-002: Identical scheduled routine wake coalescing

Pending scheduled automation wakes must not amplify a Coordinator queue while
distinct input remains lossless.

#### Acceptance criteria

- **AC-TASKS-GUARDED-MESSAGE-QUEUE-002.1:** When two pending messages target the same session and come from the same trusted scheduled automation and trigger with identical expanded payloads, the system shall retain one effective wake at the original FIFO position.
- **AC-TASKS-GUARDED-MESSAGE-QUEUE-002.2:** The coalescing key shall include the target session through queue scope, stable automation and trigger identity, and a digest of the complete expanded payload.
- **AC-TASKS-GUARDED-MESSAGE-QUEUE-002.3:** The system shall not routine-coalesce peer, task, human, webhook, pull-request event, or materially different scheduled payloads.
- **AC-TASKS-GUARDED-MESSAGE-QUEUE-002.4:** Concurrent identical wake admission and admission after process restart shall retain at least one wake and shall not consume extra queue capacity.

## Exclusions

- Reading queued message bodies through the census API.
- Clearing all entries, selecting by sender or text, or accepting unrestricted message bodies for disposal.
- Removing a durable lifecycle row already reserved for delivery.
- Coalescing distinct messages merely because they share a sender or target.
- Cross-task, cross-session, or cross-workspace queue management.
