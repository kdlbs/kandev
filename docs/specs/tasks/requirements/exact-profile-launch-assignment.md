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

## Out of scope

- Selecting or editing the task's exact profile assignment.
- Agent-profile policy and model selection within the assigned profile.
