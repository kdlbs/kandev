---
status: active
system: ui
created: 2026-08-27
owners:
  - kandev
---

# Sidebar Repository Grouping Requirements

## Overview

Saved sidebar views can group tasks by repository. The group heading must show the complete repository context for each task.

The UI system owns this presentation contract. The task system supplies the ordered repository links but does not define sidebar groups.

## Terminology

- **Repository slug:** The canonical user-facing repository name from the workspace repository record.
- **Repository combination:** The ordered repository slugs that belong to one task.

## Requirements

### REQ-UI-SIDEBAR-REPOSITORY-GROUPING-001: Complete Repository Grouping

**Intent:** Users can identify the full repository scope of each task when a sidebar view groups tasks by repository.

**User story:** As a user, I want multi-repository tasks in named combination groups, so that I can see their complete scope.

#### Acceptance criteria

- **AC-UI-SIDEBAR-REPOSITORY-GROUPING-001.1:** When a task has one repository, its group heading shall show the canonical repository slug.
- **AC-UI-SIDEBAR-REPOSITORY-GROUPING-001.2:** When a task has multiple repositories, its group heading shall join all canonical slugs with comma-space separators.
- **AC-UI-SIDEBAR-REPOSITORY-GROUPING-001.3:** The heading shall keep the repository attachment order, with the primary repository first.
- **AC-UI-SIDEBAR-REPOSITORY-GROUPING-001.4:** Tasks with the same ordered combination shall share one group. Different combinations shall use different groups.
- **AC-UI-SIDEBAR-REPOSITORY-GROUPING-001.5:** A multi-repository task shall not appear in a group for only its primary repository.
- **AC-UI-SIDEBAR-REPOSITORY-GROUPING-001.6:** Desktop and mobile task sidebars shall show the same group identity and heading.
- **AC-UI-SIDEBAR-REPOSITORY-GROUPING-001.7:** When repository metadata is incomplete, the task shall remain visible in a nonempty multi-repository group. The named group shall replace it after metadata resolves.

## Out of scope

- Changes to repository filter membership.
- Changes to repository attachment order.
- Changes to saved-view persistence or group-collapse persistence.
- Changes to group-header layout, truncation, or touch behavior.
