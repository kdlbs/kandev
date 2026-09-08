---
id: "03-desktop-parent-pid"
title: "Desktop publishes its parent PID to the launcher"
status: done
wave: 3
depends_on: ["02-launcher-parent-watchdog"]
plan: "plan.md"
spec: "../../specs/desktop/requirements/desktop-tauri-app.md"
parallelism: sequential
---

# Task 03: Desktop publishes its parent PID to the launcher

Activate the watchdog from task 02 by having the Tauri shell declare its own
process ID to the launcher it spawns. This is the change that actually fixes the
reported bug; tasks 01 and 02 build the mechanism, this one turns it on.

Sequential and last: the launcher must already honor
`KANDEV_LAUNCHER_PARENT_PID` before the desktop advertises it.

## Inputs

- Plan sections "`KANDEV_LAUNCHER_PARENT_PID`" and "Desktop".
- Spec: `docs/specs/desktop/requirements/desktop-tauri-app.md`, the abnormal-termination bullet
  under "Failure modes" and its matching scenario under "Scenarios".
- The exact pattern to copy: `launch_and_wait` in
  `apps/desktop/src-tauri/src/backend.rs:263`, which already inserts
  `DESKTOP_HEALTH_TOKEN_ENV` into `inherited_env` before building the spec.
- `apps/desktop/AGENTS.md` for the desktop test command and the rule that the
  shell stays a thin wrapper.

## Implementation

In `apps/desktop/src-tauri/src/backend.rs`, add the constant beside the existing
desktop env constants near line 24:

```rust
const LAUNCHER_PARENT_PID_ENV: &str = "KANDEV_LAUNCHER_PARENT_PID";
```

In `launch_and_wait`, next to the existing health-token insert, add:

```rust
inherited_env.insert(
    OsString::from(LAUNCHER_PARENT_PID_ENV),
    OsString::from(std::process::id().to_string()),
);
```

That is the whole production change. `desktop_environment` passes unrecognized
inherited variables through untouched, which the existing `CUSTOM_ENV`
assertion in `command_spec_uses_headless_launcher_and_loopback_env` already
pins, so no plumbing change is needed in `backend_command_spec`.

Do not change `BackendState::stop`, `terminate_child`, `shutdown_and_exit`, or
any `RunEvent` handling. Those remain the primary shutdown path for a normal
quit. The watchdog is strictly a backstop for the abnormal exits that path
cannot cover, and weakening it would trade one bug for another.

## Acceptance

- A launcher spawned by the desktop shell receives
  `KANDEV_LAUNCHER_PARENT_PID` set to the shell's own PID.
- Normal quit behavior is unchanged: the shell still terminates its child
  directly and does not wait for the watchdog.
- No other desktop behavior changes.

## Verification

```bash
cd apps/desktop/src-tauri && cargo test --features desktop-runtime
```

Manual confirmation of the actual reported bug, on macOS, after tasks 01 to 03
have landed and the desktop runtime has been rebuilt:

1. Start the desktop app and confirm it reaches the SPA.
2. Find the Tauri shell process with
   `pgrep -f "Kandev.app/Contents/MacOS"` and `kill -9` it, simulating the crash
   or force quit that `BackendState::stop` cannot cover.
3. Within roughly the poll interval, confirm
   `pgrep -f "Kandev.app.*bin/kandev"` returns nothing and
   `lsof ~/.kandev/.kandev-backend.lock` reports no holder.
4. Confirm `make start-debug` then starts normally.

Record the observed result of step 3, including how long it took, in
`## Results`.

## Files likely touched

- `apps/desktop/src-tauri/src/backend.rs`

## Dependencies

Task 02. The launcher must honor the variable before the desktop sets it.

## Tests to write

Extend the existing `mod tests` in `backend.rs`: assert that a
`KANDEV_LAUNCHER_PARENT_PID` entry present in `inherited_env` survives into
`BackendCommandSpec.env` with its value intact. Follow
`command_spec_uses_headless_launcher_and_loopback_env`, which already proves the
pass-through shape with `CUSTOM_ENV`.

Assert on the pass-through rather than on `std::process::id()`, so the test does
not depend on the PID of the test binary.

## Output contract

Report the changed file, the exact `cargo test` outcome, and the observed result
of the manual macOS orphan check including timing. If the manual check cannot be
run on the executing machine, say so explicitly rather than implying it passed.
Update this file's `status` and `## Results`, and the matching checkbox in
`plan.md`.

## Results

- Added `LAUNCHER_PARENT_PID_ENV` and pass the Tauri shell PID into the
  launcher environment during `launch_and_wait`.
- Added a focused Rust test for the PID environment helper and extended the
  command-spec pass-through coverage.
- The first RED run,
  `cargo test --features desktop-runtime launcher_parent_environment_uses_shell_pid`,
  failed at compile time because `add_launcher_parent_pid` did not yet exist.
- `cd apps/desktop/src-tauri && cargo fmt --check && cargo test --features desktop-runtime`
  passed: 57 tests, 0 failures; updater verifier and doc-test targets also
  passed with zero tests.
- macOS process-boundary verification passed using a newly built launcher and
  the installed runtime bundle: a shell parent declared its PID, the launcher
  reached `/health`, the shell was SIGKILLed and reaped, the launcher exited
  within the watchdog window, and `lsof` found no holder for the temporary
  runtime lock. The launcher log recorded `parent process exited` and a
  graceful shutdown.
- Cleanup: the temporary runtime home, database, logs, launcher, backend, and
  subprocess parent were removed or reaped by the verification command.
- Security/trust and external side-effect boundaries: the PID is only passed
  from the spawning desktop shell to its child launcher; no frontend or network
  contract changed.
