---
id: "07-lifecycle-reconciliation"
title: "Lifecycle reconciliation"
status: completed
wave: 3
depends_on: ["06-task-controller"]
plan: "plan.md"
spec: "../../specs/lsp-file-intelligence/spec.md"
---

# Task 07: Lifecycle Reconciliation

## Acceptance

- Backend startup adopts any live task-host generation before considering a new launch, restores
  capacity/watch state, stops orphan/disabled runtimes, and starts at most one missing desired
  generation.
- Task stop, archive, delete, task-environment teardown/replacement, and backend/agentctl failure
  deterministically cancel recovery, clear progress, and reap the full LSP process tree without
  making session/browser/editor lifecycle an owner.
- Crash recovery uses 1s/5s/30s bounded retries with a five-ready-minute reset; task resume,
  multi-session access, and workspace-source changes follow the spec and leak no workers/timers.

## TDD sequence

1. Add integration fakes for persisted task language rows plus live task-host snapshots. Write
   failing startup cases for adopt, missing desired start, stale transient state, disabled orphan
   stop, capacity reconstruction, unreachable host, and duplicate prevention.
2. Add failing fake-clock recovery cases for the three backoffs, retry exhaustion, explicit Stop
   cancellation, five-minute reset, long-running initialize exclusion, and controller shutdown.
3. Add task service/lifecycle tests for stop, archive, delete, environment replacement, resume,
   cleanup failure fallback, policy retention, row cascade, and two sessions sharing one runtime.
4. Implement owned watch/reconcile workers, semantic event publication, task cleanup hooks, and
   source-root dynamic-update/restart-required handling. Join all goroutines and timers on Close.
5. Run race/goleak repetitions and real child-process cleanup tests before refactoring.

## Verification

```bash
cd apps/backend && go test ./internal/lsp/... ./internal/task/service ./internal/agent/runtime/lifecycle ./internal/backendapp -run 'Test(LSP|Lsp|TaskLSP|TaskLsp)'
cd apps/backend && go test -race ./internal/lsp/... ./internal/task/service -run 'Test(LSP|Lsp|Reconcile|Recovery|Cleanup)'
cd apps/backend && go test ./internal/lsp/... -run 'Test(Reconcile|Recovery|Cleanup|Watch)' -count=20
```

## Files likely touched

- `apps/backend/internal/lsp/reconcile.go`
- `apps/backend/internal/lsp/reconcile_test.go`
- `apps/backend/internal/lsp/watch.go`
- `apps/backend/internal/lsp/watch_test.go`
- `apps/backend/internal/lsp/lifecycle.go`
- `apps/backend/internal/lsp/lifecycle_test.go`
- `apps/backend/internal/task/service/service.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/service/service_resources.go`
- `apps/backend/internal/task/service/resource_cleanup_jobs.go`
- `apps/backend/internal/task/service/service_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_execution.go`
- `apps/backend/internal/agent/runtime/lifecycle/events.go`
- `apps/backend/internal/backendapp/gateway.go`
- `apps/backend/internal/backendapp/main.go`

## Dependencies

Task 06 supplies the authorized controller, capacity, task-host client, and event snapshot contract.

## Parallelism

Sequential. It mutates controller state and task/runtime cleanup wiring used by every later surface.

## Inputs

- Spec: State machine; Failure modes; Persistence guarantees; recovery and task-cleanup scenarios.
- Existing durable task-resource cleanup inventory and `executors_running` recovery contract.
- Existing task archive/delete ordering and `GetOrEnsureExecutionForEnvironment` deduplication.

## Output contract

Report every startup/cleanup transition, retry timing evidence, adopted/new generation counts,
race/goleak results, child-process cleanup, and any recovery limitation. Update task/plan status.

## Results

Completed 2026-08-05.

- Added startup adoption before launch, capacity reconstruction, stale/disabled runtime cleanup,
  environment-ready and unarchive reconciliation, and bounded 1s/5s/30s recovery with a
  five-ready-minute reset. Reconciler shutdown cancels and joins watches/timers.
- Task archive/delete/environment reset now stop task-owned language servers before destructive
  mutation; cascade teardown retains full environment process-tree cleanup as the backstop. Policy
  and history survive temporary teardown; delete still cascades persistent rows.
