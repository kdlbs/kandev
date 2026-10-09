# ADR-2026-09-15-phone-navigation-entry-points: Phone navigation entry-point ownership

**Status:** accepted
**Date:** 2026-09-15
**Area:** frontend

## Context

The same phone hamburger opens app navigation plus display controls on Home,
but task switching on the task workbench. Other page shells also differ in
presentation. The user chose a shared menu with a task-title switcher after
considering a combined menu and global bottom navigation.

## Decision

Phone hamburgers open app navigation through a shared navigation-owned shell.
The task title opens the existing task picker. Page display controls remain
local to the listing. Existing navigation manifests, workspace gates, task
selection controllers, and saved-view owners retain authority. The shared
phone menu uses the established inset bottom-drawer pattern.

## Consequences

Future phone pages reuse the shared navigation entry instead of assigning
page-specific meanings to its hamburger. Task switching remains two taps.
Existing task actions stay in task rows, and the workbench bottom navigation
continues to select content panels. A compact pinned-task section offers
cross-page shortcuts without changing task ordering or creating recency state.
The first iteration was implemented and user-tested.

## Alternatives Considered

- A Navigate/Tasks tabbed menu centralizes controls but adds an intermediate
  choice before switching tasks.
- Global bottom navigation improves destination visibility but competes with
  the workbench's panel navigation and consumes phone content height.
- Making every hamburger open the full task list weakens access to non-task
  destinations and mixes task filtering with app navigation.

## References

- [Requirements](../specs/ui/requirements/unified-mobile-navigation.md)
- [Design](../specs/ui/system-design/unified-mobile-navigation.md)
- [Plan](../plans/unified-mobile-navigation/plan.md)

## Revision after user testing (2026-09-17)

The shared hamburger and task-title picker remain. The user rejected the separate
pinned shortcuts, Task views action, and View options button. The revised menu
embeds the existing Tasks sidebar in a collapsible middle section. The top-level
Kanban/Threads/List dropdown owns view options, including Threads saved views.
Existing sidebar pin preferences are retained within the task sidebar. This
supersedes the compact-shortcut composition above; implementation is tracked by
Task 03 in the linked package.
