---
status: active
system: ui
created: 2026-09-15
owners:
  - kandev
---

# Workspace Sidebar Task Views

## Overview

Sidebar task views are personal presentation preferences owned by UI. Each
workspace needs an independent collection so filters and organization used in
one workspace do not change another. Workspace identity and access remain owned
by the workspace system. This extends [view creation](sidebar-view-creation.md)
with workspace scope; its existing creation and editing interactions remain.

## Requirements

### REQ-UI-WORKSPACE-SIDEBAR-VIEWS-001: Personal workspace-specific task views

**Intent:** Organize each workspace independently while retaining saved views.

#### Acceptance criteria

- **AC-UI-WORKSPACE-SIDEBAR-VIEWS-001.1:** The user shall see only the active workspace's saved task views. Creating, renaming, duplicating, deleting, saving, or reordering a view shall affect only that user's current workspace. The existing 50-view limit and automatic name allocation shall apply independently in each workspace.
- **AC-UI-WORKSPACE-SIDEBAR-VIEWS-001.2:** Switching between workspaces shall restore each workspace's active view, draft filters/sort/grouping/task-row presentation, and saved collapsed groups. A workspace switch shall not save or discard another workspace's draft. Reload and another signed-in client shall restore successfully persisted state.
- **AC-UI-WORKSPACE-SIDEBAR-VIEWS-001.3:** During an account's one-time migration, all then-existing accessible workspaces shall receive independent copies of its legacy global views, selection, and valid draft. Existing scoped data shall take precedence. Migration shall preserve names, IDs, ordering, filters, presentation, and collapsed groups without remapping workspace-specific filter values. Retrying or restarting shall not duplicate or overwrite migrated views.
- **AC-UI-WORKSPACE-SIDEBAR-VIEWS-001.4:** Workspaces created after that account's migration and accounts without legacy views shall start with the canonical All tasks view, matching active selection, and no draft. Missing active references shall fall back to the first eligible view; invalid draft references shall be cleared. An empty saved collection shall resolve to the canonical default. No other workspace shall be used as a fallback.
- **AC-UI-WORKSPACE-SIDEBAR-VIEWS-001.5:** A delayed save, failure, retry, or live settings update for workspace A shall never replace workspace B's views, draft, or selection. Concurrent edits in different workspaces shall both survive. With no valid accessible workspace, view mutations shall be unavailable and views from a previously selected workspace shall not remain visible.
- **AC-UI-WORKSPACE-SIDEBAR-VIEWS-001.6:** Desktop sidebar and phone Task views/task-switcher drawer shall offer the same workspace-scoped behavior through their existing controls. A workspace switch shall dismiss an editor or deletion confirmation bound to the previous workspace and prevent its stale callback from changing the new workspace. Existing keyboard dismissal, focus return, touch reachability, and contained scrolling shall remain functional.

## Migration decision

The user selected copying existing views to existing workspaces, then separating
future edits. Existing workspaces means the accessible workspace snapshot at the
account's first successful migration. A copied filter referring to another
workspace's repository or workflow can produce an empty result; migration must
not silently broaden or rewrite it.

## Out of scope

Shared team views, copying views between workspaces on demand, Threads views,
integration saved queries, global automatic task colors, per-task colors or
manual task ordering, and sidebar visual redesign.

## Implementation plans

- [Workspace task views](../../../plans/workspace-sidebar-task-views/plan.md)
