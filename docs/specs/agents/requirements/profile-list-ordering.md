---
status: draft
system: agents
created: 2026-10-03
owners:
  - kandev
---

# Agent Profile List Ordering Requirements

## Overview

An administrator who keeps many profiles under one agent on Settings > Agents
needs to arrange them by hand instead of relying on the newest-first order. The
agent system owns this behavior because profile identity and profile catalog
order belong to agent profiles. The Settings page and Settings navigation tree
only present that order.

## Terminology

- **Profile list:** The rows of one agent's global profiles on Settings > Agents.
  Workspace-scoped (Office) profiles and the Dynamic agent's profiles are not
  part of it.
- **Profile order:** The saved, install-wide sequence of one agent's profile
  list.

## Requirements

### REQ-AGENTS-PROFILE-LIST-ORDERING-001: Manual reorder

**Intent:** Let an administrator place profiles where they want them.

**User story:** As an administrator, I want to drag a profile row to a new
position within its agent, so that the profiles I use most are easiest to find.

#### Acceptance criteria

- **AC-AGENTS-PROFILE-LIST-ORDERING-001.1:** When an administrator drags a
  profile row to another position in the same agent's profile list, the page
  shall show the new order immediately and shall save it.
- **AC-AGENTS-PROFILE-LIST-ORDERING-001.2:** Each draggable row shall expose a
  drag handle with a translated accessible name that includes the profile name.
  The handle shall work with mouse, touch, and keyboard, and a keyboard user
  shall be able to lift, move, and drop a row without a pointer.
- **AC-AGENTS-PROFILE-LIST-ORDERING-001.3:** A profile row shall not move to a
  different agent.
- **AC-AGENTS-PROFILE-LIST-ORDERING-001.4:** A user without permission to manage
  agent configuration shall see the rows in the saved order with no drag handle.
- **AC-AGENTS-PROFILE-LIST-ORDERING-001.5:** When the latest pending save fails
  and no newer reorder is queued, the page shall restore the previously saved
  order and show an error message. A failed earlier save shall not roll back a
  newer queued reorder.
- **AC-AGENTS-PROFILE-LIST-ORDERING-001.6:** Opening a profile, duplicating it,
  deleting it, and the row's action menu shall keep working, and a click on a
  drag handle shall not open the profile.
- **AC-AGENTS-PROFILE-LIST-ORDERING-001.7:** An agent with fewer than two
  profiles shall not show a drag handle.
- **AC-AGENTS-PROFILE-LIST-ORDERING-001.8:** When an administrator drags again
  before an earlier save of the same agent has finished, the page shall keep
  showing the latest drag, and the saved order shall end as the latest drag. The
  list shall not show an earlier order in between.
- **AC-AGENTS-PROFILE-LIST-ORDERING-001.9:** The Installed agents section shall
  not offer an automatic profile-sorting action. Profile-order changes shall be
  initiated through a row's drag handle, which remains operable with mouse,
  touch, and keyboard.

### REQ-AGENTS-PROFILE-LIST-ORDERING-003: Shared and durable order

**Intent:** Keep one order for the install across clients and restarts.

#### Acceptance criteria

- **AC-AGENTS-PROFILE-LIST-ORDERING-003.1:** The profile order shall persist
  across reload and backend restart, and shall be the same for every user and
  browser of the install.
- **AC-AGENTS-PROFILE-LIST-ORDERING-003.2:** When another client changes a
  profile order, an open Settings > Agents page and the Settings navigation
  tree shall show the new order without a reload.
- **AC-AGENTS-PROFILE-LIST-ORDERING-003.3:** The Settings navigation tree shall
  list an agent's profiles in the same order as the page.
- **AC-AGENTS-PROFILE-LIST-ORDERING-003.4:** An install with no saved order
  shall list each agent's profiles newest first.
- **AC-AGENTS-PROFILE-LIST-ORDERING-003.5:** A profile created or duplicated
  after a manual order exists shall appear first in its agent's list, without a
  reload, on the client that created it and on other open clients, and shall
  stay first after a reload.
- **AC-AGENTS-PROFILE-LIST-ORDERING-003.6:** A save that does not name exactly
  the agent's current global profiles (a profile was created or deleted by
  another client, or an ID is foreign or repeated) shall be rejected without
  changing the saved order. Concurrent profile membership changes and reorder
  saves shall serialize: the save either commits against the exact membership it
  validated, or rejects a set changed before it acquired the serialization lock.
- **AC-AGENTS-PROFILE-LIST-ORDERING-003.7:** When a save is rejected for that
  reason, the page shall show the current saved order of every agent.
- **AC-AGENTS-PROFILE-LIST-ORDERING-003.8:** A caller without permission to
  manage agent configuration shall not be able to save a profile order.
- **AC-AGENTS-PROFILE-LIST-ORDERING-003.9:** Reordering shall not change any
  profile's configuration, enabled state, or `updated_at`.
- **AC-AGENTS-PROFILE-LIST-ORDERING-003.10:** A save for the Dynamic agent shall
  be rejected without changing any order, and no save shall change the order or
  content of a workspace-scoped or deleted profile.
- **AC-AGENTS-PROFILE-LIST-ORDERING-003.11:** When a page load, a cached agent
  list, or an event carries a profile order older than the order the client
  already shows for that agent, the client shall keep the newer order.
- **AC-AGENTS-PROFILE-LIST-ORDERING-003.12:** Each `GET /agents` and
  `GET /agents/:id` response shall pair profile rows/order and
  `profile_order_revision` from one consistent database snapshot. A concurrent
  reorder may produce the preceding order with its preceding revision or the
  committed order with its committed revision, never a mixed pair.
- **AC-AGENTS-PROFILE-LIST-ORDERING-003.13:** When the client applies a profile
  create or delete event, a full agent-list snapshot captured earlier or an
  existing-agent save response still in flight shall not replace the newer
  membership state. A created profile remains visible first and a deleted
  profile remains absent until a full snapshot captured after the event is
  applied; equal `profile_order_revision` values do not establish membership
  freshness.
- **AC-AGENTS-PROFILE-LIST-ORDERING-003.14:** Profile selectors outside
  Settings > Agents shall retain their existing option ordering, recency, and
  default-selection behavior. The saved Settings profile order shall not change
  selector order or selection.

## Out of scope

- Changing the backend-controlled display order of agent cards.
- The Dynamic agent's profiles, workspace-scoped (Office) profiles, and the
  Office agents list.
- Profile selector ordering, recency, and default selection outside Settings >
  Agents. These retain their existing behavior; the saved order only controls
  the Settings profile list and its navigation tree.
- Automatic profile sorting by name, agent label, or any other key; persistent
  default-profile selection; and per-user profile orders.
