---
id: "06-human-assignee-takeover"
title: "Human assignee and takeover"
status: todo
wave: 3
depends_on: ["04-management-api"]
plan: "plan.md"
spec: "../../specs/workspaces/membership.md"
---

# Task 06: Human Assignee and Takeover

## Acceptance

- `tasks.assignee_user_id` is added via an idempotent migration and is fully
  independent of `assignee_agent_profile_id`. Setting one never clears the
  other, and an Office task with an agent assignee can also carry a human one.
- `PATCH /api/v1/tasks/{id}` accepts `assignee_user_id`, gated on `task.write`.
  A `viewer` receives 403; any caller holding `task.write` may assign to
  themselves.
- Assignment is advisory: it gates nothing. A caller who is not the assignee
  retains every scope they already hold.
- Setting the assignee to a user who cannot reach the workspace is refused.
- Takeover is reassign plus continue: the session, worktree, executor, and ACP
  session are untouched by a reassignment, proven by asserting the execution ID
  and worktree path are unchanged across the operation.
- `assignee_user_id` reaches the task DTO, the boot payload, and the sidebar
  filter dimensions.
- Task lifecycle events publish through the existing event path so the kanban
  updates live.

## Verification

- `go test ./internal/task/... -run 'TestAssigneeUser|TestTakeoverPreservesSession'`
- `KANDEV_TEST_POSTGRES_DSN=... go test ./internal/task/repository/sqlite/...`

## Files Likely Touched

- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/models/models.go`, `internal/task/dto/dto.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/pkg/api/v1/` task DTO

## Inputs

- Spec: What (human assignee, takeover), Permissions, Scenarios.
- Patterns: ADR 0005 Wave F on why the agent assignee is not a task column; the
  task lifecycle event rule in `apps/backend/AGENTS.md` (every task mutation
  publishes, or the kanban goes stale).

## Output Contract

Report the independence test between the two assignee fields, the
session-preservation evidence for takeover, RED/GREEN commands, and set this
task plus its plan checkbox to done.