- Published revisioned `task.lsp_state_changed` / task-subscriber `task.lsp.changed` snapshots.
  Added bounded no-file discovery persistence and task-environment-ready inherited auto-start.
- Multi-repository roots now initialize as ordered contained workspace folders. Live servers that
  advertise folder-change support receive `workspace/didChangeWorkspaceFolders`; other live
  generations remain running and publish `workspace_roots_changed` restart-required evidence.
- Restored shared-cache install mutation coordination in the task-host supervisor. Browser,
  watcher, and attachment disconnects remain non-owning.
- The first focused race run exposed a captured timer-pointer race in recovery scheduling. Replaced
  it with a locked epoch token; the exact race suite then passed.

Verification:

```text
go test ./internal/lsp/... ./internal/task/service ./internal/agent/runtime/lifecycle ./internal/backendapp -run 'Test(LSP|Lsp|TaskLSP|TaskLsp)'        PASS
go test -race ./internal/lsp/... ./internal/task/service -run 'Test(LSP|Lsp|Reconcile|Recovery|Cleanup)'                                         PASS
go test ./internal/lsp/... -run 'Test(Reconcile|Recovery|Cleanup|Watch)' -count=20                                                               PASS
go test -race ./internal/agentctl/server/lsp ./internal/lsp ./internal/gateway/websocket                                                         PASS
```

PR remediation on 2026-08-06 cancels task recovery timers, watches, and queued admissions before
an environment lookup can fail during teardown. Ready-reset callbacks retain the owned lifecycle
context instead of creating background work after controller close. Environment-ready LSP
reconciliation is scheduled only after the agent process start has been dispatched. Focused
cleanup/timer/launch-order regressions and the controller race suite pass.

A follow-up Codex review on 2026-08-06 found that a task-host `process_exited` snapshot scheduled
recovery without releasing its now-dead generation's capacity slot. Proven process-exit evidence
now releases the slot first, atomically promotes queued work, then schedules recovery. The queue
promotion regression, full controller package, race detector, and backend lint pass.

The next review found that the user-facing orchestrator `StopTask` path stopped the executor but
did not invoke task-scoped LSP cleanup, unlike archive/delete/environment teardown. Backend wiring
now registers the existing authorized task cleanup service as an orchestrator post-executor-stop
hook and runs it before the task enters review. The regression proves cleanup observes the stopped
executor and precedes the review transition; it passed 20 repetitions, the full orchestrator and
backend-app packages, and backend lint.

A later Codex review found cleanup released capacity and wrote `off` even when both per-language
Stop and task-host teardown failed, despite the process possibly remaining live. Cleanup now retains
that generation's slot and publishes `task_host_stop_failed` until process absence is proven. A
successful full task-host cleanup remains an authoritative fallback and then releases the slot and
clears the error. Both failure and fallback regressions passed 20 repetitions under `-race`, along
with the full controller race suite.

The final review audit found that the coordinator/MCP `StopTaskForCoordinator` path bypassed the
task-owned cleanup hook. After accepted session stops, the coordinator now verifies no working
session remains, invokes the same task cleanup with trusted coordinator origin before REVIEW, and
surfaces cleanup failure instead of completing the transition. Its ordering regression and the
idempotent/partial-stop cases passed 20 repetitions.

The next exact-head review found that task reconciliation could retain a stale keep-warm candidate
after the current effective policy changed to Disabled. `ActionReconcile` now validates the latest
persisted policy and current global default immediately before allocating a generation. A stale
candidate therefore returns the current off/disabled snapshot without ensuring a task host,
launching a process, or consuming capacity. The focused regression failed before the guard and then
passed 20 repetitions under `-race`; the full `internal/lsp` race suite and backend lint also pass.

The following review found cleanup could use generation N from its initial list while a serialized
Start/Restart allocated generation N+1, causing teardown to mark the successor off but retain its
capacity slot. Cleanup now queues an exclusive batch in every affected task/language command lane,
reloads both the current row and task host after prior commands finish, and holds the lanes through
the full task-host cleanup backstop. Finalization releases and clears the exact current generation
that teardown proved dead. The deterministic barrier regression failed before the fix, then passed
20 repetitions under `-race` together with the cleanup failure/fallback cases.
