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
- **Pidfile:** The per-instance file that records the launched backend and web
  process identifiers for teardown.
- **Live-state boundary:** A filesystem location whose contents are live
  state, such as the real user profile root or the production kandev home.

## Requirements

### REQ-LAUNCHER-ISOLATED-SCRIPTS-001: Safe isolated Windows dev instance

**Intent:** Let a Windows operator debug a running build without touching the
live instance, production data, or any state outside the isolated home.

#### Acceptance criteria

- **AC-LAUNCHER-ISOLATED-SCRIPTS-001.1:** The launcher shall select ports that
  are not in use and are not guarded production ports, and shall refuse an
  explicit guarded production port.
- **AC-LAUNCHER-ISOLATED-SCRIPTS-001.2:** The isolated instance shall use a
  dedicated home directory and SQLite database that are separate from the
  default user profile home.
- **AC-LAUNCHER-ISOLATED-SCRIPTS-001.3:** Before creating any directory or
  file, the launcher shall refuse a resolved `-HomeDir` that is the real user
  profile root, the production kandev home, a drive or filesystem root, or a git
  workspace root.
- **AC-LAUNCHER-ISOLATED-SCRIPTS-001.4:** The isolated backend and the Vite dev
  server shall bind `127.0.0.1` by default, and shall bind a network-reachable
  host only when an explicit host option is given.
- **AC-LAUNCHER-ISOLATED-SCRIPTS-001.5:** Teardown shall terminate the recorded
  backend and the full descendant process tree of the recorded web process, so
  that no Vite listener remains.
- **AC-LAUNCHER-ISOLATED-SCRIPTS-001.6:** Teardown shall refuse a pidfile or
  explicit target that names a guarded production port unless a force option is
  given.
