---
id: "04-agent-temp-teardown"
title: "Agent temp teardown"
status: superseded
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/system-page/storage-maintenance.md"
superseded_by: "../storage-maintenance/task-10-inherit-service-temp.md"
---

# Task 04: Agent temp teardown

> Superseded on 2026-07-22 by the inherited service-temp boundary in ADR 0045 and Storage
> Maintenance Task 10. This file records the earlier implementation packet; the per-instance
> directory and cleanup behavior below is no longer active.

## Acceptance

- Agent instance teardown removes its exact collision-resistant `<system-temp>/kandev-agent/<readable-prefix>-<identity-digest>` directory only after owned subprocesses are stopped and reaped.
- Cleanup also occurs when `Stop` observes an already-stopped main process, while sibling session directories and the shared agent-temp root remain intact.
- Empty, root-equal, or out-of-root cleanup targets fail closed; cleanup errors are returned to the teardown caller.
- Terminal teardown closes all process admission, joins VS Code startup generations, and verifies process-tree reaping before cleanup.
- Failed HTTP or process-tree teardown retains the stopping instance and port for retry. A
  temporary-directory-only failure retains a retry tombstone but releases the port, and a later
  retry cannot release a port that has already been reassigned.

## Verification

```bash
cd apps/backend
rtk go test ./internal/agentctl/server/process -run 'TestManager_.*Temp'
rtk go test -race ./internal/agentctl/server/process -run 'TestManager_.*Temp'
rtk go test ./internal/agentctl/server/process -run 'TestProcessRunnerStopAllAndWait'
rtk go test -race ./internal/agentctl/server/process -run 'TestProcessRunnerStopAllAndWait'
rtk go test ./internal/agentctl/server/instance -run 'TestStopInstance.*(CleanupFailure|CloseFails)'
rtk go test -race ./internal/agentctl/server/instance -run 'TestStopInstance.*(CleanupFailure|CloseFails)'
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/manager_temp_test.go`
- `apps/backend/internal/agentctl/server/process/manager_test.go`
- `apps/backend/internal/agentctl/server/process/runner.go`
- `apps/backend/internal/agentctl/server/process/runner_test.go`
- `apps/backend/internal/agentctl/server/process/vscode.go`
- `apps/backend/internal/agentctl/server/process/vscode_test.go`
- `apps/backend/internal/agentctl/server/process/manager_rescan.go`
- `apps/backend/internal/agentctl/server/api/vscode_handlers.go`
- `apps/backend/internal/agentctl/server/shell/manager.go`
- `apps/backend/internal/agentctl/server/shell/session.go`
- `apps/backend/internal/agentctl/server/shell/process_group_unix.go`
- `apps/backend/internal/agentctl/server/shell/process_group_windows.go`
- `apps/backend/internal/agentctl/server/shell/session_stop_unix_test.go`
- `apps/backend/internal/agentctl/server/instance/instance.go`
- `apps/backend/internal/agentctl/server/instance/manager.go`
- `apps/backend/internal/agentctl/server/instance/manager_shutdown_test.go`

## Dependencies

None.

## Inputs

- Agent-session temporary-data requirements in `docs/specs/system-page/storage-maintenance.md`.
- ADR 0045's lifecycle-owned ephemeral-resource amendment.
- Existing `ensureAgentTempEnv`, `Manager.Stop`, and process-group teardown order.

## Output contract

Report the stored ownership path, containment validation, teardown ordering, already-stopped behavior, cleanup error propagation, changed files, and targeted race-test results. Do not delete the shared temp root, sibling sessions, or any path derived solely from mutable child environment variables.
