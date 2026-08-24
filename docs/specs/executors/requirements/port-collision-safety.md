---
status: active
system: executors
created: 2026-08-07
updated: 2026-08-19
owners:
  - kandev
---
# Port collision and backend ownership safety Requirements

## Overview

Kandev can currently start against the wrong backend when an explicitly requested port is already occupied. The TypeScript development launcher and the native Go launcher accept a successful health response from any process on that port, so a second Kandev instance can be reported as ready while the newly launched backend is still failing to bind. That can open the wrong SQLite database and make the failure look like a successful startup.

## Requirements

### REQ-EXECUTORS-PORT-COLLISION-SAFETY-001: Port collision and backend ownership safety

**Intent:** Kandev can currently start against the wrong backend when an explicitly requested port is already occupied. The TypeScript development launcher and the native Go launcher accept a successful health response from any process on that port, so a second Kandev instance can be reported as ready while the newly launched backend is still failing to bind. That can open the wrong SQLite database and make the failure look like a successful startup.

#### Acceptance criteria

- **AC-EXECUTORS-PORT-COLLISION-SAFETY-001.1:** Explicit values include the CLI port flags, KANDEV_BACKEND_PORT, KANDEV_PORT, and the Makefile PORT value after PR #2368 translates it to a CLI flag.
- **AC-EXECUTORS-PORT-COLLISION-SAFETY-001.2:** An occupied explicit port is a hard startup error. The error names the numeric port and the configuration source, and does not silently select a different backend port.
- **AC-EXECUTORS-PORT-COLLISION-SAFETY-001.3:** A free explicit port continues through the existing launch path.
- **AC-EXECUTORS-PORT-COLLISION-SAFETY-001.4:** When no backend port is configured, the existing automatic preferred-port and fallback-port selection remains unchanged.
- **AC-EXECUTORS-PORT-COLLISION-SAFETY-001.5:** The preflight is a race-reduction measure, not the ownership proof: a port can still be taken between the probe and the child bind. Readiness ownership below closes that remaining race.
- **AC-EXECUTORS-PORT-COLLISION-SAFETY-001.6:** Nothing answers a loopback connect on either the IPv4 (127.0.0.1) or the IPv6 (::1) loopback address.
- **AC-EXECUTORS-PORT-COLLISION-SAFETY-001.7:** A fresh loopback bind on that port succeeds.
- **AC-EXECUTORS-PORT-COLLISION-SAFETY-001.8:** The instance allocator marks an occupied candidate unavailable and retries the next candidate.

## Migrated source detail

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

### Port-availability probe detects wildcard listeners

Wherever a launcher decides whether a port is free, whether it is preflighting an explicit
backend or web port, or selecting a preferred-then-random automatic port, the probe must treat a
port as available only when both of these hold:

- Nothing answers a loopback connect on either the IPv4 (127.0.0.1) or the IPv6 (::1) loopback
  address.
- A fresh loopback bind on that port succeeds.

The connect probe is required because a running Kandev backend binds the wildcard address
(0.0.0.0 and [::]), and a specific-address bind check alone reports that port as free. On
BSD-derived systems including macOS, and because the bind probe itself sets SO_REUSEADDR, a
loopback bind against an already-active wildcard listener succeeds, so a bind-only check falsely
reports the busy port as available. Probing both loopback families is required because the
existing backend may hold only the IPv6 wildcard socket. The bind probe is retained because it
catches reservations a connect probe misses, such as Windows phantom port reservations and ports
in TIME_WAIT.

Each connect probe is bounded by a short timeout so a silently dropped SYN, for example under
WSL2 mirrored networking to an unbound loopback port, cannot hang port selection. A timed-out or
refused connect means nothing is listening.

