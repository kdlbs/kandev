---
id: "03-task-host-supervisor"
title: "Task-host runtime supervisor"
status: pending
wave: 2
depends_on: ["02-state-contracts"]
plan: "plan.md"
spec: "../../specs/lsp-file-intelligence/spec.md"
---

# Task 03: Task-Host Runtime Supervisor

## Acceptance

- One agentctl instance owns at most one process/JSON-RPC peer for each language; Start and Restart
  generations are idempotent and old process-tree death precedes replacement launch.
- Agentctl, not a browser, performs initialize/configuration/progress handling and exposes honest
  process-started, initializing, work, ready/idle, stopping, and error snapshots.
- Closing a control/watch request does not stop the server; instance teardown does. A live LSP
  prevents the agentctl idle reaper from treating the instance as idle.

## TDD sequence

1. Add synchronized failing manager tests for duplicate Start, generation retry, concurrent
   Start/Stop/Restart, initialization progress before response, capability capture, configuration,
   graceful shutdown, forced cleanup, crash, and manager teardown.
2. Extract process/install/bridge ownership from `api/lsp.go` into an instance-owned manager with
   explicit `Start`/`Close` and joined goroutines.
3. Implement a task-host JSON-RPC peer using `internal/lsp/protocol`; perform initialize centrally
   and retain progress/capability/diagnostic snapshots by generation.
4. Add authenticated control/snapshot/watch adapters and a narrow agentctl instance background-work
   signal. Prove watch/control disconnect is non-owning.
5. Run race and leak-focused repetitions; refactor manager state only while synchronized tests stay
   green.

## Verification

```bash
cd apps/backend && go test ./internal/agentctl/server/lsp ./internal/agentctl/server/api ./internal/agentctl/server/instance ./internal/agentctl/server/process
cd apps/backend && go test -race ./internal/agentctl/server/lsp ./internal/agentctl/server/api ./internal/agentctl/server/instance
cd apps/backend && go test ./internal/agentctl/server/lsp -run 'Test(Manager|Runtime)' -count=20
```

Record child PID/process-group cleanup and `goleak` results, not only Go test exit codes.

## Files likely touched

- `apps/backend/internal/agentctl/server/lsp/manager.go`
- `apps/backend/internal/agentctl/server/lsp/runtime.go`
- `apps/backend/internal/agentctl/server/lsp/peer.go`
- `apps/backend/internal/agentctl/server/lsp/progress.go`
- `apps/backend/internal/agentctl/server/lsp/*_test.go`
- `apps/backend/internal/agentctl/server/api/lsp.go`
- `apps/backend/internal/agentctl/server/api/server.go`
- `apps/backend/internal/agentctl/server/api/lsp_test.go`
- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/instance/instance.go`
- `apps/backend/internal/agentctl/server/instance/idle_reaper_test.go`
- `apps/backend/internal/lsp/protocol/message.go`

## Dependencies

Task 02 supplies shared phases, generation, progress, and snapshot contracts.

## Parallelism

Sequential. Task 04 extends the same manager and protocol peer.

## Inputs

- Spec: lifecycle phases, progress semantics, failure modes, persistence guarantees.
- Current `api/lsp.go`, process manager piped-process lifecycle, installer registry, and idle reaper.
- Existing fake stdio LSP and `goleak` TestMain patterns.

## Output contract

Report manager/control contracts, RED/GREEN evidence, race/leak counts, process cleanup evidence,
and remaining attachment work. Update task/plan status and actual file inventory.

## Results

Pending.
