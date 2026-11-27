---
status: draft
system: platform
created: 2026-10-13
owners:
  - kandev
---

# Shared PTY lifecycle requirements

## Overview

Platform owns the shared pseudo-terminal resource used by interactive agentctl
terminals and backend login and host-shell sessions. Consumers need closing the
terminal to release pending I/O independently of the child process exiting.
Task terminal persistence and Quick Terminal descriptor ownership remain with
their existing systems.

## Requirements

### REQ-PLATFORM-SHARED-PTY-001: Interruptible Unix terminal closure

**Intent:** A pending terminal read must not prevent an explicitly closed
terminal from releasing its resources and delivering the normal terminal hangup.

#### Acceptance criteria

- **AC-PLATFORM-SHARED-PTY-001.1:** When a consumer closes a shared Unix
  terminal with a read pending and the child-side terminal still open, the read
  shall return without requiring further child output or child termination.
- **AC-PLATFORM-SHARED-PTY-001.2:** Reading, writing and resizing an open
  terminal shall retain their existing behavior. Resizing before or during a
  pending read shall not prevent subsequent closure from interrupting that read.
- **AC-PLATFORM-SHARED-PTY-001.3:** If terminal initialization fails after
  starting its child, the start operation shall return the initialization error
  and clean up the child and terminal resources it owns; it shall not return a
  usable terminal or leave an untracked running child.
- **AC-PLATFORM-SHARED-PTY-001.4:** Explicit closure shall retain each caller's
  process ownership, termination grace period, forced-termination fallback and
  completion tracking. Browser disconnection alone shall not close a retained
  host-shell session.

## Out of scope

- Changing Windows ConPTY behavior or the separate embedded shell implementation.
- Reducing termination timeouts, skipping workspace cleanup, or changing retry policy.
- Changing terminal persistence, automatic respawn, output retention or UI controls.
- Guaranteeing that arbitrary children obey hangup or promising a CI speed percentage.

## Implementation plans

- [Interruptible shared PTY closure](../../../plans/shared-pty-close/plan.md).
