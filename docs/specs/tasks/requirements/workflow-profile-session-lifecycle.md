---
status: draft
system: tasks
created: 2026-08-31
owners:
  - kandev
---

# Workflow Profile Session Lifecycle Requirements

## Overview

The task system selects the task session that executes a fixed-profile workflow
step. It also controls the session that previously owned the task. Workflow
authors need to choose whether profile handoffs end or preserve old
conversations. They can preserve a conversation for reuse or only for manual
follow-up.

## Terminology

- **Profile switch:** A workflow transition whose destination resolves to a
  different agent profile from the active task session.
- **Parked session:** A nonterminal session whose runtime is stopped and whose
  conversation can be resumed or answered later.
- **Profile re-entry:** A profile switch to an agent profile that has already
  owned a session on the task.

## Requirements

### REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001: Configurable profile-session lifecycle

**Intent:** Let a workflow author control conversation continuity across agent
profile handoffs without changing the safe behavior of existing workflows.

**User story:** As a workflow author, I want to choose how Kandev retains and
selects profile sessions. Repeated stages must use the conversation boundaries
that I intend.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.1:** When a workflow has no profile
  session policy or selects **Complete previous session**, Kandev shall mark the
  session switched away from as completed. A later re-entry to that profile
  shall reuse the newest independently eligible nonterminal session when one
  exists; otherwise, it shall create a new session.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.2:** When a workflow selects **Keep
  and reuse previous session**, Kandev shall stop the runtime switched away from
  while keeping its session nonterminal and answerable. A later profile re-entry
  shall reuse the newest eligible session for that profile and preserve its
  conversation and provider resume identity.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.3:** When a workflow selects **Keep
  previous session and start new**, Kandev shall stop the runtime switched away
  from while keeping its session nonterminal and answerable. A later profile
  re-entry shall create a new session with a fresh provider conversation.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.4:** When consecutive workflow steps
  resolve to the active session's profile, Kandev shall keep that active session
  regardless of the profile session policy.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.5:** When Kandev stops a runtime as
  part of a parked profile switch, the resulting completion or stopped event
  shall not re-run transition actions for the destination workflow step.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.6:** When a workflow is saved,
  reloaded, exported, imported, or synchronized, its profile session policy
  shall round-trip. Existing and invalid values shall resolve to **Complete
  previous session**.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.7:** When an author edits a mutable
  workflow on desktop or mobile, Kandev shall show the three policy choices with
  visible explanations of retention, reuse, and fresh-conversation behavior.
  The mobile control shall be touch-reachable without horizontal page overflow.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.8:** When a synchronized workflow is
  read-only, Kandev shall display its profile session policy without allowing it
  to be changed.
- **AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.9:** When Kandev cannot safely
  prepare the destination session or record a parked switch, it shall stop the
  switch. It shall not use the wrong profile or complete a session that the
  workflow requested to retain. The current session shall remain recoverable,
  and Kandev shall show the failure.

## Out of scope

- Selecting different policies for individual steps in one workflow.
- Reusing a session whose agent profile differs from the destination profile.
- Keeping an inactive agent process or executor backend running.
- Reviving a terminal `COMPLETED`, `FAILED`, or `CANCELLED` session.
- Automatically deleting accumulated parked or historical sessions.
- Changing Office agent-session ownership or automation thread policies.
