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
import type {
  ReorderBand,
  ReorderedTaskPosition,
  ReorderStepTasksResponse,
} from "@/lib/types/http";

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

function bandTaskIds(tasks: SnapshotTask[], stepId: string, band: ReorderBand): Set<string> {
  return new Set(bandOrder(tasks, stepId, band).map((task) => task.id));
}

/**
 * Applies `orderedTasks`' positions, restricted to `allowedIds` when given.
 * Restricting to the band under reconciliation keeps a whole-step response or
 * withheld snapshot from clobbering the sibling band's positions, which may
 * already be fresher (REQ-TASKS-KANBAN-TASK-REORDERING-001.27).
 */
function applyOrderedPositions(
  tasks: SnapshotTask[],
  orderedTasks: ReorderedTaskPosition[],
  allowedIds?: Set<string>,
): SnapshotTask[] {
  const positionById = new Map(
    orderedTasks.filter((t) => !allowedIds || allowedIds.has(t.id)).map((t) => [t.id, t.position]),
  );
  return tasks.map((task) =>
    positionById.has(task.id) ? { ...task, position: positionById.get(task.id)! } : task,
  );
}

/**
 * Picks the causally-later of a withheld WS order (received while this
 * band's own request was in flight) and the request's own resolution: the
 * higher revision wins, and the response breaks a tie since an equal
 * revision from the withheld side was necessarily observed before this
 * request's own commit (REQ-TASKS-KANBAN-TASK-REORDERING-001.27).
 */
function pickReconciledOrder(
  withheld: { revision: number; tasks: ReorderedTaskPosition[] } | undefined,
  response: { revision: number; tasks: ReorderedTaskPosition[] },
): { revision: number; tasks: ReorderedTaskPosition[] } {
  if (withheld && withheld.revision > response.revision) return withheld;
  return response;
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

      const bandKey = `${stepId}:${band}`;

      // Reconciles `candidate` against anything withheld while this band's
      // request was in flight (REQ-TASKS-KANBAN-TASK-REORDERING-001.27),
      // applies the result scoped to this band only, advances the revision
      // without ever regressing it, and clears the withheld entry.
      const reconcileAndApply = (
        current: { tasks: SnapshotTask[] },
        candidate: { revision: number; tasks: ReorderedTaskPosition[] },
      ) => {
        const withheld = store.getState().kanbanMulti.withheldReorderByBandKey[bandKey];
        const chosen = pickReconciledOrder(withheld, candidate);
        const allowedIds = bandTaskIds(
          current.tasks.filter((task) => task.workflowStepId === stepId),
          stepId,
          band,
        );
        const recordedRevision = store.getState().kanbanMulti.orderRevisionByStepId[stepId] ?? -1;
        state.setWorkflowSnapshot(workflowId, {
          ...store.getState().kanbanMulti.snapshots[workflowId]!,
          tasks: applyOrderedPositions(current.tasks, chosen.tasks, allowedIds),
        });
        state.setStepOrderRevision(stepId, Math.max(chosen.revision, recordedRevision));
        state.setWithheldReorder(stepId, band, null);
      };

      try {
        const response = await reorderStepTasks(stepId, { band, ordered_task_ids: nextBandOrder });
        const current = store.getState().kanbanMulti.snapshots[workflowId];
        if (current) {
          reconcileAndApply(current, response);
        }
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
            reconcileAndApply(current, conflictBody);
          } else {
            const withheld = store.getState().kanbanMulti.withheldReorderByBandKey[bandKey];
            if (withheld) {
              reconcileAndApply({ tasks: originalTasks }, withheld);
            } else {
              state.setWorkflowSnapshot(workflowId, { ...current, tasks: originalTasks });
            }
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
