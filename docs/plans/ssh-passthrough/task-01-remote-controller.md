---
id: "01-remote-controller"
title: "Own remote passthrough in agentctl"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-PASSTHROUGH-001
acceptance_criteria:
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.3
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.4
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.6
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.7
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.8
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.9
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.11
system_design:
  - ../../specs/executors/system-design/ssh-passthrough.md
---

# Task 01: Own remote passthrough in agentctl

## Summary

Add a per-instance remote passthrough owner using the existing PTY runner.
Expose authenticated control, process snapshots, terminal streams, and correlated
events through typed controller APIs and matching backend client methods.

## In scope

- Neutral DTOs under `internal/agentctl/types` for protocol version 1.
- Serialize start/replace/stop against the configured session and ACP admission.
- Stable start operation IDs, generation checks, automatic-input deduplication,
  and explicit uncertain-write results.
- Binary terminal I/O and resize. Emit input-submitted only after PTY acceptance.
- First-idle observation and bounded ordered event replay. Capture events before
  releasing startup readiness. Do not decide task completion in the controller.
- Authentication, instance/session isolation, credential invalidation, request
  bounds, secret redaction, and complete owned shutdown.

## Out of scope

MCP file preparation, production lifecycle selection, and browser gateway routing
belong to Task 02. Recovery policy belongs to Task 03.

## Acceptance

1. The controller starts exactly one structured-argv PTY for its session.
   `TestRemotePassthroughConcurrentStart`, `TestRemotePassthroughSiblingIsolation`,
   and `TestRemotePassthroughInputDedup` prove retry and generation boundaries.
2. `TestRemotePassthroughDisconnectPreservesProcess` and
   `TestRemotePassthroughEarlyExitReplay` prove buffered reconnect and lossless
   startup-event observation within retention. Unauthenticated, wrong-instance,
   wrong-session, stale-generation, and invalidated streams cannot control it.
3. `TestRemotePassthroughStopReapsDescendants` proves teardown closes admission,
   stops descendants, and joins all owners. A slow terminal consumer cannot
   block PTY draining or teardown. Raw commands, env values, and config bytes
   do not enter snapshots or generic logs.

## Verification

Run from the repository root. The proposed tests use `TestRemotePassthrough` names.

```bash
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agentctl/server/instance ./internal/agent/runtime/agentctl -run 'TestRemotePassthrough|TestInteractive' -count=1)
git diff --check
```

Run `bash scripts/lint-go-changed` with the exact changed Go paths after TDD.
Record that complete invocation and its result below.

## Files likely touched

- `apps/backend/internal/agentctl/types/passthrough.go` (new wire types).
- `apps/backend/internal/agentctl/server/process/passthrough.go` (new owner)
  and `passthrough_test.go` (new tests).
- `apps/backend/internal/agentctl/server/process/interactive_lifecycle.go`,
  `interactive_output.go`, and `interactive_io.go` only where shared ownership
  or atomic buffer/subscription operations require it.
- `apps/backend/internal/agentctl/server/process/manager.go`, plus small
  adjacent helpers for construction and teardown.
- `apps/backend/internal/agentctl/server/api/passthrough.go`,
  `passthrough_stream.go`, and matching tests (new).
- `apps/backend/internal/agentctl/server/api/server.go` for route registration.
- `apps/backend/internal/agent/runtime/agentctl/client_passthrough.go` and
  matching client tests (new).
- `apps/backend/internal/agentctl/server/instance/manager_passthrough_test.go`
  (new instance teardown regression).

## Dependencies

None. Existing `InteractiveRunner`, instance auth, client auth, and ownership
admission provide the source patterns.

## Risks

Requests can overlap instance teardown. Register cleanup immediately and use
manager admission for every owner. Reconnect cleanup can race a newer socket.
Remove a writer only when the cleanup still owns that connection generation.

## Parallelism

`sequential`

## Inputs

- [Controller runtime and API design](../../specs/executors/system-design/ssh-passthrough.md#controller-runtime).
- [ADR](../../decisions/2026-10-14-ssh-passthrough-process-ownership.md).
- `apps/backend/internal/agentctl/AGENTS.md` and API scoped guidance.
- `.agents/skills/tdd/references/backend-tests.md`.
- Existing `shell_terminal.go`, `client_shell_terminal.go`, interactive-runner
  tests, and instance teardown tests. Reuse the transport pattern, not shell
  command injection.

## Results

Completed. Startup/session-admission regressions failed with HTTP 404 before
the controller implementation. Event streaming failed with a WebSocket handshake
error before its implementation. Secret-log and already-exited Stop regressions
also demonstrated their failures before correction.

The work-order Go command passed with `-race`, `-trimpath`, and `fts5` for all
four packages, including existing interactive-runner tests, remote controller
tests, client capability/error tests, and instance teardown. It used task-owned
`GOCACHE=/tmp/kandev-ssh-go-cache.7r9WV7` because the shared cache lost artifacts.

Changed-package lint through `bash scripts/lint-go-changed` passed with zero
issues. The final invocation named `interactive_lifecycle.go`, API passthrough
tests, instance passthrough tests, and client passthrough tests, covering all
four changed packages. Earlier lint reported five issues. Helper extraction,
the existing query constant, and flat response-error handling corrected them.

`git diff --check` passed. No commits or publication actions were performed.
