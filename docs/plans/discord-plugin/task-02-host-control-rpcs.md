---
id: "02-host-control-rpcs"
title: "Host: CancelTurn exact execution command"
status: done
wave: 2
depends_on: ["01-host-control-rpcs-adr"]
plan: "plan.md"
requirements:
  - REQ-PLUGINS-CHAT-BRIDGE-HOST-002
acceptance_criteria:
  - AC-PLUGINS-CHAT-BRIDGE-HOST-002.1
  - AC-PLUGINS-CHAT-BRIDGE-HOST-002.2
  - AC-PLUGINS-CHAT-BRIDGE-HOST-002.3
  - AC-PLUGINS-CHAT-BRIDGE-HOST-002.4
  - AC-PLUGINS-CHAT-BRIDGE-HOST-002.5
system_design:
  - ../../specs/plugins/system-design/chat-bridge-host-controls.md
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
`HostExecutionCommands(host).CancelTurn` and the `CancelTurnExact` RPC are
admitted exactly like `StopTaskRun` (same validation, `host.v2.write:execution`
grant, manifest digest, idempotency ledger) through a shared
`runSessionExecutionEffect`. The backendapp adapter shares the claim fence and
the observed session/execution check with `StopTaskRun`, interrupts only a
`RUNNING` or `STARTING` session through `orchestrator.Service.CancelAgent`
(which pauses queued auto-run), and reports `NO_CHANGE` otherwise. Capability
discovery lists the method.

- **Files:** `pluginsdk/exact_execution.go`, `pluginsdk/exact_execution_grpc.go`,
  `pluginsdk/host_call_classification.go`, `plugin.proto` (+ generated),
  `apps/backend/internal/plugins/{host_exact.go,host_exact_execution.go,exact_execution_contract.go}`,
  `apps/backend/internal/backendapp/adapters_plugin_execution.go`, tests in
  `pluginsdk/exact_execution_grpc_test.go`,
  `apps/backend/internal/plugins/host_exact_execution_test.go`
  (`TestExactCancelTurnOutcomes`: CONFLICT, APPLIED, ALREADY_APPLIED without a
  new effect, NO_CHANGE, DENIED on a stale grant revision;
  `TestExactCancelTurnOverWire` through the real broker transport), and
  `apps/backend/internal/backendapp/adapters_plugin_execution_test.go`.
  `docs/public/plugins-authoring.md` documents the command.
- **Acceptance 3:** the end-to-end proof is the broker-transport test above;
  the in-tree fixture plugin has no stop or cancel action to extend, and a
  fixture action would only exercise the same path.
- **Tests:** `cd apps/backend && go test ./internal/plugins/... ./internal/backendapp/...`
  and `cd pluginsdk && go test ./...` passed; protos regenerated with the
  pinned buf (`make proto` toolchain); changed-package lint ran.
