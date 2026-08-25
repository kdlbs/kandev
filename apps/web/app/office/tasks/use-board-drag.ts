"use client";

import { useCallback, useState } from "react";
import type { DragEndEvent, DragStartEvent } from "@dnd-kit/core";
import { PointerSensor, TouchSensor, useSensor, useSensors } from "@dnd-kit/core";
import { useAppStoreApi } from "@/components/state-provider";
import { updateTaskStatusOrTranslateGate } from "@/lib/api/domains/office-status-gate";
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
 * The board is a projection of `office.tasks.items`, so patching the store IS
 * the visible move; there is no board-local ordering to keep in step. Rollback
 * restores the whole prior task rather than just its status, matching
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
    deps.patchTask(taskId, snapshot);
    // The approver gate arrives here already translated into a sentence
    // naming who still has to sign off.
    deps.onError(err instanceof Error ? err.message : t("task:failedToMoveTask"));
  }
}

/**
 * Wires the office task board to dnd-kit. Droppable ids are status values and
 * draggable ids are task ids, so a drag end reads as (taskId -> status).
 */
export function useBoardDrag() {
  const storeApi = useAppStoreApi();
  const [activeTaskId, setActiveTaskId] = useState<string | null>(null);

  const sensors = useSensors(
    // Distance activation is what keeps a plain click opening the task
    // instead of starting a drag; the card's onClick still navigates.
    useSensor(PointerSensor, { activationConstraint: { distance: 8 } }),
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
        getTask: (id) => storeApi.getState().office.tasks.items.find((task) => task.id === id),
        patchTask: (id, patch) => storeApi.getState().patchTaskInStore(id, patch),
        updateStatus: (id, status) => updateTaskStatusOrTranslateGate(id, status),
        onError: (message) => toast.error(message),
      });
    },
    [storeApi],
  );

  return { activeTaskId, sensors, handleDragStart, handleDragEnd, handleDragCancel };
}
