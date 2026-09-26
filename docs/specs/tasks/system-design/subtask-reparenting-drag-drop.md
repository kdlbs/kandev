---
status: current
system: tasks
requirements:
  - REQ-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001
---

# Subtask re-parenting by drag and drop system design

## Purpose and boundaries

This design owns sidebar candidate discovery for `Nest under` and drag-to-re-parent. It makes the
rendered task group the shared candidate source and carries the existing Office discriminator into
that view model so the UI can apply the same depth boundary as the backend. The complete unfiltered
task collection supplies hierarchy metadata for cycle and depth validation without adding hidden
tasks to the candidate list.

The canonical parent mutation, workspace-mode normalization, optimistic update, rollback, and
WebSocket reconciliation continues to use the canonical task event. Office project reassignment
now invokes that publisher, and the event carries the authoritative Office identity explicitly.
The persistence boundaries are described by [Detached Workspace Continuity](detached-workspace-continuity.md).
No endpoint, database column, feature flag, or user-facing copy is added.

## Requirement mapping

| Criteria                                            | Design section                          |
| --------------------------------------------------- | --------------------------------------- |
| AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.1 and .2 | Candidate source and propagation        |
| AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.3 and .4 | Eligibility algorithm                   |
| AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.5        | Responsive interaction and verification |

## Candidate source and propagation

The task HTTP DTO already exposes `is_from_office`. `toKanbanTask` preserves it as
`isFromOffice`, the Kanban snapshot task type owns that property, and both desktop and phone
sidebar projections preserve it on `TaskSwitcherItem`. This is a data projection only; Office
identity continues to come from the backend.

The mapped property remains optional on partial inputs: candidate checks treat an absent value as
non-Office, and `mergeTaskUpdate` retains the cached value when a lightweight event omits
`is_from_office`. Canonical `task.updated` events always carry an explicit true or false value, so
project assignment and removal can establish or clear the cached identity. The Office project
mutation publishes that canonical event after its task-row write. Active-board and multi-workflow
reconciliation uses the same absent-value fallback so an unrelated partial event cannot erase
Office identity from a full snapshot.

`GroupSection` already flattens the roots and descendants that it renders into `groupTasks` for
drag candidate calculation. That collection is the candidate source for both drag and context-menu
composition. Desktop and phone also pass their complete pre-view task list as hierarchy metadata.
The menu must not reread `kanbanMulti.snapshots[workflowId].tasks`: a present placeholder or
partially refreshed snapshot can contain fewer tasks than the reconciled group visible beside the
menu.

Rows receive stable getter functions for both collections. The owning switcher and group update
their refs after each committed render, so opening a menu or starting a drag reads current data
without making every memoized sibling row rerender when one task changes.

Candidate order follows rendered group order. Candidates remain scoped to the subject's workflow,
so mixed-workflow sidebar groups cannot create an invalid target.

## Eligibility algorithm

`computeNestCandidates` remains the shared pure function. Its input includes `id`, `parentTaskId`,
and `isFromOffice`. It applies these rules in order:

1. Resolve the subject from the complete hierarchy, falling back to the rendered group. With no
   subject, return no candidates.
2. Exclude the subject and its current parent in every mode.
3. Exclude every rendered task whose ancestor chain reaches the subject. Follow parent links in the
   complete hierarchy, including ancestors hidden by filtering, and track visited identifiers so
   corrupt hierarchy data cannot loop in the browser.
4. When neither the subject nor candidate is an Office task, require the candidate to be a root and
   return no candidates if the subject has a child. This preserves the one-level Kanban boundary.
5. When either endpoint is an Office task, allow a candidate at any depth and allow a subject that
   has children. This mirrors `validateReparentDepth`, which exempts the mutation when either
   endpoint is Office.

The browser filter is an affordance, not an authorization boundary. `PATCH /api/v1/tasks/:id`
continues to reject stale, archived, missing, cross-workspace, self, and cyclic targets.

## Mutation and failure flow

The context-menu selection and the drag nest zone call the existing `useNestTask` path with the
subject workflow and target identifier. The request uses the canonical task PATCH and the existing
optimistic snapshot update. A rejected request restores the prior tree and shows the existing
request error. A successful `task.updated` event reconciles the sidebar, board, and task detail.

A temporary multi-workflow placeholder cannot suppress a target that is already present in the
rendered group. A filtered-out task never becomes a candidate, but it remains available as ancestry
metadata. If the rendered group itself contains no eligible task, the menu shows its existing
disabled `No other tasks` row and drag shows no nest zone.

## Responsive interaction

Desktop and phone continue to use the same task-row context-menu and drag components. Desktop keeps
the anchored submenu; the phone task switcher keeps its inset sheet, touch drag sensor, menu event
containment, internal scroll owner, safe-area spacing, and current touch targets. Candidate data
changes do not add controls or alter geometry.

## Verification

Pure tests cover Office and Kanban eligibility, mixed Office/Kanban endpoints, filtered intermediate
ancestors, descendant exclusion, input order, and missing subjects. Projection and event tests prove
`is_from_office` reaches `TaskSwitcherItem`, survives omitted fields, and changes explicitly in both
directions. Component tests prove the context menu uses current rendered-group candidates without
rerendering unaffected rows, and that menu and drag return the same candidates.

The existing Kanban sidebar E2E remains a shallow-hierarchy regression. Focused desktop and phone
Office sidebar E2Es create a parent with a child and another target, select that target from `Nest
under`, and assert the deeper persisted and rendered hierarchy. The existing mobile re-parenting
E2E continues to cover the shared touch drag path.

## Implementation Plans

- [Fix Nest under candidates](../../../plans/fix-nest-under-candidates/plan.md)
