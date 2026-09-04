/**
 * Pure classifier for a kanban drag-and-drop end event
 * (REQ-TASKS-KANBAN-TASK-REORDERING-001.5, .10, .11, .13). Kept free of React
 * and dnd-kit so the reorder-vs-reject-vs-cross-step-vs-no-op decision is
 * unit-testable without simulating a drag gesture.
 */

import { partitionWipTasks, type WipQueueTask } from "./wip-queue";
import { moveWithinOrder } from "./reorder-merge";
import type { ReorderBand } from "@/lib/types/http";

export type DropClassification =
  | { kind: "no-op" }
  | { kind: "reject" }
  | { kind: "cross-step"; targetStepId: string }
  | {
      kind: "reorder";
      stepId: string;
      band: ReorderBand;
      visibleOrderAfterMove: string[];
    };

type ClassifiableTask = WipQueueTask & { id: string; workflowStepId: string };

export type ClassifyDropParams<T extends ClassifiableTask> = {
  draggedTaskId: string;
  /** `over.id` from the drag-end event: a task id, a step id, or absent. */
  overId: string | null;
  /** Every task currently displayed in the dragged task's step, in display order. */
  stepTasks: T[];
};

/**
 * Classifies a drop against the dragged task's OWN step's currently
 * displayed tasks. `stepTasks` must be scoped to the dragged task's step
 * (whatever step it started in) so band membership (admitted vs queued) can
 * be computed the same way the board renders it.
 */
export function classifyDrop<T extends ClassifiableTask>(
  params: ClassifyDropParams<T>,
): DropClassification {
  const { draggedTaskId, overId, stepTasks } = params;
  if (!overId) return { kind: "no-op" };

  const draggedTask = stepTasks.find((task) => task.id === draggedTaskId);
  if (!draggedTask) return { kind: "no-op" };

  const overTask = stepTasks.find((task) => task.id === overId);
  const targetStepId = overTask ? overTask.workflowStepId : overId;

  if (targetStepId !== draggedTask.workflowStepId) {
    return { kind: "cross-step", targetStepId };
  }

  // Same step. Dropped on the column itself (no specific card under the
  // pointer) or back on itself: nothing to commit.
  if (!overTask || overTask.id === draggedTaskId) {
    return { kind: "no-op" };
  }

  const { admitted, queued } = partitionWipTasks(stepTasks, draggedTask.workflowStepId);
  const draggedInAdmitted = admitted.some((task) => task.id === draggedTaskId);
  const overInAdmitted = admitted.some((task) => task.id === overId);
  if (draggedInAdmitted !== overInAdmitted) {
    return { kind: "reject" };
  }

  const band: ReorderBand = draggedInAdmitted ? "admitted" : "queued";
  const bandOrder = (draggedInAdmitted ? admitted : queued).map((task) => task.id);
  const fromIndex = bandOrder.indexOf(draggedTaskId);
  const toIndex = bandOrder.indexOf(overId);
  if (fromIndex === -1 || toIndex === -1 || fromIndex === toIndex) {
    return { kind: "no-op" };
  }

  return {
    kind: "reorder",
    stepId: draggedTask.workflowStepId,
    band,
    visibleOrderAfterMove: moveWithinOrder(bandOrder, fromIndex, toIndex),
  };
}
