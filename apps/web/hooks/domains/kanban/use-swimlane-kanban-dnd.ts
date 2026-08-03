"use client";

import { useCallback, useMemo, useState } from "react";
import {
  type DragEndEvent,
  type DragStartEvent,
  PointerSensor,
  TouchSensor,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import type { Task } from "@/components/kanban-card";
import { useAppStoreApi } from "@/components/state-provider";
import type { MoveTaskError } from "@/hooks/use-drag-and-drop";
import { useTaskActions } from "@/hooks/use-task-actions";
import { isOrphanMoveTarget } from "@/lib/kanban/orphan-step";
import { resolveKanbanDropStepId } from "@/lib/kanban/reorder-admitted";
import {
  persistCrossStepMove,
  persistSameStepReorder,
} from "@/hooks/domains/kanban/persist-kanban-moves";

type Options = {
  tasks: Task[];
  workflowId: string;
  stepIds: ReadonlySet<string>;
  onMoveError?: (error: MoveTaskError) => void;
};

export function useSwimlaneKanbanDnd({ tasks, workflowId, stepIds, onMoveError }: Options) {
  const store = useAppStoreApi();
  const { moveTaskById } = useTaskActions();
  const [activeTaskId, setActiveTaskId] = useState<string | null>(null);
  const deps = useMemo(
    () => ({ store, workflowId, moveTaskById, onMoveError }),
    [store, workflowId, moveTaskById, onMoveError],
  );

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 8 } }),
    useSensor(TouchSensor, {
      activationConstraint: { delay: 250, tolerance: 5 },
    }),
  );

  const handleDragStart = useCallback((event: DragStartEvent) => {
    setActiveTaskId(event.active.id as string);
  }, []);

  const handleDragEnd = useCallback(
    async (event: DragEndEvent) => {
      const { active, over } = event;
      setActiveTaskId(null);
      if (!over) return;

      const taskId = active.id as string;
      const overId = String(over.id);
      const task = tasks.find((t) => t.id === taskId);
      if (!task) return;

      const targetStepId = resolveKanbanDropStepId(overId, tasks, stepIds);
      if (!targetStepId || isOrphanMoveTarget(targetStepId)) return;

      if (task.workflowStepId === targetStepId) {
        await persistSameStepReorder(deps, task, overId, targetStepId);
        return;
      }

      await persistCrossStepMove(deps, task, targetStepId);
    },
    [tasks, stepIds, deps],
  );

  const handleDragCancel = useCallback(() => {
    setActiveTaskId(null);
  }, []);

  const moveTaskToStep = useCallback(
    async (task: Task, targetStepId: string) => {
      if (task.workflowStepId === targetStepId || isOrphanMoveTarget(targetStepId)) return;
      await persistCrossStepMove(deps, task, targetStepId);
    },
    [deps],
  );

  const activeTask = useMemo(
    () => tasks.find((t) => t.id === activeTaskId) ?? null,
    [tasks, activeTaskId],
  );

  return {
    sensors,
    handleDragStart,
    handleDragEnd,
    handleDragCancel,
    moveTaskToStep,
    activeTask,
  };
}
