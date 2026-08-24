---
spec: docs/specs/desktop/requirements/desktop-tauri-app.md
created: 2026-08-19
status: completed
---

# Implementation Plan: Orphaned desktop backend holds the Kandev home lock

## Overview

Quitting the macOS desktop app can leave its launcher and backend running. The
orphaned backend keeps the `flock` on `~/.kandev/.kandev-backend.lock`, so every
later start, desktop or CLI, is rejected with a message that names only the home
path and gives the operator nothing to act on. This plan fixes both halves: the
launcher learns to exit when the shell that spawned it dies, and the ownership
lock learns to say who is holding it.

Order is bottom-up. A shared process-liveness helper lands first because both
halves need it, then the two independent behaviors, then the desktop change that
activates the watchdog. Two specs are amended by this repair:
[desktop-tauri-app](../../specs/desktop/requirements/desktop-tauri-app.md) owns the launcher
lifetime guarantee, and
[port-collision-safety](../../specs/executors/requirements/port-collision-safety.md) owns the
runtime-state ownership contract.

## Root cause

Confirmed on a live machine, not inferred.

The desktop shell owns backend shutdown through one path: `shutdown_and_exit` in
`apps/desktop/src-tauri/src/main.rs` (reached from `WindowEvent::CloseRequested`
and `MenuAction::Quit`) calls `BackendState::stop`, which sends `SIGTERM` to the
launcher and escalates to `SIGKILL` after five seconds. That path is correct and
is not what failed. The defect is that it is the *only* path: nothing binds the
launcher's lifetime to the shell, so when the Tauri process ends without running
it, the launcher survives.

Observed state after a quit: launcher PID 51228
(`Kandev.app/Contents/Resources/kandev/bin/kandev --headless --port 38430`) alive
with **PPID 1**, having been reparented to `launchd` when its Tauri parent died;
backend PID 51229 alive as its child; `lsof` showing 51229 holding
`/Users/cfl12/.kandev/.kandev-backend.lock`; and no `Kandev.app/Contents/MacOS`
process at all. `SIGTERM` to 51228 shut the tree down cleanly and freed the lock,
confirming the launcher's own signal handling was never the problem.

Two consequences compound into the reported dead end:

1. `apps/backend/internal/launcher/` has no parent-death detection. Neither
   `runManagedApp` (`start.go`) nor `processSupervisor` (`process.go`) observes
   the parent, so an orphaned launcher runs until it is killed by hand.
2. `ownershiplock` never writes to the lock file it holds, so the conflict
   message in `apps/backend/internal/backendapp/main.go` can only name the path.
   The file is zero bytes, which also makes it look deletable; deleting it does
   nothing, because the `flock` is the ownership, not the file.

Smallest reliable reproduction: start the desktop app, `kill -9` the Tauri shell
process (simulating a crash or force quit), then run `make start-debug`. It fails
with `home target "..." is already owned by another backend`.

Regression-test level: Go unit and real-subprocess tests in
`internal/launcher` and `internal/backendapp/ownershiplock`, plus a Rust unit
test in `apps/desktop/src-tauri`. No E2E; see Tests.

---

## Contracts introduced

Both halves are new cross-component contracts. Naming them here so the task files
do not each re-invent them.

### `KANDEV_LAUNCHER_PARENT_PID`

The spawning process writes its own PID into this environment variable. A
launcher that sees a valid positive PID binds its lifetime to that process and
shuts its supervised tree down once the process is gone. Unset, empty, `0`, or
unparseable means no watchdog, which keeps every current invocation, including
`make dev`, plain `kandev start` in a terminal, `nohup`, and service-manager
installs, behaving exactly as it does today.

Opt-in is deliberate. Deriving the parent from `os.Getppid()` drift would also
kill deliberately detached CLI launches (`nohup kandev start &`), and under
`launchd`/`systemd` the initial parent is already PID 1, so there is no stable
signal to compare against. The spawner is the only party that knows it owns the
launcher's lifetime, so the spawner declares it.

This variable is launcher-scoped and must **not** be added to the
`allowedSupervisorEnv` allowlist in `apps/backend/internal/launcher/supervisor.go`.
That allowlist is the environment replayed into a restarted *backend* child; the
watchdog belongs to the launcher process, which survives supervisor restarts.

### Lock owner metadata

After acquiring a lock, the owner writes one line of JSON into the lock file it
already holds:

```json
{"pid":51229,"executable":"/Applications/Kandev.app/.../bin/kandev","started_at":"2026-08-19T09:14:35.123456789Z"}
```

