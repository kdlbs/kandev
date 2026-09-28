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
`Start-Process`, the launcher temporarily installs this environment in its
PowerShell process and restores the caller environment in a `finally` block.
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
direct Vite API-port path. The launcher starts the installed Vite CLI directly
through Node, so the recorded web PID is the actual listener rather than a
pnpm/cmd wrapper. A network-reachable binding requires the operator to pass an
explicit host instead of inheriting the package script's wildcard default.

## Teardown

The numeric pidfile records the backend process; sidecars record backend/web
start times, the Vite listener PID, and the actual isolated home. The listing
script uses those records rather than guessing the home from its own shell.
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
Git Bash on Windows. A fatal web-launch error tears down the backend; teardown
leaves no listener owned by the verified instance. The launcher returns once
the backend is healthy and leaves it detached, so a non-interactive caller
tears it down in the same run.
