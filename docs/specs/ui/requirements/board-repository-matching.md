---
status: active
system: ui
created: 2026-10-01
owners:
  - kandev
---

# Board repository matching Requirements

## Overview

Repository selections must retain tasks that span the selected repository even
when it is not their primary repository. This contract belongs to UI because
it defines a reusable board view lens; task and workspace membership ownership
remain with their existing systems.

## Requirements

### REQ-UI-BOARD-REPOSITORY-MATCHING-001: Match complete task repository membership

**Intent:** Users can locate multi-repository work through any linked repository.

#### Acceptance criteria

- **AC-UI-BOARD-REPOSITORY-MATCHING-001.1:** With a nonempty repository selection,
  a task shall pass the repository filter when at least one linked repository
  has a selected ID, including a secondary repository. Multiple selected IDs
  shall use OR semantics and shall not duplicate a task.
- **AC-UI-BOARD-REPOSITORY-MATCHING-001.2:** A supplied repository collection
  shall be authoritative. A scalar primary repository outside that collection
  shall not admit a task. An explicitly empty collection shall match no
  nonempty repository selection; only an absent collection shall fall back to
  the legacy primary repository.
- **AC-UI-BOARD-REPOSITORY-MATCHING-001.3:** With no repository selection, tasks
  shall pass the repository lens regardless of membership, including tasks
  with an empty collection or no repository data.
- **AC-UI-BOARD-REPOSITORY-MATCHING-001.4:** Where board search supports
  repository name or local path, a case-insensitive substring in any linked
  repository's current-workspace metadata shall match. Collection authority
  and legacy fallback shall be the same as for repository selection. Missing
  metadata shall not throw or invent a match; title and description search
  shall continue to compose with the repository selection.
- **AC-UI-BOARD-REPOSITORY-MATCHING-001.5:** Single-workflow boards and
  multi-workflow swimlanes shall apply the same membership semantics, scoped
  to each workflow, and compose them with existing step and plugin filters.
- **AC-UI-BOARD-REPOSITORY-MATCHING-001.6:** A task admitted through a secondary
  repository shall count toward its real step's occupancy. Search shall not
  remove it from occupancy, consistent with
  `AC-UI-KANBAN-AUTO-HIDE-EMPTY-COLUMNS-001.6` and `.7`.
- **AC-UI-BOARD-REPOSITORY-MATCHING-001.7:** Desktop and phone board projections
  shall use identical repository membership semantics.

## Out of scope

- Task membership writes, repository schemas, API transport and permissions.
- Sidebar repository-path filters, list grouping, or new searchable fields.
- New layout, controls, touch interactions, or persisted display settings.

## Related specifications

- [Column occupancy](kanban-auto-hide-empty-columns.md)
- [Board filter composition](board-step-visibility-filter.md)
- [System design](../system-design/board-repository-matching.md)
