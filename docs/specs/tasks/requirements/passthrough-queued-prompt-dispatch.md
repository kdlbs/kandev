---
status: active
system: tasks
created: 2026-08-31
owners:
  - cfl
---

# Passthrough Queued Prompt Dispatch Requirements

## Overview

The task system owns queued prompt selection, turn boundaries, and session
operation serialization. CLI passthrough supplies the PTY transport, but a
queued follow-up must start a successor turn without wedging the session that
owns the queue.

## Terminology

- **Passthrough session:** A task session whose agent runs as an interactive
  CLI under a PTY instead of through ACP prompt transport.
- **Ready drain:** The turn-completion path that selects an eligible queued
  prompt and starts the next turn.

## Requirements

### REQ-TASKS-PASSTHROUGH-QUEUED-PROMPT-DISPATCH-001: Responsive passthrough ready drain

**Intent:** Deliver the next queued prompt exactly once while keeping the
session responsive to subsequent lifecycle operations.

#### Acceptance criteria

- **AC-TASKS-PASSTHROUGH-QUEUED-PROMPT-DISPATCH-001.1:** When a running
  passthrough session completes a turn with an eligible queued prompt, the
  system shall accept that prompt exactly once. The system shall write the
  prompt to the PTY and start the successor turn.
- **AC-TASKS-PASSTHROUGH-QUEUED-PROMPT-DISPATCH-001.2:** When starting that
  successor turn emits the session's running-state event, the ready drain shall
  not wait on its own lock. The event handler shall complete. Later delete,
  cancel, and queue operations shall remain responsive without a backend
  restart.

## Out of scope

- Changing the synchronous ordering contract of the in-memory event bus.
- Adding lock watchdogs or WebSocket request deadlines.
- Changing frontend terminal reconnect behavior for terminal-state sessions.
- Changing passthrough prompt formatting, submit sequences, or queue policy.
