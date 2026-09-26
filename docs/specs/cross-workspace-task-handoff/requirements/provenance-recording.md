---
status: active
system: cross-workspace-task-handoff
created: 2026-09-02
updated: 2026-09-15
owners:
  - nova28
---

# Provenance recording Requirements

## Overview

Every handoff leaves a trail on both sides: a write-once forward record on
the delivery task naming where it came from, and an append-only reverse
record on the source task naming where the work went. Both are readable
through the existing task metadata projection, and both sides also get a
distinct, non-idempotent activity-log entry per call — including on a replay
— so a repair is visible even though it changes no task state.

### REQ-CROSS-WORKSPACE-TASK-HANDOFF-PROVENANCE-001: Forward and reverse provenance, and activity

#### Acceptance criteria

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-PROVENANCE-001.1:** The delivery task
  shall record, written as part of the create request that produces it, a
  single forward-provenance record containing: the source task id, the
  source workspace id, the source session id, the resolved source agent
  profile id, and the handoff timestamp (RFC 3339, UTC, millisecond
  precision, taken once from the server's clock). The record is write-once:
  it shall not be added, patched, or re-stamped on any later call that finds
  the same task, consistent with the reused idempotency contract's rule that
  a second create returns the existing task unchanged.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-PROVENANCE-001.2:** When the run token
  carries an empty source task id or source session id, the call shall be
  refused with HTTP 403 and no write shall occur: a delivery task whose
  provenance names no source, or a source task that cannot be found to
  receive the reverse link, is worse than no delivery task.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-PROVENANCE-001.3:** The source task shall
  record, as an append-only list, one reverse-link entry per handoff it has
  performed, each containing: the delivery task id, the target workspace id,
  and the same handoff timestamp as that handoff's forward record. See the
  reverse-link-integrity requirements for the write discipline this list
  demands.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-PROVENANCE-001.4:** Provenance shall not
  be injected into the delivery task's prompt or description. The prompt is
  the delivery agent's first user message, and provenance is not an
  instruction to it.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-PROVENANCE-001.5:** The system shall
  write one activity-log entry in the source workspace, targeting the source
  task, and one in the target workspace, targeting the delivery task, each
  using an action verb distinct from ordinary task creation and distinct
  from each other, so a handoff is filterable in the activity log. Both
  entries shall be written on every call that reaches the point where
  activity is logged, including a replay that resolved to an already-existing
  task: the activity log records calls, not tasks, and an outcome field on
  each entry (see criterion .6) is what distinguishes a replay's entries from
  the original's. These entries are not idempotent by design and are never
  deduplicated.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-PROVENANCE-001.6:** Each activity entry
  shall carry: an agent actor identity read directly from the signed run
  token (never looked up); the counterpart task id (on the source entry, the
  delivery task id; on the target entry, the source task id); the counterpart
  workspace id (mirrored the same way); and the create-idempotency outcome
  for this call. A run id that is empty because the token was minted without
  one shall be written as an empty run id, never skipped and never
  substituted — every call that reaches this point writes both entries
  regardless of run-id presence, and the call still succeeds.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-PROVENANCE-001.7:** Activity-entry
  failure shall not fail the call or roll back either provenance write. The
  target-side entry is written for durable audit only, on the same basis as
  the source-side entry, even though nothing renders it today.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-PROVENANCE-001.8:** Both provenance
  records shall be readable through the existing task-metadata read path, for
  each task, without a new endpoint, and neither key shall be redacted from
  that projection.

## Out of scope

A UI surface rendering either provenance record or either activity verb. The
reverse direction — writing back to the source task when the delivery task
finishes — is also excluded; the reverse-link record only makes the source
findable, and nothing in this system closes that loop or imposes a deadline
on an open handoff.
