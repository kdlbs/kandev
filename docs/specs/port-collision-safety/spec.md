---
status: building
created: 2026-08-07
---

# Port collision safety and launcher readiness

## Why

Kandev can currently start against the wrong backend when an explicitly requested port is
already occupied. The TypeScript development launcher and the native Go launcher accept a
successful health response from any process on that port, so a second Kandev instance can be
reported as ready while the newly launched backend is still failing to bind. That can open the
wrong SQLite database and make the failure look like a successful startup.

On Windows, the agentctl instance allocator has a separate failure: the operating system reports
WSAEADDRINUSE (10048), which does not match the synthetic Go syscall.EADDRINUSE value used by the
current retry check. The allocator therefore stops instead of trying the next port.

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
- UI changes, database migrations, or new public authentication semantics.

## Contract notes

The existing desktop health-token contract in
docs/specs/desktop-tauri-app/spec.md is the authority for the environment variable and response
header. This repair extends its use to CLI/native launcher ownership checks without changing the
backend route contract, so a new architecture decision record is not required.
