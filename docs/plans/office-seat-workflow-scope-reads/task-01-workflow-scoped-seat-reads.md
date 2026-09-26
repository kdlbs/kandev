---
id: "01-workflow-scoped-seat-reads"
title: "Workflow-scope Office's seat read projections"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/office/requirements/office-seat-read-workflow-scope.md"
system_design: "../../specs/office/system-design/office-seat-read-workflow-scope-01.md"
requirements:
  - REQ-OFFICE-SEAT-READ-SCOPE-001
acceptance_criteria:
  - AC-OFFICE-SEAT-READ-SCOPE-001.1
  - AC-OFFICE-SEAT-READ-SCOPE-001.2
---

# Task 01: Workflow-scope Office's seat read projections

## Acceptance

- `ListTaskParticipants` and `ListAllTaskParticipants` return a per-task seat
  cast at any step of the task's current workflow, not only the task's
  current step — matching the workflow engine's own quorum-slate visibility.
- The task DTO, inbox review-request surface, decision authorization, and the
  approval gate's pending-approver check all observe that widened visibility,
  since they all read through the two methods above. In particular, an
  undecided approver whose seat was cast at an earlier step continues to
  block a `done` transition after the task moves to a later step of the same
  workflow — the approval-gate bypass this task fixes.
- `AddTaskParticipant`'s manual registration path treats re-registering an
  identity that already holds a per-task seat elsewhere in the task's current
  workflow as a no-op (`ParticipantWriteOutcomeUnchanged`), not a duplicate
  insert.
- `retainsTaskCapacity` (session-termination capacity) is unaffected: it now
  reads a new, separate, step-scoped `ListTaskParticipantsAtCurrentStep`
  method whose query is byte-identical to the pre-fix `ListAllTaskParticipants`,
  per AC-OFFICE-SESSION-TERM-002.3.
- Zero regressions across `internal/office/...` and `internal/workflow/...`.

## Verification

```bash
cd apps/backend && go test ./internal/office/... ./internal/workflow/... -count=1
make -C apps/backend lint
```

## Files likely touched

- `apps/backend/internal/office/repository/sqlite/participants.go`
- `apps/backend/internal/office/repository/sqlite/participants_workflow_scope_test.go`
- `apps/backend/internal/office/repository/sqlite/participants_test.go`
- `apps/backend/internal/office/repository/sqlite/participant_step_resolution_test.go`
- `apps/backend/internal/office/dashboard/service.go`
- `apps/backend/internal/office/dashboard/service_tasks.go`
- `apps/backend/internal/office/dashboard/participant_seat_workflow_scope_test.go`
- `docs/specs/office/requirements/office-seat-read-workflow-scope.md`
- `docs/specs/office/system-design/office-seat-read-workflow-scope-01.md`
- `apps/backend/internal/office/AGENTS.md`

## Dependencies

None.

## Parallelism

Sequential. The read-projection fix, the write-path dedup fix, and their
regression tests share the same seat-identity model and must be verified
together.

## Inputs

- `docs/specs/office/requirements/office-seat-read-workflow-scope.md`,
  REQ-OFFICE-SEAT-READ-SCOPE-001 (AC-OFFICE-SEAT-READ-SCOPE-001.1/.2/.3),
  which extends the seat-ensuring dedup guarantee already stated in
  `docs/specs/office/requirements/review-participant-seats.md`'s
  AC-OFFICE-REVIEW-SEATS-003.5 to the manual registration write path.
- `docs/specs/office/requirements/task-session-termination.md`,
  AC-OFFICE-SESSION-TERM-002.3 — the deliberate step-scoped exception.
- The workflow engine's `gatherParticipantSlate`
  (`internal/workflow/engine/quorum.go`) as the reference algorithm for
  visibility, canonicalization, and per-task-over-template collapse.
- Existing repo/dashboard test helpers and their `workflow_id` gap (all leave
  `tasks.workflow_id` at the schema default `''`, so a new helper was needed
  to exercise the JOIN-scoped branch rather than only the any-step fallback).

## Output contract

- `internal/office/repository/sqlite/participants.go` exports
  `ListTaskParticipants`, `ListAllTaskParticipants`, and the new
  `ListTaskParticipantsAtCurrentStep`, all against the existing `Participant`
  shape — no DTO or API contract change.
- `dashboard.Repository` interface gains
  `ListTaskParticipantsAtCurrentStep(ctx, taskID) ([]sqlite.Participant, error)`.
