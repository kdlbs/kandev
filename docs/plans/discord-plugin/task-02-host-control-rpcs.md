---
id: "02-host-control-rpcs"
title: "Host: CancelTurn exact execution command"
status: pending
wave: 2
depends_on: ["01-host-control-rpcs-adr"]
plan: "plan.md"
spec: "../../specs/integrations/discord.md"
---

# Task 02: `CancelTurn` exact execution command

Repository: `kdlbs/kandev`. Use `/tdd`.

- **Acceptance:**
  1. `pluginsdk.HostExecutionCommands(host).CancelTurn(ctx, ExactTaskExecutionCommand)`:
     - Admission is the same as `StopTaskRun`: request ID, workspace, task and
       session, `ExpectedSessionResourceVersion`, `ExpectedExecutionID`,
       idempotency key, management claim fence, approval revision, and
       manifest digest under `host.v2.write:execution`.
     - It calls `orchestrator.Service.CancelAgent(sessionID)`. That
       interrupts the turn without terminating the process; the session
       becomes `WAITING_FOR_INPUT` and the queue does not drain.
     - Outcomes: `APPLIED` when a turn was cancelled, `NO_CHANGE` when no turn
       is active, `CONFLICT` on a stale version or execution, and `REPLAY` on
       a repeated idempotency key.
  2. The command appears in `host_call_classification.go`, the proto, the gRPC
     client and server, and the capability-discovery output, like its
     siblings.
  3. The in-tree fixture plugin can call it end to end in the plugin runtime
     tests.
- **Verification:**
  - Regenerate protos with the repository's documented proto target for `pluginsdk/proto`.
  - `cd apps/backend && go test ./internal/plugins/... ./internal/backendapp/...`
  - `cd pluginsdk && go test ./...`
  - `make -C apps/backend lint`
- **Files likely touched:** `pluginsdk/proto/kandev/plugin/v1/plugin.proto` (+ generated), `pluginsdk/exact_execution.go`, `pluginsdk/exact_execution_grpc.go`, `pluginsdk/host_call_classification.go`, `apps/backend/internal/plugins/host_exact.go`, `apps/backend/internal/plugins/host_exact_execution.go` (+ tests), `apps/backend/internal/backendapp/adapters_plugin_execution.go` (+ tests).
- **Dependencies:** task-01.
- **Parallelism:** sequential with task-17 (shared proto).
- **Inputs:**
  - The `StopTaskRun` implementation (`host_exact_execution.go`,
    `adapters_plugin_execution.go:84-111`) as the pattern.
  - `orchestrator.Service.CancelAgent` in
    `apps/backend/internal/orchestrator/task_operations.go`.
- **Risks:** `CancelAgent` must not race claim transfer. Wrap it in
  `withManagementClaimFence` exactly as `StopTaskRun` does.
- **Output contract:** summary, files changed, tests run, blockers, risks, and task/plan status update.

## Results
Pending.
