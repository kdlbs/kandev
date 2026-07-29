export const KANBAN_COLUMN_MIN_PX = 280;

export function getKanbanColumnGridTemplate(stepCount: number): string {
  return `repeat(${stepCount}, minmax(${KANBAN_COLUMN_MIN_PX}px, 1fr))`;
}
