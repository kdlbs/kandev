# ADR-2026-08-09-exclusive-runtime-state-ownership: Lock Runtime State Before Backend Startup

**Status:** accepted
**Date:** 2026-08-09
**Area:** backend, infra

## Context

Backend startup changes shared state before it binds the HTTP port. It opens logs, creates database
backups, applies migrations, and reconciles active sessions. Launcher port checks do not protect
direct backend commands or a bind race.

A development command started a second backend against the live Kandev home. The process changed
session state and then failed to bind the occupied HTTP port. SQLite serialized individual writes,
but it did not protect the ownership of the runtime state.

## Decision

Each backend process must acquire non-blocking operating-system advisory locks before shared-state
initialization. The process locks the canonical Kandev home. It also locks a custom SQLite database
when that database is outside the home.

The backend acquires all required locks after it loads configuration and before it initializes the
backend logger. It holds the locks until backend cleanup is complete. Lock acquisition errors stop
startup before logs, backups, migrations, session recovery, agentctl, or HTTP services start.

The implementation uses file-handle locks on Unix and Windows. A crash releases these locks. The
implementation keeps lock files after release because removing a locked file can create two lock
identities.

The lock scope is independent of ports. Intentional local instances use separate Kandev homes,
separate SQLite databases, and separate ports. The launcher supervisor stops the old backend before
it starts the replacement, so a planned restart preserves this ownership rule.

## Consequences

A stray direct backend command cannot change a live Kandev database or session state. The failure
appears before the process creates its normal backend log, so the launcher or terminal must show a
clear stderr message.

The lock adds platform-specific code and native Windows coverage. Custom SQLite paths outside the
home need a second lock target. Postgres backends still lock their local home, but this decision does
not define active-active Postgres operation.

An operator can find an old lock file after a crash. The file is harmless because the operating
system lock, not the file, proves ownership.

## Alternatives Considered

### Bind the HTTP port first

Rejected. A different port can still open the same home or SQLite database. Port ownership does not
protect logs, secrets, backups, worktrees, or migrations.

### Keep ownership checks in launchers

Rejected. Direct `__backend`, `make dev-backend`, service managers, and bind races can bypass a
launcher check. The backend owns persistent initialization and must enforce the boundary.

### Use a PID file

Rejected. A crash leaves stale PID data, and operating systems can reuse a PID. A PID file does not
provide atomic ownership.

### Depend on SQLite write locking

Rejected. SQLite protects transactions, not the semantic ownership of startup recovery. It also
does not protect other files in the Kandev home.
