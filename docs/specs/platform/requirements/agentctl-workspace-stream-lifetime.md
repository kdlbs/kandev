---
status: active
system: platform
created: 2026-10-20
owners:
  - kandev
---

# Agentctl Workspace Stream Lifetime

## Overview

Each agentctl instance serves one workspace stream WebSocket to the backend.
The handler runs an input loop that reads client frames and a forwarding loop
that writes workspace events and shell output. A stopped instance never
publishes again, so a forwarding loop that only notices a departed client on
its next write stays parked forever and keeps the whole instance (API server,
process manager, ACP adapter and its queues, MCP server) reachable. Each
stopped instance then retains several megabytes of private memory until the
agentctl process exits.

## Requirements

### REQ-PLATFORM-AGENTCTL-WORKSPACE-STREAM-LIFETIME-001: The workspace stream ends with its client

**Intent:** Stopping an instance releases its workspace stream handler and
everything it references, whether or not the instance publishes anything after
the client leaves.

**User story:** As a user who runs many sessions over a long day, I want
stopped sessions to give their agentctl memory back instead of piling up until
I restart Kandev.

#### Acceptance criteria

- **AC-PLATFORM-AGENTCTL-WORKSPACE-STREAM-LIFETIME-001.1:** When the input side
  of a workspace stream ends (the client closes the socket or a read fails),
  the forwarding loop shall return without waiting for another workspace event
  or shell output, and the handler shall release its subscriptions.
- **AC-PLATFORM-AGENTCTL-WORKSPACE-STREAM-LIFETIME-001.2:** The existing exits
  are unchanged: a write error, a closed subscription, and credential
  invalidation still end the stream, and frames sent while the client is
  attached are delivered as before.

## Compatibility and exclusions

The stream protocol, message types, ping and pong handling, and the backend
client are unchanged. Closing workspace streams from instance teardown is not
part of this requirement; the backend closing its side is enough for the
handler to end.

## Design and delivery

- [System design](../system-design/agentctl-workspace-stream-lifetime.md)
- [Delivery package](../../../plans/agentctl-workspace-stream-leak/plan.md)
