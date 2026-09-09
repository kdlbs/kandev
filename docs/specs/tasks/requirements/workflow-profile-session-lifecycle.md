---
status: draft
system: tasks
created: 2026-08-31
owners:
  - kandev
---

# Workflow Step Profile Session Lifecycle Requirements

## Overview

The task system selects the task session that executes each workflow step.
It owns this contract because a step's recipient determines task-session ownership.
Each step chooses three related settings:

- The initial agent, an earlier step's agent session, or an agent profile.
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
with a conversation-preserving default and explicit completion when required.

**User story:** As a workflow author, I want each step to define how its session
starts and ends, so repeated stages use the intended context.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.1:** When a profile-only destination step selects
  **Reuse an available session**, Kandev shall reuse the newest eligible
  nonterminal session for that profile. When no session is available, Kandev
  shall create a new session. Explicit targets use requirement 002's exact
  conversation selection rules.
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
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.5:** For existing profile-only steps,
  consecutive steps with the active session's profile shall keep that session
  when the destination uses the default or **Reuse an available session** start
  behavior. A profile-only destination with **Start a new session** shall
  always create a fresh conversation, even when its profile matches the active
  session. Explicit initial-session and earlier-step targets follow requirement
  002, including fresh conversations with the same profile.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.6:** When Kandev parks a session
  during a profile switch, its completion or stopped event shall not repeat
  transition actions for the destination step.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.7:** When a workflow step is saved,
  reloaded, exported, imported, or synchronized, its start and end settings
  shall round-trip. Missing or invalid values shall use **Reuse an available
  session** and **Park the session**. An explicitly saved **Complete the
  session** value shall remain selected and shall retain its completion
  behavior.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.8:** When an author edits a mutable
  workflow, one step selector shall show the recipient and a **Session
  lifecycle** setting above the searchable choices. That setting shall remain
  visible while the choices scroll or search has no results. It shall present separate **When this
  step starts** and **When this step ends** choices with visible explanations.
  For a new or unset step, **Park the session** shall be selected by default on
  desktop and phone surfaces.
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

### REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-002: Explicit workflow session recipients

**Intent:** Return a later step's prompt to the intended conversation without
requiring its profile to be known when the workflow is authored.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.1:** Every step shall offer
  **Initial agent session**. It shall refer to the task's original agent,
  resolved when the task obtains its first session, regardless of later
  primary-session or workflow-default changes.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.2:** The selector shall offer each
  earlier step with an explicit profile override, labelled by step and profile.
  Two steps using the same profile shall remain distinct choices. Workflow
  editing shall not imply that these sessions already exist. Steps that inherit
  the workflow default are not source-step choices. **Initial agent session**
  reaches the original conversation, not every step that uses the default.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.3:** With **Initial agent session**
  and **Reuse an available session**, a parked original conversation shall
  receive the step prompt. An unrelated newer session with the same profile
  shall not replace it.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.4:** With an earlier-step target and
  **Reuse an available session**, the latest successfully selected conversation
  for that source step shall receive the prompt if it remains eligible.
  Re-entering the source step shall update that choice for later steps.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.5:** With either explicit target and
  **Start a new session**, Kandev shall create a fresh conversation using that
  target's logical profile, even when the active session uses the same profile.
  It shall keep the identity of the original task conversation unchanged.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.6:** When the referenced conversation
  is absent or terminal but its profile is known, **Reuse an available session**
  shall create a fresh conversation for that profile. It shall not revive a
  terminal conversation or select an unrelated conversation. The selector shall
  explain this fallback and the need to park a conversation for later reuse.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.7:** When a step changes sessions,
  its predecessor's end setting shall govern the departing session. Reusing
  the already-active target shall keep it active. Each successful entry shall
  deliver the step prompt once to the selected session and show it as primary.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.8:** Saving, reloading, duplicating,
  exporting, importing, and synchronizing a workflow shall preserve target
  meaning. Runtime bindings shall survive restart but shall not be copied to
  another task. Existing workflows without explicit targets shall retain their
  profile resolution and lifecycle behavior.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.9:** An invalid, foreign, removed,
  or no-longer-earlier source step shall produce a visible configuration error.
  Unknown target kinds or conflicting routing choices shall be rejected.
  Runtime resolution failures shall stop entry without sending to another agent.
  The editor shall identify each affected step and let the author choose another
  valid target, clear the target, or undo the source change before saving.
  It shall not silently retarget a step when its source is moved or removed.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.10:** If a task skips the referenced
  step, Kandev shall create a conversation from that step's configured profile.
  It shall not execute the skipped step's prompt or actions. If the source
  profile changes, later entries shall use its new profile with a fresh
  conversation instead of reusing an incompatible earlier binding.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.11:** Desktop and phone selectors
  shall expose the same targets, lifecycle controls, explanations, and saved
  state. A phone shall use the existing inset drawer pattern, touch targets of
  at least 44 px, one scrolling list, and no document horizontal overflow.
  Synced workflows shall expose these values for inspection without editing.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.12:** Conditional **Override original
  session options** shall remain a separate behavior. The editor and API shall
  prevent combining it with an explicit session target. Choosing a target shall
  not silently discard existing conditional rules.

## Example

A task starts with Sol, which the user selects at task creation. Plan parks
its session on exit. Implement selects Luna. Review selects **Initial agent
session** and **Reuse an available session**. The Review prompt reaches the
original Sol conversation. Selecting **Start a new session** on Review instead
creates a fresh Sol conversation. Selecting **Implement · Luna** returns to
Implement's eligible conversation, subject to Implement's end setting.

## Out of scope

- A workflow-wide lifecycle setting or workflow-wide override.
- Lifecycle settings on transition edges.
- Reusing a session from a different agent profile.
- Keeping an inactive agent process or executor backend running.
- Reviving a terminal `COMPLETED`, `FAILED`, or `CANCELLED` session.
- Automatically removing parked or historical sessions.
- Changing Office agent-session ownership or automation thread policies.
- Moving conditional original-session model settings into this selector.
- Choosing arbitrary session tabs while editing a reusable workflow.
- Referencing later steps, indirect target chains, or concurrent recipient fan-out.
