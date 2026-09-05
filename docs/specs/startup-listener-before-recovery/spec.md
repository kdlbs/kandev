---
status: draft
created: 2026-08-22
revised: 2026-09-05
type: repair
---

# The Backend Must Bind and Answer Liveness Before Startup Recovery Runs

## Problem

The backend does not bind its HTTP socket until orchestrator startup
reconciliation has finished, and that reconciliation can wait on agent
processes for hours. Under the launcher's health window the process is killed
before it ever listens, restarted, and the cycle repeats without converging.

`backendapp.startGatewayAndServe` calls `orchestrator.Service.Start` at
`main.go:877` and reaches the first `net.Listen` only via `main.go:1064` ->
`httpserver.go:76`. `/health` is a route on the server built after that point
(`helpers.go:716`); no earlier path serves it. So while `Start` runs there is
no listener at all, which is exactly what `lsof` showed.

`Start` runs `reconcileTaskLifecycleTokens` synchronously
(`service.go:2255`), fanning four workers over every task with a pending
lifecycle token and waiting on a `sync.WaitGroup`. The per-task work reaches
`autoStartStepPrompt` -> `PromptTask` -> `ensureSessionRunning` ->
`attemptColdResume`.

**The per-task cost is far larger than the prompt-ready timeout suggests.**
`attemptColdResume` first waits for session readiness via `waitForSessionReady`
(`task_operations.go:2498`), whose budget is `constants.AgentLaunchTimeout` =
**15 minutes** (`timeouts.go:44`, 10m setup script + 5m allowance). Only after
that does the 30s `waitForAgentPromptReady` apply, and the attempt is retried
once. Worst case is roughly 31 minutes per task, four at a time.

Measured on the affected instance: 424 tasks, 32 carrying
`manual_move_lifecycle` metadata, on a 1.0 GB database. A boot given a full
10-minute health window still never bound, and produced only 2 successful ACP
session creations in those 10 minutes. That is the signature of workers parked
in the 15-minute session-ready wait, not of workers cycling through tasks.

Three consequences, in severity order:

1. **Socket liveness is coupled to agent readiness.** The process cannot answer
   a health probe, serve the UI, or expose the API until agents that may never
   become ready are waited out.
2. **The failure is self-sustaining.** `healthTimeoutReleaseMS` is 45s
   (`internal/launcher/constants.go`). The launcher declares a backend health
   failure and stops the process; a supervisor restarts it; the same tokens are
   still pending because none were cleared, and the runtimes orphaned by the
   kill are added to the next boot's work. Ten consecutive boots were observed,
   none of which bound a socket.
3. **The wait cannot be bounded from outside.** `attemptColdResume` and
   `reapAndReloadSession` deliberately use `context.WithoutCancel`
   (`task_operations.go:2308`, `task_operations.go:2255`) so a resume survives
   its caller's cancellation. A deadline handed to the sweep therefore does not
   stop an in-flight resume. Any fix that relies on cancelling the sweep to
   protect startup is ineffective.

This is startup ordering, not a defect in the recovery logic. The same frames
are reached whenever a lifecycle token is pending.

## Correction (2026-09-05)

This spec's original Desired-behaviour and Out-of-scope sections asserted
that the sweep's position *before* `watcher.Start` was load-bearing and that
moving it would introduce races with live traffic. Runtime evidence gathered
after this spec's socket-bind fix shipped disproves that: with the socket-bind
fix in place, boots with pending lifecycle tokens still never reached `API
configured` — the sweep itself was still holding the readiness path open
indefinitely, just behind the new bootstrap 503 handler instead of behind a
closed socket.

The reason is a **self-inflicted event blackout**, not merely a slow
individual resume. `Watcher.Start` is what registers
`{events.AgentBootReady, w.handlers.OnAgentBootReady}` (the *only* handler
that flips a launching session to `WAITING_FOR_INPUT` on the resume path).
The event bus used here has no replay, so any `agent.boot_ready` published
while the sweep runs *before* `watcher.Start` is dropped on the floor.
`waitForSessionReady`'s poll can then never observe the state change it is
waiting for, and burns its full `constants.AgentLaunchTimeout` (15 minutes)
on every recovered task, regardless of whether the underlying agent process
is actually healthy. Four workers wide, retried up to three times, this is
indistinguishable from a hang and gates the entire router.

