---
status: draft
system: platform
created: 2026-11-11
owners:
  - kandev
---

# Terminal connection keepalive requirements

## Overview

An open terminal must remain connected during ordinary periods without input
or output. An idle reverse proxy must not cause repeated reconnections and
terminal redraws while the browser remains responsive.

Platform owns this shared transport guarantee. Tasks retains terminal identity
and process ownership. Agents retains login-session policy. UI retains terminal
presentation and navigation.

## Terminology

- **Terminal connection:** The browser WebSocket for a task shell, script
  terminal, agent passthrough terminal, Quick Terminal, host shell, or agent login.
- **Responsive peer:** A connected peer that processes WebSocket pings and
  returns pongs within the connection's liveness deadline.
- **Proxy idle timeout:** The maximum period without traffic from the backend
  before an intermediary closes the upgraded connection.

## Requirements

### REQ-PLATFORM-TERMINAL-KEEPALIVE-001: Terminal connection liveness

**Intent:** Users can leave a terminal open without proxy-induced interruption.
Unavailable peers release their connection resources without stopping a retained
shell process.

#### Acceptance criteria

- **AC-PLATFORM-TERMINAL-KEEPALIVE-001.1:** With a responsive peer and a proxy idle
  timeout of at least 60 seconds, every terminal connection shall remain attached
  across repeated idle windows. The server shall send WebSocket pings every
  54 seconds, independently of PTY input and output. This applies to local and
  remote task terminals and every terminal type defined above. The socket reader
  shall continue processing pongs while a terminal message is being handled.
  It shall hold at most one additional application message; if that slot is full,
  the server shall close the connection rather than block control-frame handling.
- **AC-PLATFORM-TERMINAL-KEEPALIVE-001.2:** After connection or the last received
  pong, an unresponsive peer shall reach its liveness deadline after 60 seconds.
  The connection shall close even while the PTY produces output. Another
  responsive connection shall remain usable.
- **AC-PLATFORM-TERMINAL-KEEPALIVE-001.3:** Keepalive shall preserve terminal input,
  output, resizing, buffered replay, and final exit messages. It shall inject no
  terminal bytes, execute no command, and cause no terminal reset or redraw.
- **AC-PLATFORM-TERMINAL-KEEPALIVE-001.4:** On disconnect, deadline expiry, write
  failure, or stream completion, the connection shall release its background
  work and subscriptions. Transport loss shall preserve the existing PTY
  lifetime and explicit stop rules. Keepalive shall not extend agent-login
  inactivity limits or count as user or agent activity.
- **AC-PLATFORM-TERMINAL-KEEPALIVE-001.5:** Desktop and phone terminals shall
  provide the same liveness behavior through their existing controls. After a
  genuine transport failure, the existing reconnect and buffered-history
  behavior shall remain available without repeating a completed one-shot command.

## Compatibility and exclusions

The application data protocol, terminal routes, authentication, origin checks,
and process ownership remain unchanged. Standard WebSocket control frames carry
the keepalive traffic. No client heartbeat message or operator setting is required.

This contract does not cover proxy idle limits shorter than the ping interval,
absolute connection-age limits, or suspended browsers that stop responding.
It does not change reconnect reset/replay semantics, add output cursors, repair
component remounts, or change executor-to-agentctl transport policy.
When terminal input exceeds the bounded pending-message capacity, the server
closes the stream as a transport failure and the existing reconnect behavior
applies.

## Design and delivery

- [System design](../system-design/terminal-keepalive.md)
- [Implementation package](../../../plans/terminal-keepalive/plan.md)
