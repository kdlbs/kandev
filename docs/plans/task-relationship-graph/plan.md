---
created: 2026-10-17
status: implemented
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
system_design:
  - ../../specs/tasks/system-design/task-relationship-graph.md
legacy_specs: []
---

# Implementation Plan: Task relationship graph

## Overview

Add a read-only graph of task relationships, opened from task detail. The
Subtasks view shows a parent's subtree ordered by dependency edges. The Chain
view shows one task's transitive predecessors and dependents. Rendering uses
`@xyflow/react` and `@dagrejs/dagre` behind the renderer contract in
[ADR-2026-10-17](../../decisions/2026-10-17-task-graph-renderer-boundary.md).

The plan has three sequential work orders. All use TDD.

## Scope

### In scope

- `GET /api/v1/tasks/:id/graph` with `subtasks` and `chain` modes, bounded to
  200 nodes.
- A renderer-agnostic graph model, step numbers, chain tracing, and dagre layout.
- An xyflow renderer isolated behind `TaskGraphRendererProps`.
- A lazy-loaded dialog with a view selector, legend, and loading, error, empty,
  and truncated states.
- A status-row entry control and a "View graph" action in the dependency chip.
- Light and dark themes through app tokens.
- Native touch interaction on phones.
- Localization in seven catalogs plus pseudo.

### Out of scope

- Editing edges or parents from the graph.
- Whole-board graphs, Office pages, image export, and persisted viewport.
- Database, WebSocket, MCP, or event contract changes.

## Technical approach

1. **Backend read.** `Service.BuildTaskGraph` walks the subtree or the chain in
   batched levels and reuses `BuildDependencyViews` and the edge verdicts. A
   thin handler adds the route.
2. **Model and layout.** Pure TypeScript computes step numbers, chain
   emphasis, and dagre positions whose ranks equal step numbers.
3. **Dialog and renderer.** The xyflow renderer, node card, dialog, entry
   points, live refresh, responsive sheet, i18n, and E2E coverage.

## ASCII UI preview

Structural choices are requirements: header with title, root name, and view
selector; canvas with left-to-right step ranks; legend row; node card content.
Spacing and exact icons are illustrative. Copy is localized.

### UI-01: Entry control in the task-detail status row

Entry point: task detail, chat status row (and passthrough toolbar). State:
the task has edges, a parent, or subtasks.

```text
 ... [PR #123 ✓] [⛓ 2 dependencies ▾] [⧉ Graph]                     [auto-scroll]
                                         └─ icon button, tooltip "Task graph"
```

Dependency chip popover gains a footer action:

```text
┌ Waiting on ───────────────────┐
│ ✓ Scaffold schema             │
│ ◷ Write API                   │
├ Blocks ───────────────────────┤
│   Wire UI                     │
├───────────────────────────────┤
│                 [View graph →]│
└───────────────────────────────┘
```

### UI-02: Graph dialog, Subtasks view (desktop)

Entry point: UI-01. State: loaded, parent with five subtasks, one external
predecessor. The header and legend are fixed. The canvas pans and zooms.

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ Task graph · Ship billing                [ Subtasks | Chain ]            [×] │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌ ─ ─ ─ ─ ─ ─ ─ ─┐                                                          │
│    External        ─ ─ ─┐                                                    │
│  │ Auth refactor  │     │                                                    │
│   ─ ─ ─ ─ ─ ─ ─ ─       ▼                                                    │
│  ┌────────────────┐   ┌────────────────┐    ┌────────────────┐              │
│  │ 1  Scaffold    │──▶│ 2  Write API   │──▶ │ 3  Wire UI     │              │
│  │    schema      │   │ ● In progress  │ ┌▶ │ 🔒 Blocked     │              │
│  │ ✓ Completed    │   │ ╔ This task ╗  │ │  └────────────────┘              │
│  └────────────────┘   └────────────────┘ │                                   │
│  ┌────────────────┐                      │                                   │
│  │ 1  Seed data   │─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ┘   (dashed = pending)              │
│  │ ○ To do        │                                                          │
│  └────────────────┘                                                          │
│  ┌────────────────┐                                                          │
│  │ 1  Docs        │                                                          │
│  │ in Write API   │   ← depth > 1 names its parent                           │
│  └────────────────┘                                                [+][-][⤢]│
├──────────────────────────────────────────────────────────────────────────────┤
│ ── Resolved   - - Pending   ━━ Failed (red)   ┆ External                     │
└──────────────────────────────────────────────────────────────────────────────┘
```

States (same frame, canvas region only):

```text
Loading:    [spinner] Loading graph…
Error:      ⚠ Could not load the graph.  [Retry]
No order:   ⓘ These subtasks have no set order. They can run in parallel.
Truncated:  ⓘ Showing the first 200 tasks.
Chain only: ⓘ This task has no dependencies.      (single focused node)
Disabled:   [ Subtasks | Chain ]  Subtasks: "This task has no subtasks or parent."
```

### UI-03: Graph dialog, Chain view (desktop)

```text
│ Task graph                               [ Subtasks | Chain ]            [×] │
│  ┌──────────┐   ┌──────────┐   ╔══════════╗   ┌──────────┐                   │
│  │1 Schema  │──▶│2 API     │──▶║3 UI      ║──▶│4 Release │                   │
│  └──────────┘   └──────────┘   ║This task ║   └──────────┘                   │
│                 ┌──────────┐ ┌▶╚══════════╝                                  │
│                 │2 Designs │─┘                                               │
│                 │ Archived │  (dimmed)                                       │
│                 └──────────┘                                                 │
```

Hover or focus on "API": API, Schema, UI, Release, and their edges stay full
opacity; Designs dims.

### UI-04: Phone sheet

Entry point: the UI-01 control at ≥44 px. Full-screen sheet; header and legend
fixed; the canvas owns pan and pinch. No minimap.

```text
┌─────────────────────────────┐
│ [×]  Task graph             │
│ Ship billing                │
│ [  Subtasks  |   Chain   ]  │  ← 44 px segments
├─────────────────────────────┤
│                             │
│ ┌─────────┐   ┌─────────┐   │
│ │1 Schema │──▶│2 API    │─▶ │  ← pans beyond edge;
│ └─────────┘   └─────────┘   │    no document overflow
│ ┌─────────┐                 │
│ │1 Seed   │                 │
│ └─────────┘                 │
│                   [+][-][⤢] │  ← 44 px controls
├─────────────────────────────┤
│ ── Resolved - - Pending ━━ Failed │
└─────────────────────────────┘
 Tap node: open task.  Long press: emphasize chain.
