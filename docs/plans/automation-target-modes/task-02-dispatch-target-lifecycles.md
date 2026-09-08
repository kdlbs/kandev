---
id: "02-dispatch-target-lifecycles"
title: "Dispatch and maintain target lifecycles"
status: completed
wave: 2
depends_on:
  - "01-persist-target-contracts"
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATION-TARGETS-001
  - REQ-OFFICE-AUTOMATION-TARGETS-002
system_design:
  - ../../specs/office/system-design/automation-target-modes.md
acceptance_criteria:
  - AC-OFFICE-AUTOMATION-TARGETS-001.3
  - AC-OFFICE-AUTOMATION-TARGETS-001.4
  - AC-OFFICE-AUTOMATION-TARGETS-002.1
  - AC-OFFICE-AUTOMATION-TARGETS-002.2
  - AC-OFFICE-AUTOMATION-TARGETS-002.3
  - AC-OFFICE-AUTOMATION-TARGETS-002.4
  - AC-OFFICE-AUTOMATION-TARGETS-002.5
  - AC-OFFICE-AUTOMATION-TARGETS-002.6
---

# Task 02: Dispatch and maintain target lifecycles

## Summary

Route automation firings to hidden or visible task creation, allow valid
repository-free scratch launches, and keep continuation, exact run binding,
terminal status, profile selection, and deletion ownership correct for both
targets.

## In scope

- `internal/orchestrator/event_handlers_automation.go` and task creator
  contracts.
- Visible automation task origin and normal task-list/query behavior.
- Exact run completion, stop, stale recovery, and deletion behavior.
- Hidden versus normal MCP/profile selection.
- No-repository lifecycle integration and focused backend tests.

## Out of scope

- Form controls, translations, or browser tests.
- New executor implementations.

## Acceptance

- A hidden automation with no workflow or repository launches in scratch with
  Worktree or a Local-compatible profile.
- A normal-task automation creates a visible workflow task, supports both
  continuity policies, and does not receive `SurfaceAutomation`.
- Completion and stop terminalize only the exact visible or hidden run, and
  deleting an automation leaves visible tasks intact while cleaning hidden
  tasks through existing ownership jobs.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/automation ./internal/orchestrator ./internal/task/service ./internal/task/repository/sqlite
cd apps/backend && go test ./internal/agent/runtime/lifecycle
cd apps/backend && go test -tags fts5 ./internal/mcp/profile ./internal/mcp/scope ./internal/mcp/server ./internal/mcp/handlers
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_automation.go`
- `apps/backend/internal/orchestrator/event_handlers_git.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/repository/sqlite/task.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/agent/runtime/lifecycle/*`
- `apps/backend/internal/mcp/scope/*`
- `apps/backend/internal/automation/*target*test.go`

## Dependencies

Task 01.

## Risks

- The existing hidden lifecycle contains intentional stop and permission
  behavior that must not be applied to visible normal tasks.
- Reused visible tasks may be moved by a person, so compatibility and
  replacement behavior must remain explicit and exact-run safe.

## Parallelism

`sequential`

## Inputs

- Task 01 contract and migration.
- Existing continuation and cleanup design.
- Lifecycle scratch-workspace behavior in
  `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`.

## Results

- Added hidden scratch and visible normal-task dispatch paths with
  distinct task origins and exact automation-run bindings.
- Kept hidden coordinator MCP authority and cleanup limited to hidden tasks;
  visible automation tasks use normal profiles and remain in task lists after
  automation deletion.
- Preserved continuation, exact-turn completion/stop, publish-failure repair,
  profile selection, and cleanup ownership behavior.
- Verification: automation/orchestrator, task service/repository, MCP, and
  lifecycle suites passed (2,464 + 1,908 + 878 + 1,948 tests).
