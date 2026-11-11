---
status: active
system: cross-workspace-task-handoff
created: 2026-11-04
owners:
  - kandev
---

# Completion report and source-task wake Requirements

## Overview

A delivery task can finish after the source agent's run history has been
pruned. The handoff's protected task metadata remains the durable link to the
source task. Current collaboration grants decide whether the source task gets
a report, a prompt, or both.

### REQ-CROSS-WORKSPACE-TASK-HANDOFF-COMPLETION-001: Report and wake a source task

#### Acceptance criteria

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-COMPLETION-001.1:** When a delivery task
  changes from a non-terminal state to a terminal state, the system shall
  verify its protected `handoff_source` record, the source task's append-only
  `handoffs` reverse link, the source session, the source agent, both
  workspaces, and the delivery task's current state. It shall not require a
  source run row to remain in retention.
- **AC-CROSS-WORKSPACE-TASK-HANDOFF-COMPLETION-001.2:** The current directional
  collaboration grants shall authorize report and prompt actions separately.
  `report` alone shall create a report and shall not start a run. `prompt` alone
  shall start a source-task run without creating a report comment. A grant with
  both permissions shall do both. Neither permission shall do neither.
- **AC-CROSS-WORKSPACE-TASK-HANDOFF-COMPLETION-001.3:** A report shall be an
  idempotent comment on the source task. The comment shall identify the
  delivery task and its terminal state, and its author shall be the source
  agent recorded in trusted handoff provenance. An unassigned source task may
  receive the report. The internal report shall not be relayed to external
  channels.
- **AC-CROSS-WORKSPACE-TASK-HANDOFF-COMPLETION-001.4:** The report comment and
  its outbox receipt shall be written atomically. The outbox shall track report
  event publication and source-task wake delivery separately. A failed event
  publication or a paused source workspace shall retain its completion key and
  payload for recovery. It shall not convert a completion wake into a generic
  task-assignment wake.
- **AC-CROSS-WORKSPACE-TASK-HANDOFF-COMPLETION-001.5:** Before a pending wake
  is retried, the system shall recheck the current prompt grant and source
  assignment. If the grant is revoked, the source task changes, or the
  assignee changes, the system shall resolve the wake without starting a run.
  An authorized wake shall carry the delivery task title and terminal state.
- **AC-CROSS-WORKSPACE-TASK-HANDOFF-COMPLETION-001.6:** A completion wake shall
  use reason `handoff_completed`, keep a distinct idempotency key for each
  delivery task and terminal state, and skip run coalescing. The reason shall
  be in the attended run-provenance allowlist. Tests shall cover authorization,
  duplicate events, prompt context, source-task reassignment, paused retry,
  event-publication retry, and retention-safe provenance.

## Out of scope

Reopening or changing the state of either task, and setting a deadline for an
open handoff.