```

Preview mapping: UI-01 → `AC-…-001.1`, `.2`, `.4`; UI-02 → `002.*`, `004.*`,
`005.*`, `007.1`, `007.3`; UI-03 → `003.*`, `006.2`; UI-04 → `008.*`.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `001.1`, `001.2` | Button visibility tests; desktop E2E |
| `001.3`, `001.5` | Dialog tests; desktop and mobile E2E |
| `001.4` | Dependency chip test; desktop E2E |
| `002.1`–`002.5`, `002.8` | Go service and handler tests; desktop E2E |
| `002.6`, `002.7`, `003.3` | Model step tests; layout rank tests; dialog empty-state test |
| `003.1`, `003.2`, `003.4`, `003.5` | Go chain traversal tests; desktop E2E |
| `004.1`–`004.3` | Go mode-availability tests; dialog selector tests |
| `005.1`–`005.4` | Node card and legend tests |
| `005.5` | Renderer color-mode test; dark-theme E2E assertion |
| `005.6` | `useTaskGraph` signature and debounce tests; desktop E2E after a state change |
| `006.1`, `006.2`, `006.4` | Renderer interaction tests; desktop E2E |
| `006.3` | Renderer fit-view test; E2E node visibility |
| `006.5` | Renderer props test (not draggable/connectable/deletable) |
| `007.1`, `007.2` | Go truncation and authorization tests |
| `007.3` | Hook retry tests; dialog error-state test |
| `008.1`–`008.4` | Mobile Playwright spec |

## E2E tests

- `apps/web/e2e/tests/task/task-graph.spec.ts` (`chromium`): open from status
  row and from the chip, Subtasks order and external node, Chain view, hover
  emphasis, live state update, navigation by click, Escape close, dark theme.
- `apps/web/e2e/tests/task/mobile-task-graph.spec.ts` (`mobile-chrome`):
  full-screen sheet, 44 px targets, tap navigation, long-press emphasis, no
  horizontal overflow.
- Seed tasks and edges as in `task-dependencies.spec.ts`.

## Work orders

- [x] [Task 01: Add the task graph read](task-01-task-graph-read.md)
- [x] [Task 02: Add the graph model and layout](task-02-graph-model-layout.md)
- [x] [Task 03: Add the graph dialog and renderer](task-03-graph-dialog-renderer.md)
- [x] [Task 04: Move the graph into a workbench tab with Show in graph](task-04-workbench-tab.md)
- [x] [Task 05: Add the board Graph view](task-05-board-graph-view.md)

## Verification

Run the commands listed in each work order. The final work order also runs:

```bash
make fmt
make typecheck test lint
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Verification results

Implemented and verified on dev-01.

- Backend: `go test` for `internal/task/service` and `internal/task/handlers`
  pass, including 11 new graph tests; `golangci-lint` v2.9.0 reports 0 issues.
- Frontend: full Vitest suite 26,649 passed. The one failure,
  `e2e/scripts/native-catalog.test.ts` (`spawnSync unzip EACCES`), fails
  identically on the base commit. Typecheck, ESLint, `i18n:check`, and
  `i18n:ratchet` pass.
- E2E (Docker runner): 13 desktop and 4 mobile tests pass, including the
  existing dependency specs.
- Full `make test-backend` on this shared host still shows package timeouts in
  `lifecycle`, `backendapp`, `coordinator`, `mcp/handlers`, `orchestrator`, and
  `task/repository/sqlite`, which this change does not touch; CI is the gate
  for those.

## Risks

- Adding `@xyflow/react` and `@dagrejs/dagre` needs user approval before
  `pnpm add` on this machine.
- xyflow measures nodes after mount. The layout uses fixed node sizes, so
  dagre and the drawn cards must share the same size constants.
- Deep subtrees cost one children query per parent. The 200-node budget bounds
  the total.
- Long-press handling must not conflict with xyflow pan start on touch.
- Live refresh can thrash when many displayed tasks change at once. The 500 ms
  debounce and keeping the previous graph visible limit the impact.
