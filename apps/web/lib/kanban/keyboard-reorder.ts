/**
 * Pure move-validity helper for keyboard reordering
 * (REQ-TASKS-KANBAN-TASK-REORDERING-001.12). Kept free of React so the
 * "ignored at the band edge" rule is unit-testable without simulating a
 * keydown.
 */

import { moveWithinOrder } from "./reorder-merge";

export type ArrowDirection = "up" | "down";

/**
 * Returns `order` with `taskId` moved one place in `direction`, or `null`
 * when the move would carry it past either end of the band — an arrow press
 * there is ignored entirely, not clamped to the edge.
 */
export function moveOneStep(
  order: string[],
  taskId: string,
  direction: ArrowDirection,
): string[] | null {
  const fromIndex = order.indexOf(taskId);
  if (fromIndex === -1) return null;
  const toIndex = direction === "up" ? fromIndex - 1 : fromIndex + 1;
  if (toIndex < 0 || toIndex >= order.length) return null;
  return moveWithinOrder(order, fromIndex, toIndex);
}
