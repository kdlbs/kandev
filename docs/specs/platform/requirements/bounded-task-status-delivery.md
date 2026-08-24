---
status: active
system: platform
created: 2026-08-01
updated: 2026-08-19
owners:
  - kandev
---
# Bounded Task Status Delivery Requirements

## Overview

Task rows currently obtain compact status indicators by observing large, session-owned streams. With many agents working, one browser receives messages, shell activity, model updates, MCP state, and Git events from sessions that are not open. Switching tasks rebuilds those subscriptions. The resulting traffic can delay or drop request responses, producing an unknown message-send result and temporarily hiding active-session controls such as the model selector.

## Requirements

### REQ-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001: Bounded Task Status Delivery

**Intent:** Task rows currently obtain compact status indicators by observing large, session-owned streams. With many agents working, one browser receives messages, shell activity, model updates, MCP state, and Git events from sessions that are not open. Switching tasks rebuilds those subscriptions. The resulting traffic can delay or drop request responses, producing an unknown message-send result and temporarily hiding active-session controls such as the model selector.

#### Acceptance criteria

- **AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.1:** Every task snapshot may carry a `status_summary` containing the complete set of runtime facts needed by desktop and mobile task switchers.
- **AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.2:** The backend derives that summary from authoritative task, session, message, Git, and pull-request state. It is a rebuildable read model, not a second source of truth.
- **AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.3:** Summary updates use the existing workspace task feed. Task rows never subscribe to a session merely to obtain a badge.
- **AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.4:** Full session snapshots and live session notifications are delivered only to explicitly opened session detail surfaces.
- **AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.5:** Repeated subscribe/focus requests do not replay full session state. Git refresh has a targeted action that does not alter focus.
- **AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.6:** Correlated WebSocket responses and errors are prioritized over unsolicited notifications and cannot be silently dropped by notification pressure.
- **AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.7:** One session's streaming text or reasoning cannot monopolize persistence, notification delivery, or browser state work. Intermediate replacement updates are bounded while the final transcript remains lossless.
- **AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.8:** `message.add` uses a stable client-generated message ID so an uncertain send can be reconciled or retried without creating or dispatching a duplicate.

## System design

The migrated technical source is split into [part 1](../system-design/bounded-task-status-delivery.md).
