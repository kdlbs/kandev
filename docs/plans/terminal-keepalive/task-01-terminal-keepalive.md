---
id: "01-terminal-keepalive"
title: "Add terminal connection keepalive"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-TERMINAL-KEEPALIVE-001
acceptance_criteria:
  - AC-PLATFORM-TERMINAL-KEEPALIVE-001.1
  - AC-PLATFORM-TERMINAL-KEEPALIVE-001.2
  - AC-PLATFORM-TERMINAL-KEEPALIVE-001.3
  - AC-PLATFORM-TERMINAL-KEEPALIVE-001.4
  - AC-PLATFORM-TERMINAL-KEEPALIVE-001.5
system_design:
  - ../../specs/platform/system-design/terminal-keepalive.md
---

# Task 01: Add terminal connection keepalive

## Summary

Keep idle browser terminals attached through supported reverse proxies.
Apply shared ping/pong timings to the gateway terminal bridges and login PTY
stream, with connection-owned cleanup and regression evidence.

## In scope

- Reproduce the missing ping before changing behavior, using the plan's probe
  shape and the permanent `TestTerminalKeepaliveIdle` and
  `TestLoginStreamKeepaliveIdle` test names.
- Introduce `internal/common/wskeepalive` with shared defaults, a control-frame
  ping loop, pong deadlines, and an idempotent stop/join function.
- Reference the defaults from `client.go` without changing its existing pumps.
- Initialize per-handler timings in both constructors. Use them at
  `runUserShellBridge`, `runTerminalBridge`, `bridgeBinaryWebSockets`, and
  `Handlers.handleStream`.
- Close connections before output detachment, writer mutex acquisition, or
  joins. Preserve final exit delivery, existing unsubscribe operations, and
  retained PTY lifetime.
- Keep the socket reader processing pongs while PTY input is being written.
  Buffer up to 256 pending messages and 1 MiB of payload; allow a single larger
  message when the queue is empty, and close on overflow rather than block
  control-frame handling.
- Add the helper, handler, proxy, concurrency, and teardown tests named in the plan.
- Update `docs/public/websocket-api.md` with a terminal-specific keepalive note.
  The note is protocol reference material. State the proxy interval constraint
  and preserve the main socket's separate JSON and queue limits.

## Out of scope

- Frontend production code, reconnect reset/replay, and issue #5272.
- New settings, runtime flags, dependencies, schemas, and metrics.
- Agentctl transport policy, authorization changes, and process supervision.

## Acceptance

1. Every long-lived terminal attachment uses the shared policy. The proxy tests
   preserve one connection over repeated idle windows and deliver subsequent input/output.
2. Missed pongs, blocked control writes, disconnect, and upstream completion
   release connection work. Another responsive peer and retained PTYs remain usable.
3. A PTY input write that blocks does not prevent pong handling; the queued
   message remains ordered and is delivered after the writer resumes.
4. Race-enabled tests and the named desktop/phone flows pass. Terminal bytes,
   resize messages, replay, exit notices, and one-shot command behavior remain compatible.

## Implementation sequence

1. Read the requirement, design, affected handlers, and existing bridge fixtures.
2. Mark this work order `in_progress` and synchronize the plan.
3. Add the missing-ping tests with an active peer reader and bounded deadline.
   Observe no ping through the real production bridge or stream route.
   A missing helper or field compilation error does not count as behavioral RED.
4. Add the helper and connect all four boundaries with the production defaults.
5. Use private per-handler timing values to accelerate permanent integration tests.
   Keep a separate test for the actual production constants.
6. Add the proxy, mixed-peer, output-concurrency, exit, and teardown cases.
   Keep subscriptions and joinable goroutines observable through existing fixtures.
7. Add the public protocol note and run the exact checks below.
8. Record command results and mark this work order `done` only after all checks pass.

## Verification

