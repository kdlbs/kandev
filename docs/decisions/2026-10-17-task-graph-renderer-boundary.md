# ADR-2026-10-17-task-graph-renderer-boundary: Render the task graph with xyflow behind a renderer contract

**Status:** accepted
**Date:** 2026-10-17
**Area:** frontend

## Context

The task relationship graph
([requirements](../specs/tasks/requirements/task-relationship-graph.md),
[design](../specs/tasks/system-design/task-relationship-graph.md)) needs a
layered layout, drawn directed connectors, pan, zoom, fit view, and touch
gestures. An earlier layered view without connectors was removed before
shipping.

The web app only has `mermaid` for diagrams. Mermaid output gives limited
control over node content, state styling, keyboard access, and per-node
interaction. A hand-rolled SVG canvas, like the Office org chart, would need
our own layout, edge routing, pan, zoom, and pinch handling.

The user selected `@xyflow/react` with dagre layout and asked that the
rendering code stay replaceable by an in-house renderer.

## Decision

- Use `@xyflow/react` for rendering and interaction and `@dagrejs/dagre` for
  layout in `apps/web`.
- Keep graph semantics renderer-agnostic: step numbers, chain tracing, and
  layout output are pure TypeScript in `lib/task-graph/`.
- Each library has exactly one importing file: `lib/task-graph/layout.ts` for
  dagre and `components/task-graph/xyflow-task-graph-renderer.tsx` for xyflow.
  An ESLint `no-restricted-imports` rule enforces that.
- The dialog talks to the renderer only through `TaskGraphRendererProps`. Node
  content is a plain React card passed through `renderNode`.
- The dialog body is lazy-loaded so neither library enters the task-detail
  bundle until the graph opens.

## Consequences

- Replacing the renderer means writing one component that satisfies
  `TaskGraphRendererProps`; the model, layout, node card, dialog, and tests
  remain.
- Replacing dagre means reimplementing `layoutTaskGraph` with the same output
  type.
- Two dependencies are added to `apps/web`. Both are MIT licensed and actively
  maintained.
- xyflow theming is done by overriding its CSS variables with app tokens, so
  dark mode follows the app theme rather than xyflow defaults.

## Alternatives considered

- **Mermaid flowchart:** already installed, but static output with weak
  interaction and state styling.
- **Hand-rolled SVG renderer:** no new dependency, but substantial gesture and
  layout code to maintain now. The renderer contract keeps this option open.
