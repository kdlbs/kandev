import type { ReactNode } from "react";
import type { TaskGraphLayout, TaskGraphNodeBox } from "@/lib/task-graph/layout";
import type { TaskGraphChain, TaskGraphModel, TaskGraphNode } from "@/lib/task-graph/model";

// The only contract between the graph dialog and a renderer
// (ADR 2026-10-17-task-graph-renderer-boundary).

export type TaskGraphNodeRenderState = { dimmed: boolean; emphasized: boolean; selected: boolean };

/** A labelled background frame, e.g. one parent's subtasks on the board. */
export type TaskGraphFrame = { id: string; title: string; box: TaskGraphNodeBox };

export type TaskGraphRendererProps = {
  model: TaskGraphModel;
  layout: TaskGraphLayout;
  emphasis: TaskGraphChain | null;
  colorMode: "light" | "dark";
  /** Hides the minimap and enlarges controls for touch. */
  touch: boolean;
  renderNode: (node: TaskGraphNode, state: TaskGraphNodeRenderState) => ReactNode;
  /** Accessible name of a node's activation control. */
  nodeLabel: (node: TaskGraphNode) => string;
  onNodeActivate: (nodeId: string) => void;
  onNodeEmphasize: (nodeId: string | null) => void;
  frames?: TaskGraphFrame[];
  /** The node the user picked; rendered with a selection ring. */
  selectedId?: string | null;
  /** Double-click: go into the task. */
  onNodeOpen?: (nodeId: string) => void;
};

export function nodeRenderState(
  emphasis: TaskGraphChain | null,
  nodeId: string,
  selectedId?: string | null,
): TaskGraphNodeRenderState {
  const selected = Boolean(selectedId) && selectedId === nodeId;
  if (!emphasis) return { dimmed: false, emphasized: false, selected };
  const emphasized = emphasis.nodeIds.has(nodeId);
  return { dimmed: !emphasized, emphasized, selected };
}
