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
checks. No agent or shell process was started. Temporary overlays are removed
after delivery; no permanent test or collection rule is introduced.

## Files and dependencies

- `apps/backend/internal/agentctl/server/api/workspace.go`
- [Requirement](../../specs/platform/requirements/agentctl-workspace-stream-lifetime.md)
- [System design](../../specs/platform/system-design/agentctl-workspace-stream-lifetime.md)
- [Plan](plan.md)

The change has no dependency on URL lease recovery, cancelled-resume handling,
instance teardown changes, new endpoints, configuration or other feature PRs.
