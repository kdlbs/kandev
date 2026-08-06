---
id: "03-task-host-supervisor"
title: "Task-host runtime supervisor"
status: done
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

Implemented one agentctl-owned manager per instance with serialized language slots, monotonic and
idempotent generations, replacement-after-reap ordering, task-host-owned JSON-RPC initialization,
server configuration replies, progress/capability/diagnostic snapshots, graceful shutdown with
forced cleanup, crash evidence, and joined manager teardown. Added strict authenticated
snapshot/Start/Stop/Restart/watch adapters; request bodies cannot inject language, task, session,
generation ownership, or runtime identifiers. Watch disconnect only unsubscribes.

Added process-manager background-work reference counting and idle-reaper integration. A live LSP
keeps its task host alive without relying on an HTTP/WebSocket lease. The manager lifetime is an
owned process-manager operation, so agentctl teardown cancels install/initialize work, closes every
peer, reaps every process tree, and releases admission.

TDD evidence:

- RED: all synchronized manager cases failed with `task-host LSP lifecycle is not implemented`;
  background-work tests failed on missing methods/idle handling; control/watch routes returned 404.
- GREEN: duplicate and concurrent Start/Stop/Restart, early initialize progress, capability and
  configuration capture, graceful/forced stop, crash-without-respawn, watcher disconnect, manager
  teardown, background-work reference counting, and idle behavior pass.
- `go test ./internal/agentctl/server/lsp ./internal/agentctl/server/api
  ./internal/agentctl/server/instance ./internal/agentctl/server/process` — pass.
- `go test -race ./internal/agentctl/server/lsp ./internal/agentctl/server/api
  ./internal/agentctl/server/instance` — pass.
- `go test ./internal/agentctl/server/lsp -run 'Test(Manager|Runtime)' -count=20` — pass in 7.533s.
- `go.uber.org/goleak` runs from the new package `TestMain`; all package and repeated tests pass
  without reported goroutines.
- `TestManagerStopReapsLanguageServerProcessGroup` launches a real shell parent plus `sleep` child;
  forced Stop confirms both PIDs reach `ESRCH` after process-group teardown.

Actual files added: `server/lsp/{manager,runtime,peer,progress,types}.go`, synchronized manager tests,
goleak `TestMain`, and the Unix process-tree test. Actual files updated: agentctl LSP/API routes and
tests, process-manager background-work accounting/tests, instance idle logic/tests, this task, and
the parent plan. Attachment forwarding remains intentionally scoped to Task 04.

PR remediation on 2026-08-06 made failed replacement cleanup truthful and retryable: the old live
generation remains in `error/replacement_cleanup_failed`, the requested generation is not accepted
until the old process tree is proven gone, and retrying that same generation performs cleanup again
without overlap. Task-host mutation clients now preserve the generation-scoped failure snapshot.
The focused manager/client tests and the task-host race suite pass.

A follow-up Codex review on 2026-08-06 found that task-host `initialize` still carried a two-minute
hard deadline despite the accepted spec. Initialization now uses the owned runtime lifetime context
without an automatic deadline and follows only explicit runtime/task teardown cancellation. Stop
during an unanswered initialize skips the protocol shutdown request that the server cannot yet
service, closes the streams, and reaps the process. Focused no-deadline and stop-during-initialize
regressions failed before their changes and pass afterward; 20 repetitions, the full task-host race
suite, and backend lint pass.

The next review found that any JSON-RPC write could still block forever when a server stopped
reading stdin. Peer writes now serialize a complete frame behind the existing write lock, use an
OS pipe deadline when supported, and otherwise close the owned stdin after the same bounded wait.
Both the real process-pipe and generic blocked-writer regressions failed before the change and pass
20 repetitions under `-race`; `go test -race ./internal/agentctl/server/lsp -count=1 -timeout=90s`
and backend lint pass.
