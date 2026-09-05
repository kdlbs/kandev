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

### REQ-TASKS-GUARDED-MESSAGE-QUEUE-002: Canonical cross-sender routine wake coalescing

Pending scheduled automation wakes must not amplify a Coordinator queue while
distinct input remains lossless.

#### Acceptance criteria

- **AC-TASKS-GUARDED-MESSAGE-QUEUE-002.1:** When trusted scheduled carriers in one workspace emit the same canonical routine generation and identical expanded payload, the system shall retain one effective pending wake at the original FIFO position regardless of carrier task, session, message, automation, or trigger identity.
- **AC-TASKS-GUARDED-MESSAGE-QUEUE-002.2:** The canonical identity shall contain authenticated `workspace_id`, routine type/name, policy/version generation, and semantic scope generation. The coalescing key shall additionally contain a digest of the complete expanded payload.
- **AC-TASKS-GUARDED-MESSAGE-QUEUE-002.3:** The system shall not routine-coalesce peer, task, human, webhook, pull-request event, or materially different scheduled payloads.
- **AC-TASKS-GUARDED-MESSAGE-QUEUE-002.4:** A claimed routine row shall remain durable until prompt acceptance. Identical arrivals during the claim or resulting turn shall occupy at most one pending successor, including at normal queue capacity and after process restart.
- **AC-TASKS-GUARDED-MESSAGE-QUEUE-002.5:** A body-free Host receipt shall identify the canonical row, absorbed source entry IDs and timestamps, absorption count, leader fencing token, dirty generation, and whether execution caused a post-run successor.
- **AC-TASKS-GUARDED-MESSAGE-QUEUE-002.6:** The Host shall reject a trusted routine carrier unless its authenticated workspace matches the target task and an explicitly named target session is the task's current primary session.
- **AC-TASKS-GUARDED-MESSAGE-QUEUE-002.7:** The Host shall count absorbed routine wakes as suppressed duplicate full-board scans.

## Exclusions

- Reading queued message bodies through the census API.
- Clearing all entries, selecting by sender or text, or accepting unrestricted message bodies for disposal.
- Removing a durable lifecycle row already reserved for delivery.
- Coalescing distinct messages merely because they share a sender or target.
- Cross-task, cross-session, or cross-workspace queue management.
- Scheduler policy, dirty-generation consumption, or Coordinator-plugin leader election.
