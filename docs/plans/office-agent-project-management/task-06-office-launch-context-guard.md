---
id: "06-office-launch-context-guard"
title: "Guard Office launch context"
status: done
wave: 4
depends_on: []
plan: "plan.md"
spec: "../../specs/office/agents.md"
---

# Task 06: Guard Office launch context

## Acceptance

- Every orchestrator path selecting `ModeOffice` verifies the
  scheduler-provided CLI path, API URL, signed token, agent/workspace identity,
  and run ID before starting the agent process.
- A generic manual or workflow launch of an Office-owned task without that
  context fails closed with actionable guidance.
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

- `docs/specs/office/agents.md`, sections **Runtime**, **Failure modes**, and
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

- Added one shared Office runtime-environment validator and applied it to
  created-session starts, direct task starts, workflow auto-starts through
  `StartCreatedSession`, and prepared-workspace agent starts.
- The red regression run failed all four scenarios because each path previously
  started without Office runtime context.
- The green focused run passed all four scenarios.
- `cd apps/backend && rtk go test ./internal/orchestrator -count=1` passed.
