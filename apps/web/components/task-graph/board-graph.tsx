import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useShallow } from "zustand/react/shallow";
import { useAppStore } from "@/components/state-provider";
import { buildBoardGraph, type BoardGraphTask } from "@/lib/task-graph/board";
import {
  layoutGroupedTaskGraph,
  layoutTaskGraph,
  type TaskGraphLayout,
  type TaskGraphNodeBox,
} from "@/lib/task-graph/layout";
import { buildTaskGraphModel, type TaskGraphModel } from "@/lib/task-graph/model";
import { useTaskGraphParallelSteps } from "@/hooks/domains/task/use-task-graph-parallel-steps";
import { TaskGraphCanvas } from "./task-graph-canvas";
import { TaskGraphLegend } from "./task-graph-legend";
import { TaskGraphParallelToggle } from "./task-graph-parallel-toggle";
import type { TaskGraphFrame } from "./task-graph-renderer";
import { useStepFrames } from "./use-step-frames";

const MIN_HEIGHT_PX = 260;
const MAX_HEIGHT_PX = 720;
const CHROME_PX = 48;

type TitleLookupState = {
  kanban?: { tasks?: Array<{ id: string; title: string }> };
  kanbanMulti?: { snapshots?: Record<string, { tasks?: Array<{ id: string; title: string }> }> };
};

function findTitle(state: TitleLookupState, id: string): string | undefined {
  const hit = state.kanban?.tasks?.find((t) => t.id === id);
  if (hit) return hit.title;
  for (const snapshot of Object.values(state.kanbanMulti?.snapshots ?? {})) {
    const found = snapshot.tasks?.find((t) => t.id === id);
    if (found) return found.title;
  }
  return undefined;
}

/** One workflow lane of the board Graph view: its filtered tasks, framed by parent. */
/** The selected task and its actions; a click on a node only selects it. */
function SelectionBar({
  title,
  onOpen,
  onPreview,
}: {
  title: string | null;
  onOpen: () => void;
  onPreview: () => void;
}) {
  const { t } = useTranslation();
  if (!title) {
    return <p className="text-xs text-muted-foreground">{t("task:taskGraphSelectHint")}</p>;
  }
  return (
    <div data-testid="board-graph-selection" className="flex min-w-0 items-center gap-2">
      <span className="min-w-0 truncate text-sm font-medium">{title}</span>
      <Button
        size="sm"
        className="cursor-pointer [@media(pointer:coarse)]:min-h-11"
        data-testid="board-graph-open"
        onClick={onOpen}
      >
        {t("task:taskGraphOpenTask")}
      </Button>
      <Button
        size="sm"
        variant="outline"
        className="cursor-pointer [@media(pointer:coarse)]:min-h-11"
        data-testid="board-graph-preview"
        onClick={onPreview}
      >
        {t("task:taskGraphPreviewTask")}
      </Button>
    </div>
  );
}

type BoardGraphData = {
  model: TaskGraphModel;
  layout: TaskGraphLayout;
  frames: TaskGraphFrame[];
  height: number;
};

function layoutHeight(boxes: TaskGraphNodeBox[]): number {
  if (boxes.length === 0) return 0;
  return Math.max(...boxes.map((b) => b.y + b.height)) - Math.min(...boxes.map((b) => b.y));
}

/** Parent frames by default; with Parallel steps, step columns and step bands. */
function useBoardGraphData(
  tasks: BoardGraphTask[],
  focusId: string | null,
  parallel: boolean,
): BoardGraphData {
  const { t } = useTranslation();
  const taskIds = useMemo(() => new Set(tasks.map((task) => task.id)), [tasks]);
  const parentIds = useMemo(
    () =>
      [...new Set(tasks.map((task) => task.parentTaskId))].filter(
        (id): id is string => typeof id === "string" && !taskIds.has(id),
      ),
    [tasks, taskIds],
  );
  const parentTitles = useAppStore(
    useShallow((state) => {
      const titleState = state as unknown as TitleLookupState;
      return Object.fromEntries(
        parentIds.flatMap((id) => {
          const title = findTitle(titleState, id);
          return title === undefined ? [] : [[id, title]];
        }),
      );
    }),
  );
  const grouped = useMemo(() => {
    const { response, groups } = buildBoardGraph(tasks, focusId, (id) => parentTitles[id]);
    const model = buildTaskGraphModel(response);
    if (parallel) return { model, layout: layoutTaskGraph(model), frames: [] as TaskGraphFrame[] };
    const placed = layoutGroupedTaskGraph(model, groups);
    const frames = groups.flatMap((group) => {
      const box = placed.groups.get(group.id);
      return box
        ? [{ id: group.id, title: group.title || t("task:taskGraphModeSubtasks"), box }]
        : [];
    });
    return { model, layout: placed.nodes, frames };
  }, [tasks, focusId, parentTitles, t, parallel]);
  const bands = useStepFrames(grouped.model, grouped.layout, parallel);
  return useMemo(() => {
    const frames = parallel ? bands : grouped.frames;
    const height = layoutHeight([...grouped.layout.values(), ...frames.map((f) => f.box)]);
    return { ...grouped, frames, height };
  }, [grouped, bands, parallel]);
}

export function BoardGraph({
  tasks,
  focusId,
  onOpen,
  onPreview,
  fillHeight = false,
}: {
  tasks: BoardGraphTask[];
  focusId: string | null;
  onOpen: (taskId: string) => void;
  onPreview: (taskId: string) => void;
  /** Stretch the canvas to the lane's full height instead of sizing to content. */
  fillHeight?: boolean;
}) {
  const [parallel, setParallel] = useTaskGraphParallelSteps();
  const graph = useBoardGraphData(tasks, focusId, parallel);

  const [selectedId, setSelectedId] = useState<string | null>(focusId);
  useEffect(() => setSelectedId(focusId), [focusId]);
  const selected = tasks.find((task) => task.id === selectedId) ?? null;

  const containerRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (focusId) containerRef.current?.scrollIntoView({ block: "center", behavior: "smooth" });
  }, [focusId]);

  const contentHeight = Math.min(
    MAX_HEIGHT_PX,
    Math.max(MIN_HEIGHT_PX, graph.height * 0.75 + CHROME_PX),
  );
  return (
    <div
      ref={containerRef}
      data-testid="board-graph"
      className={
        fillHeight
          ? "flex h-full min-h-0 flex-col gap-2 px-2 pb-2"
          : "flex flex-col gap-2 px-2 pb-2"
      }
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <SelectionBar
          title={selected?.title ?? null}
          onOpen={() => selected && onOpen(selected.id)}
          onPreview={() => selected && onPreview(selected.id)}
        />
        <TaskGraphParallelToggle pressed={parallel} onPressedChange={setParallel} />
      </div>
      <div
        className={
          fillHeight
            ? "min-h-0 flex-1 overflow-hidden rounded-md border"
            : "overflow-hidden rounded-md border"
        }
        style={fillHeight ? undefined : { height: contentHeight }}
      >
        <TaskGraphCanvas
          model={graph.model}
          layout={graph.layout}
          frames={graph.frames}
          selectedId={selected?.id ?? null}
          onActivate={setSelectedId}
          onOpen={onOpen}
        />
      </div>
      <TaskGraphLegend showExternal={false} />
    </div>
  );
}
