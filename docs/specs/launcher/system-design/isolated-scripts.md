---
status: current
system: launcher
requirements:
  - REQ-LAUNCHER-ISOLATED-SCRIPTS-001
---

# Isolated instance script system design

## Purpose and boundaries

The Windows PowerShell helpers are the Windows equivalent of the Unix
`scripts/dev-isolated` and `scripts/kandev-kill` tools. They launch and tear
down one throwaway Kandev instance so an operator can debug a build without
touching the live installation. They reuse the native launcher's binaries and
the `dev` profile but do not change launcher startup, backend routes, or
production data.

The helpers are Windows-only and PowerShell 5.1 compatible; they depend on
`Get-NetTCPConnection`, `Get-CimInstance Win32_Process`, and `Stop-Process`.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-LAUNCHER-ISOLATED-SCRIPTS-001` | [Port selection](#port-selection), [Home isolation](#home-isolation), [Child environment](#child-environment), [Loopback binding](#loopback-binding), [Teardown](#teardown) |

## Components and responsibilities

- `scripts/isolated-instance.psm1` provides the safe-home checks, reparse-point
  checks for reused-home state paths, and allowlisted child-environment setup.
- `scripts/dev-isolated.ps1` resolves safe ports and a safe home, ensures the
  backend binaries, launches the backend and optional Vite frontend, waits for
  health, and writes the pidfile.
- `apps/backend/cmd/winjob` owns a private isolated-launch mode: it starts a
  suspended child with file-backed streams, assigns it to the existing
  kill-on-close Job Object before resuming it, and publishes its PID and
  creation time atomically. The supervisor remains alive until its child exits.
- `scripts/kandev-kill.ps1` reads a pidfile and terminates exactly the
  processes that belong to that instance.
- `scripts/kandev-instances.ps1` lists running kandev backends and marks the
  guarded production ports.

## Port selection

`$GuardedPorts` holds the well-known production ports. `Assert-NotGuarded`
refuses an explicitly requested guarded port, and `Select-FreePort` scans
upward from a non-production base. Agentctl bases use separate 200-port slots:
the base is followed by a reserved 100-port instance range. The entire block
is checked against listeners and guarded ports before launch.

## Home isolation

`$IsolatedHome` defaults to a unique `%USERPROFILE%\.kandev-test-<port>-<id>`
directory for each launch. An explicit `-HomeDir` may reuse a safe home.
`Resolve-SafeIsolatedHome`
canonicalizes the resolved path and fails closed when it is the real user
profile root, inside the default or configured production Kandev home, a drive
or filesystem root, or inside a git workspace. It inspects each existing path
component and refuses symbolic links and junctions. It returns the resolved
path so later reads and writes use the same value the guard checked. An
existing `.gitconfig` must match the minimal isolated configuration, and
`-CopyDb` refuses an existing destination database instead of replacing it.
Before use, the launcher also checks its database, Git config, and application
data paths inside a reused home for symbolic links and junctions.
The script then creates `data`, writes a minimal `.gitconfig` only when absent,
and starts the backend with `KANDEV_HOME_DIR`, `HOME`, and `USERPROFILE`
pointing at the isolated home, so the fresh SQLite database and provider state
never touch live data.

## Child environment

`scripts/isolated-instance.psm1` builds a small child environment from required
Windows process variables, a PATH with the backend or Node directory first,
isolated home and application-data paths, and explicit process settings. It
does not pass through the caller's other environment variables. For each
supervisor launch, the launcher temporarily installs this environment in its
PowerShell process and restores the caller environment in a `finally` block.
The supervisor passes that environment to its child and redirects stdin to
NUL and stdout/stderr to the per-launch log files. ShellExecute gives the
supervisor a separate hidden console so caller pipes can reach EOF.
The backend receives an explicit empty config file through
`KANDEV_INTERNAL_CONFIG_FILE`; this prevents config discovery from reading an
operator config in the checkout, profile, or system config directory. Vite
receives only its selected backend URL and development settings as application
configuration. Both children use the isolated home for application data and
the per-launch run directory for temporary files.

## Loopback binding

The backend, its agentctl child, and the Vite dev server bind `127.0.0.1` by
default. The launcher passes `AGENTCTL_LISTEN_HOST=127.0.0.1` explicitly so an
older backend/agentctl build cannot expose the control port. The web host is a
`-WebHost` parameter that defaults to `127.0.0.1`. When Vite runs,
`KANDEV_WEB_INTERNAL_URL` uses the selected host, while the browser opens the
backend URL. This keeps the frontend behind the Go backend proxy and avoids a
direct Vite API-port path. The launcher starts the installed Vite CLI through
Node inside the native supervisor. Host values are validated as IP addresses or
DNS hostnames before any startup work. Paths and arguments are passed directly
without a command shell, preserving literal percent signs, metacharacters, and
brackets. Listener discovery checks every row but accepts only the child PID
and creation time published by this supervisor, its parent PID, and its exact
expected executable path. Process identity is checked again during readiness,
so a concurrent compatible listener cannot be adopted. A network-reachable binding requires the operator to pass an
explicit host instead of inheriting the package script's wildcard default.

## Teardown

The numeric pidfile records the backend process; sidecars record backend/web
start times, the Vite listener PID, and the actual isolated home. The listing
script uses those records rather than guessing the home from its own shell.
Records are created without replacing existing files. Failed startup removes
only records created by that launch. The supervisor owns the Job Object until
the child exits, then terminates any remaining descendants.
`kandev-kill.ps1` verifies process start times and web launch identity before
expanding the web descendant tree. It can find the matching pidfile from an
explicit backend port and can stop the verified web tree when the backend has
already exited. It rechecks each process identity before `Stop-Process` to
avoid stopping a process that reused a PID. Missing or stale identity records
fail closed. Teardown refuses guarded production ports unless `-Force` is given.

## Failure and recovery

The launcher reports an actionable error before any write when the home fails
the safety guard or a requested port is guarded. Web prerequisites are checked
before backend startup. `-Install` runs the Unix-oriented root recipes through
Git Bash on Windows. If its own listener does not appear within `-Timeout`, or its supervisor exits,
the launcher prints recent logs and fails. Discovery and readiness each have
an independent `-Timeout` window. Discovery, health, and metadata-write
failures stop the retained supervisor handles; closing their Job Objects
terminates the entire owned descendant trees, including late-spawning
children. A fatal web-launch error also stops the backend supervisor.
Unrelated listeners and existing metadata remain untouched. The launcher returns once
the backend is healthy and leaves it detached, so a non-interactive caller
tears it down in the same run.
