"use client";

import { useCallback, useMemo, useState, type KeyboardEvent } from "react";
import { useTranslation } from "react-i18next";
import type { Task } from "@/components/kanban-card";
import { moveOneStep, type ArrowDirection } from "@/lib/kanban/keyboard-reorder";
import { partitionWipTasks } from "@/lib/kanban/wip-queue";
import type { ReorderBand } from "@/lib/types/http";
import { useStepReorder } from "./use-step-reorder";

type PickedUp = {
  taskId: string;
  stepId: string;
  band: ReorderBand;
  order: string[];
};

function bandOrderFor(tasks: Task[], stepId: string, taskId: string) {
  const stepTasks = tasks.filter((task) => task.workflowStepId === stepId);
  const { admitted, queued } = partitionWipTasks(stepTasks, stepId);
  if (admitted.some((task) => task.id === taskId)) {
    return { band: "admitted" as ReorderBand, order: admitted.map((task) => task.id) };
  }
  if (queued.some((task) => task.id === taskId)) {
    return { band: "queued" as ReorderBand, order: queued.map((task) => task.id) };
  }
  return null;
}

/**
 * Whether a card in bandInfo's band may be picked up: the band must resolve,
 * have no reorder already in flight (AC.27), and - REQ-TASKS-KANBAN-TASK-REORDERING-001.32 -
 * have at least two members, since a band that small offers no reorder at
 * all and committing with no arrow presses would otherwise still issue a
 * no-op reorder request.
 */
function canPickUp(
  bandInfo: ReturnType<typeof bandOrderFor>,
  stepId: string,
  isBandPending: (stepId: string, band: ReorderBand) => boolean,
): bandInfo is NonNullable<ReturnType<typeof bandOrderFor>> {
  return bandInfo !== null && bandInfo.order.length >= 2 && !isBandPending(stepId, bandInfo.band);
}

/**
 * Bespoke keyboard reorder (REQ-TASKS-KANBAN-TASK-REORDERING-001.12): Space
 * or Enter picks a focused card up, Up/Down Arrow move it one place among
 * the band members currently rendered, Space or Enter drops and commits,
 * Escape cancels under .9. Deliberately not dnd-kit's
 * `sortableKeyboardCoordinates` — that needs a real `SortableContext`, which
 * the virtualized column doesn't have (see the task plan's "Design
 * decisions" section).
 */
export function useKeyboardReorder(workflowId: string, tasks: Task[]) {
  const { reorderBand, isBandPending } = useStepReorder();
  const { t } = useTranslation("kanban");
  const [pickedUp, setPickedUp] = useState<PickedUp | null>(null);
  const [announcement, setAnnouncement] = useState("");

  const announcePosition = useCallback(
    (title: string, order: string[], taskId: string) => {
      const index = order.indexOf(taskId);
      setAnnouncement(
        t("kanban:reorderPositionAnnouncement", {
          title,
          position: index + 1,
          total: order.length,
        }),
      );
    },
    [t],
  );

  const pickUp = useCallback(
    (task: Task) => {
      const bandInfo = bandOrderFor(tasks, task.workflowStepId, task.id);
      if (!canPickUp(bandInfo, task.workflowStepId, isBandPending)) return;
      setPickedUp({
        taskId: task.id,
        stepId: task.workflowStepId,
        band: bandInfo.band,
        order: bandInfo.order,
      });
      announcePosition(task.title, bandInfo.order, task.id);
    },
    [tasks, isBandPending, announcePosition],
  );

  const move = useCallback(
    (task: Task, direction: ArrowDirection) => {
      setPickedUp((current) => {
        if (!current || current.taskId !== task.id) return current;
        const nextOrder = moveOneStep(current.order, task.id, direction);
        if (!nextOrder) return current;
        announcePosition(task.title, nextOrder, task.id);
        return { ...current, order: nextOrder };
      });
    },
    [announcePosition],
  );

  const commit = useCallback(
    async (task: Task) => {
      if (!pickedUp || pickedUp.taskId !== task.id) return;
      const { stepId, band, order } = pickedUp;
      setPickedUp(null);
      await reorderBand({
        workflowId,
        stepId,
        band,
        draggedId: task.id,
        visibleOrderAfterMove: order,
      });
    },
    [pickedUp, reorderBand, workflowId],
  );

  const cancel = useCallback(() => {
    setPickedUp(null);
  }, []);

  const handleKeyDown = useCallback(
    (event: KeyboardEvent, task: Task) => {
      if (pickedUp && pickedUp.taskId !== task.id) return;
      if (event.key === " " || event.key === "Enter") {
        event.preventDefault();
        if (pickedUp) void commit(task);
        else pickUp(task);
        return;
      }
      if (!pickedUp) return;
      if (event.key === "ArrowUp") {
        event.preventDefault();
        move(task, "up");
      } else if (event.key === "ArrowDown") {
        event.preventDefault();
        move(task, "down");
      } else if (event.key === "Escape") {
        event.preventDefault();
        cancel();
      }
    },
    [pickedUp, pickUp, move, commit, cancel],
  );

  return useMemo(
    () => ({
      handleKeyDown,
      pickedUpTaskId: pickedUp?.taskId ?? null,
      draftOrder: pickedUp?.order ?? null,
      draftStepId: pickedUp?.stepId ?? null,
      draftBand: pickedUp?.band ?? null,
      announcement,
    }),
    [handleKeyDown, pickedUp, announcement],
  );
}
