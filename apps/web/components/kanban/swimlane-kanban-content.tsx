"use client";

import { useCallback, useMemo, useState } from "react";
import {
  DndContext,
  DragEndEvent,
  DragOverlay,
  DragStartEvent,
  PointerSensor,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import { KanbanColumn } from "@/components/kanban-column";
import { KanbanCardPreview, type Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";
import type { WorkflowAutomation, MoveTaskError } from "@/hooks/use-drag-and-drop";
import { useTaskActions } from "@/hooks/use-task-actions";
import { useAppStoreApi } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { KanbanState } from "@/lib/state/slices/kanban/types";

export type SwimlaneKanbanContentProps = {
  workflowId: string;
  steps: WorkflowStep[];
  tasks: Task[];
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onEditTask: (task: Task) => void;
  onDeleteTask: (task: Task) => void;
  onMoveError?: (error: MoveTaskError) => void;
  onWorkflowAutomation?: (automation: WorkflowAutomation) => void;
  deletingTaskId?: string | null;
};

async function handleWorkflowAutoStart(
  task: Task,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  response: any,
  onWorkflowAutomation?: (automation: WorkflowAutomation) => void,
) {
  const hasAutoStart = response?.workflow_step?.events?.on_enter?.some(
    (a: { type: string }) => a.type === "auto_start_agent",
  );
  if (!hasAutoStart) return;

  const sessionId = task.primarySessionId ?? null;
  if (sessionId) {
    const client = getWebSocketClient();
    if (!client) return;
    try {
      await client.request(
        "orchestrator.start",
        {
          task_id: task.id,
          session_id: sessionId,
          workflow_step_id: response.workflow_step.id,
        },
        15000,
      );
    } catch (err) {
      console.error("Failed to auto-start session for workflow step:", err);
    }
  } else {
    onWorkflowAutomation?.({
      taskId: task.id,
      sessionId: null,
      workflowStep: response.workflow_step,
      taskDescription: task.description ?? "",
    });
  }
}

type SwimlaneKanbanDndOptions = {
  tasks: Task[];
  workflowId: string;
  onWorkflowAutomation?: (automation: WorkflowAutomation) => void;
  onMoveError?: (error: MoveTaskError) => void;
};

function useSwimlaneKanbanDnd({
  tasks,
  workflowId,
  onWorkflowAutomation,
  onMoveError,
}: SwimlaneKanbanDndOptions) {
  const store = useAppStoreApi();
  const { moveTaskById } = useTaskActions();
  const [activeTaskId, setActiveTaskId] = useState<string | null>(null);

  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 8 } }));

  const handleDragStart = useCallback((event: DragStartEvent) => {
    setActiveTaskId(event.active.id as string);
  }, []);

  const handleDragEnd = useCallback(
    async (event: DragEndEvent) => {
      const { active, over } = event;
      setActiveTaskId(null);
      if (!over) return;

      const taskId = active.id as string;
      const targetStepId = over.id as string;
      const task = tasks.find((t) => t.id === taskId);
      if (!task || task.workflowStepId === targetStepId) return;

      const state = store.getState();
      const snapshot = state.kanbanMulti.snapshots[workflowId];
      if (!snapshot) return;

      const targetTasks = snapshot.tasks
        .filter(
          (t: KanbanState["tasks"][number]) => t.workflowStepId === targetStepId && t.id !== taskId,
        )
        .sort(
          (a: KanbanState["tasks"][number], b: KanbanState["tasks"][number]) =>
            a.position - b.position,
        );
      const nextPosition = targetTasks.length;
      const originalTasks = snapshot.tasks;

      state.setWorkflowSnapshot(workflowId, {
        ...snapshot,
        tasks: snapshot.tasks.map((t: KanbanState["tasks"][number]) =>
          t.id === taskId ? { ...t, workflowStepId: targetStepId, position: nextPosition } : t,
        ),
      });

      try {
        const response = await moveTaskById(taskId, {
          workflow_id: workflowId,
          workflow_step_id: targetStepId,
          position: nextPosition,
        });
        await handleWorkflowAutoStart(task, response, onWorkflowAutomation);
      } catch (error) {
        const currentSnapshot = store.getState().kanbanMulti.snapshots[workflowId];
        if (currentSnapshot) {
          store
            .getState()
            .setWorkflowSnapshot(workflowId, { ...currentSnapshot, tasks: originalTasks });
        }
        const message = error instanceof Error ? error.message : "Failed to move task";
        onMoveError?.({ message, taskId, sessionId: task.primarySessionId ?? null });
      }
    },
    [tasks, workflowId, store, moveTaskById, onWorkflowAutomation, onMoveError],
  );

  const handleDragCancel = useCallback(() => {
    setActiveTaskId(null);
  }, []);

  const moveTaskToStep = useCallback(
    async (task: Task, targetStepId: string) => {
      if (task.workflowStepId === targetStepId) return;
      await handleDragEnd({ active: { id: task.id }, over: { id: targetStepId } } as DragEndEvent);
    },
    [handleDragEnd],
  );

  const activeTask = useMemo(
    () => tasks.find((t) => t.id === activeTaskId) ?? null,
    [tasks, activeTaskId],
  );

  return { sensors, handleDragStart, handleDragEnd, handleDragCancel, moveTaskToStep, activeTask };
}

export function SwimlaneKanbanContent({
  workflowId,
  steps,
  tasks,
  onPreviewTask,
  onOpenTask,
  onEditTask,
  onDeleteTask,
  onMoveError,
  onWorkflowAutomation,
  deletingTaskId,
}: SwimlaneKanbanContentProps) {
  const { sensors, handleDragStart, handleDragEnd, handleDragCancel, moveTaskToStep, activeTask } =
    useSwimlaneKanbanDnd({ tasks, workflowId, onWorkflowAutomation, onMoveError });

  const getTasksForStep = (stepId: string) => {
    return tasks
      .filter((t) => t.workflowStepId === stepId)
      .sort((a, b) => (a.position ?? 0) - (b.position ?? 0));
  };

  if (steps.length === 0) return null;

  return (
    <DndContext
      sensors={sensors}
      onDragStart={handleDragStart}
      onDragEnd={handleDragEnd}
      onDragCancel={handleDragCancel}
    >
      <div className="">
        <div
          className="grid gap-0"
          style={{ gridTemplateColumns: `repeat(${steps.length}, minmax(0, 1fr))` }}
        >
          {steps.map((step) => (
            <KanbanColumn
              key={step.id}
              step={step}
              tasks={getTasksForStep(step.id)}
              onPreviewTask={onPreviewTask}
              onOpenTask={onOpenTask}
              onEditTask={onEditTask}
              onDeleteTask={onDeleteTask}
              onMoveTask={moveTaskToStep}
              steps={steps}
              deletingTaskId={deletingTaskId}
            />
          ))}
        </div>
      </div>
      <DragOverlay dropAnimation={null}>
        {activeTask ? <KanbanCardPreview task={activeTask} /> : null}
      </DragOverlay>
    </DndContext>
  );
}
