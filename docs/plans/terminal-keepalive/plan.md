---
created: 2026-11-11
status: done
requirements:
  - REQ-PLATFORM-TERMINAL-KEEPALIVE-001
system_design:
  - ../../specs/platform/system-design/terminal-keepalive.md
legacy_specs: []
---

# Implementation plan: Terminal connection keepalive

## Overview

Repair [issue #5273](https://github.com/kdlbs/kandev/issues/5273) by applying
the main gateway's keepalive policy to every browser terminal connection.
One work order owns the helper, handler integration, regression coverage, and
public protocol note. Implementation is complete.

Platform owns connection liveness across task and login terminals. Existing
task-terminal requirements explicitly exclude transport changes. The new
[requirement](../../specs/platform/requirements/terminal-keepalive.md) fills this
behavioral gap, with its paired
[system design](../../specs/platform/system-design/terminal-keepalive.md).

## Evidence and root cause

Investigation used `fa0c48b88e774936fd28d81123e16dc96e9a5be3` on 2026-11-11.
Remote `main` was `4f736702fe0ce37a04aa87017a0c3983c744d26f` at the comparison.
Its single additional commit only changed release publication code and documents.
The affected terminal sources were unchanged.

The canonical issue contains no image attachments or comments. Its reported
91-second reconnect cadence and Safari failures are reporter evidence. The
original proxy, runtime logs, and browser were not available here.

Confirmed source trace:

1. `client.go` installs a 60-second read deadline and pong handler. Its writer
   sends pings every 54 seconds.
2. `runUserShellBridge`, `runTerminalBridge`, and `bridgeBinaryWebSockets` read
   terminal connections without that deadline, pong handler, or ping loop.
3. `loginpty.Handlers.handleStream` has the same omission. It serves login,
   host-shell, and Quick Terminal sessions through one endpoint.
4. An intermediary therefore receives no backend traffic from an idle terminal.
   On disconnect, `ws-reconnect.ts:startReconnectLoop` resets xterm before
   reconnecting. The backend replays buffered output, producing the reported redraw.

Temporary real-WebSocket probes reproduced the missing ping. Each peer actively
read control frames and waited 57 seconds, beyond the existing gateway interval:

| Probe | Result |
| --- | --- |
| `TestRepro5273IdleTerminalPing/local-shell` | No ping, read timed out after 57.03 seconds |
| `TestRepro5273IdleTerminalPing/local-agent` | No ping, read timed out after 57.03 seconds |
| `TestRepro5273IdleTerminalPing/remote-bridge` | No ping, read timed out after 57.03 seconds |
| `TestRepro5273IdleLoginStreamPing` | No ping, read timed out after 57.03 seconds |

The gateway probes used the production bridges and existing deferred-process
fixtures. The login probe used the real stream HTTP route and an idle in-memory
session. These probes prove the transport omission, without launching a user
process or reproducing the reporter's original proxy cycle.

Diagnostic command, expected failure:

```bash
(cd apps/backend && go test -tags fts5 -trimpath ./internal/gateway/websocket ./internal/agent/loginpty -run '^TestRepro5273' -count=1 -v -timeout 120s)
```

Both temporary `repro_5273_test.go` files were removed. The existing focused
baseline passed: eight gateway tests and seven host-shell tests.

```bash
(cd apps/backend && go test -tags fts5 -trimpath ./internal/gateway/websocket ./internal/agent/loginpty -run '^(TestWsWriter|TestBridgeBinaryWebSockets|TestRunTerminalBridge|TestRunUserShellBridge|TestHostShell)' -count=1 -v -timeout 120s)
```

GitHub assignment to `carlosflorencio` was applied and verified. No issue comment,
production edit, permanent test, commit, or PR was created during investigation.

## Scope

### In scope

- Share the existing gateway timing defaults without changing its pumps.
- Add ping/pong handling to local, remote, passthrough, and login terminal streams.
- Bound control writes and release connection-owned goroutines and subscriptions.
- Preserve terminal bytes, resizing, shell lifetime, replay, and exit behavior.
- Add regression coverage for idle proxies, missed pongs, and concurrent writes.
- Add a terminal-specific note to the public WebSocket reference upon implementation.

### Out of scope

- The independent Quick Terminal remount loop in issue #5272.
- Cursor-based replay, removal of `terminal.reset()`, or reconnect-policy changes.
- New application heartbeat messages, flags, settings, or dependencies.
- Agentctl transport redesign, PTY supervision changes, or terminal UI changes.

## Technical approach

Use the shared policy and ownership rules in the system design. In particular,
send terminal pings with `WriteControl`. An unsynchronized `WriteMessage` loop
would race existing PTY writers. Install deadlines before starting readers.

Own the keepalive at each bridge rather than duplicating it at individual
upgrade branches. Remote shell and passthrough routes share one browser-side
bridge policy. Their upstream connections retain their existing protocol.

On stream completion, close before joining blocked connection work. Preserve
the login stream's final exit message before closure and its unsubscribe path.
Closing a transport must not stop a retained process.

The route and executor compatibility matrix is in the
[attachment coverage](../../specs/platform/system-design/terminal-keepalive.md#attachment-coverage)
section. No rendered composition changes, ASCII UI preview, or translation
updates are needed. Desktop and phone consume the same corrected transport.

## Tests

All acceptance IDs below use the prefix `AC-PLATFORM-TERMINAL-KEEPALIVE-001`.
The cases below provide the implemented acceptance coverage.

| Criteria | Test file and cases |
| --- | --- |
| `.1`, `.2` | `internal/common/wskeepalive/keepalive_test.go`: `TestKeepaliveDefaults`, `TestKeepalivePongDeadline`, `TestKeepaliveWriteFailure`, `TestKeepaliveStop`, `TestKeepaliveRejectsInvalidTiming`, `TestKeepaliveSetupFailureClosesConnection` |
| `.1`, `.3` | `internal/gateway/websocket/terminal_keepalive_test.go`: `TestTerminalKeepaliveIdle`, covering local shell, local agent, and remote bridge |
| `.1`, `.3` | `internal/agent/loginpty/stream_keepalive_test.go`: `TestLoginStreamKeepaliveIdle`, through the actual stream route |
| `.1`, `.3`, `.5` | `TestTerminalKeepaliveIdleProxy` and `TestLoginStreamKeepaliveIdleProxy`: transparent TCP proxy, repeated idle windows, same attachment, subsequent input/output |
| `.2`, `.4` | `TestTerminalKeepaliveUnresponsivePeer`, `TestLoginStreamKeepaliveUnresponsivePeer`: mixed responsive/unresponsive peers, ongoing output, independent teardown |
| `.3`, `.4` | `TestTerminalKeepaliveConcurrentOutput`, `TestLoginStreamKeepaliveExit`, `TestLoginStreamKeepaliveDisconnect`: control/data concurrency, final exit, writer joins, subscriber cleanup |
| `.4`, `.5` | Existing gateway cleanup tests, `TestHostShellSessionSurvivesWebSocketReconnect`, and one-shot/missing-environment tests in the affected gateway package |

Recreate the missing-ping regression first and observe behavioral failure.
Use shorter per-handler intervals for permanent integration tests after the
timing seam exists. Pin production defaults independently. Use fake time for
helper tests and bounded reads for real sockets. Do not add minute-long waits
to every test or mutate global timing values.

## E2E tests

The new Go proxy regressions provide end-to-end transport evidence with actual
WebSocket control frames. Existing Playwright tests provide visible application
coverage. They must not be described as proving proxy liveness by themselves.

- `chromium`: `terminal/terminal-first-class.spec.ts`,
  `session/terminal-env-keyed.spec.ts`, and `terminal/quick-terminal.spec.ts`.
- `mobile-chrome`: `terminal/mobile-terminal-task-ownership.spec.ts` and
  `terminal/mobile-quick-terminal.spec.ts`.

Paths are relative to `apps/web/e2e/tests/`. These existing flows cover `.3` and
`.5` through real shell input/output and retained terminal state. No frontend
source changes or new browser-only heartbeat fixture is planned.

## Work orders

- [x] [Task 01: Add terminal connection keepalive](task-01-terminal-keepalive.md)

Execute sequentially in the primary session. There are no task dependencies
or authorized subagents.

## Verification results

Implementation checks passed. Before production changes, all four real-path
missing-ping cases failed at the expected received-ping assertion after 57
seconds.

Implementation verification:

- Initial race-enabled Go tests passed: `wskeepalive` 6 passed; gateway websocket 664
  passed and 1 skipped; login PTY 40 passed. The run covered 711 test events.
- Changed-Go lint passed with 0 issues.
- `make -C apps/backend build` passed.
- Chromium E2E ran 10 tests and all passed. Mobile Chrome E2E ran 3 tests and
  all passed. Both runner invocations discovered and executed tests.
- Public documentation tests passed (63), all 55 published docs pages
  validated, specification catalog validation passed (449 decisions and 1986
  specifications), specification linter tests passed (36), and full
  specification lint passed.
- `git diff --check` passed.

Review remediation on the current snapshot passed the two focused gateway
keepalive setup-failure tests and the focused login PTY idle-proxy and
unresponsive-peer tests. These verify cleanup after setup failure, retained
process lifetime, a real PTY input/acknowledgment round trip, and producer
goroutine cleanup. No build or broader suite was rerun for these review-only
fixes.

All 15 existing focused baseline tests passed during the original investigation.

Design-package checks passed:

- Catalog validation: 449 decisions and 1986 specifications.
- Specification linter tests: all 36 passed.
- Full specification lint: all files passed.
- PR documentation coverage: the repository's `validateCoverage` accepted the
  work order and its complete reference chain. The preflight supplied the
  planned `terminal_pumps.go` change as a synthetic trigger. No runtime file changed.
- Package whitespace and path checks passed.

## Risks

- A second ordinary WebSocket writer can panic or corrupt terminal output.
- A ping failure that does not close the connection can leave a reader blocked.
- Joining a writer before connection closure can prevent handler cleanup.
- Browser suspension can exceed the pong timeout and still require reconnection.
- Proxies below the supported idle interval or with absolute age limits can close
  healthy connections. This repair does not promise otherwise.
- The six-second production gap between ping and pong deadlines follows the
  existing gateway policy. Large scheduling delays can still cause a reconnect.
