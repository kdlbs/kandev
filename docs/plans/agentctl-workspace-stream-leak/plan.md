---
created: 2026-10-20
status: done
updated: 2026-10-23
requirements:
  - REQ-PLATFORM-AGENTCTL-WORKSPACE-STREAM-LIFETIME-001
system_design:
  - ../../specs/platform/system-design/agentctl-workspace-stream-lifetime.md
legacy_specs: []
---

# Implementation Plan: Agentctl workspace stream leak

## Overview

An agentctl instance can leave its workspace stream handler parked in
`forwardWorkspaceStream`. The input loop returns on a read error without
telling the forwarding loop, and the forwarding loop only exits on a write
error, a closed subscription, credential invalidation, or the handler's own
`done`, none of which fires after the instance stops. The parked goroutine
keeps the instance's API server, process manager, ACP adapter, and MCP server
reachable after the client has disconnected.

## Scope

In scope: `apps/backend/internal/agentctl/server/api/workspace.go`
(`handleWorkspaceStreamWS`, `forwardWorkspaceStream`) and its linked lifetime
requirement and system design. Disposable checks use the existing API runner;
no permanent test is added.

Excluded: closing workspace streams from instance teardown, the agent event
stream, the backend client, and the stream protocol.

## Technical approach

- The handler runs `handleWorkspaceStreamInput` in a wrapper goroutine that
  closes `inputDone` when the loop returns.
- `forwardWorkspaceStream` takes `inputDone` and returns when it closes.

## Work orders

- [x] [Task 01: End the workspace stream with its input side](task-01-workspace-stream-input-end.md)

## Changed-path coverage

The work order owns `apps/backend/internal/agentctl/server/api/workspace.go`,
this plan, its work order, and the workspace stream lifetime requirement and
system design. The original source's input-end case is used only through a
temporary overlay outside repository test collection and removed afterward.

## Validation status

The original endpoint check failed before the fix: after a client disconnected
without further instance output, the handler remained active at the three-second
bound. The candidate passed the same endpoint check, prepared input-error,
shutdown and independent-connection controls, and existing stream, shell-input,
concurrent-write and credential-rotation selections in one `-race` run (2.514s).
The temporary overlays are removed after delivery. No standing test is added.
