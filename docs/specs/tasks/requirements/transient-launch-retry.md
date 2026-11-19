---
status: active
system: tasks
created: 2026-11-19
owners:
  - kandev
---

# Transient Launch Retry Requirements

## Overview

An interactive session launch can fail for reasons that clear on their own a
moment later: the agent runtime is not ready yet, a startup step hits its
deadline, or the workspace launch is not ready. Today such a failure leaves the
session and the task `FAILED` until the user notices and retries by hand. The
task system owns this contract because it owns the session and task states and
the existing recovery actions that a retry reuses.

## Terminology

- **Transient launch failure:** A launch failure whose sanitized error says the
  agent runtime was not ready, a deadline was exceeded, the workspace launch
  failed, or the workspace execution was not ready.
- **Runtime retry recovery:** The existing recovery action that resumes a
  session holding a resume token and otherwise starts it fresh.

## Requirements

### REQ-TASKS-TRANSIENT-LAUNCH-RETRY-001: Retry a transient launch failure once

**Intent:** Recover a session from a startup failure that clears by itself,
without retrying failures that need the user, and without retry loops.

**User story:** As a user who starts a task, I want a launch that failed only
because a dependency was briefly unavailable to recover by itself, so that I do
not have to notice the failure and retry by hand.

#### Acceptance criteria

- **AC-TASKS-TRANSIENT-LAUNCH-RETRY-001.1:** When an interactive launch fails
  with a transient launch failure and leaves both the session and the task
  `FAILED`, the system shall retry that session once, after a short delay,
  through runtime retry recovery, keeping the session's profile and resume
  token.
- **AC-TASKS-TRANSIENT-LAUNCH-RETRY-001.2:** The system shall not retry a
  launch failure that is not transient, a cancelled launch (for example a user
  stop while the agent runtime is still starting), a failure of an Office or
  automation task, or a session parked by a workflow profile switch.
- **AC-TASKS-TRANSIENT-LAUNCH-RETRY-001.3:** When the retried launch fails
  again, the task shall stay `FAILED` with its existing recovery actions, and
  the system shall log the cause. A session shall not be retried automatically
  more than once within an hour.
- **AC-TASKS-TRANSIENT-LAUNCH-RETRY-001.4:** When the task or session leaves
  `FAILED` before the delay ends, the system shall not relaunch it. When the
  backend stops, a pending retry shall be cancelled.
- **AC-TASKS-TRANSIENT-LAUNCH-RETRY-001.5:** A retry shall retain the failed
  launch identity and automatic launch origin through admission and capacity
  deferral. It shall not act on a later failure or on a workspace-only prepare
  that did not request an agent start.

## Out of scope

- Retrying failures after the agent started a turn, or prompts that failed.
- Changing manual recovery actions, their labels, or the failure UI.
- Configurable retry counts or delays.
