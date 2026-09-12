/**
 * Pure order-computation helpers for within-band drag/keyboard reordering
 * (REQ-TASKS-KANBAN-TASK-REORDERING-001.5, .12, .34). Kept free of React and
 * of dnd-kit so the merge algorithm is unit-testable without simulating a
 * drag gesture.
 */

/** `arrayMove` semantics without importing `@dnd-kit/sortable` for one helper. */
export function moveWithinOrder<T>(order: T[], fromIndex: number, toIndex: number): T[] {
  if (fromIndex < 0 || toIndex < 0 || fromIndex >= order.length || toIndex >= order.length) {
    return order;
  }
  const next = [...order];
  const [moved] = next.splice(fromIndex, 1);
  next.splice(toIndex, 0, moved);
  return next;
}

/**
 * Reinserts `draggedId` into `fullBandOrder` (the band's complete membership,
 * including any task the board is currently hiding behind a filter) so that
 * it sits adjacent to the same neighbor it now has in `visibleOrderAfterMove`
 * (the reordered subsequence of members the board is currently showing).
 *
 * Every other task's relative order is left exactly as it was — only the
 * dragged task moves — which is what
 * REQ-TASKS-KANBAN-TASK-REORDERING-001.34 requires: a reorder made under a
 * board filter must not rearrange the tasks that filter is hiding.
 */
export function mergeVisibleReorderIntoBand(
  fullBandOrder: string[],
  visibleOrderAfterMove: string[],
  draggedId: string,
): string[] {
  const withoutDragged = fullBandOrder.filter((id) => id !== draggedId);
  const draggedIndex = visibleOrderAfterMove.indexOf(draggedId);
  if (draggedIndex === -1) return fullBandOrder;

  const nextVisibleId = visibleOrderAfterMove[draggedIndex + 1];
  if (nextVisibleId !== undefined) {
    const insertAt = withoutDragged.indexOf(nextVisibleId);
    if (insertAt === -1) return fullBandOrder;
    const result = [...withoutDragged];
    result.splice(insertAt, 0, draggedId);
    return result;
  }

  // The dragged card is last among the currently visible members: per
  // REQ-TASKS-KANBAN-TASK-REORDERING-001.34 it lands last in the whole band,
  // not merely after its visible predecessor — a hidden member trailing that
  // predecessor must not keep outranking a card the user moved to the bottom
  // of what they can see.
  if (visibleOrderAfterMove[draggedIndex - 1] !== undefined) {
    return [...withoutDragged, draggedId];
  }

  // Only one visible member — a reorder should never have been offered
  // (REQ-TASKS-KANBAN-TASK-REORDERING-001.32); nothing to do.
  return fullBandOrder;
}

export function arraysEqual(a: string[], b: string[]): boolean {
  return a.length === b.length && a.every((id, index) => id === b[index]);
}
