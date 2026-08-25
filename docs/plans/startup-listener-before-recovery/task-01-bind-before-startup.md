---
id: "01-bind-before-startup"
title: "Bind listeners and serve liveness before startup work"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/startup-listener-before-recovery/spec.md"
parallelism: sequential
---

# Task 01: Bind Listeners and Serve Liveness Before Startup Work

## Intent

Move socket binding ahead of orchestrator startup so the process can answer a
liveness probe while startup work is still running. `Service.Start` and the
startup sweeps are not touched.

## Acceptance

- Listeners are bound before `orchestratorSvc.Start(ctx)` is invoked
  (`main.go:877`), using the same host and port resolution as today
  (`serverListenAddr`, `startHTTPServers`).
- While startup work runs, the bound socket is served by a bootstrap handler
  that answers the launcher's liveness path with 200. The launcher probes
  `/health` (`internal/launcher/health.go`); confirm the exact path and method
  before choosing what the bootstrap handler answers.
- The bootstrap handler answers **every** path deterministically. No path may
  fall through to a nil handler or hang.
- Once `buildHTTPServer` returns, the real router replaces the bootstrap
  handler on the already-bound listeners, with no window in which the socket is
  closed and reopened and no second bind of the same port.
- The existing readiness flag (`ready`, consulted by the `/health` handler at
  `main.go:179`) keeps its current meaning for the real router. Task 02 owns
  making "starting" visible; this task must not make liveness depend on
  startup progress.
- Shutdown closes each listener exactly once. Follow the existing ownership in
  `awaitShutdown` (`helpers.go:1740`, `helpers.go:1755`) rather than adding a
  cleanup that races the orchestrator stop.
- Bind failure still aborts startup with the same behaviour and logging as
  today.

## Suggested mechanism

Serve an `http.Server` whose `Handler` is a small delegate holding an
`atomic.Pointer[http.Handler]`. It starts as the bootstrap handler and is
swapped to the Gin engine once built. This avoids closing and rebinding the
socket, and keeps a single shutdown owner.

## Regression test (write first, must fail)

`apps/backend/internal/backendapp/binds_before_startup_test.go`

- Drive startup with a substituted orchestrator start function that blocks on a
  channel the test controls, following the existing
  `startOrchestratorAndAutomationConsumers` seam (`main.go:877`) already used
  by `startup_order_test.go`.
- Assert a TCP connection to the configured port succeeds and the liveness path
  returns 200 while the start function is still blocked.
- Release the block and assert the real router then serves a normal API route
  on the same port.

Expected pre-fix failure: the connection is refused; nothing is bound.

## Files likely touched

- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/httpserver.go`
- `apps/backend/internal/backendapp/binds_before_startup_test.go` (new)

## Validation

```bash
cd apps/backend
go test ./internal/backendapp/... -run 'Bind|Start|Health|Ready' -count=1
make -C . lint
```

## Notes

Do not modify `apps/backend/internal/orchestrator/service.go` in this task. The
sweeps stay where they are; only the bind moves.
