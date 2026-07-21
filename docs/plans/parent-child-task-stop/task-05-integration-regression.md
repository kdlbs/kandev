---
id: "05-integration-regression"
title: "Integrated stop regression"
status: done
wave: 5
depends_on: ["01-execution-stop-semantics", "02-coordinator-stop-operation", "03-mcp-stop-handler", "04-task-mcp-tool"]
plan: "plan.md"
spec: "../../specs/tasks/parent-child-task-stop.md"
---

# Task 05: Integrated stop regression

## Acceptance

- Real orchestrator, dispatcher, and MCP backend-handler wiring lets a direct
  parent stop a long-running child without waiting for turn completion or
  dispatching a replacement prompt.
- Action response returns promptly; child session becomes `CANCELLED`, eligible
  child task becomes `REVIEW`, and the simulated lifecycle reports the
  execution is no longer running. The test does not require simulated-manager
  map removal.
- A repeat call returns `not_running` and changes no additional state.
- The test does not claim to exercise the local task-mode MCP transport adapter;
  its exact action/payload forwarding is covered in Task 04.

## Verification

```bash
cd apps/backend && go test ./internal/integration -run TestMCPStopTask
```

## Files Likely Touched

- `apps/backend/internal/integration/mcp_stop_task_test.go`

## Dependencies

- Tasks 01-04 complete the lifecycle, coordinator operation, backend action, and
  tool adapter.

## Inputs

- Spec scenarios for direct-parent success, no replacement prompt, and
  idempotent retry.
- Existing orchestrator integration harness and mock-agent long-turn patterns.

## Output Contract

Update this task to `done`, update `plan.md`, and report observed timing/state,
files changed, tests run, blockers, and any unverified process-cleanup risks.

## Completion

- Added a real orchestrator, MCP-handler, and shared-dispatcher regression for a
  long-running direct child.
- Deliberately blocked simulated runtime teardown and proved the response lands
  after session `CANCELLED` and task `REVIEW`, but before process stop finishes.
- Verified no replacement prompt or queued message is created, runtime stop
  completes after release, and a repeat returns `not_running` without mutation.
- Passed the focused test, 20 repeated runs, and the focused race-detector run.
- Remaining lifecycle boundary: the test proves the simulated runtime reaches
  stopped status, not operating-system cleanup for every production provider.
