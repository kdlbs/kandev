---
id: "01-workspace-stream-input-end"
title: "End the workspace stream with its input side"
status: done
updated: 2026-10-23
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-AGENTCTL-WORKSPACE-STREAM-LIFETIME-001
acceptance_criteria:
  - AC-PLATFORM-AGENTCTL-WORKSPACE-STREAM-LIFETIME-001.1
  - AC-PLATFORM-AGENTCTL-WORKSPACE-STREAM-LIFETIME-001.2
system_design:
  - ../../specs/platform/system-design/agentctl-workspace-stream-lifetime.md
---

# Task 01: End the workspace stream with its input side

## Summary

Make the workspace stream forwarding loop return as soon as the input loop
ends, so a stopped instance's handler no longer waits for an event that never
comes.

## In scope

- `handleWorkspaceStreamWS`: run the input loop through a wrapper that closes
  `inputDone` on return.
- `forwardWorkspaceStream`: add an `inputDone` arm that returns.

## Out of scope

Instance teardown, the agent event stream, the backend client, and the stream
protocol.

## Verification

Use a disposable overlay for the original input-end case, input decoding failure,
explicit shutdown and independent-connection controls. Reuse existing workspace
stream delivery, ping/shell-input, concurrent writes and credential-rotation
checks through `internal/agentctl/server/api`. The checks use in-process
WebSockets over `httptest`; no agent or shell process is started.

Observed result: the original input-end endpoint check failed on the baseline
(handler still active after three seconds, package 3.172s). The candidate's one
selected `-race` run passed (2.514s), including that endpoint, invalid JSON,
normal close frame, explicit done, credential invalidation, independent
connections, and the existing stream delivery, shell-input and concurrent-write
checks. No agent or shell process was started. Those original temporary
overlays were removed after verification.

The existing `workspace_stream_ws_test.go` now retains the endpoint regression.
Its shared helper exposes handler completion without changing existing callers.
The test reads the connected frame, closes the client, publishes no event or
shell output, and waits for the handler itself to return before cleanup runs.
Removing only the inputDone exit makes this test fail (10.527s); the committed
candidate and selected existing input/event controls pass under fts5/race
detection (2.255s). Cleanup can still release a failed test's listener. Hosted
CI remains a separate result.

## Files and dependencies

- `apps/backend/internal/agentctl/server/api/workspace.go`
- [Requirement](../../specs/platform/requirements/agentctl-workspace-stream-lifetime.md)
- [System design](../../specs/platform/system-design/agentctl-workspace-stream-lifetime.md)
- [Plan](plan.md)

The change has no dependency on URL lease recovery, cancelled-resume handling,
instance teardown changes, new endpoints, configuration or other feature PRs.

## Current upstream CI baseline

The normal upstream merge retains the existing input-close implementation and
endpoint regressions while adopting current backend dependencies and CI/cache
configuration. The workspace stream selection passed on the merged source
with race detection (2.802s), including idle client close, shell input, ping,
ignored messages and tracker forwarding.

The hosted Windows persistence compile-only step missed both caches and
reached its 10-minute deadline. A separate mobile clone-recovery failure and
its cleanup/reset errors do not establish an input-close regression; their
backend artifact needs baseline comparison if the current-base CI repeats it.
No recovery refusal, timeout or assertion is weakened.
