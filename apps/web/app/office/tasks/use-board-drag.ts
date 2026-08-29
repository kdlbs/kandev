"use client";

import { useCallback, useState } from "react";
import type { DragEndEvent, DragStartEvent } from "@dnd-kit/core";
import { MouseSensor, TouchSensor, useSensor, useSensors } from "@dnd-kit/core";
import { useAppStoreApi } from "@/components/state-provider";
import {
  ApprovalGateError,
  updateTaskStatusOrTranslateGate,
} from "@/lib/api/domains/office-status-gate";
import { toast } from "@/lib/toast/sonner";
import { t } from "@/lib/i18n";
import type { OfficeTask, OfficeTaskStatus } from "@/lib/state/slices/office/types";

/**
 * Collaborators for `applyStatusDrop`, injected so the drop rules can be
 * tested without a store, a network layer or a DOM.
 */
export type StatusDropDeps = {
  getTask: (taskId: string) => OfficeTask | undefined;
  patchTask: (taskId: string, patch: Partial<OfficeTask>) => void;
  updateStatus: (taskId: string, status: OfficeTaskStatus) => Promise<void>;
  onError: (message: string) => void;
};

/**
 * Applies a card drop onto a status column: optimistic patch, the mutation,
 * then a rollback to the pre-drop snapshot if the backend refuses.
 *
 * The regular board is a projection of `office.tasks.items`. During server
 * search, the displayed task list is a separate result projection, so the
 * caller also receives each patch to keep that view in step. Rollback restores
 * the whole prior task rather than just its status, matching
 * useOptimisticTaskMutation, so a snapshot's `rawStatus` survives the trip.
 *
 * Drops are column-to-column only. Cards are not reordered within a column:
 * no board in kandev does that, and an Office column is a page-limited window
 * of a server-sorted query, so a manual index would not survive pagination.
 */
export async function applyStatusDrop(
  taskId: string,
  targetStatus: OfficeTaskStatus,
  deps: StatusDropDeps,
): Promise<void> {
  const snapshot = deps.getTask(taskId);
  if (!snapshot) return;
  // A drop back onto the card's own column is a click that travelled a few
  // pixels, not a move. Sending it would burn a PATCH and a WS round-trip.
  if (snapshot.status === targetStatus) return;

  deps.patchTask(taskId, { status: targetStatus });
  try {
    await deps.updateStatus(taskId, targetStatus);
  } catch (err) {
    if (err instanceof ApprovalGateError) {
      // The backend already redirected and persisted this status server-side
      // before returning the error (see ApprovalGateError), so the board is
      // wrong if it rolls back to the pre-drop snapshot here. Patch status
      // only: spreading the snapshot would reinstate its stale rawStatus and
      // the card would re-normalize back to the old column.
      deps.patchTask(taskId, { status: err.redirectedStatus });
    } else {
      deps.patchTask(taskId, snapshot);
    }
    // The approver gate arrives here already translated into a sentence
    // naming who still has to sign off.
    deps.onError(err instanceof Error ? err.message : t("task:failedToMoveTask"));
  }
}

/**
 * Wires the office task board to dnd-kit. Droppable ids are status values and
 * draggable ids are task ids, so a drag end reads as (taskId -> status).
 */
export function useBoardDrag(
  tasks: OfficeTask[] = [],
  onTaskPatch?: (taskId: string, patch: Partial<OfficeTask>) => void,
) {
  const storeApi = useAppStoreApi();
  const [activeTaskId, setActiveTaskId] = useState<string | null>(null);

  const getTask = useCallback(
    (taskId: string) =>
      tasks.find((task) => task.id === taskId) ??
      storeApi.getState().office.tasks.items.find((task) => task.id === taskId),
    [storeApi, tasks],
  );

  const patchTask = useCallback(
    (taskId: string, patch: Partial<OfficeTask>) => {
      storeApi.getState().patchTaskInStore(taskId, patch);
      onTaskPatch?.(taskId, patch);
    },
    [onTaskPatch, storeApi],
  );

  const sensors = useSensors(
    // MouseSensor, not PointerSensor: the board is an overflow-x-auto row
    // (see task-board.tsx) and PointerSensor also captures touch via
    // pointer events, where its 8px distance activates before TouchSensor's
    // delay and hijacks swipe-scroll. MouseSensor + TouchSensor keep the
    // input streams separate so a quick touch swipe scrolls natively while
    // a press-and-hold starts a drag. Same convention as sidebar-view-chips.tsx.
    useSensor(MouseSensor, { activationConstraint: { distance: 8 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 250, tolerance: 5 } }),
  );

  const handleDragStart = useCallback((event: DragStartEvent) => {
    setActiveTaskId(String(event.active.id));
  }, []);

  const handleDragCancel = useCallback(() => setActiveTaskId(null), []);

  const handleDragEnd = useCallback(
    async (event: DragEndEvent) => {
      setActiveTaskId(null);
      const { active, over } = event;
      if (!over) return;
      await applyStatusDrop(String(active.id), String(over.id) as OfficeTaskStatus, {
        getTask,
        patchTask,
        updateStatus: (id, status) => updateTaskStatusOrTranslateGate(id, status),
        onError: (message) => toast.error(message),
      });
    },
    [getTask, patchTask],
  );

  return { activeTaskId, sensors, handleDragStart, handleDragEnd, handleDragCancel };
}
