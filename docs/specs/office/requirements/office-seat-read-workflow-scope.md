---
status: draft
system: office
created: 2026-09-26
owners:
  - kandev
---

# Office Seat Read Workflow Scope Requirements

## Overview

`review-participant-seats.md`'s REQ-OFFICE-REVIEW-SEATS-003 requires that a
seat be woken by its step's fan-out and counted by its step's quorum guard —
both of which read the workflow engine's own `gatherParticipantSlate`, which
has always been workflow-scoped: a per-task seat is visible at every step of
the task's current workflow, not only the step it was cast at.

Office's own dashboard read projections — `ListTaskParticipants` and
`ListAllTaskParticipants` (`internal/office/repository/sqlite`) — reached the
same seat table through a separate, narrower query: the task's *current* step
only. A seat cast, or manually registered, while a task stood at one step went
invisible to the task DTO's participant list, the inbox review-request
surface, decision authorization, and the approval gate's pending-approver
check the moment the task moved to another step of the same workflow — up to
and including the approval gate reading the resulting empty approver list as
"nothing to wait for" and letting a `done` transition through with an
undecided approver still pending. The manual per-task registration path had
the mirror defect: it recognized an existing seat only at the exact current
step, so re-registering an identity after its seat had moved with the task
wrote a duplicate rather than recognizing the one already there.

This document states the fix, scoped to the Office-specific readers and
writer that REQ-OFFICE-REVIEW-SEATS-003's own acceptance criteria do not name.

## Requirements

### REQ-OFFICE-SEAT-READ-SCOPE-001: Office's own seat readers and writer agree with the engine's slate

Every Office-specific consumer of the participant seat table — not only the
step's own fan-out and quorum guard — shall observe the same workflow-scoped
visibility the engine's slate has always had, and the manual registration
write path shall not create a duplicate seat for an identity the workflow-
scoped read already treats as present.

#### Acceptance criteria

- **AC-OFFICE-SEAT-READ-SCOPE-001.1:** The task DTO's participant list, the
  inbox review-request surface, decision authorization, and the approval
  gate's pending-approver check shall each treat a per-task seat as visible
  across every step of the task's current workflow, not only its current
  step, matching the engine's quorum-slate visibility. A step move shall not
  remove a reviewer's or approver's seat from any of these, nor let the gate
  treat an approver seated at an earlier step as absent.
- **AC-OFFICE-SEAT-READ-SCOPE-001.2:** When a per-task seat already exists
  elsewhere in the task's current workflow for the same task, role and agent
  profile, the manual registration path shall treat re-registering that
  identity as a no-op leaving the seat unchanged, not a second, duplicate
  seat. This extends AC-OFFICE-REVIEW-SEATS-003.5's step-move dedup guarantee
  — stated there for the seat-ensuring auto-cast action — to manual
  registration as well.
- **AC-OFFICE-SEAT-READ-SCOPE-001.3:** Session-termination capacity
  determination (AC-OFFICE-SESSION-TERM-002.3) is unaffected by
  AC-OFFICE-SEAT-READ-SCOPE-001.1: it shall continue to read a seat
  projection scoped to the task's current step alone, because a seat naming a
  previous occupant must not suppress a termination the current step no
  longer justifies. That requirement takes precedence over this one for that
  single caller.

## Out of scope

- **Every other criterion of REQ-OFFICE-REVIEW-SEATS-001, -002, -004 and
  -005.** This document extends only the read-alignment concern of
  REQ-OFFICE-REVIEW-SEATS-003; it makes no claim about seat casting,
  observability, or the onboarding/reconciliation requirements.
- **The engine's own slate-gathering algorithm.** This document constrains
  Office's readers and writer to agree with it, not the algorithm itself.