The fix inverts the ordering this spec forbade: the sweep now runs in a
background goroutine *after* `watcher.Start` and `scheduler.Start` succeed,
so the events its own resume path depends on are deliverable from the
moment recovery starts. Readiness is not gated on the sweep at all — the
socket-bind / bootstrap-503 behavior this spec introduced is unaffected and
still the mechanism that makes an in-progress sweep observable as "starting"
rather than a hang. The Desired-behaviour and Out-of-scope sections below
are amended accordingly; the regression scenarios still hold, since a bound
socket does not depend on this ordering.

## Broken behavior

- No socket is bound, and `/health` cannot be answered, until orchestrator
  startup reconciliation returns.
- A backend whose recovery exceeds the health window is killed and restarted
  with at least as much recovery work as before, indefinitely.

## Desired behavior

- The backend binds its socket and answers a liveness probe within the release
  health window, regardless of how much recovery work is pending.
- Startup recovery runs in the background *after* the watcher and scheduler
  start (amended 2026-09-05 — see Correction above): its own resume path
  depends on events only the watcher's subscriptions deliver, so running it
  earlier does not protect anything and instead guarantees every recovered
  task times out. Running concurrently with live watcher/scheduler traffic is
  safe because the sweep's operations are already built for concurrent
  delivery — per-task locks (`queuedMoveLifecycleLocks`) and one-shot tokens
  claimed by an atomic metadata-key removal (`claimTaskEventMetadata`) — and
  this must be confirmed by test, not by assertion.
- Requests that need a started application are answered with an explicit
  "starting" response rather than by a hung connection or a refused socket.
- Readiness is observable and distinct from liveness: an operator can tell
  "bound, still starting" from "fully started".

## Regression scenarios

- **GIVEN** a backend with pending lifecycle tokens whose agent sessions never
  reach ready, **WHEN** it starts, **THEN** the socket is bound and the
  liveness probe returns success well inside `healthTimeoutReleaseMS`, and the
  launcher does not shut the process down.
- **GIVEN** the same backend while startup reconciliation is still running,
  **WHEN** an API route is requested, **THEN** it returns an explicit starting
  status rather than hanging, and readiness reports not-ready while liveness
  reports healthy.
- **GIVEN** startup reconciliation that has completed, **WHEN** readiness is
  requested, **THEN** it reports ready and all routes serve normally.

## Out of scope

- Changing WIP promotion, feeder pull, or manual-move lifecycle semantics.
- Changing `AgentLaunchTimeout`, `agentPromptReadyTimeout`, or the cold-resume
  retry shape (`context.WithoutCancel`). See the follow-up note below — a
  caller-side deadline on the sweep bounds admission and reporting, not an
  in-flight resume, precisely because of this contract.
- Raising `healthTimeoutReleaseMS`, which hides the coupling instead of fixing
  it. Empirically a 10-minute window did not help.

## Known follow-up, addressed 2026-09-05

The note below originally described work deliberately deferred past this
spec's socket-bind fix. It has since been addressed in the same change that
corrected the ordering above (see Correction): the sweep now runs off the
readiness path entirely (background goroutine, not gating `API configured`),
carries its own bounded overall deadline separate from the interactive
`AgentLaunchTimeout`, and no longer accumulates already-completed
`manual_move_lifecycle_completed` tokens as permanent per-boot work. The
`context.WithoutCancel` resume contract itself is unchanged, so an
individual in-flight resume still cannot be aborted early — the sweep's
deadline bounds its own dispatch loop returning, not that resume.

> Fixing the ordering stops the crash loop but does not make a large board
> start quickly: a backend with many pending tokens still spends a long time
> in "starting" because bulk recovery inherits the interactive 15-minute
> `AgentLaunchTimeout` per task. A bulk startup sweep needs its own, much
> smaller per-task budget, and a resume path that can actually be abandoned.
