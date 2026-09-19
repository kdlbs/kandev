---
id: "02-launcher-parent-watchdog"
title: "Launcher parent-death watchdog"
status: done
wave: 2
depends_on: ["01-proclive-shared-helper"]
plan: "plan.md"
spec: "../../specs/desktop/requirements/desktop-tauri-app.md"
parallelism: parallel-safe
---

# Task 02: Launcher parent-death watchdog

Bind the launcher's lifetime to the process that spawned it, so a shell that dies
without running its own shutdown path cannot leave the launcher and backend
holding the Kandev home lock forever.

Parallel-safe with task 04: this task touches only `internal/launcher`, task 04
touches only `internal/backendapp/ownershiplock`, and they share no schema,
migration, generated contract, lockfile, or package config. Both depend on task
01.

## Inputs

- Plan sections "`KANDEV_LAUNCHER_PARENT_PID`" and "Launcher parent watchdog".
- Spec: `docs/specs/desktop/requirements/desktop-tauri-app.md`, the abnormal-termination bullet
  under "Failure modes" and its matching scenario under "Scenarios".
- Pattern to mirror: `processSupervisor.attachSignals` in
  `apps/backend/internal/launcher/process.go:141`. The watchdog performs the same
  terminal sequence as a received signal, so reuse that shape rather than
  inventing a second way to stop the tree.
- Test seams already in the package: `launcherExit` (`process.go:27`) and
  `launcherStatusOutput` (`process.go:26`); the `attachSignalsFn` indirection in
  `start.go:18`.
- `apps/backend/AGENTS.md`, "Goroutine ownership and leak testing", for the
  required `start`/`stop` shape.

## Implementation

Add the env var name to `apps/backend/internal/launcher/constants.go`:

```go
// parentPIDEnv lets a spawning process bind this launcher's lifetime to its
// own. Desktop shells set it so an abnormal shell exit cannot strand a backend
// holding the Kandev home's runtime-state lock.
parentPIDEnv = "KANDEV_LAUNCHER_PARENT_PID"
```

Add `parentWatchPollInterval = 2 * time.Second` next to the existing shutdown
timing constants in `process.go`, or in the new file; keep it a package constant
either way.

Create `apps/backend/internal/launcher/parentwatch.go` with a `parentWatchdog`
type owning exactly one goroutine:

- Construct from the env var. A missing, empty, non-numeric, zero, or negative
  value yields a watchdog whose `start` and `stop` are no-ops, so every current
  invocation keeps today's behavior.
- `start()` launches the poll goroutine and registers it on a `sync.WaitGroup`.
- `stop()` closes a stop channel and waits for drain. Idempotent: safe to call
  twice.
- The loop uses `time.NewTicker(interval)` inside a `select` that also watches
  the stop channel. Do not use `time.Sleep`.
- On a tick, call the liveness probe. Act **only** on `(alive=false, known=true)`.
  `(false, false)` means the platform cannot tell and must be ignored, which is
  what makes this a no-op on Windows rather than a process killer.
- When the parent is positively gone, emit both an operator-visible
  `launcherInfof` line naming the watched PID and a `shutdownDebugf` line, then
  call `supervisor.shutdown(reason)` followed by `launcherExit(0)`.

Expose two injectable seams as struct fields or package `var`s so tests do not
depend on real time or real processes: the liveness probe (defaulting to
`proclive.Alive`) and the poll interval.

Wire it in `runManagedApp` (`apps/backend/internal/launcher/start.go:86`),
immediately after `attachSignalsFn(supervisor)`, through a new package-level
`var startParentWatchFn = ...` in the same style as the existing
`attachSignalsFn`, so tests can substitute it. Stop the watchdog before
`runManagedApp` returns.

Do **not** wire `dev.go`, and do **not** add `KANDEV_LAUNCHER_PARENT_PID` to
`allowedSupervisorEnv` in `supervisor.go`. That allowlist is replayed into a
restarted backend child; the watchdog belongs to the launcher process, which
outlives supervisor restarts. Adding it there would be a real bug.