Run from the repository root. Install the web workspace once if its dependencies
are absent. Keep browser commands sequential and use the managed runner's limits.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -tags fts5 -trimpath -race ./internal/common/wskeepalive ./internal/gateway/websocket ./internal/agent/loginpty -count=1 -timeout 10m)
bash scripts/lint-go-changed apps/backend/internal/common/wskeepalive/keepalive.go apps/backend/internal/common/wskeepalive/keepalive_test.go apps/backend/internal/common/wskeepalive/goleak_test.go apps/backend/internal/gateway/websocket/client.go apps/backend/internal/gateway/websocket/terminal_handler.go apps/backend/internal/gateway/websocket/terminal_pumps.go apps/backend/internal/gateway/websocket/terminal_keepalive_test.go apps/backend/internal/agent/loginpty/handlers.go apps/backend/internal/agent/loginpty/stream_keepalive_test.go
(cd apps/web && pnpm e2e:run --project chromium tests/terminal/terminal-first-class.spec.ts tests/session/terminal-env-keyed.spec.ts tests/terminal/quick-terminal.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/terminal/mobile-terminal-task-ownership.spec.ts tests/terminal/mobile-quick-terminal.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

The pre-fix missing-ping tests must fail at a received-ping assertion.
After integration, the real proxy tests must fail without the heartbeat wiring
and pass with it. Record the actual test counts and results, including browser
discovery, without inferring coverage from a zero-test run.

Use `testing/synctest` for helper timing with a fake connection where feasible.
The real-socket fixtures use bounded reads and explicit shutdown joins.
The proxy forwards bytes without terminating or answering WebSocket control frames.
Its timer observes only traffic from the terminal backend.

## Files likely touched

All paths below are relative to the repository root. New files are marked.

- `apps/backend/internal/common/wskeepalive/keepalive.go` (new)
- `apps/backend/internal/common/wskeepalive/keepalive_test.go` (new)
- `apps/backend/internal/common/wskeepalive/goleak_test.go` (new)
- `apps/backend/internal/common/wsinput/reader.go` and `reader_test.go` (new)
- `apps/backend/internal/gateway/websocket/client.go`
- `apps/backend/internal/gateway/websocket/terminal_handler.go`
- `apps/backend/internal/gateway/websocket/terminal_pumps.go`
- `apps/backend/internal/gateway/websocket/terminal_keepalive_test.go` (new)
- `apps/backend/internal/agent/loginpty/handlers.go`
- `apps/backend/internal/agent/loginpty/stream_keepalive_test.go` (new)
- `docs/public/websocket-api.md`
- This work order and `plan.md` for execution results.

## Dependencies

None. This is one implementation slice. Existing route admission and executor
stream acquisition remain prerequisites supplied by current code.

## Risks

- Gorilla permits one ordinary data writer. Use `WriteControl` for concurrent pings.
- Install pong handlers and read deadlines before starting each reader.
- Stop must join after cancellation and connection closure can unblock writes.
- Preserve the final login exit frame before writer-driven closure.
- A socket timeout must detach the stream without stopping its retained PTY.
- Production logs and activity counters must not receive heartbeat traffic.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/platform/requirements/terminal-keepalive.md), all five criteria.
- [System design](../../specs/platform/system-design/terminal-keepalive.md), all sections.
- [Investigation and test matrix](plan.md#evidence-and-root-cause).
- `apps/backend/AGENTS.md` and `.agents/skills/tdd/references/backend-tests.md`.
- Gateway patterns: `terminal_wsutil_test.go`, `terminal_pumps_bridge_test.go`,
  `terminal_pumps_pty_test.go`, and remote passthrough handler tests.
- Login patterns: `handlers_test.go` and `manager_test.go`.
- `.agents/skills/e2e/SKILL.md` for the existing browser checks.

## Results

Implemented shared WebSocket ping/pong timing and connection-owned cleanup for
local shell, local agent, remote bridge, and login PTY streams. Added helper,
idle-proxy, concurrent-output, unresponsive-peer, exit, disconnect, and cleanup
coverage. Updated the public WebSocket reference.

The pre-fix real-path regression failed at the expected missing-ping assertion
for all four attachments. Initial implementation checks passed:

- Race-enabled Go tests: `wskeepalive` 6 passed; gateway websocket 664 passed
  and 1 skipped; login PTY 40 passed.
- Changed-Go lint: 0 issues.
- `make -C apps/backend build`.
- Chromium terminal E2E: 10 passed. Mobile Chrome terminal E2E: 3 passed.
- Public docs tests: 63 passed; 55 published docs pages validated.
- Specification catalog validation: 449 decisions and 1986 specifications.
- Specification linter tests: 36 passed; full specification lint passed.
- `git diff --check`.

Web dependencies were present, so the conditional install command was not run.

Review remediation fixed local bridge cleanup when keepalive setup returns an
error, changed the login proxy test to verify a real PTY input/acknowledgment
round trip, made the continuous-output producer cancelable and joinable on
every exit path, and made frame-observer cleanup cancellable before joining.
Gateway regression cases cover invalid timing on open and closed sockets and a
closed socket whose initial read-deadline setup fails. Each case verifies peer
closure, output tracking cleanup, writer closure, and retained process lifetime.
No build or broader suite was rerun for these review-only fixes.

The PR review finding about blocked PTY input was reproduced by
`TestLoginStreamKeepaliveReadsPongsDuringBlockedInput` before the reader change.
The shared `wsinput.Reader` now keeps pongs flowing while handlers process a
message and closes when its 256-message or 1 MiB pending-payload budget fills.
A single larger message remains valid when the queue is otherwise empty. The
expanded blocked-input regression failed against the former one-message limit,
then passed while queuing four additional messages and checking pong handling
and ordered PTY delivery. Targeted verification passed:

- `go test -tags fts5 -trimpath -race ./internal/common/wsinput -run '^TestReader' -count=1 -timeout 60s`.
- `go test -tags fts5 -trimpath -race ./internal/agent/loginpty -run '^TestLoginStreamKeepalive(ReadsPongsDuringBlockedInput|IdleProxy|UnresponsivePeer)$' -count=1 -timeout 90s`.
- `go test -tags fts5 -trimpath -race ./internal/gateway/websocket -run '^(TestRunTerminalBridgeKeepaliveSetupFailureCleansUp|TestRunUserShellBridgeKeepaliveSetupFailureCleansUp|TestTerminalKeepaliveUnresponsivePeer|TestTerminalKeepaliveConcurrentOutput)$' -count=1 -timeout 90s`.

The corresponding non-race focused test commands also passed. Changed-Go lint
passed with 0 issues after the final review changes. No build or broader suite
was rerun. Public documentation tests passed (63), all 55 pages validated, the
catalog validated (449 decisions and 1986 specifications), and full spec lint
passed after the documentation update.