Written with `Truncate(0)` then `WriteAt(1)`, leaving the first byte reserved for
the Windows lock range so a conflicting process can read the metadata through a
separate handle while the lock is held. A concurrent reader sees either empty
or a valid prefix, never a stale tail from a longer previous record. It is
advisory only: never read to decide ownership, staleness, or takeover, and a
write failure never fails acquisition. The lock sidecar is opened with
platform no-follow semantics and must be a regular file; symlink or reparse
point paths are rejected before any write.

---

## Backend

### Shared process liveness (`internal/common/proclive`, new)

`apps/backend/internal/system/storage/tempartifacts/process_alive_unix.go` and
`process_alive_windows.go` already implement exactly the probe both halves of
this fix need, including the `(alive, known bool)` shape that distinguishes "the
process is gone" from "this platform cannot tell". Two more copies would violate
the repo's rule against duplicating cross-package helpers, so promote it to
`internal/common/proclive` and migrate `tempartifacts` onto it.

The signature stays `Alive(pid int64) (alive, known bool)`. Keeping `int64`
preserves `tempartifacts`' existing `artifact.OwnerPID` call site byte for byte
and avoids a 32-bit truncation question at the boundary; the two new callers pass
`int64(pid)`.

Behavior is unchanged from the current implementation: Unix uses signal 0,
treating `EPERM` as alive and `ESRCH` as gone; Windows returns `(false, false)`,
meaning unknown. Both new callers must therefore be correct when liveness is
unknown, and both are: the watchdog only acts on a positive `(false, true)`, and
the lock reader only suppresses owner detail on a positive `(false, true)`.

### Launcher parent watchdog (`internal/launcher/parentwatch.go`, new)

A `parentWatchdog` owning one goroutine, following the package's existing
lifecycle conventions: `start` registers on a `sync.WaitGroup`, `stop` closes a
channel and waits for drain, and the poll loop uses `time.NewTicker` inside a
`select` that also watches the stop channel, never `time.Sleep`.

On a poll where the watched PID is positively gone, it mirrors what
`attachSignals` already does for a signal: emit an operator-visible
`launcherInfof` line and a `shutdownDebugf` line, call
`supervisor.shutdown(reason)`, then `launcherExit(0)`. Reusing that exact
sequence means the watchdog inherits the tested graceful-shutdown path
(`managedProcessShutdownGrace`, process-group cleanup) rather than introducing a
second way to stop the tree.

Poll interval 2 seconds, as a package constant next to the existing shutdown
timing constants. Injectable seams for tests: the liveness probe function and the
ticker interval.

Wiring: `runManagedApp` in `apps/backend/internal/launcher/start.go`, immediately
after `attachSignalsFn(supervisor)`, through a package-level `var` in the same
style as the existing `attachSignalsFn` indirection so tests can substitute it.
`runManagedApp` covers `start`, `run`, and `service`; the desktop spawns
`kandev --headless --port <n>`, which is `start`. `dev.go` is intentionally not
wired: `make dev` is a foreground developer command whose parent is `make`, and
no spawner sets the variable for it.

The env var name belongs in `apps/backend/internal/launcher/constants.go`
alongside the other launcher constants.

### Ownership-lock owner diagnostics (`internal/backendapp/ownershiplock/`)

`owner.go` (new) holds the record struct plus `writeOwner(*os.File)` and
`readOwner(path string)`. `lock.go` gains a `writeOwner` call after `lockFile`
succeeds, ignoring its error, and a `readOwner` call on the `ErrConflict` branch
to populate a new `Owner *OwnerRecord` field on `ConflictError`.

`ConflictError.Error()` extends its existing sentence rather than replacing it,
so the message keeps naming the path and the existing
`use a separate KANDEV_HOME_DIR` guidance appended in `main.go` still reads
correctly:

```text
home target "/Users/cfl12/.kandev" is already owned by another backend (pid 51229, /Applications/Kandev.app/.../bin/kandev, started 2026-08-19T09:14:35Z)
```

Owner detail is omitted, falling back to today's exact wording, when the file is
empty or unparseable, or when `proclive.Alive` positively reports the recorded
PID as gone. It is included when liveness is unknown, which is what makes the
detail useful on Windows.

No change is needed in `apps/backend/internal/backendapp/main.go`: it already
prints `%v` of the error, so the enriched text reaches stderr, and from there
reaches the desktop startup-error dialog through the launcher output capture in
`apps/desktop/src-tauri/src/backend.rs`.

