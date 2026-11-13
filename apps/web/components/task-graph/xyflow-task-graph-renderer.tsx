import { memo, useEffect, useMemo, type CSSProperties } from "react";
import { useTranslation } from "react-i18next";
import {
  Background,
  Controls,
  Handle,
  MarkerType,
  Position,
  ReactFlow,
  ReactFlowProvider,
  useReactFlow,
  useStore,
  type Edge,
  type Node,
  type NodeProps,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { cn } from "@/lib/utils";
import type { TaskGraphNode } from "@/lib/task-graph/model";
import { taskGraphEdgeStyle } from "./task-graph-edge-styles";
import { nodeRenderState, type TaskGraphRendererProps } from "./task-graph-renderer";
import { useNodePress } from "./use-node-press";

// The only importer of xyflow (ADR 2026-10-17-task-graph-renderer-boundary).

type TaskNodeData = {
  node: TaskGraphNode;
  props: TaskGraphRendererProps;
};
type TaskFlowNode = Node<TaskNodeData, "task">;
type FrameFlowNode = Node<{ title: string }, "frame">;

const DIMMED_OPACITY = 0.2;
// Long chains pan instead of shrinking cards below a readable size.
const FIT_VIEW_OPTIONS = { padding: 0.15, minZoom: 0.75, maxZoom: 1 };
const HIDDEN_HANDLE: CSSProperties = { opacity: 0, pointerEvents: "none" };
// xyflow disables pointer events on non-selectable, non-draggable nodes; the
// node's own button needs them.
const INTERACTIVE_NODE: CSSProperties = { pointerEvents: "all" };

// xyflow theme variables mapped onto the app's design tokens.
const THEME_VARIABLES = {
  "--xy-background-color": "var(--background)",
  "--xy-background-pattern-color": "var(--border)",
  "--xy-controls-button-background-color": "var(--card)",
  "--xy-controls-button-background-color-hover": "var(--accent)",
  "--xy-controls-button-color": "var(--foreground)",
  "--xy-controls-button-color-hover": "var(--foreground)",
  "--xy-controls-button-border-color": "var(--border)",
  "--xy-attribution-background-color": "transparent",
} as CSSProperties;

export function toFlowNodes(props: TaskGraphRendererProps): TaskFlowNode[] {
  return props.model.nodes.map((node) => {
    const box = props.layout.get(node.id);
    return {
      id: node.id,
      type: "task",
      position: { x: box?.x ?? 0, y: box?.y ?? 0 },
      width: box?.width,
      height: box?.height,
      data: { node, props },
      style: INTERACTIVE_NODE,
      draggable: false,
      connectable: false,
      selectable: false,
      deletable: false,
      focusable: false,
    };
  });
}

export function toFrameNodes(props: TaskGraphRendererProps): FrameFlowNode[] {
  return (props.frames ?? []).map((frame) => ({
    id: frame.id,
    type: "frame",
    position: { x: frame.box.x, y: frame.box.y },
    width: frame.box.width,
    height: frame.box.height,
    data: { title: frame.title },
    zIndex: -1,
    draggable: false,
    connectable: false,
    selectable: false,
    deletable: false,
    focusable: false,
  }));
}

function FrameNode({ data }: NodeProps<FrameFlowNode>) {
  return (
    <div
      data-testid="task-graph-frame"
      className="pointer-events-none h-full w-full rounded-lg border border-dashed border-muted-foreground/40 bg-muted/20"
    >
      <span className="block truncate px-3 pt-1.5 text-[11px] font-medium text-muted-foreground">
        {data.title}
      </span>
    </div>
  );
}

export function toFlowEdges(props: TaskGraphRendererProps): Edge[] {
  return props.model.edges.map((edge) => {
    const style = taskGraphEdgeStyle(edge.kind, edge.status);
    const dimmed = props.emphasis !== null && !props.emphasis.edgeIds.has(edge.id);
    return {
      id: edge.id,
      source: edge.from,
      target: edge.to,
      type: "smoothstep",
      markerEnd:
        edge.kind === "related" ? undefined : { type: MarkerType.ArrowClosed, color: style.stroke },
      style: { ...style, opacity: dimmed ? DIMMED_OPACITY : 1 },
      selectable: false,
      deletable: false,
      focusable: false,
    };
  });
}

const TaskNode = memo(function TaskNode({ data }: NodeProps<TaskFlowNode>) {
  const { node, props } = data;
  const press = useNodePress(
    () => props.onNodeActivate(node.id),
    () => props.onNodeEmphasize(node.id),
  );
  return (
    <>
      <Handle type="target" position={Position.Left} isConnectable={false} style={HIDDEN_HANDLE} />
      <button
        type="button"
        data-testid={`task-graph-node-${node.id}`}
        aria-label={props.nodeLabel(node)}
        className="block h-full w-full cursor-pointer rounded-md focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
        onMouseEnter={() => props.onNodeEmphasize(node.id)}
        onMouseLeave={() => props.onNodeEmphasize(null)}
        onFocus={() => props.onNodeEmphasize(node.id)}
        onBlur={() => props.onNodeEmphasize(null)}
        onDoubleClick={props.onNodeOpen ? () => props.onNodeOpen?.(node.id) : undefined}
        {...press}
      >
        {props.renderNode(node, nodeRenderState(props.emphasis, node.id, props.selectedId))}
      </button>
      <Handle type="source" position={Position.Right} isConnectable={false} style={HIDDEN_HANDLE} />
    </>
  );
});

const NODE_TYPES = { task: TaskNode, frame: FrameNode };

const ANCHOR_PX = 24;

/**
 * Re-fits when the displayed node set changes (mode switch, new subtask). A
 * graph larger than the canvas at the readable zoom floor is centred on the
 * focused task when it is displayed, otherwise anchored at its top-left (step 1).
 */
function FitOnNodeSetChange({ nodeKey, focusId }: { nodeKey: string; focusId: string }) {
  const { fitView, getNode, getNodes, getNodesBounds, getViewport, setViewport, setCenter } =
    useReactFlow();
  const paneWidth = useStore((state) => state.width);
  const paneHeight = useStore((state) => state.height);
  // Node sizes are explicit, so the fit only has to wait for the canvas size.
  const ready = paneWidth > 0 && paneHeight > 0;
  useEffect(() => {
    if (!ready) return;
    const frame = requestAnimationFrame(() => {
      void fitView(FIT_VIEW_OPTIONS).then(() => {
        const viewport = getViewport();
        const bounds = getNodesBounds(getNodes());
        const overflows =
          bounds.width * viewport.zoom > paneWidth || bounds.height * viewport.zoom > paneHeight;
        if (!overflows) return;
        const focus = getNode(focusId);
        if (focus?.width && focus.height) {
          void setCenter(focus.position.x + focus.width / 2, focus.position.y + focus.height / 2, {
            zoom: viewport.zoom,
          });
          return;
        }
        const x = Math.max(viewport.x, ANCHOR_PX - bounds.x * viewport.zoom);
        const y = Math.max(viewport.y, ANCHOR_PX - bounds.y * viewport.zoom);
        void setViewport({ ...viewport, x, y });
      });
    });
    return () => cancelAnimationFrame(frame);
    // Pane size changes alone must not re-fit and discard the user's pan.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, nodeKey, focusId]);
  return null;
}

function useAriaLabels() {
  const { t } = useTranslation();
  return useMemo(
    () => ({
      "node.a11yDescription.default": t("task:taskGraphNodeHint"),
      "node.a11yDescription.keyboardDisabled": t("task:taskGraphNodeHint"),
      "controls.ariaLabel": t("task:taskGraphControls"),
      "controls.zoomIn.ariaLabel": t("task:taskGraphZoomIn"),
      "controls.zoomOut.ariaLabel": t("task:taskGraphZoomOut"),
      "controls.fitView.ariaLabel": t("task:taskGraphFitView"),
    }),
    [t],
  );
}

export function XyflowTaskGraphRenderer(props: TaskGraphRendererProps) {
  const { t } = useTranslation();
  const ariaLabelConfig = useAriaLabels();
  const nodes = useMemo(() => [...toFrameNodes(props), ...toFlowNodes(props)] as Node[], [props]);
  const edges = useMemo(() => toFlowEdges(props), [props]);
  // Frames are part of the key: switching parent frames for step bands re-lays
  // out the cards, so the view must re-fit.
  const nodeKey = [
    ...props.model.nodes.map((n) => n.id),
    ...(props.frames ?? []).map((f) => f.id),
  ].join(",");
  return (
    <ReactFlowProvider>
      <ReactFlow
        aria-label={t("task:taskGraphCanvas")}
        className="h-full w-full"
        style={THEME_VARIABLES}
        colorMode={props.colorMode}
        nodes={nodes}
        edges={edges}
        nodeTypes={NODE_TYPES}
        ariaLabelConfig={ariaLabelConfig}
        nodesDraggable={false}
        nodesConnectable={false}
        nodesFocusable={false}
        edgesFocusable={false}
        elementsSelectable={false}
        deleteKeyCode={null}
        zoomOnDoubleClick={false}
        minZoom={0.2}
        maxZoom={2}
        onPaneClick={() => props.onNodeEmphasize(null)}
      >
        <Background gap={16} />
        <Controls
          showInteractive={false}
          position="bottom-left"
          // xyflow's stylesheet is unlayered, so it beats layered utilities.
          className={cn(props.touch && "[&>button]:size-11!")}
        />
        <FitOnNodeSetChange nodeKey={nodeKey} focusId={props.model.focusId} />
      </ReactFlow>
    </ReactFlowProvider>
  );
}
