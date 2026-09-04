"use client";

import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { ApiError } from "@/lib/api/client";
import { reorderStepTasks } from "@/lib/api/domains/kanban-api";
import { compareStepOrder } from "@/lib/kanban/task-order";
import { arraysEqual, mergeVisibleReorderIntoBand } from "@/lib/kanban/reorder-merge";
import { partitionWipTasks } from "@/lib/kanban/wip-queue";
import { getTaskReorderErrorMessage } from "@/components/task/task-move-error-message";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import type { ReorderBand, ReorderStepTasksResponse } from "@/lib/types/http";

type SnapshotTask = KanbanState["tasks"][number];

function bandOrder(tasks: SnapshotTask[], stepId: string, band: ReorderBand): SnapshotTask[] {
  const { admitted, queued } = partitionWipTasks(tasks, stepId);
  const stepTasks = band === "admitted" ? admitted : queued;
  return [...stepTasks].sort(compareStepOrder);
}

function applyBandPositions(
  tasks: SnapshotTask[],
  stepId: string,
  band: ReorderBand,
  orderedIds: string[],
  admittedCount: number,
): SnapshotTask[] {
  const offset = band === "admitted" ? 0 : admittedCount;
  const positionById = new Map(orderedIds.map((id, index) => [id, offset + index]));
  return tasks.map((task) =>
    task.workflowStepId === stepId && positionById.has(task.id)
      ? { ...task, position: positionById.get(task.id)! }
      : task,
  );
}

function applyResponsePositions(
  tasks: SnapshotTask[],
  response: ReorderStepTasksResponse,
): SnapshotTask[] {
  const positionById = new Map(response.tasks.map((t) => [t.id, t.position]));
  return tasks.map((task) =>
    positionById.has(task.id) ? { ...task, position: positionById.get(task.id)! } : task,
  );
}

/**
 * Submits a within-band reorder (REQ-TASKS-KANBAN-TASK-REORDERING-001.5,
 * .12): applies the new order optimistically, calls the reorder endpoint —
 * the only request surface, per the frozen design — then reconciles to the
 * server's authoritative positions. A `step_changed` conflict (.19)
 * reconciles silently to the authoritative order carried on the error body;
 * any other failure restores the pre-drag order and shows a localized
 * message (.20).
 */
export function useStepReorder() {
  const store = useAppStoreApi();
  const { toast } = useToast();
  const { t } = useTranslation("task");
  const pendingBandKeys = useAppStore((state) => state.kanbanMulti.pendingReorderBandKeys);

  const isBandPending = useCallback(
    (stepId: string, band: ReorderBand) => Boolean(pendingBandKeys[`${stepId}:${band}`]),
    [pendingBandKeys],
  );

  const reorderBand = useCallback(
    async (params: {
      workflowId: string;
      stepId: string;
      band: ReorderBand;
      draggedId: string;
      visibleOrderAfterMove: string[];
    }) => {
      const { workflowId, stepId, band, draggedId, visibleOrderAfterMove } = params;
      const state = store.getState();
      if (state.kanbanMulti.pendingReorderBandKeys[`${stepId}:${band}`]) return;

      const snapshot = state.kanbanMulti.snapshots[workflowId];
      if (!snapshot) return;

      const stepTasks = snapshot.tasks.filter((task) => task.workflowStepId === stepId);
      const currentBandOrder = bandOrder(stepTasks, stepId, band).map((task) => task.id);
      const nextBandOrder = mergeVisibleReorderIntoBand(
        currentBandOrder,
        visibleOrderAfterMove,
        draggedId,
      );
      if (arraysEqual(nextBandOrder, currentBandOrder)) return;

      const admittedCount = partitionWipTasks(stepTasks, stepId).admitted.length;
      const originalTasks = snapshot.tasks;

      state.setWorkflowSnapshot(workflowId, {
        ...snapshot,
        tasks: applyBandPositions(snapshot.tasks, stepId, band, nextBandOrder, admittedCount),
      });
      state.setBandReorderPending(stepId, band, true);

      try {
        const response = await reorderStepTasks(stepId, { band, ordered_task_ids: nextBandOrder });
        const current = store.getState().kanbanMulti.snapshots[workflowId];
        if (current) {
          state.setWorkflowSnapshot(workflowId, {
            ...current,
            tasks: applyResponsePositions(current.tasks, response),
          });
        }
        state.setStepOrderRevision(stepId, response.revision);
      } catch (error) {
        const current = store.getState().kanbanMulti.snapshots[workflowId];
        if (current) {
          const conflictBody =
            error instanceof ApiError &&
            error.status === 409 &&
            error.body &&
            typeof error.body === "object" &&
            Array.isArray((error.body as ReorderStepTasksResponse).tasks)
              ? (error.body as ReorderStepTasksResponse)
              : null;
          if (conflictBody) {
            // step_changed (.19): reconcile silently, no error toast.
            state.setWorkflowSnapshot(workflowId, {
              ...current,
              tasks: applyResponsePositions(current.tasks, conflictBody),
            });
            if (typeof conflictBody.revision === "number") {
              state.setStepOrderRevision(stepId, conflictBody.revision);
            }
          } else {
            state.setWorkflowSnapshot(workflowId, { ...current, tasks: originalTasks });
            toast({
              title: t("task:failedToReorderTasks"),
              description: getTaskReorderErrorMessage(error, t("task:taskReorderErrorGeneric"), t),
              variant: "error",
            });
          }
        }
      } finally {
        state.setBandReorderPending(stepId, band, false);
      }
    },
    [store, toast, t],
  );

  return { reorderBand, isBandPending };
}
