---
status: active
system: ui
created: 2026-11-16
owners:
  - kandev
---

# Plugin Grouping in Saved Sidebar Views

## Overview

UI owns personal sidebar organization and its saved-view lifecycle. Plugins own
classification values; Tasks owns authorized task membership. This extends the
[workspace sidebar view contract](workspace-sidebar-task-views.md), not the
completed plugin layout-panel feature. The user selected complete saved-view
grouping rather than grouping only a loaded page.

## Requirements

### REQ-UI-SIDEBAR-PLUGIN-GROUPING-001: Complete plugin-backed saved sidebar views

**Intent:** Select Tag directly from the left task-list view selector and use it
as a normal saved view, including across reloads and workspace switches.

#### Acceptance criteria

- **AC-UI-SIDEBAR-PLUGIN-GROUPING-001.1:** When an active plugin provides a compatible complete grouping, the sidebar task-list view selector shall offer its localized grouping label, including Tag. Selecting it shall create and select a normal saved view, or reuse its existing unmodified generated view, without duplicating views on repeated selection. Existing saved/custom views shall remain available. Unsaved drafts and the per-workspace view limit shall retain their existing protections.
- **AC-UI-SIDEBAR-PLUGIN-GROUPING-001.2:** Saved plugin grouping views shall preserve their selected grouping, filters, sorting, disclosure and task-row presentation through save, rename, duplicate, reload and workspace switching. Changes shall remain personal and workspace-scoped. Grouping shall not modify task state, relationships, tags or another workspace's preferences.
- **AC-UI-SIDEBAR-PLUGIN-GROUPING-001.3:** Grouping shall cover the complete authorized filtered task set before the existing 100-row page boundary. Tagged root tasks shall appear in each matching colored, labeled group, and roots without values shall appear in a host-owned, localized Unassigned group. Descendants shall retain existing tree context beneath their root in each occurrence. Group headings shall not consume task-row allowance. Counts shall distinguish unique tasks from repeated display occurrences; groups and continuation context shall remain correct across pages and collapse changes.
- **AC-UI-SIDEBAR-PLUGIN-GROUPING-001.4:** Tag creation, rename, recolor, application and removal shall update headings and memberships without reload. Registration changes, plugin replacement, account/workspace switching and delayed responses shall not display another context's groups. Built-in views and the existing page-local /tasks Sort/Group capability shall remain unchanged.
- **AC-UI-SIDEBAR-PLUGIN-GROUPING-001.5:** Missing, disabled, failed, incompatible or incomplete providers shall not erase saved preferences or classify every task as Unassigned. An unavailable view shall identify the problem and offer retry or another view. Eligible retained results may remain marked stale within the same authorized context; access denial shall clear them. Recovery shall restore the saved grouping without rewriting it.
- **AC-UI-SIDEBAR-PLUGIN-GROUPING-001.6:** Desktop and phone Task views/task-switcher navigation shall offer the same saved grouping. Keyboard selection, dismissal and focus return shall work. Phone controls shall be touch reachable, menus and lists shall scroll internally within safe areas, and the document shall not overflow horizontally.

## Compatibility and exclusions

Only providers with a complete server-resolvable grouping participate. A plugin
that supplies only a page-local facet remains usable on /tasks, but is not
advertised as a complete sidebar grouping. No Tags-specific host allowlist,
plugin-state database inspection, browser-side full-task scan, automatic page
traversal, provider publication or deployment is included. Legacy private Tags
storage is not silently mixed into the workspace-shared grouping.

Existing [sidebar pagination](sidebar-task-pagination.md) and workspace access
contracts remain authoritative. For repeated grouped trees, pagination counts
display occurrences while unique-task totals remain unique.

## Implementation plans

- [Sidebar plugin grouping](../../../plans/sidebar-plugin-grouping/plan.md)
