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
| `REQ-LAUNCHER-ISOLATED-SCRIPTS-001` | [Port selection](#port-selection), [Home isolation](#home-isolation), [Loopback binding](#loopback-binding), [Teardown](#teardown) |

## Components and responsibilities

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
upward from a non-production base and skips a port when `Test-PortInUse`
reports a listener, so an isolated instance never collides with a running
instance or a production process.

## Home isolation

`$IsolatedHome` defaults to `%USERPROFILE%\.kandev-test`. `Resolve-SafeIsolatedHome`
canonicalizes the resolved path and fails closed when it is the real user
profile root, the production `~/.kandev` home, a drive or filesystem root, or a
git workspace root; it returns the resolved path so later reads and writes use
the same value the guard checked. The script then creates `data`, writes a
minimal `.gitconfig`, and starts the backend with `KANDEV_HOME_DIR`,
`HOME`, and `USERPROFILE` pointing at the isolated home, so the fresh SQLite
database and provider state never touch live data.

## Loopback binding

The backend and the Vite dev server bind `127.0.0.1` by default. The web host
is a `-WebHost` parameter that defaults to `127.0.0.1`; the frontend is started
as `pnpm exec vite --host <host>`, so a network-reachable binding requires the
operator to pass an explicit host instead of inheriting the package script's
wildcard default.

## Teardown

The pidfile records the backend process and the launched web wrapper process.
`kandev-kill.ps1` expands the full descendant tree of the recorded web process
with `Get-DescendantPids` before calling `Stop-Process`, so the actual Vite
listener is terminated instead of orphaned when the recorded wrapper exits
first. Teardown refuses a pidfile entry or explicit target whose port is a
guarded production port unless `-Force` is given, and acts only on the
processes named by that pidfile.

## Failure and recovery

The launcher reports an actionable error before any write when the home fails
the safety guard or a requested port is guarded. Teardown leaves no listening
process on the instance's ports; the launcher returns once the backend is
healthy and leaves it detached, so a non-interactive caller tears it down in the
same run.
