---
status: active
system: tasks
created: 2026-09-25
owners:
  - kandev
---

# Exact Profile Assignment at Launch

## Overview

Tasks can carry an exact agent-profile assignment across workflow session
changes. The task system owns the selected session and must keep that session's
identity aligned with the assignment before launching it.

## Requirements

### REQ-TASKS-EXACT-PROFILE-LAUNCH-001: Preserve assigned profile identity

**Intent:** Ensure a launch governed by a task's exact profile assignment uses
the assigned concrete profile even when workflow processing redirects the
launch to another active session.

#### Acceptance criteria

- **AC-TASKS-EXACT-PROFILE-LAUNCH-001.1:** When an exact-profile launch is
  redirected to another active task session, the replacement session shall
  match the active assignment's profile before its exact generation and
  revision are bound or the agent is launched. If that session update cannot
  be persisted safely, the launch shall stop without launching the mismatched
  profile.
- **AC-TASKS-EXACT-PROFILE-LAUNCH-001.2:** Assignment shall capture a
  concrete, enabled profile in the task's workspace with a persistent change
  token. Assignment shall reject a profile changed during admission. Every
  governed launch or resume shall reject a disabled, deleted, moved, or changed
  profile instead of selecting a fallback profile.
- **AC-TASKS-EXACT-PROFILE-LAUNCH-001.3:** Session creation, deferred workflow
  moves, prompts, and resumes shall carry the assignment generation and change
  token through the selected session. A stale generation, mismatched profile,
  or conflicting model request shall stop before inference begins. Legacy
  assignments without a verifiable change token shall fail closed.
- **AC-TASKS-EXACT-PROFILE-LAUNCH-001.4:** A governed launch shall record a
  durable receipt identifying its assignment, session, model, and outcome.

## Out of scope

- Agent-profile policy and model selection within the assigned profile.
