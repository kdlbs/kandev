---
created: 2026-09-26
status: done
requirements:
  - REQ-OFFICE-SEAT-READ-SCOPE-001
system_design:
  - "../../specs/office/system-design/office-seat-read-workflow-scope-01.md"
---

# Implementation Plan: Workflow-scope Office's seat read projections

## Root cause

`ListTaskParticipants` and `ListAllTaskParticipants`
(`internal/office/repository/sqlite/participants.go`) filtered
`workflow_step_participants` by the task's **current** `step_id` only. The
workflow engine's own quorum slate (`gatherParticipantSlate`,
`internal/workflow/engine/quorum.go`) has always read per-task seats across
**every** step of the task's current workflow. A seat cast — or manually
registered — while a task stood at one step went invisible to every Office
consumer of the step-scoped reads the moment the task moved to another step
of the same workflow: the task DTO's participant list, the inbox
review-request surface, decision authorization (`resolveDeciderRole`), and
critically the approval gate's `allApproversApproved`/`pendingApprovers`,
which read the resulting empty approver list as "nothing to wait for" and let
a `done` transition through with an undecided approver still pending. The
manual registration write path (`probeExistingIdentity`) had the mirror bug:
it only checked for an existing seat at the exact current step, so
re-registering a reviewer/approver after their seat had moved with the task
inserted a duplicate seat instead of recognizing the existing one.

## Overview

Add a workflow-scoped read helper (`listWorkflowScopedSeats`) that mirrors the
engine's slate-gathering, canonicalization, and per-task-over-template collapse
for a single task, and route both `ListTaskParticipants` and
`ListAllTaskParticipants` through it. Widen `probeExistingIdentity`'s
existing-seat check to also recognize the same (task, role, agent) identity
elsewhere in the task's current workflow. Introduce a new, deliberately
step-scoped `ListTaskParticipantsAtCurrentStep` method — an unchanged copy of
the old query — for the one caller (`retainsTaskCapacity`, session-termination
capacity) that AC-OFFICE-SESSION-TERM-002.3 requires to stay narrower than the
rest. No engine, `ensure_participant_seat`, `RemoveTaskParticipant`,
`FindParticipantID`, auto-seat claim, frontend, or API contract change.

## Backend

### Workflow-scoped read projection

Files:

- `apps/backend/internal/office/repository/sqlite/participants.go`

`listWorkflowScopedSeats(ctx, taskID, roleFilter)` resolves the task's current
step (empty → return `[]Participant{}`) and `workflow_id`, lists per-task rows
whose step belongs to that workflow (falling back to every step when
`workflow_id` is empty, matching the engine's no-`WorkflowScopedParticipantStore`
fallback), lists template rows at the current step, canonicalizes the per-task
rows by `(role, agent_profile_id)` (current-step row wins, else lowest id),
then collapses per-task over template by the same key. `ListTaskParticipants`
and `ListAllTaskParticipants` both delegate to it.

`ListTaskParticipantsAtCurrentStep` is an exact copy of the pre-fix query,
kept for `retainsTaskCapacity`
(`apps/backend/internal/office/dashboard/service_tasks.go`) alone, per
AC-OFFICE-SESSION-TERM-002.3. Added to the `dashboard.Repository` interface
(`apps/backend/internal/office/dashboard/service.go`), whose sole implementer
is `*sqlite.Repository`.

### Manual registration write-path dedup

Files:

- `apps/backend/internal/office/repository/sqlite/participants.go`

`hasParticipantIdentityElsewhereInWorkflowTx` extends `probeExistingIdentity`'s
miss path: when no seat exists at the exact current step, check whether the
same (task, role, agent) identity holds a per-task seat elsewhere in the
task's current workflow. `workflow_id` is resolved on the write's own
transaction handle (`tx.QueryRowContext`, never `r.ro`), inside
`AddTaskParticipant`'s existing exclusion, for the same reason
`stepIDForTaskTx` reads the step there. A match reports the identity
unchanged, with no insert and no provenance promotion (`promoteIfSoleUndecided`
stays step-scoped, unaffected).

## Tests

- **What:** Workflow-scoped read visibility, dedup across steps, template
  fallback-to-current-step-only, cross-workflow exclusion, and write-path
  no-op-after-move, at the repository layer.
  **File:** `apps/backend/internal/office/repository/sqlite/participants_workflow_scope_test.go`.
  **How:** New two-step-workflow fixtures with an explicit `tasks.workflow_id`
  (existing dashboard/repo test helpers leave it at the schema default `''`,
  which only exercises the any-step fallback).
- **What:** Dashboard-layer consequences of the same fix — a reviewer/approver
  is no longer wrongly forbidden after a step move, and the approval gate no
  longer bypasses an undecided approver whose seat was cast before the move.
  **File:** `apps/backend/internal/office/dashboard/participant_seat_workflow_scope_test.go`.
  **How:** A new two-step-workflow-with-`workflow_id` helper exercises
  `ApproveTask`, `RequestTaskChanges`, and `UpdateTaskStatus` through the
  service layer (dashboard test files are `package dashboard_test`, so only
  the exported surface is reachable).

## Verification Results

- `cd apps/backend && go test ./internal/office/... ./internal/workflow/... -count=1` — passed, zero regressions (including the pinned `TestRemoveParticipant_StaleSeatAtLeftStepDoesNotSuppress`).
- `make -C apps/backend lint` — 0 issues.
- RED verification (both repo-level and dashboard-level suites): implementation files reverted via a saved `git diff` patch, new tests confirmed to fail for the exact root-caused reasons (`ListAllTaskParticipants` empty after a step move; `AddTaskParticipant` outcome `"inserted"` instead of `"unchanged"`; `ApproveTask` returning `ErrForbidden`; `UpdateTaskStatus` returning `nil` instead of `*ApprovalsPendingError`, i.e. the approval-gate bypass), then restored and confirmed byte-identical to the pre-revert diff via `diff`.

## Implementation Waves

Wave 1 (sequential):

- [x] [task-01-workflow-scoped-seat-reads](task-01-workflow-scoped-seat-reads.md) (done)

## Risks

- **AC-OFFICE-SESSION-TERM-002.3 conflict (resolved).** The requirement
  formally binds session-termination capacity determination to the task's
  current step, explicitly to prevent a seat naming a previous occupant from
  suppressing every future termination. Widening `ListAllTaskParticipants`
  unconditionally would have silently violated it. Resolved by splitting
  `retainsTaskCapacity` onto a new, separate, unchanged-behavior
  `ListTaskParticipantsAtCurrentStep` method — confirmed via the full
  regression suite passing, including the pinned stale-seat test.
- Both repository read methods and the manual write path now query
  `workflow_steps` via a JOIN; no index changes were needed (existing indexes
  on `workflow_step_participants(task_id)` and `workflow_steps(id)` cover the
  join and filter).

## Open Questions

None.
