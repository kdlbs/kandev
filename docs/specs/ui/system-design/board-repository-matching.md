---
status: current
system: ui
created: 2026-10-01
requirements:
  - REQ-UI-BOARD-REPOSITORY-MATCHING-001
owners:
  - kandev
---

# Board repository matching System Design

## Purpose and boundaries

Board filtering is a UI projection over existing task repository links and
workspace metadata. This design preserves the compatibility contract already
stated by `KanbanState.tasks`: `repositoryId` is primary-only and
`repositories` contains complete ordered membership. It adds no transport,
persistence, permission, or membership-write boundary.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-BOARD-REPOSITORY-MATCHING-001` | Membership, search, and projections below |

## Membership

`apps/web/lib/kanban/filters.ts` owns the minimal `FilterableTask` shape and
`filterTasksByRepositories`. Its shape accepts the existing optional repository
links with `repository_id`; link `id` identifies the attachment, not the repo.

A small exported membership helper in that module derives IDs from a supplied
collection, preserving an empty array. Only when the collection is absent does
it derive zero or one IDs from `repositoryId`. The selection predicate uses
existential overlap with the selected ID set. No selection returns the input
array directly, preserving the existing no-filter reference behavior. Filtering
retains task identity, ordering, and other task fields without mutation.

## Repository metadata search

`apps/web/hooks/domains/kanban/use-kanban-data.ts` uses the same membership
helper for its repository-name/path search arm. It resolves each linked ID
against the active workspace's repository records, using case-insensitive
substring matching of `name` and `local_path`. Unknown IDs produce no repository
match; title and description remain independent search arms. Search runs over
repository-filtered tasks and remains memoized on tasks, query, workspace and
repository metadata.

Live board cards are rendered through `SwimlaneContainer`, not through the
legacy hook's `filteredTasks`. The shared `task-projections.ts` search predicate
must therefore receive repository metadata for the owning workflow's workspace
and apply the same name/path arm. Build an ID lookup once for the relevant
workspace rather than repeatedly scanning all repository records per task.
Overview projection caches must include that lookup as an input; metadata
arrival, rename, removal, or workflow workspace changes must recompute search.
Focused projections use the same workspace boundary. Search metadata remains
absent from the occupancy lens.

A later data-source migration may supply workspace repositories through a hook
rather than Zustand; the matching semantics and workspace boundary stay the
same. No new searchable repository fields or unrelated search surface is added.

## Shared board and swimlane projections

`apps/web/lib/kanban/task-projections.ts` already calls the shared repository
filter for both `visibleTasks` and `occupancyTasks`. Keep those membership call sites and lens separation while adding repository
metadata to the visible search predicate. `useSwimlaneRenderData` and `useWorkflowSwimlaneData` consume
that projection for multi-workflow lanes and focused workflow views.

The latter exposes occupancy separately so auto-hide logic retains an occupied
step when a task matches a secondary repository, even if search removes its
card. Existing plugin, priority, hidden-step, workflow and PR/MR search behavior
remains governed by its current contracts.

## Responsive behavior and verification

Desktop and phone use shared projections. There is no breakpoint-dependent
membership decision, new layout, scrolling, or interaction. Apply the
`mobile-parity` skill's state/data normalization exception: focused helper,
real-hook and swimlane tests prove identical semantics without a new mobile
Playwright case.

Cover collection authority versus contradictory scalar values, explicit empty
collections, absent legacy collections, no selection, multiple selected IDs,
secondary name/path search, missing/current-workspace metadata, per-workflow
isolation, and occupied-step retention. Use real projection functions and real
hooks; mock network/persistence effects only as needed for deterministic hooks.

## Related specifications

- [Requirements](../requirements/board-repository-matching.md)
- [Column occupancy](../requirements/kanban-auto-hide-empty-columns.md)
- [Column visibility](../requirements/board-step-visibility-filter.md)
