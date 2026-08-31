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
workflow step. A step's agent profile and its conversation-entry behavior are
one routing decision: the destination step chooses which agent runs and whether
that profile continues an eligible conversation or starts a new one.

Workflow authors need to configure that decision per step. One workflow can
therefore preserve context when work returns to a planning step and require a
fresh conversation when it enters an independent review step.

## Terminology

- **Profile switch:** A workflow transition whose destination resolves to a
  different agent profile from the active task session.
- **Destination-step policy:** The session behavior stored on the workflow step
  being entered. It applies only when entry changes the effective profile.
- **Parked session:** A nonterminal session whose runtime is stopped and whose
  conversation can be resumed or answered later.
- **Profile re-entry:** A profile switch to an agent profile that has already
  owned a session on the task.

## Requirements

### REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001: Configurable step profile-session lifecycle

**Intent:** Let a workflow author configure conversation continuity with the
agent profile on each step, without changing the safe behavior of existing
workflow definitions.

**User story:** As a workflow author, I want each step to choose its agent and
session-entry behavior together. Repeated stages must use the conversation
boundaries that I intend.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.1:** When a destination step has no
  profile session policy or selects **Complete previous session**, Kandev shall
  mark the session switched away from as completed. Entry shall reuse the newest
  independently eligible nonterminal destination-profile session when one
  exists; otherwise, it shall create a new session.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.2:** When a destination step selects
  **Keep and reuse previous session**, Kandev shall stop the runtime switched
  away from while keeping its session nonterminal and answerable. Entry shall
  reuse the newest eligible session for the destination profile and preserve
  its conversation and provider resume identity.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.3:** When a destination step selects
  **Keep previous session and start new**, Kandev shall stop the runtime switched
  away from while keeping its session nonterminal and answerable. Entry shall
  create a new destination-profile session with a fresh provider conversation.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.4:** When consecutive workflow steps
  resolve to the active session's profile, Kandev shall keep that active session
  regardless of the destination step's profile session policy.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.5:** When Kandev stops a runtime as
  part of a parked profile switch, the resulting completion or stopped event
  shall not re-run transition actions for the destination workflow step.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.6:** When a workflow step is saved,
  reloaded, exported, imported, or synchronized, its profile session policy
  shall round-trip. Existing, omitted, and invalid values shall resolve to
  **Complete previous session**.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.7:** When an author edits a mutable
  workflow on desktop or mobile, the step's agent selector shall provide one
  configuration surface for selecting the agent profile and its session
  behavior. It shall support profile search, show the current profile and
  behavior in the closed control, and explain retention, reuse, and
  fresh-conversation behavior before selection.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.8:** When a synchronized workflow is
  read-only, Kandev shall display each step's selected agent profile and session
  behavior without allowing either value to be changed.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.9:** When Kandev cannot safely
  prepare the destination session or record a parked switch, it shall stop the
  switch. It shall not use the wrong profile or complete a session that the
  destination step requested to retain. The current session shall remain
  recoverable, and Kandev shall show the failure.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.10:** Changing one step's profile
  session policy shall not change any other step's behavior. A single workflow
  may use different policies on steps that resolve to the same agent profile.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.11:** On a phone viewport, the
  combined selector shall use a touch-sized trigger and an inset bottom drawer
  with one internal scroll region, safe-area spacing, keyboard-safe navigation,
  and no document-level horizontal overflow.

## Out of scope

- A workflow-wide profile session policy or workflow-wide override.
- Selecting session behavior on a transition edge instead of the destination
  step.
- Reusing a session whose agent profile differs from the destination profile.
- Keeping an inactive agent process or executor backend running.
- Reviving a terminal `COMPLETED`, `FAILED`, or `CANCELLED` session.
- Automatically deleting accumulated parked or historical sessions.
- Changing Office agent-session ownership or automation thread policies.
- Moving conditional original-session model configuration into this selector.
