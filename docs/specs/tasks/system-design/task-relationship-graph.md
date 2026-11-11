---
status: current
system: tasks
requirements:
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-001
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-002
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-003
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-004
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-005
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-006
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-007
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-008
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-009
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-010
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-011
created: 2026-10-17
owners:
  - kandev
---

# Task relationship graph System Design

## Purpose and boundaries

This design adds a read-only graph read to the task service and a graph dialog
to the web task-detail surface. It reuses the dependency edges in
`task_blockers` and the subtask hierarchy in `tasks.parent_id`. It does not
change edge storage, validation, cycle checks, launch gating, or events defined
in [`task-dependencies.md`](task-dependencies.md).

[`task-create-relationships.md`](task-create-relationships.md) adds a second,
non-blocking edge kind: the related-to link, stored in its own `task_relations`
table. This design (REQ-TASKS-TASK-RELATIONSHIP-GRAPH-011) draws that edge kind
wherever both of its endpoints are already displayed by the existing
Subtasks/Chain/board inclusion rules. It does not expand those inclusion rules,
does not change step computation, and does not add related-to editing.

The task system owns this design because it owns both relationships. The
graph is the shipped answer to the "whole-graph view" exclusion in
[`task-dependencies.md`](task-dependencies.md#out-of-scope): it draws real
connectors and focuses one subtree or one chain.

Renderer isolation follows
[ADR-2026-10-17 task graph renderer boundary](../../../decisions/2026-10-17-task-graph-renderer-boundary.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-TASK-RELATIONSHIP-GRAPH-001` | [Entry points](#entry-points), [Dialog](#dialog) |
| `REQ-TASKS-TASK-RELATIONSHIP-GRAPH-002` | [Graph read contract](#graph-read-contract), [Subtasks traversal](#subtasks-traversal), [Graph model](#graph-model) |
| `REQ-TASKS-TASK-RELATIONSHIP-GRAPH-003` | [Graph read contract](#graph-read-contract), [Chain traversal](#chain-traversal), [Graph model](#graph-model) |
| `REQ-TASKS-TASK-RELATIONSHIP-GRAPH-004` | [Mode selection](#mode-selection), [Dialog](#dialog) |
| `REQ-TASKS-TASK-RELATIONSHIP-GRAPH-005` | [Node and edge presentation](#node-and-edge-presentation), [Live refresh](#live-refresh) |
| `REQ-TASKS-TASK-RELATIONSHIP-GRAPH-006` | [Renderer boundary](#renderer-boundary), [Interaction](#interaction) |
| `REQ-TASKS-TASK-RELATIONSHIP-GRAPH-007` | [Bounds and failure](#bounds-and-failure) |
| `REQ-TASKS-TASK-RELATIONSHIP-GRAPH-008` | [Responsive behavior](#responsive-behavior) |
| `REQ-TASKS-TASK-RELATIONSHIP-GRAPH-009` | [Workbench tab](#workbench-tab), [Show in graph](#show-in-graph) |
| `REQ-TASKS-TASK-RELATIONSHIP-GRAPH-010` | [Board Graph view](#board-graph-view) |
| `REQ-TASKS-TASK-RELATIONSHIP-GRAPH-011` | [Graph read contract](#graph-read-contract), [Node and edge presentation](#node-and-edge-presentation), [Graph model](#graph-model) |

## Graph read contract

`GET /api/v1/tasks/:id/graph?mode=subtasks|chain` is registered with the other
task routes in `internal/task/handlers/task_handlers.go`. It is a read with the
same authorization and not-found handling as `GET /api/v1/tasks/:id`. No
WebSocket action, MCP tool, migration, or event is added.

`mode` is optional. When it is omitted, the server selects the default by the
rule in [Mode selection](#mode-selection). A requested mode that is not
available returns HTTP `400` with `{ "error": "...", "code": "graph_mode_unavailable" }`.

Response (`dto.TaskGraphDTO` in `internal/task/dto/task_graph.go`):

```json
{
  "mode": "subtasks",
  "available_modes": ["subtasks", "chain"],
  "focus_task_id": "task-b",
  "root": { "id": "task-parent", "title": "Ship billing" },
  "nodes": [
    {
      "id": "task-b",
      "title": "Write API",
      "state": "IN_PROGRESS",
      "parent_id": "task-parent",
      "parent_title": "Ship billing",
      "depth": 1,
      "role": "member",
      "archived": false,
      "blocked": false,
      "blocked_reason": ""
    }
  ],
  "edges": [
    { "from": "task-a", "to": "task-b", "kind": "dependency", "status": "resolved" },
    { "from": "task-b", "to": "task-c", "kind": "related" }
  ],
  "truncated": false
}
```

- `root` is present only in `subtasks` mode. The root is not a node; it is the
  header subject.
- `role` is `member` or `external`. The focused node is identified by
  `focus_task_id`, never by role, so a focused node keeps its membership.
- `depth` is the `parent_id` distance from the root, starting at 1. It is `0`
  for external nodes and for every node in `chain` mode.
- `kind` is `dependency` or `related` (REQ-TASKS-TASK-RELATIONSHIP-GRAPH-011).
  A `dependency` edge points from predecessor to dependent and carries
  `status`. A `related` edge has no direction and omits `status`; the service
  always emits it with the lower task ID as `from` so the same pair never
  produces two edges.
- An edge points from predecessor to dependent. `status` uses the existing edge
  verdicts `resolved`, `failed`, and `pending` from
  `internal/task/service/service_dependencies.go`, and applies only to
  `dependency` edges.
- `blocked` and `blocked_reason` come from `BuildDependencyViews`, so node
  blockage matches the board and the dependency chip.
- `nodes` are ordered by first discovery. Edges are deduplicated by
  `(from, to, kind)`.

## Mode selection

`available_modes` always contains `chain`. It contains `subtasks` when the
opened task has a non-archived, non-ephemeral child or a non-empty
`parent_id`. The default mode is `subtasks` when available and `chain`
otherwise.

## Subtasks traversal

`Service.BuildTaskGraph` in `internal/task/service/service_task_graph.go`:

1. Selects the root: the opened task when `ListChildrenLimited` returns a
   child, otherwise the opened task's parent.
2. Walks descendants breadth-first, one `ListChildrenLimited` call per parent,
   using the same non-archived, non-ephemeral filter as `ListChildren`. The
   remaining node budget is the limit for each call. A visited set guards
   against corrupt hierarchy loops.
3. Reads the edges of all members in one batch with `ListBlockersForTasks` and
   `ListDependentsForTasks`, plus related-to links with
   `ListRelationsForTasks` (`internal/task/service/repository_relations.go`,
   added by [`task-create-relationships.md`](task-create-relationships.md)).
4. Adds each edge end that is not a member as an `external` node, without
   following its other edges. This can include the root itself. A related-to
   link whose other end is not a member is dropped rather than added as an
   external node (AC-TASKS-TASK-RELATIONSHIP-GRAPH-011.4); only `dependency`
   edges pull in external nodes.
5. Resolves external rows with `GetTasksByIDs` and computes every node's
   `DependencyView` with `BuildDependencyViews`.

## Chain traversal

The chain walk is two independent breadth-first searches from the focus task.
The upstream search follows `ListBlockersForTasks` one level per batch. The
downstream search follows `ListDependentsForTasks` one level per batch. The
walk never follows an edge in the opposite direction, so a sibling predecessor
of a dependent is excluded. Archived tasks remain nodes with `archived: true`.
The result contains every `dependency` edge between two displayed nodes,
including an edge between two upstream nodes, plus every `related` link
between two displayed nodes (read with `ListRelationsForTasks` over the final
node set, since a related-to link does not define chain membership).

Edges are acyclic because writes reject cycles. Visited sets still bound the
walk if a corrupt row forms a loop.

## Bounds and failure

- `maxTaskGraphNodes = 200` counts every node, including external nodes and the
  focus. The walk stops adding nodes when the budget is reached, sets
  `truncated: true`, and returns only edges between returned nodes.
- Each traversal level is one batched repository read, so a 200-node graph
  costs at most a few hundred bounded queries in the deepest case and a
  handful in the common case.
- A repository error returns HTTP `500` without a partial graph. A missing or
  unauthorized task returns the existing task not-found response.
- When no blocker repository is wired, the read returns nodes without edges
  and the withheld dependency views that `BuildDependencyViews` already
  produces.

## Frontend structure

```text
lib/api/domains/task-graph-api.ts        fetch + response types
lib/task-graph/model.ts                  renderer-agnostic model: steps, chains
lib/task-graph/layout.ts                 dagre layout → node positions
components/task-graph/
  task-graph-button.tsx                  status-row entry control
  task-graph-dialog.tsx                  Dialog / phone sheet shell (lazy body)
  task-graph-content.tsx                 states, notices, canvas, navigation
  task-graph-mode-selector.tsx           Subtasks / Chain toggle
  task-graph-legend.tsx                  edge-style legend
  task-graph-edge-styles.ts              stroke, dash, width per verdict
  task-graph-node-card.tsx               renderer-agnostic node card
  use-node-press.ts                      tap vs. touch long press
  xyflow-task-graph-renderer.tsx         only importer of @xyflow/react
  task-graph-renderer.ts                 TaskGraphRendererProps contract
hooks/domains/task/use-task-graph.ts           fetch, refresh, retry
hooks/domains/task/use-task-graph-available.ts entry-point visibility
hooks/domains/task/use-task-dependencies.ts    dependency projection (moved from the chip)
```

`@xyflow/react` and `@dagrejs/dagre` are new `apps/web` dependencies.
`task-graph-dialog.tsx` loads `task-graph-content.tsx` with `React.lazy`, so
neither library enters the task-detail bundle until the dialog opens.

## Graph model

`buildTaskGraphModel(response)` in `lib/task-graph/model.ts` is a pure
function. It returns the nodes with a `step` number, the edges, the focus ID,
and adjacency maps. The step number is the longest-path rank over displayed
`dependency` edges: a node without a displayed predecessor has step 1, and
every other node has `1 + max(step of displayed predecessors)`. The
computation is a Kahn topological pass. A node left in a cycle receives the
highest assigned step plus one, so corrupt data cannot hang the dialog.
`related` edges are excluded from this pass entirely
(AC-TASKS-TASK-RELATIONSHIP-GRAPH-011.3): they contribute no adjacency to the
step computation, so a related-to link can never introduce a step dependency
or a false cycle.

`traceChain(model, nodeId)` returns the transitive predecessor and dependent
node IDs and the edge IDs between them. It powers emphasis
(AC-TASKS-TASK-RELATIONSHIP-GRAPH-006.2) and is independent of the renderer.

`noOrderSet` is true in `subtasks` mode when no member has a displayed edge to
another member (AC-TASKS-TASK-RELATIONSHIP-GRAPH-002.7).

## Renderer boundary

`TaskGraphRendererProps` in `components/task-graph/task-graph-renderer.ts` is
the only contract between the dialog and a renderer:

```ts
type TaskGraphRendererProps = {
  model: TaskGraphModel;
  layout: TaskGraphLayout;            // node id -> { x, y, width, height }
  emphasis: TaskGraphChain | null;    // from traceChain
  colorMode: "light" | "dark";
  touch: boolean;                     // coarse pointer: no minimap, 44 px controls
  renderNode: (node: TaskGraphNode, state: TaskGraphNodeRenderState) => ReactNode;
  nodeLabel: (node: TaskGraphNode) => string; // accessible name
  onNodeActivate: (nodeId: string) => void;
  onNodeEmphasize: (nodeId: string | null) => void;
};
```

- `layoutTaskGraph(model)` in `lib/task-graph/layout.ts` is the only importer of
  `@dagrejs/dagre`. dagre (`rankdir: "LR"`) only orders nodes within a
  column to limit crossings; it takes no rank input, so x comes from the model
  step and each column is re-packed to avoid overlaps. The drawn column always
  equals the step badge.
- `xyflow-task-graph-renderer.tsx` is the only importer of `@xyflow/react`. It
  maps the model and layout into xyflow nodes and edges, uses one custom node
  type that calls `renderNode`, and uses the built-in smooth-step edge with a
  marker arrow for `dependency` edges and the same edge type with no marker for
  `related` edges. Nodes are not draggable or connectable, and edges are not
  selectable or deletable (AC-TASKS-TASK-RELATIONSHIP-GRAPH-006.5).
- `task-graph-node-card.tsx` is plain React with design tokens, so a
  replacement renderer reuses it unchanged.

A lint rule (`no-restricted-imports` in `apps/web/eslint.config.mjs`) rejects
`@xyflow/react` and `@dagrejs/dagre` outside their single owning file.

## Node and edge presentation

- A node card shows the step badge, title (two-line clamp), the existing task
  state indicator, a lock icon with "Blocked" or a warning icon with
  "Predecessor failed", and, for depth greater than 1, "in <parent title>".
- An `external` node uses a dashed border and muted fill. An `archived` node
  uses reduced opacity and an "Archived" label. The focused node uses a
  primary-color ring and a "This task" label.
- Edge styles: `resolved` is a solid muted line; `pending` is a dashed
  primary-color line; `failed` is a solid destructive-color line; `related` is
  a dotted neutral-color line with no arrowhead marker, so it reads as an
  association rather than an order. The legend shows all four styles with text
  labels.
- Colors come from the app's CSS variables (`--border`, `--primary`,
  `--destructive`, `--muted-foreground`, `--card`). The renderer passes
  `colorMode` from `useTheme().resolvedTheme` to `ReactFlow` and overrides the
  xyflow CSS variables (`--xy-background-color`, `--xy-edge-stroke`,
  `--xy-controls-button-background-color`, and related) with those tokens.
  Theme changes re-render through `useTheme` without reopening the dialog
  (AC-TASKS-TASK-RELATIONSHIP-GRAPH-005.5).

## Entry points

`TaskGraphButton` uses the shared `@kandev/ui/button` and tooltip primitives.
It shows only `IconHierarchy3`, with the existing `task:taskGraphTitle` key for
its accessible name and tooltip. It keeps `data-testid="task-graph-button"`.
The glyph is decorative. The button has no visible text label or pill border.
Its compact desktop target is 24 by 24 pixels, like the adjacent Share and
auto-scroll controls. Phone and coarse-pointer variants expand both dimensions
to at least 44 pixels. A tap opens the graph directly.

`ChatStatusBarRightControls` in `components/task/chat/chat-status-bar.tsx`
mounts the button immediately after `TranscriptNavGroup`. That group already
orders transcript navigation, Usage, and Share. The graph therefore follows
Share when Share is present. It remains before the desktop workflow action.
The left status children no longer mount a graph control.

`getRightControlVisibility` admits the right-control shell whenever a task ID
exists, including a task without a session. `TaskGraphButton` retains sole
ownership of graph availability through `useTaskGraphAvailable`. This avoids a
second availability subscription at the row level. The shell keeps its existing
`empty:hidden` behavior when no child qualifies. Graph access does not depend on
the session-owned Usage or Share conditions.

`useTaskGraphAvailable` reuses dependency projections, `parentTaskId`,
`useTaskSubtasks`, and related-to links. Only when those sources do not establish
availability does it read `GET /tasks/:id/subtask-count`. That read covers a
parent whose subtasks are not on a loaded board. No new graph read or polling
loop is required before activation.

`components/task/passthrough-toolbar.tsx` retains its existing action area and
consumes the shared icon presentation. It does not gain session usage or share
controls as part of this revision.

Phone composition reuses `ChatStatusBarActions`: banners and the workflow
action precede status items, then the utility group wraps with right alignment.
The graph icon stays in that utility group. The composer keeps its existing
scroll and safe-area owners. `useResponsiveBreakpoint` remains the layout
authority. Phone sizing also applies to narrow fine-pointer windows.

`useOpenTaskGraph().openTab(taskId)` remains the activation path. Desktop opens
or focuses the workbench tab. Phones keep the full-screen graph sheet and its
focus-return path. The icon does not introduce an intermediate menu or drawer.
The existing graph sheet is the local example for dense content. The mobile
UI language selects a full-height surface for this content depth. The adjacent
`AutoScrollToggleButton` supplies the compact desktop and touch target pattern.

`TaskDependencyChip` adds a "View graph" action to its popover and drawer that
opens the same dialog with the default mode.

## Mode selection in the UI

The dialog header uses the `@kandev/ui` toggle group with Subtasks and Chain
options. The first request omits `mode`; the response's `mode` and
`available_modes` initialize the selector. A disabled Subtasks option has a
tooltip, or helper text on touch, saying the task has no subtasks or parent.

## Dialog

`TaskGraphDialog` uses `@kandev/ui` `Dialog` on desktop at
`min(1200px, 95vw)` by `min(800px, 90vh)`. On phones it renders full screen,
following the existing full-screen task edit dialog pattern. Radix restores
focus only to a `DialogTrigger`, so `TaskGraphButton` passes its button as the
trigger and the dependency chip passes `returnFocusTo` with its own trigger
(AC-TASKS-TASK-RELATIONSHIP-GRAPH-001.5). The header contains the title and
close; the body shows the root name in Subtasks mode and the view selector. A footer row contains the legend. A truncation notice appears above the
canvas when `truncated` is true.

## Interaction

- Node activation navigates with the existing task route helper used by
  `TaskLink` and then closes the dialog.
- The custom node renders a focusable button with
  `aria-label` "<title>, <state>, step <n>". Tab order follows step, then
  discovery order, through xyflow's node keyboard support with `tabIndex`
  ordering.
- `onMouseEnter`, `onFocus`, `onMouseLeave`, and `onBlur` call
  `onNodeEmphasize`. Emphasized elements keep full opacity; others drop to
  reduced opacity.
- `fitView` runs on initial render and after a mode change, with a zoom floor
  of 0.75 so cards stay readable. A graph larger than the canvas at that floor
  is centred on the focused task when it is a node, and otherwise anchored at
  its top-left (step 1). The fit waits for the canvas size, not node
  measurement, because node sizes are explicit. The xyflow
  `Controls` provide zoom in, zoom out, and fit view.

## Live refresh

`useTaskGraph` refetches with a 500 ms trailing debounce when its store
signature changes. The signature is derived from store tasks whose ID is a
displayed node or whose `parentTaskId` is a displayed member or the root, and
includes each one's `state`, `title`, `parentTaskId`, `archivedAt`, `dependsOn`
IDs, `blocks` IDs, and `related` IDs. Existing `task.updated`, `task.created`, and
`task.deleted` handling keeps the store current, so the graph does not add a
WebSocket subscription. A refetch keeps the previous graph visible and does not
reset the viewport unless the node set changes.

## Responsive behavior

- Phone: the full-screen sheet has the header and selector fixed at the top and
  the legend fixed at the bottom; the canvas fills the rest and owns all
  gestures. xyflow provides one-finger pan and pinch zoom. The minimap is not
  rendered on phones.
- Long press (500 ms without movement) on a node calls `onNodeEmphasize`;
  a tap activates the node (AC-TASKS-TASK-RELATIONSHIP-GRAPH-008.3).
- Touch controls use `min-h-11 min-w-11` targets. xyflow `Controls` buttons get
  the same size through the CSS variable override on coarse pointers.
- The sheet uses `overflow-hidden` with the canvas at `100%` width, so the
  document cannot scroll horizontally.

## Workbench tab

On desktop the graph is the `task-graph` dockview panel, a fixed-id panel
listed in `KNOWN_PANEL_IDS`, `RENDERABLE_COMPONENT_NAMES`, both
`PANEL_RENDERERS` tables, and `PANEL_REGISTRY` (`task:panelTaskGraph`), so the
layout serializer keeps it. `addTaskGraphPanel` in
`lib/state/dockview-extra-panel-actions.ts` opens it with `inCenter: true`
through `addSidePanel`, which focuses an existing tab. The panel carries no
params: `TaskGraphPanel` reads `tasks.activeTaskId`, so it follows task
switches and node activation (which only navigates). The dialog shell is now
the phone sheet only.

## Show in graph

`linkToTask(taskId, { panel: "task-graph" })` adds `?panel=task-graph`.
`TaskGraphPanelRequest`, mounted once in `TaskLayout`, reads the param; on
desktop it waits for the dockview `api` and `isRestoringLayout === false`, calls
`addTaskGraphPanel`, and on phones it opens the sheet. When the active session
has an environment, it also waits until `currentLayoutEnvId` equals that
environment, because applying an environment layout replaces the panels; a
session that still belongs to the previous task counts as not ready. A first
visit can create the environment after the request is fulfilled, so the
component re-adds the tab once when that environment's layout is adopted. It then replaces the URL
without the param so reloads and back navigation do not reopen it. The sidebar
context menu (`TaskShowInGraphItem` in the relationships group) and the Kanban
card menu (`buildShowGraphEntry`) push that link. The status-row button and
the chip action call the same open path for the current task.

## Board Graph view

`TaskListingView` gains `"graph"`; `useTaskListingView` maps it to
`userSettings.kanbanViewMode = "graph"`, and `VIEW_REGISTRY` resolves that to
`SwimlaneTaskGraphContent`, which renders once per workflow lane with the lane's
already-filtered `tasks` (`ViewContentProps`). Phones fall back to Kanban in both
`getEffectiveTaskListingView` and `getEffectiveView`, matching Pipeline. No
backend change is needed: `kanbanViewMode` is an opaque string.

The lane lazy-loads `BoardGraph`, which builds the graph client-side with
`buildBoardGraph` (`lib/task-graph/board.ts`): nodes are the lane's tasks,
`dependency` edges are `dependsOn` refs whose predecessor is also on the lane,
`related` edges are `related` refs whose other end is also on the lane
(AC-TASKS-TASK-RELATIONSHIP-GRAPH-011.4: this view's existing same-lane
inclusion rule is not widened for related-to), and each parent becomes a
frame. `layoutGroupedTaskGraph` uses dagre's compound mode to keep a
parent's subtasks contiguous and sizes each frame from its members. Frames are
non-interactive xyflow background nodes (`TaskGraphRendererProps.frames`).
Node activation calls the board's `onPreviewTask`. The shared `TaskGraphCanvas`
supplies theme, touch, emphasis and accessible names to both this view and the
task graph.

A node click only selects (`TaskGraphRendererProps.selectedId`); the selection
bar offers Open task (`onOpenTask`) and Preview (`onPreviewTask`), and
`onNodeOpen` (double-click) opens directly.

The Parallel steps toggle (`useTaskGraphParallelSteps`, device-local
`kandev.taskGraph.parallelSteps.v1`) switches the board to the step-column
`layoutTaskGraph` and draws `stepFrames` bands in place of parent frames; the
task graph keeps its step layout and adds the bands. Bands share one top and
bottom so their labels form a header row above the cards, clear of connectors.

`linkToTaskOverview({ graphFocusTaskId })` adds `?graphFocus=`; the lane that
contains the task marks it focused, centres it, and scrolls into view.
`useOpenTaskGraph().showInGraph` stores the Graph view and navigates there; the
status-row button and chip keep `openTab`, which opens the workbench tab.

## Localization

Copy uses flat `task:taskGraph*` keys in all seven catalogs plus the
pseudo-locale. The node accessible name interpolates `{{title}}`, `{{state}}`,
and `{{step}}`; the truncation notice uses `{{count}}` with `_one` and `_other`
forms. xyflow's built-in English aria labels are replaced through
`ariaLabelConfig`.

## Testing

- Go service tests: root selection, multi-level subtree, archived and ephemeral
  filtering, external nodes, chain direction rules, archived chain nodes,
  truncation, dedupe, mode availability, and `related` edges (included when
  both ends are displayed, dropped when one end is external, excluded from
  step computation).
- Go handler tests: response shape, default mode, `graph_mode_unavailable`,
  not-found, and authorization.
- Vitest: API client, `buildTaskGraphModel` step numbers and cycle guard,
  `traceChain`, `layoutTaskGraph` rank equals step, `useTaskGraph` debounce and
  retry, button visibility, and the dialog's loading, error, empty, truncated,
  and disabled-mode states with a stub renderer.
- Composer entry tests: localized icon name, tooltip, keyboard activation,
  placement in the right group, a qualifying task without a session, and one
  control across responsive changes. Existing availability tests retain the parent,
  child, dependency, related-only, and isolated-task cases.
- Playwright: desktop and mobile specs in `apps/web/e2e/tests/task/`.
  The entry revision adds action-group containment, compact desktop geometry,
  phone and coarse-pointer targets, wrapping, and narrow fine-pointer coverage.
