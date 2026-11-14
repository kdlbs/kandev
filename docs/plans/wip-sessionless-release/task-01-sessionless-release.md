---
id: "01-sessionless-release"
title: "Sessionless release and occupancy read"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WIP-LIMIT-PULL-SYSTEM-001
acceptance_criteria:
  - AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.12
  - AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.13
system_design:
  - ../../specs/tasks/system-design/wip-limit-pull-system.md
---

# Task 01: Sessionless release and occupancy read

## Summary

Add the workspace policy, apply it to every admitted-occupant count, pull
queued work after the grace from the existing reaper tick, and expose the
occupancy explanation.

## In scope

- Workspace columns `wip_sessionless_release_enabled` (default `false`) and
  `wip_sessionless_release_grace_minutes` (default `10`, 1 to 525600), with
  schema, replayable migration, repository, service validation, HTTP and
  WebSocket update, DTO, events, and boot payload.
- One release predicate appended to every admitted-occupant count only when
  the step's workspace has the policy on.
- `ListSessionlessWIPReleaseSteps` and a reaper-tick pass that runs the normal
  pull for each listed step.
- `GET /api/v1/workflow/steps/:id/wip-occupants` with the task-count access
  rule.
- Workspace settings card with a switch and grace input, saved through the
  settings save bar, translated in every shipped locale.

## Out of scope

- Moving, re-stating, or un-admitting the released task.
- Board UI for the occupancy read.
- Treating stuck `CREATED` or other live session states as released.

## Acceptance

1. With the policy off, every count, promotion, and capacity check is unchanged.
2. With it on, an `IN_PROGRESS` or `FAILED` admitted task with no live session
   stops counting after the grace and the next queued task is promoted; a live
   or re-entered session counts again.
3. The occupancy read's `counted` equals the admission count and names a reason
   for every admitted task.

## Verification

```bash
(cd apps/backend && go test -trimpath -count=1 -run 'TestWIPSessionlessRelease|TestWIPStepOccupancy' ./internal/task/repository/sqlite/)
(cd apps/backend && go test -trimpath -count=1 -run 'TestReleaseSessionlessWIPSlots|TestWorkflowStepQueueLimitChange' ./internal/orchestrator/)
(cd apps/backend && go test -trimpath -count=1 -run 'TestWorkspaceWIPSessionlessRelease|TestWorkspaceIdlePolicy' ./internal/task/service/)
(cd apps/backend && go test -trimpath -count=1 -run 'TestHTTPGetStepWIPOccupants|TestHTTPGetStepTaskCount' ./internal/task/handlers/)
(cd apps/web && pnpm exec vitest run lib/ws/handlers/workspaces.test.ts lib/routing/route-bootstrap.test.ts app/settings/workspace/ components/settings/workspaces/workspace-wip-release-card.test.tsx)
(cd apps/web && pnpm run i18n:check && pnpm run typecheck)
```

All listed backend and frontend checks passed before this work order was
marked done.

## Files

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/models/workspace_field_updates.go`
- `apps/backend/internal/task/models/wip_occupancy.go`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/workspace.go`
- `apps/backend/internal/task/repository/sqlite/workspace_field_updates.go`
- `apps/backend/internal/task/repository/sqlite/task.go`
- `apps/backend/internal/task/repository/sqlite/task_transfer.go`
- `apps/backend/internal/task/repository/sqlite/wip_sessionless_release.go`
- `apps/backend/internal/task/repository/sqlite/wip_sessionless_release_test.go`
- `apps/backend/internal/task/service/*` workspace request, validation, and occupancy read
- `apps/backend/internal/task/handlers/*` workspace update fields and occupancy route
- `apps/backend/internal/task/dto/dto.go`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/backend/internal/orchestrator/workflow_store_wip_release.go`
- `apps/backend/internal/orchestrator/idle_session_reaper.go`
- `apps/web/app/settings/workspace/workspace-wip-release-*.ts(x)`
- `apps/web/components/settings/workspaces/workspace-wip-release-card.tsx`
- `apps/web/lib/**` workspace types, WebSocket handler, boot mapping
- `apps/web/src/locales/*/workspaces.json`
- `docs/public/use-kandev.md`
- `docs/public/workflow-tips.md`
