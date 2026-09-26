---
id: "01-create-runner-seat"
title: "Validate and seat the create-time Office assignee"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-SCHEDULER-003
acceptance_criteria:
  - AC-OFFICE-SCHEDULER-003.1
  - AC-OFFICE-SCHEDULER-003.2
  - AC-OFFICE-SCHEDULER-003.3
  - AC-OFFICE-SCHEDULER-003.4
  - AC-OFFICE-SCHEDULER-003.5
system_design:
  - ../../specs/office/system-design/scheduler-02.md
---

# Task 01: Validate and seat the create-time Office assignee

## Acceptance

- `POST` task-create accepts `assignee_agent_profile_id` in the request body
  and forwards it into `service.CreateTaskRequest`.
- A request naming an `assignee_agent_profile_id` that resolves to an
  Office agent instance scoped to the request's own workspace succeeds
  regardless of the profile's `enabled` flag, and the created task's runner
  seat (`workflow_step_participants`, `role='runner'`) names that profile.
- A request naming an `assignee_agent_profile_id` that does not resolve to
  any profile, resolves to a profile scoped to a different workspace, or
  resolves to a profile with no workspace (`WorkspaceID == ""`) is rejected
  with an HTTP 4xx before any task row is written.
- A duplicate request sharing an already-used `external_id` returns the
  previously created task without re-running this validation against the
  duplicate's own `assignee_agent_profile_id` and without writing a second
  runner seat.
- A request that omits `assignee_agent_profile_id` is unaffected: the task
  is created with no runner seat, exactly as before this change.
- A request that names an Office assignee for a non-Office task is rejected
  before insertion. Project-linked tasks and tasks on the workspace's
  canonical Office workflow remain eligible.
- The New Task dialog sends `assignee_agent_profile_id` as a top-level
  create-payload field, not nested inside `metadata`.
- The dead reviewer/approver "Stages" UI (`execution_policy`,
  `new-task-stages.tsx`) is removed, since it had no backend reader and is
  superseded by derived seat-casting on gated-step entry.
- The dialog does not show reviewer or approver controls until their create
  request contract is implemented.

## Verification

- `TMPDIR=/root/.cache/kandev-pr3978-scratch.IEyzod go test ./internal/task/handlers -run '^(TestCreateTaskRejectsOfficeAssigneeForKanbanWorkflow|TestBetaOfficeCreateContract)$' -count=1`
  passed. The regression uses a real temp-file SQLite database because
  `assignee_agent_profile_id` is a computed projection over
  `workflow_step_participants`, not a stored column.
- `pnpm test -- app/office/components/new-task-dialog.test.tsx app/office/components/new-task-dialog-create-payload.test.tsx`
  passed: 2 files and 5 tests.
- `pnpm e2e:run --project mobile-chrome tests/office/mobile-new-task-dialog.spec.ts`
  passed: 1 mobile test. The managed runner built the backend and web assets.
- `python3 scripts/lint-spec-files.py --all` passed.
- `python3 scripts/list-docs.py validate` passed.

The full backend suite and full E2E suite were not run for this fixup.