---

## Desktop

### `apps/desktop/src-tauri/src/backend.rs`

`launch_and_wait` already inserts `KANDEV_DESKTOP_HEALTH_TOKEN` into
`inherited_env` before building the spec. Insert `KANDEV_LAUNCHER_PARENT_PID`
set to `std::process::id()` the same way, one line, next to the existing token
insert. `desktop_environment` passes unrecognized inherited variables through
untouched, which the existing `CUSTOM_ENV` assertion in
`command_spec_uses_headless_launcher_and_loopback_env` already pins.

No Rust logic changes anywhere else. `BackendState::stop` remains the primary
shutdown path for a normal quit; the watchdog is the backstop for the abnormal
exits `stop` cannot cover.

---

## Frontend

None. This repair has no SPA surface: no component, store slice, API client, or
user-facing copy changes, so no i18n work either. The one operator-visible string
is backend stderr, which stays English per the backend i18n scope in
`apps/backend/AGENTS.md`.

---

## Tests

| What | File | How |
|---|---|---|
| `Alive` reports self alive, reports a reaped PID gone, treats non-positive PIDs as gone-and-known | `internal/common/proclive/proclive_unix_test.go` | Separate Unix tests; reap a real short-lived `os/exec` child for the dead case |
| `Alive` reports unknown on Windows | `internal/common/proclive/proclive_windows_test.go` | Windows-only test asserts `(false, false)` for the current process |
| `tempartifacts` keeps its current reconciliation behavior after migrating | `internal/system/storage/tempartifacts/registry_test.go` | Existing tests must pass unchanged |
| Watchdog shuts the tree down when the watched PID dies | `internal/launcher/parentwatch_test.go` | `testing/synctest` with an injected liveness probe flipping to `(false, true)`; assert `supervisor.shutdown` and `launcherExit(0)` via the package's existing `launcherExit` seam |
| Watchdog stays quiet while the parent lives, and while liveness is unknown | `internal/launcher/parentwatch_test.go` | Same harness with probes returning `(true, true)` and `(false, false)`; assert no shutdown after several ticks |
| Unset/empty/`0`/unparseable env means no watchdog goroutine | `internal/launcher/parentwatch_test.go` | Table-driven over env values; assert `start` returns a no-op and `stop` is safe |
| `stop` drains the goroutine and is idempotent | `internal/launcher/parentwatch_test.go` | Call `stop` twice; assert no panic and no goroutine left |
| Watchdog fires against a real killed process | `internal/launcher/parentwatch_unix_test.go` | Spawn a real child, kill and reap it, assert the probe path observes it gone |
| Conflict names the owning PID and executable | `internal/backendapp/ownershiplock/lock_test.go` | Acquire in-process, attempt a second acquire, assert `ConflictError.Owner` and the rendered message |
| Empty, unparseable, and known-dead metadata fall back to the path-only message | `internal/backendapp/ownershiplock/lock_test.go` | Table-driven over lock-file contents with an injected liveness probe |
| Owner-metadata write failure does not fail acquisition | `internal/backendapp/ownershiplock/lock_test.go` | Force the write to fail; assert `Acquire` still returns an owner |
| Metadata write truncates a longer previous record | `internal/backendapp/ownershiplock/owner_test.go` | Pre-fill the file with a longer record, re-write, assert no trailing bytes |
| Existing startup-conflict behavior is unchanged | `internal/backendapp/main_test.go` | Existing `TestBackendStartupConflictStopsBeforeSharedStateInitialization` must pass unchanged |
| Desktop spec carries the parent PID to the launcher | `apps/desktop/src-tauri/src/backend.rs` (`mod tests`) | Extend the existing command-spec test to assert the variable survives into `BackendCommandSpec.env` |

Commands, each runnable from the repo root:

```bash
cd apps/backend && go test ./internal/common/proclive/... ./internal/launcher/... ./internal/backendapp/... ./internal/system/storage/tempartifacts/...
cd apps/desktop/src-tauri && cargo test --features desktop-runtime
```

Lint gate for the changed Go packages:

```bash
cd apps/backend && golangci-lint run ./internal/common/proclive/... ./internal/launcher/... ./internal/backendapp/... --new-from-rev=origin/main --timeout=5m
```

---

## E2E Tests

None, deliberately. The spec changes have no SPA surface, so there is no
Playwright scenario to write. The desktop smoke harness
(`pnpm --filter @kandev/desktop e2e`) runs under Xvfb on Linux and would have to
abnormally kill the shell and then assert on host process state and a host lock
file, which is exactly the coupling that harness avoids. The real-subprocess Go
test in `internal/launcher` covers the mechanism, and the Rust unit test covers
the wiring that activates it.