This restores the dual connect-and-bind, dual-stack behavior the TypeScript launcher had before
`make dev` moved to the native Go launcher (PR #2411), where the Go probe was bind-only on the
IPv4 loopback. The contract is not English error-text matching on socket errors.

### Backend readiness ownership

The process that owns user-visible readiness must create one fresh opaque health token for the
launch. An ordinary TypeScript or native Go launcher invocation owns readiness, creates the token,
passes it to the backend through the existing KANDEV_DESKTOP_HEALTH_TOKEN environment variable,
and retains it for supervisor-managed backend restarts.

The Tauri desktop shell is a nested-launch exception because it owns the outer readiness check and
WebView navigation. It creates the token before invoking `kandev --headless`, identifies the launch
as desktop-owned with `KANDEV_DESKTOP_NATIVE_NOTIFICATIONS=true`, and passes the token to the native
launcher. When both the desktop-owned marker and a non-empty token are present, the native launcher
must preserve that exact token for its backend child and its own health poll. It must not replace the
desktop-owned token with a second generated value. Without that marker, the native launcher must
replace any ambient KANDEV_DESKTOP_HEALTH_TOKEN with a fresh token so a stale shell environment
cannot claim readiness ownership.

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

That message also names the current owner when the owner is knowable. After a backend acquires a
lock it records advisory owner metadata, its process ID, executable path, and start time, into the
lock file it already holds, replacing any previous content. A process rejected for a conflict reads
that metadata and includes it in its stderr message, so an operator who has no leftover terminal can
identify the process to stop instead of being told only that the home is busy.

This metadata is diagnostics, not ownership. The operating-system lock remains the sole proof: the
metadata is never consulted to decide whether a lock is held, whether it is stale, or whether it may
be taken over, and acquisition never waits on it. Failing to write it does not fail acquisition, and
absent, empty, or unparseable metadata falls back to the message that names only the path. Because a
crashed owner leaves its last-written metadata in place, a reader that can positively determine the
recorded process is no longer running omits the owner detail rather than naming a dead process; a
platform that cannot determine liveness reports the recorded values as-is.

The lock sidecar is opened with platform no-follow semantics and must resolve to a regular file before
the backend can write metadata. A symlink or platform reparse point at the lock path is rejected
without modifying its target. The first byte of the sidecar is reserved for the Windows lock range,
so a conflicting process can read the advisory metadata through a separate handle while the lock is
held.

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

### Wildcard-listener port availability

1. Given a running backend holds the preferred port through a wildcard bind (0.0.0.0 and [::]),
   when the launcher selects an automatic port with no explicit port configured, then the
   availability probe reports the preferred port as busy and the launcher falls back to a free
   random port instead of choosing the occupied preferred port.
2. Given a wildcard listener holds an explicitly requested backend or web port, when the launcher
   preflights that port, then it reports the port as busy and exits with the existing hard error
   naming the port and its source.
3. Given nothing is listening on a candidate port but a loopback connect to an unbound port is
   silently dropped, when the availability probe runs, then the bounded connect timeout elapses,
   the fresh bind succeeds, and the port is reported available.

### Issue #2372: readiness from the wrong process

1. Given a stranger responds 2xx without the expected token, when the launcher polls health, then
   it does not announce readiness or open the browser.
2. Given a stranger responds 2xx with a mismatched token, when the launcher polls health, then it
   continues polling and does not treat that response as success.
3. Given the launched backend responds 2xx with the matching token, when the launcher polls
   health, then it announces readiness exactly once.
4. Given the supervisor restarts the backend for the same launcher invocation, when the restarted
   backend responds with the retained token, then health succeeds without accepting a stranger.
5. Given the Tauri shell marks a launch as desktop-owned and supplies a non-empty token, when the
   nested native launcher starts and polls the backend, then the backend and both readiness checks
   use that same token and the WebView can navigate without waiting for the startup timeout.
6. Given an ordinary CLI launch inherits a stale health token without the desktop-owned marker,
   when the native launcher starts, then it replaces the stale value with a fresh token for the
   backend and its own health poll.

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
6. Given a backend owns a Kandev home and has recorded its owner metadata, when a second backend
   starts against the same home, then the rejection names the owning process ID and executable in
   addition to the conflicting home path.
7. Given a lock file still holds metadata from an owner that has since died, when a second backend
   is rejected by whichever process holds the lock now, then the message omits owner detail it
   cannot confirm and still names the conflicting home path.
8. Given a backend cannot write its owner metadata, when it has already acquired the lock, then it
   starts normally and a later conflicting process falls back to the path-only message.
9. Given a lock sidecar path is a symlink or platform reparse point, when the backend starts, then
   it rejects the lock before writing and leaves the link target unchanged.

## Out of scope

- Changing default ports, fallback ranges, or automatic-port selection policy.
- Choosing a different port when the user explicitly requested one.
- PID or process-tree identity matching as an ownership, staleness, or takeover decision; the
  launcher wrapper chain makes that unreliable for dev mode, and a recorded PID can be stale or
  reused. Advisory owner metadata written into the lock file is diagnostic text only and is
  explicitly not such a decision input.
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
docs/specs/desktop/requirements/desktop-tauri-app.md is the authority for the environment variable and response
header. Backend runtime-state ownership follows
[ADR-2026-08-09-exclusive-runtime-state-ownership](../../../decisions/2026-08-09-exclusive-runtime-state-ownership.md).
The repair plan is
[Backend runtime-state ownership](../../../plans/backend-runtime-state-ownership/plan.md).
