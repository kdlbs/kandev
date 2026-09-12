"use client";

import { useCallback, useMemo, useRef, useState } from "react";
import {
  DragEndEvent,
  DragStartEvent,
  PointerSensor,
  TouchSensor,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import { type Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";
import type { MoveTaskError } from "@/hooks/use-drag-and-drop";
import { useTaskActions } from "@/hooks/use-task-actions";
import { useAppStoreApi } from "@/components/state-provider";
import { isOrphanMoveTarget } from "@/components/kanban/swimlane-orphan-display";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import { classifyDrop } from "@/lib/kanban/drop-classification";
import { useStepReorder } from "@/hooks/domains/kanban/use-step-reorder";
import { getTaskMoveErrorMessage } from "@/components/task/task-move-error-message";
import {
  getDragDisplaySteps,
  getTemporaryStepIds,
  useKanbanDragScrollAnchor,
} from "@/hooks/domains/kanban/use-kanban-drag-scroll-anchor";
import { t } from "@/lib/i18n";

const TASK_POINTER_SENSOR_OPTIONS = { activationConstraint: { distance: 8 } };
const TASK_TOUCH_SENSOR_OPTIONS = { activationConstraint: { delay: 250, tolerance: 5 } };

export type SwimlaneKanbanDndOptions = {
  tasks: Task[];
  workflowId: string;
  onMoveError?: (error: MoveTaskError) => void;
};

export function useCrossStepMove(workflowId: string, onMoveError?: (error: MoveTaskError) => void) {
  const store = useAppStoreApi();
  const { moveTaskById } = useTaskActions();

  return useCallback(
    async (taskId: string, targetStepId: string, task: Task) => {
      const state = store.getState();
      const snapshot = state.kanbanMulti.snapshots[workflowId];
      if (!snapshot) return;

      const originalStepId = task.workflowStepId;

      // The server always computes a cross-step arrival's position itself
      // (REQ-TASKS-KANBAN-TASK-REORDERING-001.28: it sorts last in the
      // destination), so there is no locally-computed position to apply
      // here — the eventual task.updated/task.moved event carries the real
      // one. Until then the card just needs to show the right step.
      state.setWorkflowSnapshot(workflowId, {
        ...snapshot,
        tasks: snapshot.tasks.map((t: KanbanState["tasks"][number]) =>
          t.id === taskId ? { ...t, workflowStepId: targetStepId } : t,
        ),
      });

      try {
        await moveTaskById(taskId, {
          workflow_id: workflowId,
          workflow_step_id: targetStepId,
        });
      } catch (error) {
        const currentSnapshot = store.getState().kanbanMulti.snapshots[workflowId];
        if (currentSnapshot) {
          // Revert only this task's optimistic step change, not the whole
          // array: a concurrent update to another task while this move was
          // in flight must survive the rollback.
          store.getState().setWorkflowSnapshot(workflowId, {
            ...currentSnapshot,
            tasks: currentSnapshot.tasks.map((t: KanbanState["tasks"][number]) =>
              t.id === taskId ? { ...t, workflowStepId: originalStepId } : t,
            ),
          });
        }
        const message = getTaskMoveErrorMessage(error, t("task:taskMoveErrorGeneric"), t);
        onMoveError?.({ message, taskId, sessionId: task.primarySessionId ?? null });
      }
    },
    [workflowId, store, moveTaskById, onMoveError],
  );
}

export function useSwimlaneKanbanDnd({ tasks, workflowId, onMoveError }: SwimlaneKanbanDndOptions) {
  const { reorderBand } = useStepReorder();
  const moveTaskAcrossSteps = useCrossStepMove(workflowId, onMoveError);
  const [activeTaskId, setActiveTaskId] = useState<string | null>(null);
  const tasksRef = useRef(tasks);
  tasksRef.current = tasks;

  const sensors = useSensors(
    useSensor(PointerSensor, TASK_POINTER_SENSOR_OPTIONS),
    useSensor(TouchSensor, TASK_TOUCH_SENSOR_OPTIONS),
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
      const overId = over.id as string;
      const task = tasksRef.current.find((t) => t.id === taskId);
      if (!task) return;

      const classification = classifyDrop({
        draggedTaskId: taskId,
        overId,
        stepTasks: tasksRef.current.filter((t) => t.workflowStepId === task.workflowStepId),
        allTasks: tasksRef.current,
      });

      if (classification.kind === "reorder") {
        await reorderBand({
          workflowId,
          stepId: classification.stepId,
          band: classification.band,
          draggedId: taskId,
          visibleOrderAfterMove: classification.visibleOrderAfterMove,
        });
        return;
      }

      if (classification.kind !== "cross-step" || isOrphanMoveTarget(classification.targetStepId)) {
        return;
      }

      await moveTaskAcrossSteps(taskId, classification.targetStepId, task);
    },
    [workflowId, reorderBand, moveTaskAcrossSteps],
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

  return {
    sensors,
    handleDragStart,
    handleDragEnd,
    handleDragCancel,
    moveTaskToStep,
    activeTask,
  };
}

export function useSwimlaneKanbanPresentationDnd({
  displaySteps,
  moveTargetSteps,
  isMobile,
  ...dndOptions
}: SwimlaneKanbanDndOptions & {
  displaySteps: WorkflowStep[];
  moveTargetSteps: WorkflowStep[];
  isMobile: boolean;
}) {
  const dnd = useSwimlaneKanbanDnd(dndOptions);
  const renderedSteps = useMemo(
    () => getDragDisplaySteps(displaySteps, moveTargetSteps, !!dnd.activeTask, isMobile),
    [displaySteps, moveTargetSteps, dnd.activeTask, isMobile],
  );
  const { boardRef, handleAnchoredDragStart } = useKanbanDragScrollAnchor({
    tasks: dndOptions.tasks,
    activeTask: dnd.activeTask,
    renderedSteps,
    onDragStart: dnd.handleDragStart,
  });
  const temporaryStepIds = useMemo(
    () => getTemporaryStepIds(displaySteps, moveTargetSteps),
    [displaySteps, moveTargetSteps],
  );
  return {
    ...dnd,
    renderedSteps,
    temporaryStepIds,
    boardRef,
    handleDragStart: handleAnchoredDragStart,
  };
}
