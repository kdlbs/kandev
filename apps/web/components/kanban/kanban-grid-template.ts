export const KANBAN_COLUMN_MIN_PX = 280;

export function getKanbanColumnGridTemplate(stepCount: number): string {
  return `repeat(${stepCount}, minmax(${KANBAN_COLUMN_MIN_PX}px, 1fr))`;
}

export function shouldUseWindowedKanban(containerWidth: number, stepCount: number): boolean {
  return containerWidth > 0 && stepCount > 1 && containerWidth < stepCount * KANBAN_COLUMN_MIN_PX;
}

export function getLeadingKanbanColumnIndex(scrollLeft: number, offsets: number[]): number {
  if (offsets.length === 0) return 0;

  let closestIndex = 0;
  let closestDistance = Math.abs(offsets[0] - scrollLeft);
  for (let index = 1; index < offsets.length; index++) {
    const distance = Math.abs(offsets[index] - scrollLeft);
    if (distance < closestDistance) {
      closestIndex = index;
      closestDistance = distance;
    }
  }
  return closestIndex;
}
