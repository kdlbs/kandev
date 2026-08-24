---
status: draft
system: tasks
created: 2026-08-12
owners:
  - nova28
---


# Workflow task-step transition ledger Requirements



## Overview



The task system records forward-looking workflow-step boundaries so downstream consumers can attribute work to a step without reconstructing history.



## Why

Kandev records *when a card was worked on* but not *which workflow step it was
in at the time*. Cost, token, and duration events can be joined to a task and a
session, but attributing them to a step requires reconstructing step boundaries
from the message timeline. On the reference store that reconstruction leaves
**47.0%** of spend cleanly inside one step, **30.5%** merely "dominant of two
steps in the window", and **22.5%** unattributable — so "what does Review cost"
cannot be answered without a caveat larger than the answer.

The one table that looks like it should answer this, `session_step_history`, is
keyed by **session** while Kandev tracks step state on the **task**
(`base_migrations.go` removed `task_sessions.workflow_step_id` for exactly that
reason). A card can change step with no active session, or with several, so
attributing a task-level transition to one session manufactures ownership that
does not exist.

## What this feature is, and is not

**It is a forward-looking record of step boundaries.** From its activation point
onward, every durable change to a task's workflow step is recorded with its
timestamp, its cause, and its actor, so downstream analysis can bound a step
interval from data rather than infer it from chat.

**It cannot repair the past.** No historical transition is reconstructed or
backfilled. Rows before the activation point do not exist and MUST NOT be
synthesised.

**It does not make per-step cost exact.** Cost events bill windows that cross
step boundaries; better boundaries improve attribution but do not decide how one
window's cost divides between two steps. The "dominant of two steps" bucket
shrinks; it does not disappear.

**It is not `workflow_step_decisions`.** That table has a live writer on the
Office approval path; its zero rows mean an unused feature, not a missing one.
The two mechanisms MUST NOT be merged.

**It is not a replacement for `session_step_history`.** This spec neither wires,
removes, nor changes that table or the `GET /api/v1/sessions/:id/workflow/history`
endpoint that reads it. See *Relationship to `session_step_history`*.

## What

The feature ships as two independently valuable slices. **Slice 1 SHALL ship and
be measured before Slice 2 is built** (see *Gate between slices*).

### Slice 1 — turn-start step stamp

- A turn created for a session records the workflow step its task was in at the
  moment the turn started.
- The stamp is present on every turn created after activation whose task has a
  workflow step, and **absent** — never empty, never `0` — when the task has
  none.
- The stamp is immutable once written. A step change during the turn does not
  rewrite it.
- No column is added to `task_session_turns`.

### Slice 2 — task-level transition ledger

- Every committed change to a task's workflow step produces exactly one ledger
  row, written in the same database transaction as the change.
- A row that is not committed with its transition does not exist: a rolled-back
  or WIP-rejected move leaves no row.
- A move that does not change the step (position-only reorder, re-issued move to
  the current step) produces **no** row.
- Each row names the task, the step it left and the step it entered, when, why
  (trigger), what kind of actor caused it, and that actor's **identifier** —
  never a display name.
- The initiating session is recorded when one exists and is genuinely the
  initiator; it is `NULL` otherwise. A transition with no session, or with
  several candidate sessions and no single initiator, MUST record `NULL` rather
  than pick one.
- Each row carries the collection-contract version it was written under.

### Cross-cutting

- Legacy rows and unstamped turns get `NULL` / absent — **never `0`, never
  `""`**.
- Both slices publish a machine-readable activation point, because the
  downstream extract is a point-in-time snapshot with no schema versioning: a
  column whose meaning changes mid-series is silently discontinuous.
- Both writers are observable at runtime, and the set of code paths that may
  mutate a task's workflow step is pinned by a test (see *Writer health*).

## Requirements



### REQ-TASKS-TRANSITION-LEDGER-001: Workflow task-step transition ledger



**Intent:** The task system records forward-looking workflow-step boundaries so downstream consumers can attribute work to a step without reconstructing history.



#### Acceptance criteria



- **AC-TASKS-TRANSITION-LEDGER-001.1:** When a task turn or workflow-step change occurs after activation, the system shall record the applicable step boundary and its provenance.
- **AC-TASKS-TRANSITION-LEDGER-001.2:** When consumers inspect the ledger, the system shall distinguish post-activation evidence from historical intervals that were not backfilled.



## Out of scope



Historical reconstruction, exact per-step cost allocation, and replacement of session_step_history are excluded.