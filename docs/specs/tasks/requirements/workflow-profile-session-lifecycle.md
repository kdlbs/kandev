---
status: draft
system: tasks
created: 2026-08-31
owners:
  - kandev
---

# Workflow Step Profile Session Lifecycle Requirements

## Overview

The task system selects the task session that executes each fixed-profile
workflow step. Each step chooses three related settings:

- The agent profile that executes the step.
- How the step obtains a session when it starts after a profile switch.
- What happens to its session when the workflow leaves it for another profile.

These settings let one workflow continue context for repeated work and use a
fresh conversation for independent work.

## Terminology

- **Profile switch:** A workflow transition that changes the effective agent
  profile.
- **Start behavior:** The destination step chooses whether to reuse an available
  session or start a new session.
- **End behavior:** The source step chooses whether to complete or park its
  session.
- **Parked session:** A nonterminal session whose runtime is stopped. Its
  conversation remains available.
- **Profile re-entry:** A profile switch to an agent profile that already has a
  session on the task.

## Requirements

### REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001: Configurable step profile-session lifecycle

**Intent:** Let a workflow author configure each step's conversation boundary
without changing existing workflow behavior.

**User story:** As a workflow author, I want each step to define how its session
starts and ends, so repeated stages use the intended context.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.1:** When a destination step selects
  **Reuse an available session**, Kandev shall reuse the newest eligible
  nonterminal session for that profile. When no session is available, Kandev
  shall create a new session.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.2:** When a destination step selects
  **Start a new session**, Kandev shall create a fresh conversation. It shall not
  reuse another session for that profile.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.3:** When a source step selects
  **Complete the session**, Kandev shall complete its session during a profile
  switch. Kandev shall not reuse that completed session later.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.4:** When a source step selects
  **Park the session**, Kandev shall stop its runtime and keep the session
  nonterminal. The conversation shall remain available for reuse or manual
  follow-up.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.5:** When consecutive workflow steps
  resolve to the active session's profile, Kandev shall keep the active session.
  The start and end settings shall have no effect.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.6:** When Kandev parks a session
  during a profile switch, its completion or stopped event shall not repeat
  transition actions for the destination step.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.7:** When a workflow step is saved,
  reloaded, exported, imported, or synchronized, its start and end settings
  shall round-trip. Missing or invalid values shall use **Reuse an available
  session** and **Complete the session**.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.8:** When an author edits a mutable
  workflow, one step selector shall show the agent profile and a **Session
  lifecycle** setting. The lifecycle setting shall present separate **When this
  step starts** and **When this step ends** choices with visible explanations.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.9:** When the selector shows an agent
  profile, it shall show the same agent logo used by the new-task profile
  selector. The workflow-default choice shall use the generic agent icon.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.10:** When a synchronized workflow is
  read-only, Kandev shall show the selected profile and both lifecycle settings.
  It shall not allow changes.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.11:** When Kandev cannot prepare the
  destination session or record a parked switch, it shall stop the switch. The
  current session shall remain recoverable, and Kandev shall show the error.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.12:** Changing one step's start or end
  setting shall not change another step's settings. A workflow can use all four
  start-and-end combinations.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.13:** On a phone viewport, the
  selector shall use a touch-sized trigger and an inset bottom drawer. The
  drawer shall have one scroll region, safe-area spacing, keyboard-safe
  navigation, and no document-level horizontal overflow.

## Out of scope

- A workflow-wide lifecycle setting or workflow-wide override.
- Lifecycle settings on transition edges.
- Reusing a session from a different agent profile.
- Keeping an inactive agent process or executor backend running.
- Reviving a terminal `COMPLETED`, `FAILED`, or `CANCELLED` session.
- Automatically removing parked or historical sessions.
- Changing Office agent-session ownership or automation thread policies.
- Moving conditional original-session model settings into this selector.