## Acceptance

- With `KANDEV_LAUNCHER_PARENT_PID` set to a live PID, the launcher runs exactly
  as it does today; when that PID is positively gone, it runs the standard
  graceful shutdown and exits 0.
- With the variable unset, empty, `0`, negative, or unparseable, no goroutine
  starts and behavior is byte-for-byte unchanged.
- An unknown-liveness probe result never triggers shutdown.

## Verification

```bash
cd apps/backend && go test ./internal/launcher/...
```

```bash
cd apps/backend && gofmt -l internal/launcher && golangci-lint run ./internal/launcher/... --new-from-rev=origin/main --timeout=5m
```

## Files likely touched

- `apps/backend/internal/launcher/parentwatch.go` (new)
- `apps/backend/internal/launcher/parentwatch_test.go` (new)
- `apps/backend/internal/launcher/parentwatch_unix_test.go` (new)
- `apps/backend/internal/launcher/constants.go`
- `apps/backend/internal/launcher/start.go`

Note `apps/backend/AGENTS.md`'s 800-effective-line file limit, which applies to
test files: keep the new tests in the new files above rather than appending to
`process_test.go` or `start_test.go`.

## Dependencies

Task 01 (`proclive.Alive`).

## Tests to write

In `parentwatch_test.go`, using `testing/synctest` so ticker waits resolve
instantly, with an injected probe and the `launcherExit` seam captured:

- Parent dies: probe flips to `(false, true)`; assert the supervisor shut down
  and `launcherExit` was called with `0`.
- Parent lives: probe stays `(true, true)` across several ticks; assert no
  shutdown and no exit.
- Liveness unknown: probe returns `(false, false)` across several ticks; assert
  no shutdown and no exit. This is the Windows path and the most important
  negative case.
- Env parsing, table-driven over unset, `""`, `"0"`, `"-1"`, `"abc"`, and a
  valid PID; assert a watchdog is created only for the last, and that `start`
  and `stop` on the others are safe no-ops.
- `stop` is idempotent and drains: call it twice, assert no panic and that the
  goroutine returned.

In `parentwatch_unix_test.go` (`//go:build !windows`), one real-process test:
spawn a child, kill and `Wait` it so the PID is released, then assert the
watchdog's real (non-injected) probe path observes it gone. This is what proves
the mechanism against a real OS rather than a fake.

## Output contract

Report the new files, the wiring point in `start.go`, exact commands with
outcomes, and explicit confirmation that `allowedSupervisorEnv` and `dev.go` were
left untouched. Update this file's `status` and `## Results`, and the matching
checkbox in `plan.md`.

## Results

- Added `parentWatchdog` with explicit start/stop ownership, a ticker-driven
  liveness loop, positive-death-only shutdown, and injectable liveness/timing
  seams.
- Added `KANDEV_LAUNCHER_PARENT_PID` parsing and wired the watchdog into
  `runManagedApp`; `dev.go` and `allowedSupervisorEnv` remain untouched.
- Added fake-time tests for parent death, live parents, unknown liveness, invalid
  environment values, idempotent stop, and a real Unix subprocess test that
  kills and reaps the watched process.
- `cd apps/backend && go test -run 'TestParentWatchdog|TestParentPIDFromEnv|TestRunManagedAppAttachesSignalsBeforeBackendLaunch' ./internal/launcher/...`
  passed.
- `cd apps/backend && go test ./internal/launcher/...` passed for both launcher
  packages.
- `gofmt -w` completed for all changed launcher Go files.
- Cleanup: the real subprocess test kills and reaps its child, and every
  watchdog registers a cleanup stop; no background watchdog remains.
- Security/trust and external side-effect boundaries: the PID is an explicit
  lifetime declaration from the desktop spawner; unknown liveness never causes
  shutdown.
