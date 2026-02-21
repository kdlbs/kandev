"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { DragStartEvent, DragEndEvent } from "@dnd-kit/core";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useTaskActions } from "@/hooks/use-task-actions";
import type { Task } from "@/components/kanban-card";
import type { KanbanState } from "@/lib/state/slices";
import type { WorkflowStepDTO, MoveTaskResponse } from "@/lib/types/http";
import { getWebSocketClient } from "@/lib/ws/connection";

export type WorkflowAutomation = {
  taskId: string;
  sessionId: string | null;
  workflowStep: WorkflowStepDTO;
  taskDescription: string;
};

export type MoveTaskError = {
  message: string;
  taskId: string;
  sessionId: string | null;
};

export type DragAndDropOptions = {
  visibleTasks: Task[];
  onWorkflowAutomation?: (automation: WorkflowAutomation) => void;
  onMoveError?: (error: MoveTaskError) => void;
};

type UseDragAndDropRefsInput = Pick<DragAndDropOptions, "onWorkflowAutomation" | "onMoveError">;

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

// eslint-disable-next-line @typescript-eslint/no-explicit-any
async function triggerAutoStart(task: Task, response: any): Promise<void> {
  const sessionId = task.primarySessionId ?? null;
  if (!sessionId) return;
  const client = getWebSocketClient();
  if (!client) return;
  try {
    await client.request(
      "orchestrator.start",
      { task_id: task.id, session_id: sessionId, workflow_step_id: response.workflow_step.id },
      15000,
    );
  } catch (err) {
    console.error("Failed to auto-start session for workflow step:", err);
  }
}

function useDragAndDropRefs({ onWorkflowAutomation, onMoveError }: UseDragAndDropRefsInput) {
  const onWorkflowAutomationRef = useRef(onWorkflowAutomation);
  const onMoveErrorRef = useRef(onMoveError);
  useEffect(() => {
    onWorkflowAutomationRef.current = onWorkflowAutomation;
    onMoveErrorRef.current = onMoveError;
  }, [onMoveError, onWorkflowAutomation]);
  return { onWorkflowAutomationRef, onMoveErrorRef };
}

async function maybeHandleWorkflowAutomation(
  task: Task,
  response: MoveTaskResponse,
  onWorkflowAutomationRef: {
    current: DragAndDropOptions["onWorkflowAutomation"];
  },
) {
  const hasAutoStart = response.workflow_step?.events?.on_enter?.some(
    (a: { type: string }) => a.type === "auto_start_agent",
  );
  if (!hasAutoStart) return;
  const sessionId = task.primarySessionId ?? null;
  if (sessionId) {
    await triggerAutoStart(task, response);
    return;
  }
  onWorkflowAutomationRef.current?.({
    taskId: task.id,
    sessionId: null,
    workflowStep: response.workflow_step,
    taskDescription: task.description ?? "",
  });
}

export function useDragAndDrop({
  visibleTasks,
  onWorkflowAutomation,
  onMoveError,
}: DragAndDropOptions) {
  const [activeTaskId, setActiveTaskId] = useState<string | null>(null);
  const [isMovingTask, setIsMovingTask] = useState(false);
  const { moveTaskById } = useTaskActions();
  const kanban = useAppStore((state) => state.kanban);
  const store = useAppStoreApi();
  const { onWorkflowAutomationRef, onMoveErrorRef } = useDragAndDropRefs({
    onWorkflowAutomation,
    onMoveError,
  });
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
        const response = await moveTaskById(task.id, {
          workflow_id: currentKanban.workflowId,
          workflow_step_id: targetStepId,
          position: nextPosition,
        });
        await maybeHandleWorkflowAutomation(task, response, onWorkflowAutomationRef);
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
    [moveTaskById, onMoveErrorRef, onWorkflowAutomationRef, store],
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
