---
status: active
system: agents
created: 2026-11-03
owners:
  - kandev
---

# Profile Delete Conflict Workspaces Requirements

## Overview

Deleting an agent profile that still has open tasks or quick chats shows a
conflict dialog: "This profile is currently in use. Deleting it will affect the
following:". The dialog lists the affected task titles but not where they
live. A user with several workspaces cannot tell which workspace to open to
find, finish, or reassign those tasks before deleting the profile.

The agents system owns this contract because it owns agent profile lifecycle,
including the delete-conflict inventory returned when a profile is in use. The
task system only supplies task facts to that inventory.

## Terminology

- **Delete conflict dialog:** The dialog shown when a profile deletion request
  reports that the profile is still in use.
- **Affected item:** A task or quick chat listed by the delete conflict dialog
  because one of its sessions uses the profile.
- **Workspace group:** A sub-heading in the delete conflict dialog that names
  one workspace and lists the affected items that belong to it.

## Requirements

### REQ-AGENTS-PROFILE-DELETE-WORKSPACES-001: Locate affected items by workspace

**Intent:** Let the user find every task and quick chat that blocks a profile
deletion without searching each workspace.

**User story:** As a user deleting an agent profile, I want the in-use warning
to group affected tasks by workspace, so that I can open the right workspace
and deal with them.

#### Acceptance criteria

- **AC-AGENTS-PROFILE-DELETE-WORKSPACES-001.1:** When the delete conflict
  dialog lists tasks, the system shall group them under the existing Tasks
  section into workspace groups, each headed by the name of the workspace that
  contains its tasks, and each task shall appear in exactly one group.
- **AC-AGENTS-PROFILE-DELETE-WORKSPACES-001.2:** When the delete conflict
  dialog lists quick chats, the system shall group them under the existing
  Quick Chats section into workspace groups in the same way.
- **AC-AGENTS-PROFILE-DELETE-WORKSPACES-001.3:** The system shall order
  workspace groups by workspace name (case-insensitive, then workspace ID) and
  items within a group by title (case-insensitive, then task ID), so repeated
  attempts show the same order.
- **AC-AGENTS-PROFILE-DELETE-WORKSPACES-001.4:** When an affected item's
  workspace name is unavailable, the system shall place it in a group headed by
  a localized "Unknown workspace" label instead of hiding the item or showing
  an empty heading.
- **AC-AGENTS-PROFILE-DELETE-WORKSPACES-001.5:** The workspace headings shall
  be localized in every shipped locale, render on desktop and phone layouts,
  and wrap long workspace names inside the dialog's existing scrollable
  conflict body without moving the Cancel or Delete anyway actions out of view.
- **AC-AGENTS-PROFILE-DELETE-WORKSPACES-001.6:** Grouping shall not change
  which items are listed, whether deletion is blocked, or what Delete anyway
  does.

## Out of scope

- Links from affected items to their task pages.
- Workspace labels for watcher, automation, routing tier, utility agent, or
  dynamic profile conflict sections; automation and routing tier rows already
  name their workspace.
- Office workspace agent deletion flows.
- Changing which sessions count as using a profile.
