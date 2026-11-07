import { useCallback, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useTheme } from "@/components/theme/app-theme";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { layoutTaskGraph, type TaskGraphLayout } from "@/lib/task-graph/layout";
import { traceChain, type TaskGraphModel, type TaskGraphNode } from "@/lib/task-graph/model";
import { formatTaskStateLabel } from "@/lib/ui/state-labels";
import type { TaskState } from "@/lib/types/http";
import { TaskGraphNodeCard } from "./task-graph-node-card";
import type { TaskGraphFrame } from "./task-graph-renderer";
import { XyflowTaskGraphRenderer } from "./xyflow-task-graph-renderer";

/** Shared canvas: theme, touch, chain emphasis and accessible node names. */
export function TaskGraphCanvas({
  model,
  onActivate,
  layout,
  frames,
  selectedId = null,
  onOpen,
}: {
  model: TaskGraphModel;
  onActivate: (id: string) => void;
  /** Defaults to the step-column layout. */
  layout?: TaskGraphLayout;
  frames?: TaskGraphFrame[];
  selectedId?: string | null;
  onOpen?: (id: string) => void;
}) {
  const { t } = useTranslation();
  const { resolvedTheme } = useTheme();
  const touch = useTouchDrawer();
  const [emphasized, setEmphasized] = useState<string | null>(null);
  const stepLayout = useMemo(() => layout ?? layoutTaskGraph(model), [layout, model]);
  // Hover wins; otherwise the selected node's chain stays highlighted.
  const emphasisId = emphasized ?? selectedId;
  const emphasis = useMemo(
    () => (emphasisId && model.predecessors.has(emphasisId) ? traceChain(model, emphasisId) : null),
    [model, emphasisId],
  );
  const nodeLabel = useCallback(
    (node: TaskGraphNode) =>
      t("task:taskGraphNodeLabel", {
        title: node.title,
        state: formatTaskStateLabel(node.state as TaskState),
        step: node.step,
      }),
    [t],
  );
  return (
    <XyflowTaskGraphRenderer
      model={model}
      layout={stepLayout}
      frames={frames}
      selectedId={selectedId}
      onNodeOpen={onOpen}
      emphasis={emphasis}
      colorMode={resolvedTheme}
      touch={touch}
      renderNode={(node, state) => <TaskGraphNodeCard node={node} state={state} />}
      nodeLabel={nodeLabel}
      onNodeActivate={onActivate}
      onNodeEmphasize={setEmphasized}
    />
  );
}
