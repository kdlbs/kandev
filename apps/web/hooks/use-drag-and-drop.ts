"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { DragStartEvent, DragEndEvent } from "@dnd-kit/core";
import { useAppStore } from "@/components/state-provider";
import { useTaskActions } from "@/hooks/use-task-actions";
import { useKanbanSnapshotMutator } from "@/hooks/domains/kanban/use-kanban-snapshots";
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
  // The active workflow selection is client-only state; its task/step snapshot
  // lives in the TanStack Query `qk.kanban.multi()` cache.
  const workflowId = useAppStore((state) => state.workflows.activeId);
  const { getSnapshot, setSnapshot } = useKanbanSnapshotMutator();
  const { onMoveErrorRef } = useDragAndDropRefs({ onMoveError });
  const performTaskMove = useCallback(
    async (task: Task, targetStepId: string) => {
      if (!workflowId) return;
      const snapshot = getSnapshot(workflowId);
      if (!snapshot) return;
      const nextPosition = calcNextPosition(snapshot.tasks, task.id, targetStepId);
      const originalTasks = snapshot.tasks;
      setSnapshot(workflowId, {
        ...snapshot,
        tasks: applyOptimisticMove(snapshot.tasks, task.id, targetStepId, nextPosition),
      });
      try {
        setIsMovingTask(true);
        await moveTaskById(task.id, {
          workflow_id: workflowId,
          workflow_step_id: targetStepId,
          position: nextPosition,
        });
      } catch (error) {
        const current = getSnapshot(workflowId);
        if (current) setSnapshot(workflowId, { ...current, tasks: originalTasks });
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
    [moveTaskById, onMoveErrorRef, workflowId, getSnapshot, setSnapshot],
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
      if (!workflowId || isMovingTask) return;
      const movedTask = visibleTasks.find((t) => t.id === taskId);
      if (!movedTask) return;
      await performTaskMove(movedTask, targetStepId);
    },
    [workflowId, isMovingTask, visibleTasks, performTaskMove],
  );
  const handleDragCancel = () => {
    setActiveTaskId(null);
  };
  const moveTaskToStep = useCallback(
    async (task: Task, targetStepId: string) => {
      if (!workflowId || isMovingTask) return;
      if (task.workflowStepId === targetStepId) return;
      await performTaskMove(task, targetStepId);
    },
    [workflowId, isMovingTask, performTaskMove],
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
