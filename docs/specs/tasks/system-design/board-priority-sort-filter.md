---
status: current
system: tasks
requirements:
  - REQ-TASKS-BOARD-PRIORITY-SORT-FILTER-001
  - REQ-TASKS-BOARD-PRIORITY-SORT-FILTER-002
  - REQ-TASKS-BOARD-PRIORITY-SORT-FILTER-003
  - REQ-TASKS-BOARD-PRIORITY-SORT-FILTER-004
  - REQ-TASKS-BOARD-PRIORITY-SORT-FILTER-005
created: 2026-09-13
owners:
  - kandev
---

# Board priority sort and filter System Design

## Purpose and boundaries

The task system owns the two board view preferences and their user-settings
contract. The web board owns the display controls, task projection, and local
view of those preferences. The sort is a display order; it does not change a
task priority or a workflow position.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-TASKS-BOARD-PRIORITY-SORT-FILTER-001 | Components and responsibilities, Task projection |
| REQ-TASKS-BOARD-PRIORITY-SORT-FILTER-002 | Ordering and bulk moves |
| REQ-TASKS-BOARD-PRIORITY-SORT-FILTER-003 | Responsive controls |
| REQ-TASKS-BOARD-PRIORITY-SORT-FILTER-004 | State and persistence |
| REQ-TASKS-BOARD-PRIORITY-SORT-FILTER-005 | Data contract and localization |

## Components and responsibilities

- The backend user-settings model, DTO, controller, service, store, and boot
  payload carry `kanban_sort` and `kanban_priority_filter_tokens`.
- The existing settings revision and WebSocket update path owns persistence and
  cross-client convergence. The board does not add a second persistence API.
- `useKanbanDisplaySettings` reads the hydrated settings and sends the complete
  board display snapshot when a person changes either value.
- The desktop display dropdown and mobile menu sheet expose the same two
  controls. They use localized labels and the canonical four priority tokens.
- `filterTasks`, `projectWorkflowTasks`, and the board view components project
  the stored state into the kanban and pipeline views. The occupancy projection
  remains separate from the displayed task projection.
- `sortTasksForPipelineView`, `task-order.ts`, and the multi-select hook share
  deterministic workflow-step and priority ordering for rendering and moves.

## Data contract and localization

The board sort is `created_desc` or `priority_desc`. The priority filter is a
list of `critical`, `high`, `medium`, and `low` tokens. An empty list means no
priority filter. Tokens are stored and sent unchanged; the active locale maps
them to labels at render time.

The backend normalizes stored values and preserves fields that are absent from
an update. The frontend keeps the selected value after a best-effort write and
replaces it when a newer settings event arrives. No database migration or new
task field is required.

## Task projection and ordering

The filter runs after the board has the task snapshots and before view-specific
rendering. It removes only tasks whose priority is outside a non-empty valid
selection. Unranked tasks remain visible when the filter is empty and remain
reachable under priority sorting.

The kanban view keeps its existing created-time order for `created_desc`. The
pipeline view keeps workflow-step order and position order for `created_desc`.
For `priority_desc`, priority is the first key and the native view order is the
tie-breaker. The pipeline step index uses the effective active workflow filter,
so an explicitly selected hidden workflow receives a real index. Equal unknown
step indices still use the within-step comparator instead of producing a
`NaN` comparison.

## Ordering and bulk moves

The multi-select hook sorts selected task IDs with the same effective workflow
and step ordering used by the rendered board. A bulk move then assigns target
positions in that deterministic order. Hidden workflows are included when a
person explicitly selects them, while workflow selection still limits the
ordering universe.

## State lifecycle

The flow is:

`boot payload or settings event -> user-settings store -> display controls and task projection -> settings update -> backend CAS persistence -> settings event -> all connected boards`

The backend remains the owner of persisted state. The frontend owns only the
current rendered selection and derives visible and occupancy task sets from
the canonical task snapshots. Task updates can change a card's priority while
the board is open; the projection and ordering recompute without changing the
stored view preferences.

## Responsive behavior

Desktop and mobile use separate presentation components with the same settings
hook and token contract. The mobile control is in the existing menu sheet, so
the feature does not add a route, a new overlay, or a mobile-only state store.
