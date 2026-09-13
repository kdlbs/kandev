---
status: active
system: ui
created: 2026-09-11
owners:
  - Kandev
---
# Mobile task-view access requirements

## Overview

The shared phone navigation should expose the task-sidebar views available from desktop page chrome. UI owns this reusable navigation contract; task filtering and persistence retain their existing owners.

## Requirements

### REQ-UI-MOBILE-TASK-VIEWS-001: Shared phone task-view access

**Intent:** Reach existing task views without first opening a task.

#### Acceptance criteria

- **AC-UI-MOBILE-TASK-VIEWS-001.1:** Shared phone navigation in a Kanban workspace shall expose Task views. Selecting it shall close navigation and open the existing mobile task-view surface with its saved views and editor. Task dialogs opened from that surface shall retain their drafts across phone orientation changes.
- **AC-UI-MOBILE-TASK-VIEWS-001.2:** Choosing a task shall navigate to that task and preserve browser Back to the originating page. Dismissal shall return focus to a visible navigation opener. GitHub queries, Threads views, and sidebar task views shall remain separate collections.

## Out of scope

Office navigation changes, new task-view storage, and a global navigation redesign.

## Implementation plans

- [GitHub mobile parity](../../../plans/github-mobile-parity/plan.md)
