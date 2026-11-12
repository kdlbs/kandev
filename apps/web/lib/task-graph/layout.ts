import dagre from "@dagrejs/dagre";
import type { TaskGraphModel } from "./model";

// The only importer of dagre (ADR 2026-10-17-task-graph-renderer-boundary).

export const TASK_GRAPH_NODE_WIDTH = 220;
export const TASK_GRAPH_NODE_HEIGHT = 88;
const COLUMN_GAP = 72;
const ROW_GAP = 24;

export type TaskGraphNodeBox = { x: number; y: number; width: number; height: number };
export type TaskGraphLayout = Map<string, TaskGraphNodeBox>;

/** Top-left node boxes; the column always equals the model step. */
export function layoutTaskGraph(model: TaskGraphModel): TaskGraphLayout {
  const g = new dagre.graphlib.Graph();
  g.setGraph({ rankdir: "LR", nodesep: ROW_GAP, ranksep: COLUMN_GAP });
  g.setDefaultEdgeLabel(() => ({}));
  for (const n of model.nodes) {
    g.setNode(n.id, { width: TASK_GRAPH_NODE_WIDTH, height: TASK_GRAPH_NODE_HEIGHT });
  }
  for (const e of model.edges) g.setEdge(e.from, e.to);
  dagre.layout(g);

  // dagre orders rows to limit crossings; x comes from the step so the drawn
  // column matches the step badge. Rows are re-packed to avoid overlaps.
  const columns = new Map<number, Array<{ id: string; y: number }>>();
  for (const n of model.nodes) {
    const column = columns.get(n.step) ?? [];
    column.push({ id: n.id, y: g.node(n.id)?.y ?? 0 });
    columns.set(n.step, column);
  }
  const layout: TaskGraphLayout = new Map();
  for (const [step, column] of columns) {
    column.sort((a, b) => a.y - b.y);
    let nextFree = -Infinity;
    for (const { id, y } of column) {
      const top = Math.max(y - TASK_GRAPH_NODE_HEIGHT / 2, nextFree);
      nextFree = top + TASK_GRAPH_NODE_HEIGHT + ROW_GAP;
      layout.set(id, {
        x: (step - 1) * (TASK_GRAPH_NODE_WIDTH + COLUMN_GAP),
        y: top,
        width: TASK_GRAPH_NODE_WIDTH,
        height: TASK_GRAPH_NODE_HEIGHT,
      });
    }
  }
  return layout;
}

const FRAME_PADDING = 16;
const FRAME_LABEL_SPACE = 24;

export type TaskGraphLayoutGroup = { id: string; memberIds: string[] };

/**
 * Board layout with parent frames. dagre's compound graph keeps each group's
 * members contiguous; frames are the members' bounding box plus label space.
 */
export function layoutGroupedTaskGraph(
  model: TaskGraphModel,
  groups: TaskGraphLayoutGroup[],
): { nodes: TaskGraphLayout; groups: Map<string, TaskGraphNodeBox> } {
  const g = new dagre.graphlib.Graph({ compound: true });
  g.setGraph({ rankdir: "LR", nodesep: ROW_GAP + FRAME_PADDING, ranksep: COLUMN_GAP });
  g.setDefaultEdgeLabel(() => ({}));
  for (const n of model.nodes) {
    g.setNode(n.id, { width: TASK_GRAPH_NODE_WIDTH, height: TASK_GRAPH_NODE_HEIGHT });
  }
  const displayed = new Set(model.nodes.map((n) => n.id));
  for (const group of groups) {
    g.setNode(group.id, {});
    for (const id of group.memberIds) if (displayed.has(id)) g.setParent(id, group.id);
  }
  // Only dependency edges imply an execution order; a related edge is
  // non-blocking and must not shift dagre's horizontal ranks.
  for (const e of model.edges) {
    if (e.kind === "dependency") g.setEdge(e.from, e.to);
  }
  dagre.layout(g);

  const nodes: TaskGraphLayout = new Map();
  for (const n of model.nodes) {
    const p = g.node(n.id);
    nodes.set(n.id, {
      x: (p?.x ?? 0) - TASK_GRAPH_NODE_WIDTH / 2,
      y: (p?.y ?? 0) - TASK_GRAPH_NODE_HEIGHT / 2,
      width: TASK_GRAPH_NODE_WIDTH,
      height: TASK_GRAPH_NODE_HEIGHT,
    });
  }
  const frames = new Map<string, TaskGraphNodeBox>();
  for (const group of groups) {
    const boxes = group.memberIds.flatMap((id) => nodes.get(id) ?? []);
    if (boxes.length === 0) continue;
    const left = Math.min(...boxes.map((b) => b.x)) - FRAME_PADDING;
    const top = Math.min(...boxes.map((b) => b.y)) - FRAME_PADDING - FRAME_LABEL_SPACE;
    const right = Math.max(...boxes.map((b) => b.x + b.width)) + FRAME_PADDING;
    const bottom = Math.max(...boxes.map((b) => b.y + b.height)) + FRAME_PADDING;
    frames.set(group.id, { x: left, y: top, width: right - left, height: bottom - top });
  }
  return { nodes, groups: frames };
}

export type TaskGraphStepFrame = { step: number; count: number; box: TaskGraphNodeBox };

/**
 * One band per step column: tasks sharing a step have no displayed ordering
 * between them, so they can run in parallel. Bands share one top and bottom so
 * their labels form a row above every card, clear of the connectors.
 */
export function stepFrames(model: TaskGraphModel, layout: TaskGraphLayout): TaskGraphStepFrame[] {
  const byStep = new Map<number, TaskGraphNodeBox[]>();
  for (const n of model.nodes) {
    const box = layout.get(n.id);
    if (!box) continue;
    byStep.set(n.step, [...(byStep.get(n.step) ?? []), box]);
  }
  const all = [...byStep.values()].flat();
  if (all.length === 0) return [];
  const top = Math.min(...all.map((b) => b.y)) - FRAME_PADDING - FRAME_LABEL_SPACE;
  const bottom = Math.max(...all.map((b) => b.y + b.height)) + FRAME_PADDING;
  return [...byStep.entries()]
    .sort(([a], [b]) => a - b)
    .map(([step, boxes]) => {
      const left = Math.min(...boxes.map((b) => b.x)) - FRAME_PADDING;
      const right = Math.max(...boxes.map((b) => b.x + b.width)) + FRAME_PADDING;
      return {
        step,
        count: boxes.length,
        box: { x: left, y: top, width: right - left, height: bottom - top },
      };
    });
}
