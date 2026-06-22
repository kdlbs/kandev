---
id: "03-agentctl-process-group-shutdown"
title: "Agentctl process group shutdown"
status: completed
wave: 2
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/runtime-cleanup.md"
---

# Task 03: Agentctl Process Group Shutdown

## Acceptance

- Agentctl instance shutdown cannot leave an ACP subprocess group reparented to
  PID 1.
- Processes that ignore stdin EOF are killed by process group after the stop
  timeout.
- Regression tests prove child processes are reaped on Unix.

## Verification

```bash
cd apps/backend && go test ./internal/agentctl/server/process
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/procattr_unix.go`
- `apps/backend/internal/agentctl/server/process/manager_test.go`
- `apps/backend/internal/agentctl/server/process/testmain_test.go`

## Dependencies

None. Can run in parallel with Task 01/02 because it is isolated to agentctl
process shutdown.

## Inputs

- Existing process group setup and `waitForProcessExit` in `process.Manager`
- Spec failure mode: ACP children must not survive agentctl shutdown

## Output contract

Report shutdown behavior, Unix-only guards if added, tests run, and any platform
differences that remain.

## Result

- Existing Linux process attributes already start agentctl children in their own
  process group with parent-death signaling.
- Existing process manager coverage already verifies process-group kill on stop
  timeout.
- Verified with `go test ./internal/agentctl/server/process ./internal/agentctl/server/instance`.
