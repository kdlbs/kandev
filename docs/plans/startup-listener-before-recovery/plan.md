---
spec: docs/specs/startup-listener-before-recovery/spec.md
created: 2026-08-22
revised: 2026-08-23
status: done
---

# Fix Plan: Bind and Answer Liveness Before Startup Recovery

## Confirmed root cause

`backendapp.startGatewayAndServe` starts the orchestrator at `main.go:877` and
reaches the first `net.Listen` only at `main.go:1064` -> `httpserver.go:76`.
`/health` is a route on the server built after that point (`helpers.go:716`),
so no socket exists while `Service.Start` runs.

`Start` runs `reconcileTaskLifecycleTokens` synchronously (`service.go:2255`)
with four workers (`event_handlers_workflow.go:843`) and waits on a
`sync.WaitGroup`. Each task can reach `attemptColdResume`, which waits
`constants.AgentLaunchTimeout` = **15 minutes** (`timeouts.go:44`) for session
readiness (`task_operations.go:2500`), then 30s for prompt readiness, and
retries once (`task_operations.go:2235`). Worst case is roughly 31 minutes per
task, four at a time, against a 45s health window
(`internal/launcher/constants.go`).

Corroborated empirically: a boot given a full 10-minute window still never
bound, producing only 2 ACP session creations in 10 minutes across 32 pending
tokens. Workers were parked in the session-ready wait, not cycling.

## Approach

**Bind the socket first; keep recovery exactly where it is.**

An earlier revision of this plan proposed extracting the sweeps and running
them after the listener, under a deadline. That approach is rejected:

- `attemptColdResume` and `reapAndReloadSession` use `context.WithoutCancel`
  (`task_operations.go:2308`, `task_operations.go:2255`) specifically so a
  resume survives caller cancellation. An external deadline would not stop an
  in-flight resume, so the promised bound was fictional.
- The sweeps currently run *before* `watcher.Start` and `scheduler.Start`
  (`service.go:2251` vs `service.go:2262`). Moving them after the listener
  would race live watcher events, scheduler activity, GitHub polling, and API
  mutations. The per-task mutex covers only the manual-move lifecycle body; it
  does not serialise queued WIP reconciliation or dependency launches.

So the fix changes **when the socket is bound**, not when recovery runs:

```
bind listeners early, serve a bootstrap handler
  -> liveness endpoint returns 200 from this moment
  -> every other route returns an explicit "starting" status
orchestrator.Start(ctx)            unchanged position, unchanged ordering
buildHTTPServer(...)               unchanged
swap the bootstrap handler for the real router
mark ready
```

## Correction to a claim in the earlier revision

`workflowStore.ReconcileQueuedTasks` was listed as a third synchronous
launching sweep. That overclaims: it promotes and publishes `task.moved`
(`workflow_store.go:472`, `workflow_store.go:650`), but the watcher that
handles that event does not start until later in `Start` (`service.go:2262`),
so it does not itself synchronously reach the auto-start path. Its promotion is
instead picked up by the following lifecycle-token sweep through the durable
promotion marker. Two sweeps launch synchronously:
`reconcileTaskLifecycleTokens` and `reconcileDependencyLaunchesOnStartup`
(`event_handlers_dependencies.go:172`). This does not change the fix, which no
longer depends on classifying the sweeps.

## Implementation waves and parallel candidates

- [x] [Task 01: Bind listeners and serve liveness before startup work](task-01-bind-before-startup.md) (`done`)
- [x] [Task 02: Gate application routes behind an explicit starting state](task-02-gate-routes-while-starting.md) (`done`)
- [x] [Task 03: Guard the ordering against regression](task-03-ordering-guard.md) (`done`)

No task is `parallel-safe`; all three touch the same startup sequence.

## Validation results

Implemented and validated across 5 Build/Verify/Review rounds; landed in PR
[#2944](https://github.com/kdlbs/kandev/pull/2944)
(`feature/backend-never-binds-k15` -> `main`). `/health` is now bound and
answering liveness immediately at listen time; `/ready` gates readiness
separately per `docs/specs/health-endpoint-version/spec.md`. Full command
receipts and round-by-round findings are recorded in the task's Kandev plan.

## Regression test that must fail first

`apps/backend/internal/backendapp/binds_before_startup_test.go` (new): run
startup with a collaborator that blocks inside `orchestrator.Start`, and assert
that the configured port accepts a TCP connection and answers the liveness
probe with 200 while `Start` is still blocked.

Before Task 01 this fails: no socket exists. After Task 01 it passes.

## Validation commands

```bash
make -C apps/backend test
make -C apps/backend lint
cd apps/backend && go test ./internal/backendapp/... -run 'Bind|Ready|Start|Health' -count=1
```

## Risks and guardrails

- **Do not move, reorder, or parallelise the startup sweeps.** Their position
  before `watcher.Start` is load-bearing. This plan deliberately leaves
  `Service.Start` untouched.
- **Do not rely on cancelling recovery.** `context.WithoutCancel` makes that
  ineffective; anything that claims to bound it must change the resume contract
  first, which is out of scope.
- **Shutdown ordering is not what it looks like.** `awaitShutdown` stops the
  orchestrator and lifecycle manager first (`helpers.go:1740`), then runs
  cleanups (`helpers.go:1755`). Do not register the bootstrap listener's
  teardown as a cleanup that must run before those stops; hand the early
  listeners to the same shutdown path that owns the real ones so a single
  socket is never closed twice or leaked.
- The liveness endpoint must not become a readiness endpoint. If it starts
  reporting startup progress in its status code, the launcher will kill the
  process again and this fix is undone.
- Binding before the router exists means an early request can arrive before any
  route is registered. The bootstrap handler must answer every path
  deterministically, never fall through to a nil handler.

## Out of scope

- The time a large board spends in "starting". Bulk recovery inheriting the
  interactive 15-minute `AgentLaunchTimeout` per task is a real defect, but it
  needs its own spec and a change to the `context.WithoutCancel` resume
  contract. Recorded in the spec's follow-up section.
- WIP promotion, feeder pull, and manual-move lifecycle semantics.
- Launcher health-window configuration.
