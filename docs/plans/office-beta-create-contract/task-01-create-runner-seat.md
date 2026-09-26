---
id: "01-create-runner-seat"
title: "Validate and seat the create-time Office assignee"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/office/requirements/scheduler.md"
---

# Task 01: Validate and seat the create-time Office assignee

## Acceptance

- `POST` task-create accepts `assignee_agent_profile_id` in the request body
  and forwards it into `service.CreateTaskRequest`.
- A request naming an `assignee_agent_profile_id` that resolves to an
  enabled Office agent instance scoped to the request's own workspace
  succeeds, and the created task's runner seat
  (`workflow_step_participants`, `role='runner'`) names that profile.
- A request naming an `assignee_agent_profile_id` that does not resolve to
  any profile, resolves to a disabled profile, resolves to a profile scoped
  to a different workspace, or resolves to a profile with no workspace
  (`WorkspaceID == ""`) is rejected with an HTTP 4xx before any task row is
  written.
- A duplicate request sharing an already-used `external_id` returns the
  previously created task without re-running this validation against the
  duplicate's own `assignee_agent_profile_id` and without writing a second
  runner seat.
- A request that omits `assignee_agent_profile_id` is unaffected: the task
  is created with no runner seat, exactly as before this change.
- The New Task dialog sends `assignee_agent_profile_id` as a top-level
  create-payload field, not nested inside `metadata`.
- The dead reviewer/approver "Stages" UI (`execution_policy`,
  `new-task-stages.tsx`) is removed, since it had no backend reader and is
  superseded by derived seat-casting on gated-step entry.

## Verification

- RED/GREEN backend: `TestBetaOfficeCreateContract`
  (`apps/backend/internal/task/handlers/assignee_agent_profile_contract_test.go`)
  fails against pre-fix `task_http_handlers.go` (no assignee field decoded)
  and passes after the fix. Uses a real temp-file SQLite database because
  `assignee_agent_profile_id` is a computed projection over
  `workflow_step_participants`, not a stored column.
- RED/GREEN frontend:
  `apps/web/app/office/components/new-task-dialog-create-payload.test.tsx`
  fails when the dialog nests the assignee under `metadata` (reproduced by
  temporarily reintroducing that shape) and passes against the fix.
- `cd apps/backend && go build ./...`
- `cd apps/backend && go test ./internal/task/handlers/... -run TestBetaOfficeCreateContract -v`
- `cd apps/backend && go test ./internal/task/... ./internal/backendapp/... ./internal/mcp/...`
- `cd apps/web && pnpm test -- app/office/components/new-task-dialog-create-payload.test.tsx app/office/components/new-task-dialog.test.tsx`
- `python3 scripts/lint-spec-files.py --all`
- `python3 scripts/list-docs.py validate`
