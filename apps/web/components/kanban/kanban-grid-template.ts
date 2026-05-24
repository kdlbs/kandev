export const COMPACT_KANBAN_COLUMN_MIN_PX = 260;

export function getKanbanColumnGridTemplate(stepCount: number, isCompactDesktop: boolean): string {
  const minWidth = isCompactDesktop ? `${COMPACT_KANBAN_COLUMN_MIN_PX}px` : "0";
  return `repeat(${stepCount}, minmax(${minWidth}, 1fr))`;
}
