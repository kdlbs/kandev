---
status: active
system: launcher
created: 2026-09-28
owners:
  - kandev
---

# Isolated instance script requirements

## Overview

Windows contributors need the same isolated debugging instance that the Unix
`scripts/dev-isolated` and `scripts/kandev-kill` helpers provide. The PowerShell
helpers must let an operator run a throwaway instance and tear it down exactly,
without ever touching the live installation, production data, or another
instance. The launcher system owns this behavior because it already owns launch
mode dispatch, port selection, managed-process startup, and shutdown.

## Terminology

- **Isolated instance:** A backend and optional web frontend launched against a
  dedicated home and SQLite database, with mock providers and non-colliding
  ports.
- **Guarded production port:** A well-known port (backend `38429`, web `37429`,
  agentctl `39429`) that the helpers must never occupy or terminate without an
  explicit force option.
- **Pidfile:** The per-instance file that records the launched backend process.
  Adjacent sidecars record the web process, process start times, and isolated
  home for safe teardown and inspection.
- **Live-state boundary:** A filesystem location whose contents are live
  state, such as the real user profile root or the production kandev home.

## Requirements

### REQ-LAUNCHER-ISOLATED-SCRIPTS-001: Safe isolated Windows dev instance

**Intent:** Let a Windows operator debug a running build without touching the
live instance, production data, or any state outside the isolated home.

#### Acceptance criteria

- **AC-LAUNCHER-ISOLATED-SCRIPTS-001.1:** The launcher shall select ports that
  are not in use and are not guarded production ports, and shall refuse an
  explicit guarded production port. Agentctl instance port ranges shall not
  overlap between automatically selected instances.
- **AC-LAUNCHER-ISOLATED-SCRIPTS-001.2:** The isolated instance shall use a
  distinct home directory and SQLite database for each default launch,
  separate from the user profile root. An explicit `-HomeDir` may reuse a safe
  isolated home intentionally.
- **AC-LAUNCHER-ISOLATED-SCRIPTS-001.3:** Before creating any directory or
  file, the launcher shall refuse a resolved `-HomeDir` that is the real user
  profile root, inside the default or configured production Kandev home, a
  drive or filesystem root, or inside a git workspace. It shall also refuse
  paths that pass through symbolic links or junctions, including the database
  and application-data paths in a reused isolated home. It shall not replace an
  existing git configuration or SQLite database in a chosen isolated home.
- **AC-LAUNCHER-ISOLATED-SCRIPTS-001.4:** The isolated backend, agentctl, and
  Vite dev server shall bind `127.0.0.1` by default. The web server may bind a
  network-reachable host only when an explicit host option is given. When Vite
  runs, the backend proxy target shall use the selected web host and the
  browser shall open the backend URL.
- **AC-LAUNCHER-ISOLATED-SCRIPTS-001.5:** Teardown shall terminate the recorded
  backend and the full descendant process tree of the recorded web process, even
  when the backend has already exited. It shall verify process start identities
  before stopping a pidfile target, so a reused PID cannot stop an unrelated
  process.
- **AC-LAUNCHER-ISOLATED-SCRIPTS-001.6:** Teardown shall refuse a pidfile or
  explicit target that names a guarded production port unless a force option is
  given.
- **AC-LAUNCHER-ISOLATED-SCRIPTS-001.7:** Instance listing shall report the
  recorded isolated home when its pidfile still names the launched backend, and
  raw output shall be usable in a pipeline.
- **AC-LAUNCHER-ISOLATED-SCRIPTS-001.8:** The backend and Vite child processes
  shall receive an allowlisted environment plus explicit isolated settings.
  They shall not inherit host database, config-file, provider, Docker, or Vite
  configuration. The backend shall load an empty isolated config file instead
  of discovering operator config files. They shall use the isolated home for
  application data and the per-launch directory for temporary files.
- **AC-LAUNCHER-ISOLATED-SCRIPTS-001.9:** Windows CI shall run PowerShell tests
  for live-home boundaries, junction paths, inherited environment isolation,
  and restoration of the caller environment.
