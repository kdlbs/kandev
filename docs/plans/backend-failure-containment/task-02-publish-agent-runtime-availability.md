---
id: "02-publish-agent-runtime-availability"
title: "Publish agent runtime availability"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/agent-runtime-availability.md"
---

# Task 02: Publish agent runtime availability

## Intent

Turn an unexpected standalone `agentctl` exit into an authoritative,
sanitized, replayable runtime snapshot without changing process ownership or
backend health semantics.

## Acceptance

- The launcher invokes one unexpected-exit callback for zero or non-zero child
  exits that were not caused by `Stop`; intentional shutdown invokes none.
- The availability tracker is concurrency-safe, does not expose startup as
  available, becomes available only after health and authentication succeed,
  and transitions monotonically to unavailable with reason `agentctl_exited`
  within one boot.
- Connected clients receive `system.agent_runtime.status_changed`, and clients
  that subscribe later receive the latest full snapshot.
- Every authenticated boot/app-state response includes `agentRuntime`; the
  unauthenticated minimal boot payload does not disclose it.
- Process error text, PID, and exit code remain logs-only.
- No `/health`, child restart, token, PID, lifecycle-client, or execution
  rebinding behavior changes.

## TDD sequence

1. Add launcher tests for unexpected non-zero exit, unexpected clean exit, and
   intentional stop; confirm callback cases fail.
2. Add tracker tests for internal startup, post-handshake availability, one-way
   unavailable transition, concurrent snapshot reads, stable reason, and event
   publication; confirm RED before the tracker exists.
3. Add boot-state and gateway tests for live broadcast and late subscription
   replay, including auth-disabled/authenticated versus unauthenticated boot
   payloads.
4. Implement the callback, tracker, composition, event/action constants, boot
   hydration, and replay wiring. Rerun focused tests and refactor only after
   GREEN.

## Files likely touched

- `apps/backend/internal/agent/runtime/agentctl/availability.go`
- `apps/backend/internal/agent/runtime/agentctl/availability_test.go`
- `apps/backend/internal/agent/runtime/agentctl/launcher/launcher.go`
- `apps/backend/internal/agent/runtime/agentctl/launcher/launcher_test.go`
- `apps/backend/internal/backendapp/agentctl.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/boot_state.go`
- `apps/backend/internal/backendapp/helpers_test.go`
- `apps/backend/internal/events/types.go`
- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/gateway/websocket/system_notifications.go`
- `apps/backend/internal/gateway/websocket/system_notifications_test.go`
- `apps/backend/cmd/kandev/main.go`

## Dependencies

None.

## Parallelism

`parallel-safe` with Task 01. This task does not touch ACP normalized stream or
adapter files.

## Verification

- `cd apps/backend && go test ./internal/agent/runtime/agentctl/... ./internal/backendapp ./internal/gateway/websocket`

## Inputs

- Existing launcher `monitorExit` intentional-shutdown classification.
- Existing `system/jobs.Tracker`, `SystemEventBroadcaster`, boot-state builder,
  and `Hub.AddUserSubscriptionListener` replay patterns.
- ADR 0019 supervisor-owned restart boundary.

## Output contract

Record focused test results and the final public JSON/action shape. Note any
startup-composition file that differs from the likely list because wiring moved.

## Results

Implemented the launcher callback, concurrency-safe monotonic tracker, event
and WebSocket action, authenticated boot hydration, and user-subscription
replay. The wire snapshot is `{status, reason?, occurred_at?}`; startup is
unpublished, healthy startup is `{status: "available"}`, and the only failure
transition is `{status: "unavailable", reason: "agentctl_exited"}` with a UTC
occurrence timestamp. Raw process errors, PID, and exit code remain logs-only.

RED coverage was added for the missing launcher callback, tracker publication
and monotonicity, boot hydration/authenticated boundary, and live/replayed
WebSocket delivery. Focused verification passed:

```text
go test ./internal/agent/runtime/agentctl/... ./internal/backendapp ./internal/gateway/websocket
562 passed in 4 packages
```
