---
id: "06-office-launch-context-guard"
title: "Guard Office launch context"
status: done
wave: 4
depends_on: []
plan: "plan.md"
spec: "../../specs/office/requirements/agents.md"
---

# Task 06: Guard Office launch context

## Acceptance

- Every orchestrator path selecting `ModeOffice` verifies the
  scheduler-provided CLI path, API URL, signed token, agent/workspace identity,
  run ID, and task-bound task ID before starting the agent process.
- A generic manual or workflow launch of an Office-owned task without that
  context fails closed with actionable guidance.
- Generic Office resume and prepared-workspace relaunch paths fail closed and
  require a scheduler-owned fresh run.
- Scheduler launches through `StartTaskWithEnv` and non-Office task launches
  keep their existing behavior.

## Verification

```bash
cd apps/backend && rtk go test ./internal/orchestrator -run 'Test(StartCreatedSession|StartTask).*Office.*(RuntimeEnv|LaunchContext)' -count=1
```

## Files Likely Touched

- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/task_operations_test.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow_profile_test.go`
  only when an existing generic Office auto-start assertion must be updated to
  the fail-closed contract.

## Inputs

- `docs/specs/office/requirements/agents.md`, sections **Runtime**, **Failure modes**, and
  **Scenarios / CLI and MCP**.
- `StartCreatedSession`, `startTask`, and `StartTaskWithEnv` in
  `apps/backend/internal/orchestrator/task_operations.go`.
- Scheduler environment construction in
  `apps/backend/internal/office/service/env_builder.go`.

## Output Contract

Use TDD. Report the red and green test results, exact launch paths guarded,
files changed, risks, and any path that can still select `ModeOffice` without
the signed runtime context. Update this task to `done` only after targeted
verification passes.

## Completion Evidence

- Added a shared Office runtime-environment validator and task binding, then
  applied it to created-session starts and direct task starts.
- Guarded `ResumeTaskSession`, lazy session recovery, and prepared-workspace
  relaunches so Office recovery cannot bypass the scheduler.
- The red regression run failed the new resume/task-binding scenarios before
  the guard and exposed the existing direct-guard wording assertions.
- The green focused run passed six Office launch and recovery scenarios,
  including a complete context map bound to a different task.

Files changed:

- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/task_operations_test.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow_profile_test.go`
- `apps/backend/cmd/agentctl/kandev_test.go`
- `docs/specs/office/requirements/agents.md`
- `docs/public/automation-and-mcp.md`
- This plan and its task records.

Risks:

- Generic manual, workflow, resume, and prepared-workspace paths now reject
  Office-owned work until the Office scheduler supplies a fresh context. This
  is intentional fail-closed behavior and may surface a clearer error where a
  previously unsafe launch was attempted.
- The task binding prevents context reuse across tasks; JWT signature and
  expiry validation remains owned by the Office runtime API middleware.

Remaining ModeOffice paths:

- None in the orchestrator launch and recovery paths. Scheduler launches use
  `StartTaskWithEnv` with the task-bound context; direct executor unit tests do
  not select `ModeOffice` through production task operations.
- `cd apps/backend && rtk go test ./internal/orchestrator -count=1` passed.
