"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { DragStartEvent, DragEndEvent } from "@dnd-kit/core";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useTaskActions } from "@/hooks/use-task-actions";
import type { Task } from "@/components/kanban-card";
import type { KanbanState } from "@/lib/state/slices";

export type MoveTaskError = {
  message: string;
  taskId: string;
  sessionId: string | null;
};

export type DragAndDropOptions = {
  visibleTasks: Task[];
  onMoveError?: (error: MoveTaskError) => void;
};

type UseDragAndDropRefsInput = Pick<DragAndDropOptions, "onMoveError">;

/** Compute optimistic task list after a move. */
function applyOptimisticMove(
  tasks: KanbanState["tasks"],
  taskId: string,
  targetStepId: string,
  nextPosition: number,
): KanbanState["tasks"] {
  return tasks.map((t: KanbanState["tasks"][number]) =>
    t.id === taskId ? { ...t, workflowStepId: targetStepId, position: nextPosition } : t,
  );
}

/** Calculate the next position in the target column. */
function calcNextPosition(
  tasks: KanbanState["tasks"],
  taskId: string,
  targetStepId: string,
): number {
  return tasks
    .filter(
      (t: KanbanState["tasks"][number]) => t.workflowStepId === targetStepId && t.id !== taskId,
    )
    .sort(
      (a: KanbanState["tasks"][number], b: KanbanState["tasks"][number]) => a.position - b.position,
    ).length;
}

function useDragAndDropRefs({ onMoveError }: UseDragAndDropRefsInput) {
  const onMoveErrorRef = useRef(onMoveError);
  useEffect(() => {
    onMoveErrorRef.current = onMoveError;
  }, [onMoveError]);
  return { onMoveErrorRef };
}

export function useDragAndDrop({ visibleTasks, onMoveError }: DragAndDropOptions) {
  const [activeTaskId, setActiveTaskId] = useState<string | null>(null);
  const [isMovingTask, setIsMovingTask] = useState(false);
  const { moveTaskById } = useTaskActions();
  const kanban = useAppStore((state) => state.kanban);
  const store = useAppStoreApi();
  const { onMoveErrorRef } = useDragAndDropRefs({ onMoveError });
  const performTaskMove = useCallback(
    async (task: Task, targetStepId: string) => {
      const currentKanban = store.getState().kanban;
      if (!currentKanban.workflowId) return;
      const nextPosition = calcNextPosition(currentKanban.tasks, task.id, targetStepId);
      const originalTasks = currentKanban.tasks;
      store.getState().hydrate({
        kanban: {
          ...currentKanban,
          tasks: applyOptimisticMove(currentKanban.tasks, task.id, targetStepId, nextPosition),
        },
      });
      try {
        setIsMovingTask(true);
        await moveTaskById(task.id, {
          workflow_id: currentKanban.workflowId,
          workflow_step_id: targetStepId,
          position: nextPosition,
        });
      } catch (error) {
        store.getState().hydrate({ kanban: { ...store.getState().kanban, tasks: originalTasks } });
        const message = error instanceof Error ? error.message : "Failed to move task";
        onMoveErrorRef.current?.({
          message,
          taskId: task.id,
          sessionId: task.primarySessionId ?? null,
        });
      } finally {
        setIsMovingTask(false);
      }
    },
    [moveTaskById, onMoveErrorRef, store],
  );
  const handleDragStart = (event: DragStartEvent) => {
    setActiveTaskId(event.active.id as string);
  };
  const handleDragEnd = useCallback(
    async (event: DragEndEvent) => {
      const { active, over } = event;
      setActiveTaskId(null);
      if (!over) return;
      const taskId = active.id as string;
      const targetStepId = over.id as string;
      if (!kanban.workflowId || isMovingTask) return;
      const movedTask = visibleTasks.find((t) => t.id === taskId);
      if (!movedTask) return;
      await performTaskMove(movedTask, targetStepId);
    },
    [kanban.workflowId, isMovingTask, visibleTasks, performTaskMove],
  );
  const handleDragCancel = () => {
    setActiveTaskId(null);
  };
  const moveTaskToStep = useCallback(
    async (task: Task, targetStepId: string) => {
      if (!kanban.workflowId || isMovingTask) return;
      if (task.workflowStepId === targetStepId) return;
      await performTaskMove(task, targetStepId);
    },
    [kanban.workflowId, isMovingTask, performTaskMove],
  );
  const activeTask = visibleTasks.find((task) => task.id === activeTaskId) ?? null;

  return {
    activeTaskId,
    activeTask,
    isMovingTask,
    handleDragStart,
    handleDragEnd,
    handleDragCancel,
    moveTaskToStep,
  };
}
