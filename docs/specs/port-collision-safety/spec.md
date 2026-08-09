---
status: building
created: 2026-08-07
updated: 2026-08-09
---

# Port collision and backend ownership safety

## Why

Kandev can currently start against the wrong backend when an explicitly requested port is
already occupied. The TypeScript development launcher and the native Go launcher accept a
successful health response from any process on that port, so a second Kandev instance can be
reported as ready while the newly launched backend is still failing to bind. That can open the
wrong SQLite database and make the failure look like a successful startup.

On Windows, the agentctl instance allocator has a separate failure: the operating system reports
WSAEADDRINUSE (10048), which does not match the synthetic Go syscall.EADDRINUSE value used by the
current retry check. The allocator therefore stops instead of trying the next port.

A direct backend command can bypass launcher checks. A second backend can then open the same
Kandev home before its HTTP bind fails. This sequence can migrate the live database and reconcile
active sessions while the first backend still owns them.

This repair covers GitHub issues
[#2370](https://github.com/kdlbs/kandev/issues/2370),
[#2372](https://github.com/kdlbs/kandev/issues/2372), and
[#2371](https://github.com/kdlbs/kandev/issues/2371). PR
[#2368](https://github.com/kdlbs/kandev/pull/2368) remains the correct Makefile wiring for
passing PORT and WEB_PORT, but it does not provide the safety checks described here.

## What

### Explicit backend-port preflight

The TypeScript and native Go launchers must probe an explicitly configured backend port before
starting their backend child:

- Explicit values include the CLI port flags, KANDEV_BACKEND_PORT, KANDEV_PORT, and the Makefile
  PORT value after PR #2368 translates it to a CLI flag.
- An occupied explicit port is a hard startup error. The error names the numeric port and the
  configuration source, and does not silently select a different backend port.
- A free explicit port continues through the existing launch path.
- When no backend port is configured, the existing automatic preferred-port and fallback-port
  selection remains unchanged.
- The preflight is a race-reduction measure, not the ownership proof: a port can still be taken
  between the probe and the child bind. Readiness ownership below closes that remaining race.

### Backend readiness ownership

Every TypeScript or native Go launcher invocation must create a fresh opaque health token before
starting its backend. It passes the token to the child through the existing
KANDEV_DESKTOP_HEALTH_TOKEN environment variable and retains it for supervisor-managed backend
restarts.

The launcher health poll succeeds only when the response is a 2xx response and its
X-Kandev-Desktop-Health-Token response header exactly matches the token generated for that
invocation. A 2xx response without the header or with a different value is not readiness from the
launched backend; polling continues until the child exits or the normal timeout is reached.

The existing backend health route and desktop token names are reused. Direct backend health
requests without a launcher-supplied token remain compatible with the current route behavior.
The token is not printed in startup output or failure diagnostics. The existing owner-only
supervisor manifest is allowed to carry the launch environment so an intentional backend restart
continues to answer with the same token.

### Windows address-in-use handling

The agentctl instance allocator and websocket tunnel must use one shared cross-platform
address-in-use classifier. It must recognize wrapped address-in-use errors on Unix and both the
Go syscall value and x/sys/windows.WSAEADDRINUSE on Windows.

- The instance allocator marks an occupied candidate unavailable and retries the next candidate.
- The websocket tunnel retains its existing user-facing “port is already in use” error.
- Non-address-in-use bind errors still release the candidate and fail immediately.
- String matching on the English error text is not part of the contract.

### Exclusive runtime-state ownership

Every backend process must acquire exclusive ownership before it initializes the backend logger or
opens a persistent store. The ownership boundary covers these targets:

- The canonical Kandev home, because it owns logs, secrets, worktrees, supervisor files, and the
  default SQLite database.
- A custom SQLite database outside that home, because separate homes can still reference the same
  database file.

The backend holds each operating-system advisory lock until all backend cleanup is complete. A
crash releases the lock. The lock file can remain after exit because file existence does not prove
ownership.

If another process holds a required lock, startup exits non-zero. It names the conflicting home or
database path and tells the operator to use a separate `KANDEV_HOME_DIR` for an intentional second
instance. The rejected process must not initialize file logging, create a database backup, apply a
migration, reconcile a session, launch agentctl, or start an HTTP server.

The backend fails closed when it cannot create, open, or acquire a required lock. Launcher port
preflight and health-token ownership remain useful readiness checks, but they are not persistent
state ownership checks.

Intentional local instances need separate Kandev homes and separate SQLite databases. A backend
that uses Postgres still locks its local Kandev home. Active-active Postgres deployment behavior is
not part of this contract.

## Scenarios

### Issue #2370: explicit port collisions

1. Given a free CLI or environment-selected backend port, when dev, start, or run launches,
   then the requested port is used and the normal readiness flow follows.
2. Given another process already owns an explicitly selected port, when the launcher starts, then
   it exits non-zero before starting the backend child or opening the browser, and the message
   includes the port and source.
3. Given no explicit backend port and the preferred port is occupied, when the launcher starts,
   then it chooses an available fallback as it does today.

### Issue #2372: readiness from the wrong process

1. Given a stranger responds 2xx without the expected token, when the launcher polls health, then
   it does not announce readiness or open the browser.
2. Given a stranger responds 2xx with a mismatched token, when the launcher polls health, then it
   continues polling and does not treat that response as success.
3. Given the launched backend responds 2xx with the matching token, when the launcher polls
   health, then it announces readiness exactly once.
4. Given the supervisor restarts the backend for the same launcher invocation, when the restarted
   backend responds with the retained token, then health succeeds without accepting a stranger.

### Issue #2371: Windows allocator retry

1. Given the first agentctl instance candidate is occupied on Windows, when an instance is
   allocated, then the allocator marks that candidate unavailable and binds the next candidate.
2. Given every candidate is occupied, when an instance is allocated, then the allocator returns
   its existing exhaustion error after releasing candidates correctly.
3. Given a tunnel port is occupied on Windows, when a tunnel is requested, then the caller gets
   the existing clear “port is already in use” error.

### Concurrent backend startup

1. Given one backend owns a Kandev home, when another backend uses the same home, then the second
   process exits before any persistent backend initialization.
2. Given two homes reference the same external SQLite database, when both backends start, then
   only one process opens or changes that database.
3. Given a backend process stops or crashes, when its successor starts with the same home, then the
   successor acquires ownership without manual lock-file removal.
4. Given a live backend uses the default home, when a developer runs a direct backend target
   against that home, then the direct command fails without changing task or session state.
5. Given two distinct homes, databases, and ports, when two local backends start, then both can run
   as independent instances.

## Out of scope

- Changing default ports, fallback ranges, or automatic-port selection policy.
- Choosing a different port when the user explicitly requested one.
- PID or process-tree identity matching; the launcher wrapper chain makes that unreliable for
  dev mode.
- Changing the health response body, the backend health status semantics, or the desktop
  WebView flow.
- Renaming KANDEV_DESKTOP_HEALTH_TOKEN to a neutral variable; that would be a separate contract
  migration.
- Changing service-install handling of KANDEV_SERVER_PORT, which is a separate installer issue.
- Database-schema changes or new public authentication semantics.
- Automatic home selection for direct backend commands.
- Active-active backend support for one Postgres database or one event namespace.
- New UI diagnostics for ownership or task-summary fields.

## Contract notes

The existing desktop health-token contract in
docs/specs/desktop-tauri-app/spec.md is the authority for the environment variable and response
header. Backend runtime-state ownership follows
[ADR-2026-08-09-exclusive-runtime-state-ownership](../../decisions/2026-08-09-exclusive-runtime-state-ownership.md).
The repair plan is
[Backend runtime-state ownership](../../plans/backend-runtime-state-ownership/plan.md).
