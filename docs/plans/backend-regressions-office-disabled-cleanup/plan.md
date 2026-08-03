---
spec: docs/specs/platform/cron-office-disabled-safety.md
created: 2026-08-03
status: complete
---

# Implementation Plan: Three Backend Regressions

Three independently reviewable backend regressions observed in
`~/.kandev/logs/backend-logs.log`, repaired with TDD. Each task owns a disjoint
package and is separately reviewable.

## Related Specs

- `docs/specs/platform/cron-office-disabled-safety.md` (Task 01, new repair spec)
- `docs/specs/tasks/title-length-limit.md` (Task 02, amended)
- `docs/specs/tasks/runtime-cleanup.md` (Task 03, amended)

## Root Causes

### 1. Office-disabled routines cron panic

When Office is disabled, `services.OfficeSvcs` is nil so `officeRoutines` stays a
nil `*office/routines.RoutineService` pointer
(`backendapp/main.go:1280-1283`). `startCronScheduler`
(`backendapp/cron.go:39`) passes that nil pointer straight into
`schedulercron.NewRoutinesHandler(routineSvc, nil, log)`, which stores it in the
`RoutineTicker` interface field. A typed-nil pointer inside an interface is a
non-nil interface value, so `RoutinesHandler.Tick`'s `h.ticker == nil` guard
(`scheduler/cron/routines.go:57-61`) is bypassed. `TickScheduledTriggers` then
runs on a nil receiver and dereferences the service's repository, panicking every
30 seconds. The loop's per-handler panic recovery re-arms each tick.

### 2. GitHub review/issue titles exceed the 60-rune limit

`buildReviewTaskRequest` (`orchestrator/event_handlers_github.go:451`) builds
`fmt.Sprintf("PR #%d: %s", pr.Number, pr.Title)` with no length cap, and the
issue watcher (`event_handlers_github.go:1368`) builds
`fmt.Sprintf("Issue #%d: %s", ...)` the same way. Both feed `CreateReviewTask` /
`CreateIssueTask` → `Service.CreateTask`, whose `validateTaskTitle(req.Title)`
(`task/service/service_tasks.go:341`) rejects titles over 60 runes.
`TruncateTaskTitle` is only applied to the auto-title derivation path
(`service_tasks.go:212`), never to caller-supplied titles, so long PR/issue
titles make the whole review/issue task fail to create.

### 3. Missing-session runtime cleanup preserves stale rows

`Executor.Stop` (`orchestrator/executor/executor_interaction.go:21-27`) maps
every session-lookup error to `ErrExecutionNotFound`, and `StopExecution`
(`:173-192`) wraps every stop error — including `lifecycle.ErrExecutionNotFound`
— as `ErrExecutionNotFound` without letting callers distinguish "already gone"
from "real failure". `handleMissingSessionOnStartup`
(`orchestrator/service.go:1725-1748`) preserves the row whenever
`StopAgentWithReason` returns any error, including a legitimate not-found for a
dead process. In the durable worker, `executeTaskResourceCleanupJob`
(`task/service/resource_cleanup_jobs.go:396,413-419`) counts every failed stop —
not-found included — as a `failedStops` entry and returns an error, so the job
re-enters `retry_wait` and exhausts its full retry budget (the documented
eight-claim bound, whose eighth failed attempt becomes terminal `failed`) even
though the runtime is already gone. Confirmed-dead local runtimes are never
pruned and every cleanup for them burns the whole retry budget before failing.

## Backend Changes

- **Task 01 (cron):** In `backendapp/cron.go`, only assign the routines service
  into a `schedulercron.RoutineTicker` variable when non-nil, so a genuinely nil
  interface reaches `NewRoutinesHandler`. Optionally harden
  `NewRoutinesHandler`/`Tick` to defend against a typed-nil concrete value.
  Preserve normal forwarding when Office is enabled.
- **Task 02 (titles):** Route the complete generated title through
  `taskservice.TruncateTaskTitle` in `buildReviewTaskRequest` and the GitHub
  issue task builder, before the request leaves the orchestrator.
- **Task 03 (cleanup):** Add a runtime-aware "already stopped" classification so
  a not-found stop for a confirmed-dead local row is a success, then thread it
  through startup missing-session handling and the durable cleanup worker so such
  rows are pruned/repaired under `RowMustBePreserved` and no longer counted as
  failed stops. Preserve alive/unknown/remote rows and non-not-found errors.

## Tests (RED before GREEN)

- **Task 01:** Two complementary layers. Behavioral: `scheduler/cron/routines_test.go`
  proves a handler built from a typed-nil `RoutineTicker` (the exact production
  panic shape) takes the no-op branch on `Tick` without panicking. Static guard:
  `backendapp/scheduler_wiring_test.go` asserts (via AST) that `startCronScheduler`
  never passes the raw `routineSvc` pointer straight into `NewRoutinesHandler`,
  so the nil-gating (`RoutineTicker` interface variable) cannot be dropped and
  reintroduce the typed-nil interface. The backendapp check is intentionally
  static — a runtime `startCronScheduler` test would require standing up the full
  `*Repositories`/dispatcher graph for no additional behavioral coverage beyond
  the handler test.
- **Task 02:** `orchestrator/event_handlers_github_review_test.go` (and the issue
  builder test) — ASCII and multibyte over-limit titles keep the `PR #`/`Issue #`
  prefix, stay ≤60 runes, and end with `…`.
- **Task 03:** `orchestrator/reconcile_restart_test.go` and
  `task/service/service_tasks_stop_test.go` /
  `task/service/resource_cleanup_jobs_test.go` — dead/alive/unknown-remote/
  missing-session/terminal-session cases; confirmed-dead not-found prunes/repairs
  and the cleanup job does not retry; alive/unknown preserved; non-not-found stays
  retryable.

## Implementation Waves And Parallel Candidates

Task 02 and Task 03 both modify `internal/orchestrator` (Task 02 the GitHub
title builders, Task 03 the stop-error classification), so they are not
package-disjoint — but their files and symbols are disjoint, so they do not
collide. Task 01 is fully isolated in `backendapp`+`scheduler/cron`. All three
share no schema, migration, generated contract, lockfile, or package config.

- Wave 1 (parallel-safe): Task 01, Task 02.
- Wave 1 (sequential within itself): Task 03.

`parallelism: sequential` remains the default per task; waves are a human aid,
not authorization to delegate.

- [x] [Task 01: Office-disabled routines cron no-op](task-01-office-disabled-routines-noop.md)
- [x] [Task 02: Truncate generated review/issue task titles](task-02-truncate-generated-titles.md)
- [x] [Task 03: Prune confirmed-dead missing-session runtime rows](task-03-confirmed-dead-runtime-cleanup.md)

## Documentation Impact

Durable specs are amended/added in this design package. No public CLI, config,
API, or UI documentation changes are required.

## Risks

- Task 03 must classify only typed not-found sentinels for confirmed-dead local
  rows; matching error strings or ignoring all not-found could hide real stop
  failures or prune alive/remote rows. Judge "dead" with the existing
  runtime-aware liveness probe, and gate deletion on `RowMustBePreserved`.
- Task 01's typed-nil fix is small but easy to reintroduce; the regression test
  must construct the typed-nil interface explicitly, not a plain `nil`.
