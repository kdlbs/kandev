---
status: current
system: ui
requirements:
  - REQ-UI-SIDEBAR-EFFECTIVE-TASK-TREE-STATE-001
---

# Sidebar Effective Task Tree State System Design

## Purpose and boundaries

The task system publishes authoritative task and session states. The UI derives one effective
state for sidebar placement because a parent and its nested descendants render as one tree under
one group heading.

This design changes no backend API, persisted task state, saved sidebar-view schema, or row status
presentation. It complements the task system's
[runtime state publication order](../../tasks/system-design/runtime-state-publication-order.md).

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-SIDEBAR-EFFECTIVE-TASK-TREE-STATE-001` | [Effective-state resolution](#effective-state-resolution), [Shared grouping and sorting](#shared-grouping-and-sorting), [Responsive behavior](#responsive-behavior), [Tests](#tests) |

## Components and responsibilities

- `TaskSwitcherItem` carries each task's persisted task state, projected session state, and parent
  identity without adding an aggregate-state field.
- `separateSubtasks` builds the included parent-to-children map after sidebar filters run.
- The pure resolver in `effective-task-tree-state.ts` builds an iterative, memoized aggregate for
  each included task and returns its effective group identity and sort bucket.
- `applySort` and `applyGroup` consume the same resolved value so their state semantics cannot
  diverge.
- `TaskSwitcher` and `SessionTaskSwitcherSheet` render the shared grouped result on desktop and
  mobile while each row continues to render its own status.

## Effective-state resolution

The resolver traverses every included descendant, not only direct children. It builds the reachable
task graph once, then resolves each task's aggregate with an iterative postorder walk. Memoized
aggregates keep shared descendants linear in the number of tasks and edges. An active-path set
prevents malformed parent cycles from scheduling an unbounded walk.

Resolution follows these rules:

1. If any included member has an in-progress task state or a running session classification, the
   tree resolves to `IN_PROGRESS`.
2. Otherwise, if an included member is scheduling, the tree resolves to `SCHEDULING`.
3. Otherwise, the resolver keeps the existing non-active state priority, except that it cannot
   select `COMPLETED` while any included member is not completed. Failed or cancelled members
   therefore remain visible in their existing attention state instead of being hidden by a
   completed ancestor.
4. A tree resolves to `COMPLETED` only when every included member has persisted state
   `COMPLETED`.

A member with a running session but a temporarily absent task state resolves the tree to
`IN_PROGRESS`. This is a display-only recovery for an incomplete projection; it does not write or
synthesize persisted task state.

Filtering runs before tree resolution. A descendant that is absent from the filtered task list is
not part of the included tree and cannot move its visible ancestor between groups.

## Shared grouping and sorting

The resolver returns both the state-group key and the action bucket needed by state sorting.
`applyView` computes this value from the filtered tree map and reuses it for root sorting and state
group creation. The resolver's action-bucket ordering and state-group ordering remain private to
the state module so the two consumers cannot drift.

The current implementation independently minimizes `STATE_BUCKET_ORDER` in both paths. Because
the `review` bucket includes `COMPLETED`, `FAILED`, `CANCELLED`, and waiting session states and is
ordered before `in_progress`, a parent in one of those states can defeat a running child. Reusing
the effective tree result removes that mismatch without changing the ordering of unrelated root
tasks.

Row components continue to receive the original `TaskSwitcherItem`. They do not receive or render
the effective tree state as the row's own state.

## Responsive behavior

Desktop `TaskSwitcher` and mobile `SessionTaskSwitcherSheet` already consume the same `applyView`
result. The change alters shared state normalization only. It does not change composition,
navigation, scrolling, safe-area handling, pointer behavior, or touch targets.

The nearest mobile exemplar is `SessionTaskSwitcherSheet`. Existing mobile subtask coverage proves
that nested rows consume the shared task-tree model. New mobile-specific Playwright coverage is not
required for this state-only correction.

## Tests

`apply-view-effective-state.test.ts` covers completed, review, waiting-for-input, and missing-state
parents with running descendants. It also covers nested descendants, completion gating, filtering,
agreement between state sorting and grouping, and linear child-map lookups for a deep chain.

`sidebar-subtask-state-sort.spec.ts` seeds a completed parent with an in-progress child and asserts
through the rendered sidebar that the whole tree is under the `IN_PROGRESS` group. The existing
mobile sidebar subtask tests continue to cover the shared tree-rendering path.

## Failure and recovery

An absent descendant projection cannot contribute to the included tree. The next authoritative
snapshot or task event recomputes `displayTasks`, and `applyView` derives the new effective state
without a separate cache or retry path.

## Persistence

The effective state is computed during rendering and is never persisted. Existing group-collapse
keys and saved view settings remain unchanged.

## Security

The resolver uses only task and session state already available to the current sidebar. It adds no
data source, permission, or trust boundary.
