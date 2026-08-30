---
status: draft
system: agents
created: 2026-08-27
owners:
  - kandev
---

# Agent Profile Recent Use Requirements

## Overview

Agent profile lists can contain many provider and model combinations. Users
need the profiles they successfully used in a given operational flow to remain
easy to reach without changing which profile the flow selects by default. The
agent system owns this behavior because the remembered identity and eligibility
rules belong to agent profiles, while task, quick-chat, and configuration-chat
surfaces consume the same contract.

## Terminology

- **Recent use:** A successful operation that starts work with an effective
  agent profile.
- **Context:** One of `task_create`, `task_session`, `quick_chat`, or
  `config_chat`.
- **Source order:** The stable order supplied by the eligible profile catalog
  before recent-use or current-selection ordering is applied.

## Requirements

### REQ-AGENTS-PROFILE-RECENT-USE-001: Contextual recent-use order

**Intent:** Keep frequently used profiles near the top of the operational
selector where they are useful without changing defaults or unrelated profile
configuration surfaces.

**User story:** As a user, I want each operational agent selector to remember
the profiles I used there, so that repeated choices require less scanning.

#### Acceptance criteria

- **AC-AGENTS-PROFILE-RECENT-USE-001.1:** When an operation successfully starts
  with an agent profile, the system shall put that effective profile first in
  the history for the operation's context and keep the other remembered
  profiles in their prior relative order.
- **AC-AGENTS-PROFILE-RECENT-USE-001.2:** The `task_create` context shall apply
  to standard task creation and subtask creation; `task_session` shall apply to
  add-agent, new-session, and handoff launches; `quick_chat` and `config_chat`
  shall apply only to their respective chat launchers.
- **AC-AGENTS-PROFILE-RECENT-USE-001.3:** When a selector opens, the system
  shall order eligible remembered profiles before eligible unseen profiles and
  preserve source order among unseen profiles.
- **AC-AGENTS-PROFILE-RECENT-USE-001.4:** A selected profile shall remain the
  first displayed option, and recent-use ordering shall not change any flow's
  default-selection, compatibility, availability, disabled-profile, or search
  rules.
- **AC-AGENTS-PROFILE-RECENT-USE-001.5:** Agent selectors that configure
  workspace, workflow, automation, agent settings, or Office assignments shall
  retain their authoritative source order and shall not record operational
  recent use.

### REQ-AGENTS-PROFILE-RECENT-USE-002: Successful-use semantics

**Intent:** Treat recency as evidence of completed use rather than transient
picker interaction.

#### Acceptance criteria

- **AC-AGENTS-PROFILE-RECENT-USE-002.1:** Changing a selection, closing a
  selector, cancelling a dialog, or receiving a launch failure shall not update
  recent use.
- **AC-AGENTS-PROFILE-RECENT-USE-002.2:** When the backend reports an effective
  profile that differs from the submitted profile, the system shall record the
  effective profile.
- **AC-AGENTS-PROFILE-RECENT-USE-002.3:** When a quick-chat or configuration-chat
  launch is superseded and its task is discarded, the system shall not record
  that launch as recent use.
- **AC-AGENTS-PROFILE-RECENT-USE-002.4:** A failure to save recent use after a
  successful launch shall not fail, undo, or delay the launched operation.
- **AC-AGENTS-PROFILE-RECENT-USE-002.5:** A programmatic launch that supplies
  an agent profile without an explicit selector-backed attribution shall not
  update operational recent use.

### REQ-AGENTS-PROFILE-RECENT-USE-003: Portable bounded history

**Intent:** Keep ordering consistent across clients without allowing preference
storage or synchronization costs to grow with normal product use.

#### Acceptance criteria

- **AC-AGENTS-PROFILE-RECENT-USE-003.1:** Recent-use order shall be scoped to
  the authenticated user and shall be available after reload and on another
  connected client for that user.
- **AC-AGENTS-PROFILE-RECENT-USE-003.2:** The system shall retain at most ten
  distinct profile IDs for each supported context and at most four context
  histories for each user.
- **AC-AGENTS-PROFILE-RECENT-USE-003.3:** Missing, deleted, disabled,
  incompatible, or otherwise ineligible remembered IDs shall not appear in a
  selector and shall not prevent eligible profiles from appearing.
- **AC-AGENTS-PROFILE-RECENT-USE-003.4:** Reusing the profile already first in a
  context shall not create a durable write or synchronization event.
- **AC-AGENTS-PROFILE-RECENT-USE-003.5:** Desktop and mobile presentations of
  the same operational selector shall use the same contextual order.

## Out of scope

- Changing default agent-profile selection or introducing recommendations.
- Workspace-, repository-, workflow-, task-, or device-specific histories.
- User-managed pinning, manual ordering, history clearing, or recency UI.
- Reordering non-operational configuration selectors.