---

## Verification Results

- `cd apps/backend && go test -race ./internal/common/proclive/... ./internal/launcher/... ./internal/backendapp/... ./internal/system/storage/tempartifacts/...`
  passed for all six affected packages.
- `cd apps/backend && gofmt -l internal/common/proclive internal/launcher internal/backendapp/ownershiplock internal/system/storage/tempartifacts`
  produced no output.
- `cd apps/backend && golangci-lint run ./internal/common/proclive/... ./internal/launcher/... ./internal/backendapp/... ./internal/system/storage/tempartifacts/... --new-from-rev=origin/main --timeout=5m`
  passed with 0 issues.
- `GOOS=windows GOARCH=amd64 go test -c` passed for `proclive`, `launcher`, and
  `backendapp/ownershiplock`; temporary Windows test binaries were removed.
- `cd apps/desktop/src-tauri && cargo fmt --check && cargo test --features desktop-runtime`
  passed: 57 tests, 0 failures.
- macOS parent-death process-boundary verification passed with a newly built
  launcher and the installed runtime bundle: after SIGKILLing and reaping the
  declared parent shell, the launcher logged `parent process exited`, completed
  graceful shutdown, exited, and the temporary runtime lock had no holder.
- `git diff --check` passed. Generated Tauri schema output from the test build
  was removed; no generated artifact remains in the diff.

---

## Implementation Waves And Parallel Candidates

```text
Wave 1:
- [x] [task-01-proclive-shared-helper](task-01-proclive-shared-helper.md)

Wave 2 (parallel candidates, user authorization required):
- [x] [task-02-launcher-parent-watchdog](task-02-launcher-parent-watchdog.md)
- [x] [task-04-lock-owner-diagnostics](task-04-lock-owner-diagnostics.md)

Wave 3:
- [x] [task-03-desktop-parent-pid](task-03-desktop-parent-pid.md)
```

Task 01 is a shared dependency of both wave-2 tasks and must land first. Tasks 02
and 04 touch disjoint packages (`internal/launcher` and
`internal/backendapp/ownershiplock`), share no schema, migration, generated
contract, lockfile, or package config, and are genuinely parallel-safe. Task 03
is last because it activates the watchdog task 02 introduces, and the product
should not advertise the binding before the launcher honors it.

Waves are a review aid. The default remains sequential execution in the primary
conversation; they do not authorize subagents.

## Risks

- **Contradicts a rejected ADR alternative.** ADR
  [2026-08-09-exclusive-runtime-state-ownership](../../decisions/2026-08-09-exclusive-runtime-state-ownership.md)
  rejected "Use a PID file" because stale PIDs and PID reuse do not provide
  atomic ownership, and `port-collision-safety` listed PID identity matching as
  out of scope. Both objections are about using a PID to *decide* ownership. This
  plan keeps `flock` as the sole ownership proof and uses the PID only as text in
  an error message, and the spec amendment narrows the out-of-scope line to say
  so explicitly. Reviewers should confirm that framing is acceptable before the
  code lands; if it is not, task 04 drops and task 02 still fixes the reported
  dead end.
- **PID reuse weakens the watchdog silently.** If the shell dies and its PID is
  reused before the next poll, the watchdog sees a live PID and never fires. The
  failure mode is exactly today's behavior, so it degrades rather than breaks. A
  handle-based or pipe-EOF mechanism would close it and is the natural follow-up
  if this proves insufficient in practice.
- **Windows gets the watchdog as a no-op**, because `proclive.Alive` cannot
  determine liveness there. Windows also has no reparent-to-PID-1 semantics, and
  the reported bug is macOS, so this is scoped rather than solved. The lock
  diagnostics do work on Windows, since unknown liveness still reports the
  recorded values.
- **A 2 second poll leaves a window** where a restarted desktop still sees the
  old lock. The new error message names the owning PID, which makes that window
  self-explanatory instead of mysterious.

## Out of scope

- Changing `flock` for a different ownership primitive, or any takeover,
  lock-breaking, or stale-lock-reclaim behavior.
- Wiring the watchdog into `dev.go`, or changing `make dev` process supervision.
- Any UI affordance for detecting or killing a conflicting backend from the
  desktop startup-error screen.
- Changing `BackendState::stop`, the health-token contract, or the desktop
  updater restart path.
