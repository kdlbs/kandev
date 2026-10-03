---
status: draft
system: office
requirements:
  - REQ-OFFICE-SEAT-READ-SCOPE-001
---

# Office Seat Read Workflow Scope System Design

## Purpose and boundaries

`office-seat-read-workflow-scope.md` asks for two things: Office's own read
projections must observe the workflow engine's existing workflow-scoped seat
visibility, not only the task's current step; and the manual registration
write path must not create a duplicate seat for an identity a workflow-scoped
read already treats as present. This document owns neither the engine's slate
algorithm nor the seat-ensuring auto-cast action — both frozen by
`review-participant-seats.md` — only Office's own agreement with them.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| `REQ-OFFICE-SEAT-READ-SCOPE-001` | *Read projection*, *Write-path dedup*, *Deliberate exception* |

## Read projection

`listWorkflowScopedSeats` (`internal/office/repository/sqlite/participants.go`)
mirrors the engine's `gatherParticipantSlate` for a single task: resolve the
task's current `step_id` (empty → return no seats — there is no template
context to project onto) and `workflow_id`; list per-task rows whose step
joins to that `workflow_id` (falling back to every step when the task carries
no `workflow_id`, the same fallback the engine takes with no
`WorkflowScopedParticipantStore`); list template rows (`task_id = ''`) at the
current step only, since template rows are never workflow-scoped; canonicalize
the per-task rows by `(role, agent_profile_id)` — the row at the current step
wins, else the lowest id — then collapse per-task over template by the same
key. `ListTaskParticipants` and `ListAllTaskParticipants` both delegate to it,
so the task DTO, inbox, decision authorization, and approval gate all inherit
the fix without their own call sites changing.

## Write-path dedup

`probeExistingIdentity`'s exact-step miss now falls through to
`hasParticipantIdentityElsewhereInWorkflowTx`: does the same (task, role,
agent) identity already hold a per-task seat elsewhere in the task's current
workflow? `workflow_id` is resolved on the write's own transaction handle
(`tx.QueryRowContext`, never the read pool), inside `AddTaskParticipant`'s
existing exclusion — the same reason `stepIDForTaskTx` reads the step there
rather than through `r.ro`: a read outside the exclusion could observe a
`workflow_id` the task has since left, racing the write it is meant to guard.
A match reports the identity unchanged, with no insert and no provenance
promotion; `promoteIfSoleUndecided` and the auto-seat claim search stay
step-scoped, unaffected by this widening.

A read failure here (as opposed to a clean "no match") propagates as an
error, aborting `AddTaskParticipant` before any insert is attempted — fail
closed, since a manual registration that cannot confirm the identity is
genuinely absent must not risk writing a duplicate.

## Deliberate exception

`retainsTaskCapacity` (`internal/office/dashboard/service_tasks.go`) decides
whether removing a participant should also end a shared session, and
AC-OFFICE-SESSION-TERM-002.3 requires that determination to stay bound to the
task's *current* step: counting a seat naming a previous occupant would
suppress a termination the current step no longer justifies. It reads a new,
separate method, `ListTaskParticipantsAtCurrentStep` — an unchanged copy of
the pre-fix query — rather than the now workflow-scoped
`ListAllTaskParticipants`. Both methods carry a doc comment pointing at the
other so the split does not look like an oversight.

## Persistence

No schema change. Both new read queries join the existing
`workflow_step_participants` and `workflow_steps` tables on their existing
primary/foreign keys (`step_id`, `workflow_id`); no new index was needed.

## Security

No new authorization surface. Every query here is already scoped to a task
identifier the caller was authorized to read or write before reaching this
layer; this document only widens which *step* of that same task's workflow a
seat is visible at, not which task or workspace.

## Observability

No new counters. This is a read/write-projection correctness fix, not a new
signal — the surfaces it corrects (task DTO, inbox, decision authorization,
approval gate) already have their own existing observability, unaffected in
shape.
